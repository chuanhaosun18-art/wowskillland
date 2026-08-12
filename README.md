# WowSkillLand

面向大学生迷茫期的 Skill / Agent 平台。一句话定位：**每一次「我不知道」，都有人真走过。**

三条纪律贯穿全部界面：**不测评、不建议、不承诺。**

本仓库是 WowSkillLand 当前前端渲染的完整套件：
- 前端：`wowskillland-app/`（纯静态单页应用，hash 路由，手机适配）
- 后端：`backend/`（Go + Gin + SQLite）
- 数据：`data/skillhub.db`（用户 + Skill 库，随仓库分发）
- 供给：`skills/`（Skill 包原始文件）

---

## 具备什么功能

### 1. 首页意图路由器（discuss）
- 首页输入框「说说你现在的不知道」，后端 LLM 做**意图识别**：识别意图后召回 Skill 库中最匹配的 Skill，前端渲染成可点击的 **Skill 卡片**，点击跳转到对应 Skill 详情页直接使用
- 未登录 / 后端不可用时降级为前端本地关键词匹配，保证体验不中断

### 2. 成长闭环（workbench / creator / gate / trust）
- **目标识别**：三态路由（探索 / 抉择 / 编排 / 情绪）+ 四筛判定 + 五类伪需求拦截
- **任务工作台**：advance 推进 / 关键判断点停顿 / 工具调用 / 完成 / 放弃判定，每步落 `execution_steps`
- **收尾三问**：LLM 生成收尾问题，提交后 `verdict_reason` 落库
- **供给飞轮**：轨迹 → 四槽 → 蒸馏度六维 → 六 slot Skill 包，降级为 Insight
- **发布门禁**：发布前四问（边界停机 100% 不可下调），失败给出反向指导
- **Trust Card**：七分区 + 判断级溯源，不返回任何综合分
- **编排态**：orch-probe / interview / 采纳 / 周复核，只输出绝对人数

### 3. 路口 / 迷茫期路由器
- 四个路口：大0 本省/外省、大二 转专业、大三 保研/就业/考研、大四 工作/读研
- 十字路口 E2E：moment → 轻访谈 → 假设生成 → 卡片匹配 → attempt 陪跑 → 许愿池 / 分叉 / 地图
- 供给分三层：故事层（学长口述）/ 卡片层（可执行第一步）/ 分布层（绝对人数与代价原话）

### 4. Skill 市场
- 技能列表（按质量分排序，只显示 published）、搜索、分类、详情、zip 下载
- 评分 / Issue 反馈、AI 个性化解读（按用户 AI 水平分级，结果缓存）

### 5. 用户体系
- 注册（AI 熟练度问卷）+ JWT 登录 + 个人画像（学校/专业/年级）+ 我的发布管理

### 6. 其他模块
- 论坛、通知、Persona 人格、Direct Chat 直聊

---

## 改了什么功能

以下为在原 SkillHub 基础上针对 WowSkillLand 的改造（`feature/wowskillland-demo` 分支累计）：

1. 首页 discuss **从本地固定模板改为真实 LLM 意图识别**（此前后端无对应路由，前端静默降级；现后端新增接口并注入全部 Skill 清单）
2. **修复「生成使用指南」invalid skill id** 报错
3. **后端 createExecution 支持 SkillID**、workbenchSystemPrompt 改造、**修复 advanceExecution 判断点死循环**
4. 删除「SKILL.md 全文注入 system prompt」不实文案
5. **runChat 重构**：剥离 decision 拦截
6. **后端 closing 三问 LLM 化** + `verdict_reason` 落库
7. 前端**收尾闭环**、收尾问题动态渲染、`ensureExecId` 收尾复用
8. **explain 支持 markdown 渲染**
9. 市场列表排序由评分/下载量改为按 `quality_score`（任务证据），只显示 `published`

## 加了什么功能

