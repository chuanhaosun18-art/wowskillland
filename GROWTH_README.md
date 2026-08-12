# SkillHub 成长闭环改造说明（PRD P0）

本轮在原有 SkillHub（Skill 市场）之上，把产品主张落成了代码。原有功能全部保留，数据不删。

## 一句话变化

改造前是**Skill 商城**（评分 + 下载量排序 + 直接发布）；
改造后是**先做一遍再固化**的成长系统：做事发生在平台内 → 供给是执行的副产品 → 信任来自证据而非评分。

## 三条不可妥协的主张，以及它们落在哪

| 主张 | 代码位置 | 验证方式 |
|---|---|---|
| 做事发生在平台内 | `growth_workbench.go`，所有执行必须落 `execution_steps` | 走完一次任务后查 `SELECT COUNT(*) FROM execution_steps` |
| 供给是执行的副产品 | `growth_creator.go`，草稿由轨迹自动生成，用户只确认 | 四槽候选每条都带 `source_step_index` |
| 信任来自证据而非评分 | `growth_trust_loop.go` 的 Trust Card 不返回任何综合分 | 响应里搜 `rating` 应为空 |

## 新增文件

**后端（Go）**

| 文件 | 内容 |
|---|---|
| `growth_db.go` | 11 张新表的增量迁移、枚举常量、门禁阈值、语料种子（20 条真实原话） |
| `growth_intent.go` | F1 目标识别 + 四筛判定 + 五类伪需求拒绝策略 |
| `growth_workbench.go` | F4 任务工作台：推进、关键判断停顿、工具调用、完成信号、放弃判定 |
| `growth_creator.go` | F5 轨迹抽取、蒸馏度六维、降级为 Insight、六 slot 文件夹生成 |
| `growth_gate.go` | F6 发布前四问 + 发布门禁 |
| `growth_route.go` | F7 准入四层与质量分 + F8 两段式路由排序与解释 |
| `growth_trust_loop.go` | F10 Trust Card 与判断级溯源 + F12 反馈闭环与版本升级 |

**前端（Vue 3）**

| 文件 | 内容 |
|---|---|
| `src/api/growth.js` | 成长闭环 API 层，409 门禁错误会带出 `blocked` / `still_missing` |
| `src/views/Grow.vue` | 我要成长：一句人话 → 任务卡 → 路由结果（含「为什么没选另一个」） |
| `src/views/Workbench.vue` | 任务工作台三栏：上下文 / 协作执行 / 实时轨迹 |
| `src/views/Creator.vue` | 四槽确认 + 蒸馏度六项实时显示 + 生成 Skill 包 |
| `src/views/Gate.vue` | 发布前四问，可发现性未过会列出没被召回的原话 |
| `src/views/TrustCard.vue` | 七分区 + 判断级溯源浮层（脱敏） |

## 改动的既有文件

| 文件 | 改了什么 | 为什么 |
|---|---|---|
| `db.go` | 末尾调用 `initGrowthSchema()` | 挂载增量迁移 |
| `main.go` | 新增 `/api/growth/*` 路由组 | — |
| `handlers.go` | 列表排序移除评分/下载量，改按 `quality_score`；只显示 `published`；上传落地为 `gated` 并建版本行 | 热度反映注意力，任务证据才说明能力；路线二必须补一次真实执行 |
| `AppNavbar.vue` | 主按钮改为「我要成长」，发布降级为「上传已有 Skill」 | 前台卖下一步，Skill 是底层设施 |
| `Home.vue` | 首屏改为「你现在卡在哪」+ 我要成长入口，搜索降为次级 | 同上 |
| `SearchResults.vue` | 去掉星级与按评分排序，热度改为「注意力参考」 | 信任面不出现综合评分 |
| `SkillDetail.vue` | 星级换成 Trust Card 入口；下载/浏览弱化；评价区加"不参与排序"说明 | 同上 |

## 数据库迁移

纯增量，**不会丢现有数据**：新建 11 张表，给 `skills` 补 6 列（`status` / `task_intent` / `origin` / `maintainer_id` / `current_version_id` / `quality_score`）。老记录的 `status` 默认 `published`，因此历史 Skill 不会从市场消失。

