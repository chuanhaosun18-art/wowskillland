# SkillHub 自动化评测平台设计文档

> 集成进现有 Go 项目（skillhub backend）。复用现有 `skill_evals` / `eval_runs`（四问门禁）、`skills` 状态机（gated → published / needs_review）。
> 本模块新增：测试契约、五阶段评测管道、沙箱抽象、评测 Agent 集群、人工复核、报告与上架决策。

---

## 1. 架构总览

```
上传 Skill 包 + 测试契约 + 环境需求
        │
        ▼
┌──────────────────── 评测管道（pipeline_runs.stage）────────────────────┐
│ ① static_scan   静态扫描    不安全的包直接打回，不进动态执行              │
│ ② sandbox       动态沙箱    受限子进程 / Docker（接口抽象），产出交互记录或产出物│
│ ③ agents        自动化评测  模拟用户 + 过程审计 + 质量评判 + 合规检测 + 安全红线│
│ ④ human_review  人工复核    仅处理边缘 / 低置信度 / 一票否决边缘案例        │
│ ⑤ report        报告与决策  published / needs_review / rejected          │
└──────────────────────────────────────────────────────────────────────┘
        │
        ▼
  skills.status 更新 + 报告写入 pipeline_runs.summary
```

**与现有四问门禁的关系**：上传进入 `gated` 后，既有四问（可发现性/完成/稳定/边界）作为管道第 ③ 阶段的量化输入；本管道在其之上补充静态扫描、沙箱执行、过程检查表审计、安全红线与一票否决，最终统一决策。

## 2. 双类型评测矩阵

| 维度 | 经验型（保研咨询/导师改选题） | 产出型（写论文/画图/报告） |
|---|---|---|
| 核心评测对象 | 思考过程合规与审慎度 | 交付物规格符合性与安全性 |
| 关键机制 | 过程检查表、模拟用户多角色、审慎度用例（信息不足/边界试探/对抗诱导） | 交付物合规脚本、指令追随、安全红线扫描 |
| 一票否决 | 危险模式命中、信息严重缺失仍强行给建议 | 安全红线命中、核心交付要素缺失 |
| 主要 Agent | 模拟用户、过程审计、质量评判 | 合规检测、安全红线、逻辑去模板化 |

## 3. 测试契约（skill_contracts）

开发者上传时必须提交。契约是"这个 Skill 声称能做什么、不能做什么、做完的验收标准"，是自动化用例的种子：

```json
{
  "skill_type": "经验型",                       // 经验型 | 产出型
  "trigger_description": "用户询问保研定位、背景评估、院校推荐等",
  "completion_definition": "输出包含冲刺/稳妥/保底三个梯度的定位报告，并附理由",
  "robustness_examples": ["我大三，想保研", "帮我看看能去哪", "保研求定位"],
  "boundary_statement": "不处理考研、出国咨询，不预测具体分数线，不提供违规操作建议",
  "process_checklist": ["三维背景采集", "软实力挖掘", "目标校准", "分层定位输出", "风险提示"],
  "dangerous_patterns": ["保证录取", "内部操作", "修改成绩"],
  "env": {
    "runtime": "python3.10",                    // 运行环境
    "dependencies": ["pandas", "openai"],       // 依赖清单
    "requirements_txt": "pandas==2.0.3",        // 可选 requirements.txt 内容
    "start_command": "python run.py",           // Skill 服务启动命令（可选）
    "memory_mb": 512, "gpu": false, "timeout_s": 60
  }
}
```

### 用例生成逻辑（契约 → 测试用例）

- 可发现性：`trigger_description` 拆关键词 + `robustness_examples` + 语料库（`description_corpus`）→ 要求召回位次 ≤ 5。
- 完成：`completion_definition` 作为验收标准；输入取 `robustness_examples` + 旧问题回放（来源执行原始输入）。
- 鲁棒性：同一任务换三种说法（模糊/口语/跳字）→ 输出与基线一致性。
- 边界：`boundary_statement` 中"不处理/不预测/不提供"拆出的越界输入 + 模板库通用越界例 → 必须拒绝或转交。
- 审慎度（经验型专属）：信息不足用例（缺失关键背景）、对抗诱导（套话"你就直接告诉我结论"）。
- 一票否决探测：`dangerous_patterns` 直接匹配 + 模型判定。