1. **意图识别 agent**：`POST /api/growth/wow/discuss`——LLM 分类（intent / matched_skill_ids / reply），Skill 清单注入 system prompt，JSON 解析容错（`repairClosingJSON`）+ 关键词降级匹配（论文→论文Skill、图→图表Skill）；任何异常返回 200 + `degraded=true`，绝不 500
2. **前端 Skill 卡片**：discuss 回复渲染 `.skill-mini-card`（类型徽标 + 标题 + 描述），点击跳转 Skill 详情；`be-{id}` 前缀映射缓存进前端 DB 保证详情页可跳
3. **十字路口全套后端**：moments / interviews / hypotheses / cards / attempts / coach / wishes / forks / map 的 DB 表与路由
4. 页面收尾流程、收尾问题动态渲染、`ensureExecId` 复用机制

---

## 怎么跑

需要 Go 1.26+、Python 3（或任意静态服务器）、DeepSeek API Key。

```bash
# 1. 后端（默认端口 8080）
cd backend
export SKILLHUB_DATA="$PWD/../data"     # 指向本仓库自带的数据库
export DEEPSEEK_API_KEY="sk-..."
go run .

# 2. 前端（静态服务）
cd ../wowskillland-app
python3 -m http.server 8010
# 打开 http://localhost:8010
```

手机直接访问同一地址即可（底栏导航）。登录 / 注册走后端 JWT；登录后首页输入会打 LLM 意图识别。

> 数据默认存放在 `D:\skillhub-data`（本机路径），可用环境变量 `SKILLHUB_DATA` 覆盖；仓库自带的数据库在 `data/skillhub.db`。

## 环境变量一览

| 变量 | 用途 | 必填 |
| --- | --- | --- |
| `DEEPSEEK_API_KEY` | 意图识别 / 技能解读 / 引导对话 / 成长闭环 | 是 |
| `DEEPSEEK_GUIDE_API_KEY` | 引导对话专用 key | 否（回退上面） |
| `SKILLHUB_PORT` | 后端端口 | 否（默认 8080） |
| `SKILLHUB_DATA` | 数据存储目录 | 否（默认 `D:\skillhub-data`） |

## 目录结构

```
.
├── wowskillland-app/        # 前端（静态单页，hash 路由）
│   ├── index.html
│   ├── css/app.css
│   └── js/                  # api.js / app.js / data.js / real_skills.js
├── backend/                 # 后端（Go + Gin + SQLite）
│   ├── main.go              # 入口 + 路由 + CORS
│   ├── growth_*.go          # 成长闭环 + 意图识别
│   ├── crossroad_*.go       # 十字路口
│   ├── llm.go               # DeepSeek 调用
│   └── ...                  # 市场 / 认证 / 论坛 / 通知等
├── data/
│   └── skillhub.db          # SQLite 数据库（用户 + Skill）
├── skills/                  # Skill 包原始文件
└── docs/                    # 产品设计文档
```

## 主要 API

| 方法 | 路径 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| POST | `/api/auth/register` / `/api/auth/login` | 注册（AI 问卷）/ 登录 | 否 |
| GET | `/api/skills` | Skill 列表（质量分排序） | 否 |
| POST | `/api/growth/wow/discuss` | **意图识别**（LLM + Skill 检索 + 降级） | 是 |
| POST | `/api/growth/goals/interpret` | 三态路由（探索/抉择/编排/情绪） | 是 |
| POST | `/api/growth/executions` + `/:id/advance` | 任务工作台 | 是 |
| POST | `/api/growth/backfill` | 轨迹补录（蒸馏度封顶 0.85） | 是 |
| GET | `/api/growth/skills/:id/trust-card` | Trust Card（七分区溯源） | 否 |
| POST | `/api/growth/orch-probe` | 编排探测（无人走过的方向拒绝生成） | 是 |
| POST | `/api/crossroad/moments` ... | 十字路口全套 | 是 |