## 启动

```powershell
cd backend
$env:DEEPSEEK_API_KEY="sk-..."          # 必须：目标识别、轨迹抽取、eval 判定都要
$env:DEEPSEEK_GUIDE_API_KEY="sk-..."    # 可选：工作台与抽取用，未配置则回退
$env:SKILLHUB_DATA="D:\skillhub-data"   # 可选
go run .

cd ..\qianduan
npm run dev
```

## 冒烟验证（按 Demo 顺序）

```powershell
$T = "<登录后拿到的 token>"
$H = @{ Authorization = "Bearer $T"; "Content-Type" = "application/json" }

# 1. 伪需求必须被拦住：不创建任务、不落 Experience
irm -Method POST http://localhost:8080/api/growth/goals/interpret -Headers $H `
  -Body '{"utterance":"我最近很焦虑，不知道该干什么"}'      # 期望 mode=rejected

# 2. 真需求识别 + 四筛
irm -Method POST http://localhost:8080/api/growth/goals/interpret -Headers $H `
  -Body '{"utterance":"我的选题被导师退了两次，说范围太大"}'  # 期望 mode=task

# 3. 建执行 → 反复 advance（会出现关键判断停顿）→ decide → complete
irm -Method POST http://localhost:8080/api/growth/executions -Headers $H `
  -Body '{"task_intent":"thesis_topic","task_title":"收窄选题","goal":"让导师放行","material":"我想研究大模型"}'

# 4. 固化 → 看蒸馏度六项
irm -Method POST http://localhost:8080/api/growth/executions/1/distill -Headers $H

# 5. 生成文件夹（未达门槛会返回 409 + still_missing）
irm -Method POST http://localhost:8080/api/growth/drafts/1/generate-folder -Headers $H

# 6. 跑四问（边界停机必须 100%）
irm -Method POST http://localhost:8080/api/growth/skills/1/evals/run -Headers $H

# 7. 发布（任一项没过会返回 409 + blocked 逐条原因）
irm -Method POST http://localhost:8080/api/growth/skills/1/publish -Headers $H