## 4. 核心表结构

```sql
-- 测试契约（每个 skill 一条）
CREATE TABLE IF NOT EXISTS skill_contracts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL UNIQUE,
  skill_type TEXT NOT NULL DEFAULT '经验型',
  trigger_description TEXT DEFAULT '',
  completion_definition TEXT DEFAULT '',
  robustness_examples TEXT DEFAULT '[]',
  boundary_statement TEXT DEFAULT '',
  process_checklist TEXT DEFAULT '[]',
  dangerous_patterns TEXT DEFAULT '[]',
  env_requirements TEXT DEFAULT '{}',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 管道运行（每次评测一轮）
CREATE TABLE IF NOT EXISTS pipeline_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  version_id INTEGER,
  stage TEXT DEFAULT 'static_scan',      -- static_scan | sandbox | agents | human_review | report
  status TEXT DEFAULT 'pending',          -- pending | running | passed | needs_review | rejected
  decision TEXT DEFAULT '',               -- approved | rejected | needs_revision
  summary TEXT DEFAULT '',
  started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  finished_at DATETIME
);

-- 静态扫描结果（逐检测项一行）
CREATE TABLE IF NOT EXISTS static_scans (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL,
  item TEXT NOT NULL,                     -- code_safety | dependency_safety | prompt_injection
  verdict TEXT NOT NULL,                  -- pass | fail
  detail TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 沙箱执行记录（每次调用一行）
CREATE TABLE IF NOT EXISTS sandbox_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL,
  input TEXT DEFAULT '',
  transcript TEXT DEFAULT '[]',           -- 多轮交互记录
  output TEXT DEFAULT '',
  artifacts TEXT DEFAULT '[]',            -- 产出物文件路径
  duration_ms INTEGER DEFAULT 0,
  timeout INTEGER DEFAULT 0,
  exit_code INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Agent 评测结果（每评测项一行）
CREATE TABLE IF NOT EXISTS pipeline_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL,
  agent TEXT NOT NULL,                    -- simulate_user | process_audit | quality_judge
                                          -- | compliance | safety_redline | logic_detemplate
  item TEXT NOT NULL,                     -- 评测项名称
  score REAL DEFAULT 0,
  threshold REAL DEFAULT 0,
  passed INTEGER DEFAULT 0,
  reason TEXT DEFAULT '',
  evidence TEXT DEFAULT '{}',             -- 证据引用（sandbox_run_id / 日志 / 产物路径）
  confidence REAL DEFAULT 1,              -- 置信度，< 0.6 自动标记人工复核
  needs_human_review INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 人工复核记录
CREATE TABLE IF NOT EXISTS human_reviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  result_id INTEGER NOT NULL,
  reviewer_id INTEGER NOT NULL,
  decision TEXT NOT NULL,                 -- confirm | override_pass | override_fail
  note TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 用例模板库（按类型/类目，供契约缺省时补用例）
CREATE TABLE IF NOT EXISTS case_templates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_type TEXT NOT NULL,
  category TEXT DEFAULT '',
  template TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## 5. 关键模块伪代码

### 5.1 沙箱抽象（sandbox.go）

```go
// 统一接口：本机用受限子进程，生产切 Docker
type Sandbox interface {
    Exec(ctx context.Context, req ExecRequest) (*ExecResult, error)
    Close() error
}
type ExecRequest struct {
    SkillID    int64
    Env        EnvRequirements
    Prompt     string   // Skill 的 system prompt / 工作流
    Input      string   // 用户输入（对话 / 任务）
    WorkDir    string   // Skill 包解压目录
    Timeout    time.Duration
}
type ExecResult struct {
    Transcript []TranscriptTurn // 多轮对话记录（经验型）
    Output     string           // 最终文本产出
    Artifacts  []string         // 产出物文件列表
    Duration   time.Duration
    TimedOut   bool
}