# 8. Trust Card 里不应出现任何综合评分
irm http://localhost:8080/api/growth/skills/1/trust-card
```

## 关键阈值（`growth_db.go` 里集中定义）

| 阈值 | 值 | 能不能调 |
|---|---|---|
| 蒸馏度发布线 | 0.75 | 可以下调 |
| 关键判断维度下限 | 0.5（至少两个槽位） | 可以下调 |
| 适用边界 | 必须为 1 | **不可下调，安全项** |
| 可发现性 recall@5 | 0.80 | 可以下调 |
| 完成 | 0.80 | 可以下调 |
| 稳定 | 0.70 | 可以下调 |
| 边界停机 | 1.00 | **不可下调，安全项** |

## F13 成长路径（补做）

个人中心的主体现在是「我的成长路径」，不是资料表单。全部从真实数据派生，没有任何自填字段：

| 模块 | 数据来源 | 说明 |
|---|---|---|
| 当前位置 | 最近一次 execution + 学业阶段 | 空状态引导去「我要成长」 |
| 成长路线 | executions 按时间排 | 每个节点带步数、关键判断数、是否已固化为 Skill，可点回那次执行 |
| 成长状态四阶 | 按 task_intent 分组 | learned=用过别人的能力；did=有完成的执行；succeeded=完成且产物用出去了；taught=自己发布的 Skill 被别人成功调用过 |
| 能力资产 | 我维护的 skills | 含状态、版本、可溯源判断数、被调用次数；自己看能看到草稿与经验笔记，别人只看到已发布 |
| 影响力 | 他人的 executions 聚合 | 后继者、用过我方法的人、有效完成、贡献的判断、被采纳的反馈、接手维护 |
| 停下来的地方 | abandoned executions + insights | 刻意保留，成长身份才可信 |
| 可见性 | `users.profile_visibility` | 五段独立开关，**默认全部不公开** |

接口：`GET /api/growth/my-profile`、`GET /api/growth/profile/:id`（按可见性过滤）、`PATCH /api/growth/my-profile/visibility`。
页面：个人中心顶部嵌入 `components/GrowthPath.vue`；`/growth/:id` 看别人的成长身份；Trust Card 的创作者与维护者可点进去（前路关系入口）。

注意两处近似：`successors`（后继者）目前用「用过我的方法且自己也做成过的人」近似，等 Growth Graph 的 `path_follows` 上线后改用真实跟走关系；`taught` 要求对方执行状态为 completed，所以新库里一开始都是空的。

## F17 编排态（PRD v1.1/v1.2 新增）

长周期方向性需求（保研、考研、出国、求职季）的第三个出口。交付的不是产物，是一份带时间的编排。

**三态路由现在是这样：**

| 输入 | 出口 | 交付物 |
|---|---|---|
| 我的选题被导师退了两次 | 任务态 | 一份改好的选题 |
| 我决定保研了，接下来怎么准备 | **编排态** | 一份 8 周的编排 |
| 我到底该不该保研 | 拒绝态 | 别人走过的分支与人数，不给建议 |
| 我最近很崩溃 | 拒绝态（全拦） | 一段回应 + 心理支持入口，不落任何记录 |

**硬约束在代码里的落点：**

| 约束 | 实现位置 |
|---|---|
| 无来源 Path 不生成编排 | `orchestration_items.source_path_id` 建表时 NOT NULL；`probeOrchestration` 先拦一道；`createOrchestration` 里无来源节点的项直接丢弃 |
| 不出现任何成功率 | `branchSummaryText` 只输出绝对人数；Prompt 里明令禁止百分比；响应带 `no_outcome_promise: true` |
| 不可控项独立分组 | `respondOrchestration` 按 `controllable` 分流，前端 `uncontrollable` 区无勾选框 |
| retrospective 不给耗时与卡点 | `loadPathNodes` 不返回 `typical_duration_days` / `common_blocker`；界面显示来源提示 |

**新增接口：**`POST /orch-probe`、`POST /orch-interview`、`POST/GET /orchestrations`、`GET /orchestrations/:id`、`POST /orchestrations/:id/adopt`、`PATCH /orchestrations/:id/items/:itemId`、`POST /orchestrations/:id/reviews`。
页面：`/orchestration`（访谈 → 编排 → 采纳 → 周复核）。

**预置数据：**`seedPaths()` 建了一条保研 Path，10 个节点，`provenance=retrospective`（诚实标注是回忆整理，不是平台内观测）。Demo 时选「考研准备」会触发"没人走过、拒绝生成"那一幕——**这一幕比生成成功更能说明平台的判断力，建议放进路演**。

## v1.2 的四条规则级改动

| 改动 | 实现 |
|---|---|
| intent 分层 | `ProductiveIntents` 白名单。模拟面试、面试复盘、内容脚本属于只消费类，不要求关键判断、不提示固化 |
| 轨迹补录 | `POST /growth/backfill`。承认用户在平台外做事，但 `proof_type=artifact_upload` 时蒸馏度**封顶 0.85**（`distillDetail.total()` 里实现），Trust Card 标注"来源为补录，无执行轨迹" |
| 冷启动反馈门槛 | `checkVersionTriggers` 里按调用量切换：≥20 次用 3 次/3 人，<20 次用 2 次/2 人 |
| choose_if | 路由每个结果附一句"选它取决于什么"，依据该 Skill 流程第一步是否为盘点类判断。**不引入任何分数** |

## 已知未完成

- F2 Growth Graph / F3 同行组：后端未实现，前端未接（PRD 里是 P1）
- F14 毕业移交 / F15 定价与归属：表结构已在 PRD，本轮未建表（P2）
- F16 运营后台：未实现
- `reused_within_7d` 需要定时任务回填，当前恒为 false
- 工具 `topic_similarity_check` 目前用站内语料做朴素相似度，不是真实学术检索
- **本轮代码未经编译验证**（开发沙箱无 Go）。已做静态自检：无未使用 import、无括号不平衡、无重复定义、无跨文件未定义调用；前端 12 个 SFC 全部通过 `@vue/compiler-sfc` 编译。首次 `go run .` 若报错，大概率是类型细节，请把报错发我。