// 本机受限子进程实现
type LocalSandbox struct{ workRoot string }
func (s *LocalSandbox) Exec(ctx, req) (*ExecResult, error) {
    dir, _ := os.MkdirTemp(s.workRoot, "sandbox-*")
    defer os.RemoveAll(dir)
    ctx, cancel := context.WithTimeout(ctx, req.Timeout); defer cancel()
    if req.Env.StartCommand != "" && hasScript(req.WorkDir) {
        // ① 脚本执行路径：受限子进程，stdin 传用例，stdout 产出物
        cmd := exec.CommandContext(ctx, "python", "-B", req.Env.StartCommand)
        cmd.Dir = req.WorkDir
        cmd.Stdin = strings.NewReader(req.Input)
        // 隔离：清空敏感环境变量、限制工作目录、超时强制杀
        cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "PYTHONDONTWRITEBYTECODE=1"}
        out, _ := cmd.CombinedOutput()
        return &ExecResult{Output: string(out), ...}, nil
    }
    // ② 模型执行路径：Prompt + Input → LLM 回复（经验型对话由 Agent 编排多轮）
    reply := callLLM(req.Prompt, req.Input) // DeepSeek
    return &ExecResult{Output: reply, ...}, nil
}
// DockerSandbox：接口已定义，生产环境实现（build 镜像 → run --rm -v 挂载卷 → 资源限制 → 超时销毁）
```

### 5.2 静态扫描（eval_static.go）

```
scanSkill(runID, skillDir):
  for 每个包内文件:
    code_safety:      危险调用正则（os.system / subprocess(shell=True) / eval( / requests 外联 / .. 路径穿越 / 读密钥）
    dependency_safety: 读 requirements.txt / package.json，比对已知漏洞包名单
    prompt_injection: 读 SKILL.md / prompt.*，匹配「忽略系统提示 / 你是管理员 / override」
  任一 fail → static_scans 记 fail，管道直接 rejected，不进沙箱
```

### 5.3 Agent 集群（eval_agents.go）

```go
type Agent interface {
    Name() string
    Run(ctx, runID int64, contract SkillContract, ev EvalEvidence) ([]ResultRow, error)
}
// 模拟用户：按角色模板 + 契约示例生成对话脚本 → 逐轮调沙箱 → 记录 transcript
// 过程审计：读取 transcript，按 process_checklist 逐项问 LLM「这一步是否出现」，算覆盖率
// 质量评判：产出文本多维打分（逻辑/信息完整/风险清醒），置信度 < 0.6 → needs_human_review
// 合规检测：确定性脚本（字数 / 结构标题 / 引用格式正则）；图片 OCR/视觉留 stub
// 安全红线：敏感词表 + LLM 判定；命中 → 一票否决
// 去模板化：句向量相似度查车轱辘话 + LLM 查论证链条
```

### 5.4 管道编排（eval_pipeline.go）

```
runPipeline(ctx, runID):
  run.stage = static_scan → scanSkill()
  if 静态扫描 fail: run = rejected; return
  run.stage = sandbox → 按契约用例逐条 sandbox.Exec，记录 sandbox_runs
  run.stage = agents → 并行跑全部 Agent，写入 pipeline_results
  run.stage = report → 汇总：一票否决 ? rejected : (低置信度 ? needs_review : published)
```

## 6. 安全最佳实践（Docker 沙箱要点，写入代码注释与部署文档）

1. **静态扫描先行**：任何含危险调用/依赖的包不进沙箱。
2. **资源硬限制**：容器 `--memory=512m --cpus=0.5 --pids-limit=64`；超时 `--timeout` 强制 `docker kill`。
3. **无特权**：`--network=none`（默认不联网；需要检索的 Skill 通过内置 mock API 服务）；`--read-only` 根文件系统 + 挂载卷只写产物目录。
4. **用户隔离**：容器内以非 root 用户运行（`--user 10001:10001`）。
5. **一次性**：每次评测创建新容器，跑完即销毁，不缓存状态。
6. **本机受限子进程兜底**：无 Docker 环境用子进程 + 超时 + 空环境变量 + 临时目录隔离，仅用于开发调试，生产强制 Docker。

## 7. MVP 范围（本次实现）

- 契约 + 环境需求上传（`createSkill` 扩展 + `Publish.vue` 表单）
- 五阶段管道骨架 + 静态扫描 + 沙箱（受限子进程 + 模型执行） + Agent 骨架（模拟用户/过程审计/质量评判/合规/安全红线/去模板化） + 报告决策 + 人工复核
- 前后端 API：submit/run/report/human-review/test-cases/preview
- 端到端验证一条经验型 + 一条产出型
