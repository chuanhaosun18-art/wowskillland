---
title: "SkillX WowLand 产品需求文档（PRD）"
subtitle: "大学生成长复利系统 · 完整产品范围 · 工程可实施版"
---

**版本 PRD v1.0 · 2026-08-12 · 对应产品方案 V0.2**

**读者：**前端、后端、AI 工程、设计、运营。本文档的目标是让实现者不需要回头问"这里到底要什么"。

# 目录

PRD 说明与优先级定义　·　1. 产品定位与范围（含 Out of Scope 与伪需求拒绝策略）　·　2. 角色与权限矩阵　·　3. 术语与核心概念　·　4. 领域模型与数据表 schema　·　5. 状态机（Skill / Execution / 版本候选 / 移交）　·　6. 功能模块 F1–F16　·　7. AI 能力规格 P1–P6　·　8. 接口契约　·　9. 指标与埋点　·　10. 非功能需求　·　11. 风险与降级　·　12. 48 小时 P0 交付切片　·　附录 A 验收标准索引　·　附录 B 不可妥协清单　·　附录 C 与方案 V0.2 的对应关系



# 0. 文档说明

## 0.1 本文档与产品方案的关系

产品方案 V0.2 回答"为什么做、做什么、凭什么相信"；本 PRD 回答"具体做成什么样、字段叫什么、什么算做完"。两者冲突时以本 PRD 为准，并回改方案。

## 0.2 优先级定义

| 级别 | 含义 | 48 小时黑客松 |
|---|---|---|
| P0 | 缺了它整个产品主张不成立 | 必须做 |
| P1 | 缺了它产品能跑但不完整 | 有余力做，可用假数据演示 |
| P2 | 完整产品需要，赛后做 | 不做，仅保留数据结构位 |

本文档覆盖完整产品（P0+P1+P2）。第 12 章给出 48 小时只做 P0 的切片。

## 0.3 阅读约定

- 数据库字段名用 `snake_case`，接口字段与之保持一致，不做驼峰转换。
- 所有时间字段为 UTC 时间戳，类型 `timestamptz`，客户端负责本地化。
- 所有 ID 为 `uuid v7`（可按时间排序），主键统一命名 `id`，外键命名 `<table_singular>_id`。
- 枚举值全部小写下划线，定义见 4.3，代码中不得使用字面量魔法字符串。
- 验收标准写成 Given / When / Then，可直接转成测试用例。
- 标注「兜底」的分支是 Demo 现场必须能走通的降级路径。

# 1. 产品定位与范围

## 1.1 一句话定位

一个让大学生在平台内把真实任务做完、并让这次执行自动沉淀为可被他人调用的可信能力单元（Skill）的成长系统。

## 1.2 产品要成立必须同时满足的三件事

| 命题 | 工程含义 | 对应模块 |
|---|---|---|
| 做事发生在平台内 | 必须有承载真实任务执行的工作台，并完整记录轨迹 | F4 任务工作台 |
| 供给是执行的副产品 | Skill 草稿由轨迹自动生成，用户只做确认，不做撰写 | F5 Skill Creator |
| 信任来自证据而非评分 | 任何展示信任的地方都不出现综合评分，必须可下钻到单条判断 | F10 Trust Card |

## 1.3 In Scope

| 域 | 能力 | 优先级 |
|---|---|---|
| 需求入口 | 自然语言目标输入、任务识别、四筛判定、下一步生成 | P0 |
| 执行 | 任务工作台、AI 协作执行、关键判断停顿、轨迹记录 | P0 |
| 供给 | 轨迹抽取、四类判断确认、蒸馏度、Skill 文件夹生成、发布前四问 | P0 |
| 信任 | Trust Card、判断级溯源、授权与审计、执行日志 | P0 |
| 流通 | 准入四层、排序四变量、排序解释、Skill 组合 | P0（组合为 P1） |
| 闭环 | 行为信号采集、版本候选生成、版本升级 | P0 |
| 路径 | Growth Graph、跟走、路径裁剪 | P1 |
| 关系 | 同行组、前路关系、后继者 | P1 |
| 身份 | 个人成长主页、成长状态四阶 | P1 |
| 继承 | 毕业移交、平台代管存档 | P2 |
| 市场 | 定价信号、贡献归属链 | P2（数据结构 P1） |
| 运营 | 种子蒸馏后台、description 语料库、时间窗口推送 | P1 |

## 1.4 Out of Scope（明确不做）

| 不做 | 原因 |
|---|---|
| 支付、结算、提现、分润发放 | 冷启动期分不出有意义的金额，且引入合规成本。只做定价信号与归属记录 |
| 关注、粉丝、私信、评论区 | 本产品的关系是"跟走"与"同行"，不是内容社交。加了会把产品拖回内容平台 |
| Skill 综合评分与排行榜 | 违反核心信任主张（见 1.2） |
| 情绪陪伴、心理咨询类能力 | 需求真实但形态不匹配，且有伦理与安全风险。见 1.5 |
| 人生抉择建议（是否考研/出国/换专业） | 无完成标准、结果不可验证，Skill 化等于算命 |
| 名额竞争类结果承诺（保研、评奖） | 结果由分配规则与他人表现决定，无法归因 |
| 实时信息查询（招聘政策、招聘岗位） | Skill 保存行动方法而非事实，过期的 Skill 比没有更危险。用外部检索承载 |
| 多端适配、国际化、暗色模式 | 非机制必要 |

## 1.5 伪需求的产品级拒绝策略

不做，不等于用户不会来问。系统必须有明确的处理路径，否则会静默地把这些请求也当成任务去执行。

| 场景 | 判定方式 | 系统行为 |
|---|---|---|
| 情绪与心理表达 | P1 任务识别返回 `intent=emotional_support` | 不进入任务流。返回一段不做评判的回应 + 校内心理支持资源入口。不生成任务、不推荐 Skill、不记录为 Experience |
| 人生抉择 | `intent=life_decision` | 不给建议。展示 Growth Graph 上的多条真实分支与各自代价，附"这是别人的路，不是建议"的说明 |
| 名额竞争 | `intent=zero_sum_competition` | 只提供规则信息与可控部分（如材料准备），显式声明结果不可控，禁止出现任何完成率承诺 |
| 实时信息 | `intent=realtime_fact` | 转外部检索，结果标注时效与来源，不落为 Skill |
| 资源依赖 | `intent=resource_dependent` | 拆出可转移部分（如联系邮件写法）走正常任务流，不可转移部分显式标注 |

**硬性约束：**以上五类 `intent` 一律不允许创建 `experiences` 记录，也不允许进入 Skill Creator。

# 2. 角色与权限

## 2.1 角色定义

角色不是互斥身份，而是同一用户在不同对象上的能力集合。用户表不存"角色"字段，权限由下表的条件动态判定。

| 角色 | 判定条件 | 核心能力 |
|---|---|---|
| 成长者 | 任何登录用户 | 输入目标、查看路径、调用已发布 Skill |
| 实践者 | 有进行中的 `execution` | 在工作台执行任务、产生轨迹 |
| 经验者 | 有 `status=completed` 的 execution | 可对该次执行发起固化 |
| 创作者 | 是某 skill 的 `creator_user_id` | 编辑该 skill 草稿、发布、处理版本候选 |
| 维护者 | 是某 skill 的当前 `maintainer_user_id` | 同创作者权限（不含改署名）；接收问题与版本候选 |
| 运营 | `is_staff=true` | 种子蒸馏、语料管理、准入复核、推送配置 |
| 管理员 | `is_admin=true` | 强制下架、处理申诉、移交仲裁 |

## 2.2 权限矩阵

`✓` 允许，`—` 不允许，`△` 有条件。

| 操作 | 成长者 | 创作者 | 维护者 | 运营 | 管理员 |
|---|---|---|---|---|---|
| 调用已发布 Skill | ✓ | ✓ | ✓ | ✓ | ✓ |
| 查看 Skill 判断级溯源 | ✓ | ✓ | ✓ | ✓ | ✓ |
| 查看他人执行日志 | — | — | — | △ 仅脱敏聚合 | ✓ |
| 从自己的执行发起固化 | △ 仅自己的 execution | ✓ | ✓ | ✓ | ✓ |
| 发布 Skill | — | △ 须通过发布前四问 | △ 同左 | △ 同左 | ✓ |
| 编辑已发布 Skill | — | ✓ | ✓ | — | ✓ |
| 修改 Skill 署名 | — | — | — | — | ✓ |
| 接受版本候选 | — | ✓ | ✓ | — | ✓ |
| 发起移交 | — | ✓ | ✓ | — | ✓ |
| 接受移交 | △ 须为提名后继者 | — | — | — | ✓ |
| 手工创建种子 Skill | — | — | — | ✓ | ✓ |
| 管理 description 语料 | — | △ 仅自己的 skill | △ | ✓ | ✓ |
| 强制下架 | — | — | — | — | ✓ |

## 2.3 数据可见性

| 数据 | 默认可见性 | 说明 |
|---|---|---|
| 个人成长主页 | 仅公开用户显式勾选的节点与 Skill | 默认全部不公开，逐项开启 |
| Experience 原始材料 | 私有 | 即使 Skill 已发布，来源材料默认不公开；溯源展示的是判断与场景摘要，不是原文 |
| 执行日志 | 仅本人可见 | 用于审计，不对外 |
| 行为信号（放弃、修正） | 仅聚合可见 | 任何界面不得展示"某人放弃了这个 Skill" |
| 成绩、求职结果、心理相关 | 不采集 | 见 10.3 |

# 3. 术语与核心概念

| 术语 | 定义 | 数据载体 |
|---|---|---|
| Experience | 真实发生过的实践事件，最小可信事实 | `experiences` |
| Decision | 会改变结果的单条关键判断，最小可溯源单位 | `decisions` |
| Insight | 解释了原因但尚未形成可执行方法的中间资产 | `insights` |
| Skill | 最小可调用、可组合、可评估、可版本化、可定价的能力单元 | `skills` + `skill_versions` |
| 蒸馏度 | 一次经验被结构化到何种程度的量化分值 | `skill_versions.distillation_score` |
| 发布前四问 | 可发现性、可完成性、稳定性、边界停机四项发布门禁 | `eval_runs` |
| 准入四层 | 准入检查、离线评测、在线证据、维护状态四层判别 | `skill_scores` |
| 前路关系 | 走过我正在走的路的人 | `path_follows` |
| 同行关系 | 与我在同一段路、同一时间窗口的人 | `companion_groups` |
| 后继者 | 正在沿着某人贡献的方法继续成长的人 | `path_follows` + `executions` 派生 |
| 时间窗口 | 由学期日历驱动的高需求区间 | `time_windows` |

# 4. 领域模型

## 4.1 对象关系

一个用户在一个任务上产生一次执行；执行完成后可固化为一个 Skill 版本；Skill 版本由若干判断构成，每条判断指回来源执行；他人调用该 Skill 产生新的执行，新执行产生行为信号与反馈，反馈累积成版本候选，版本候选被接受后产生新版本。

| 关系 | 基数 | 说明 |
|---|---|---|
| users → executions | 1:N | |
| executions → experiences | 1:0..1 | 完成且用户选择固化时生成 |
| experiences → decisions | 1:N | |
| experiences → insights | 1:N | 蒸馏度不足时的降级产物 |
| skills → skill_versions | 1:N | 当前版本由 `current_version_id` 指向 |
| skill_versions → decisions | M:N | 通过 `skill_version_decisions` |
| skills → executions | 1:N | 调用记录 |
| executions → feedbacks | 1:N | |
| feedbacks → version_candidates | M:N | 同类反馈聚合 |
| skills → handovers | 1:N | 移交历史 |
| paths → path_nodes → skills | 1:N:M | |

## 4.2 数据表定义

### users

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| nickname | text | not null | |
| school_id | uuid | FK, nullable | |
| department | text | nullable | 院系，服务裂变单元统计 |
| cohort | text | nullable | 入学届，如 `2024` |
| grade_stage | enum | nullable | 见 4.3 `grade_stage` |
| direction | text | nullable | 方向自述，自然语言，不做枚举 |
| is_staff | bool | default false | |
| is_admin | bool | default false | |
| graduation_expected_at | date | nullable | 触发移交提醒 |
| created_at / updated_at | timestamptz | not null | |

索引：`(department, cohort)` 用于同行组与院系运营。

### user_profile_signals

画像不是问卷，是执行副产品。每条信号独立存储、可追溯、可失效。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| user_id | uuid | FK not null | |
| signal_type | enum | not null | `artifact_level` / `blind_spot` / `constraint` / `preference` / `stage` |
| signal_key | text | not null | 如 `time_budget`、`data_access` |
| signal_value | jsonb | not null | |
| confidence | numeric(3,2) | 0–1 | |
| source_execution_id | uuid | FK nullable | 该信号从哪次执行推出 |
| expires_at | timestamptz | nullable | 阶段类信号需要过期 |
| created_at | timestamptz | not null | |

**规则：**同 `signal_key` 保留最近 3 条，取 `confidence` 加权最新值。任何画像展示必须能回答"这条是从哪次执行推出来的"。

### experiences

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| user_id | uuid | FK not null | |
| execution_id | uuid | FK nullable | 平台内执行则非空；路线二导入则为空 |
| task_title | text | not null | |
| task_intent | enum | not null | 见 4.3 `task_intent`，五类伪需求禁止入库 |
| context | jsonb | not null | 起点、约束、资源 |
| process_summary | text | not null | |
| result | jsonb | not null | 含 `done_criteria_met` bool |
| failures | jsonb | default '[]' | |
| proof_type | enum | not null | `platform_trace` / `artifact_upload` / `self_report` |
| proof_ref | text | nullable | |
| visibility | enum | default `private` | |
| created_at | timestamptz | not null | |

**约束：**`proof_type='platform_trace'` 时 `execution_id` 必须非空。发布 Skill 要求至少一个 `proof_type='platform_trace'` 的来源 experience（见 F6 门禁）。

### decisions

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| experience_id | uuid | FK not null | 来源，不可为空——这是判断级溯源的根 |
| slot | enum | not null | `when_to_check` / `when_to_probe` / `when_to_use_tool` / `when_to_switch` |
| trigger_signal | text | not null | 出现什么信号 |
| judgment | text | not null | 就要怎么做 |
| scope | text | not null | 在什么场景下成立 |
| counter_example | text | nullable | 已知反例 |
| verified_by_count | int | default 0 | 被多少次独立执行验证过 |
| invalidated_at | timestamptz | nullable | 被证伪时间；非空则不参与流程渲染 |
| created_at | timestamptz | not null | |

**规则：**一条判断被证伪时只失效自身，不整体降级所属 Skill；但若 Skill 的 `when_to_switch` 槽位全部失效，Skill 自动转 `needs_review`。

### insights

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| experience_id | uuid | FK not null | |
| claim | text | not null | |
| why | text | not null | 解释了什么 |
| missing_for_skill | jsonb | not null | 距离成为 Skill 还缺哪些维度，直接来自蒸馏度缺口 |
| created_at | timestamptz | not null | |

### skills

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| name | text | not null | |
| creator_user_id | uuid | FK not null | 永久署名，不可变更 |
| maintainer_user_id | uuid | FK not null | 可随移交变更 |
| current_version_id | uuid | FK nullable | 已发布的当前版本 |
| status | enum | not null | 见 4.3 `skill_status` |
| task_intent | enum | not null | |
| origin | enum | not null | `route_one_execution` / `route_two_import` / `ops_seed` |
| created_at / updated_at | timestamptz | not null | |

### skill_versions

一个 Skill 的所有可执行内容都在版本上，Skill 本身只存身份与状态。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| skill_id | uuid | FK not null | |
| version | text | not null | 语义化，如 `1.0`、`1.1` |
| description | text | not null | 用用户真实表达写，决定何时被召回 |
| goal | text | not null | |
| done_criteria | jsonb | not null | 可判断的完成标准，至少一条 |
| stage_fit | jsonb | not null | 适用成长阶段 |
| workflow | jsonb | not null | 有序步骤，每步可挂 decision_id |
| boundary | jsonb | not null | `not_applicable[]`、`handoff_trigger[]`、`fallback_path` |
| contract | jsonb | not null | `input`、`output`、`permissions[]`、`side_effects[]` |
| dependencies | jsonb | default '{}' | `tools[]`、`data_sources[]`、`prerequisite_skill_ids[]` |
| cost_budget_ms | int | nullable | 预算时延 |
| cost_budget_token | int | nullable | 预算成本 |
| distillation_score | numeric(3,2) | 0–1 | 见 F5.3 |
| changelog | text | nullable | 相对上一版改了什么、为什么 |
| published_at | timestamptz | nullable | |
| created_at | timestamptz | not null | |

唯一约束：`(skill_id, version)`。

### skill_version_decisions

| 字段 | 类型 | 说明 |
|---|---|---|
| skill_version_id | uuid | FK |
| decision_id | uuid | FK |
| workflow_step_index | int | 挂在流程第几步，用于 Trust Card 下钻 |

主键：`(skill_version_id, decision_id)`。

### skill_files

对应材料给出的文件夹分工。文件内容存对象存储，表内存元数据。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| skill_version_id | uuid | FK not null | |
| slot | enum | not null | `skill_md` / `references` / `scripts` / `assets` / `gotchas` / `evals` |
| path | text | not null | 相对路径，如 `gotchas/scope-too-broad.md` |
| storage_uri | text | not null | |
| bytes | int | not null | |
| checksum | text | not null | |

**约束：**每个版本必须且只能有一个 `slot='skill_md'` 的文件。`slot='evals'` 至少一个文件才允许发布。

### skill_relations

| 字段 | 类型 | 说明 |
|---|---|---|
| from_skill_id | uuid | FK |
| to_skill_id | uuid | FK |
| relation | enum | `prerequisite` / `next` / `composable` / `alternative` |

主键 `(from_skill_id, to_skill_id, relation)`。禁止 `prerequisite` 成环，写入时做环检测。

### evals / eval_runs

| evals 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| skill_version_id | uuid | FK |
| eval_type | enum | `discoverability` / `completion` / `stability` / `boundary_stop` |
| input | jsonb | 可发现性为用户原话；其余为任务输入 |
| expected | jsonb | 期望行为，边界类期望为"触发人工接管" |
| is_replay | bool | 是否为旧问题回放 |

| eval_runs 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| skill_version_id | uuid | FK |
| eval_type | enum | 同上 |
| passed_count / total_count | int | |
| pass_rate | numeric(3,2) | |
| detail | jsonb | 每条用例结果 |
| ran_at | timestamptz | |

### executions

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| user_id | uuid | FK not null | |
| skill_version_id | uuid | FK nullable | 空表示无 Skill 的裸任务执行 |
| task_intent | enum | not null | |
| user_context | jsonb | not null | 执行时的画像快照，用于后续归因 |
| input | jsonb | not null | |
| output | jsonb | nullable | |
| status | enum | not null | 见 4.3 `execution_status` |
| completion_signal | jsonb | nullable | 见下方规则 |
| correction_ratio | numeric(3,2) | nullable | 人工修正比例 |
| abandoned_at_step | int | nullable | 放弃发生在第几步 |
| latency_ms | int | nullable | |
| token_cost | int | nullable | |
| started_at / ended_at | timestamptz | | |

**`completion_signal` 规则（这是本产品替代"成功率"的核心）：**

| 字段 | 含义 | 采集方式 |
|---|---|---|
| exported | 是否导出或提交了产物 | 用户点击导出/提交 |
| artifact_delta | 产物相对初始版本的变化量 | diff 计算 |
| reused_within_7d | 7 天内是否再次使用该 Skill | 定时任务回填 |
| manual_rework | 是否在输出后大幅重写 | 编辑距离超阈值 |

### execution_steps

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| execution_id | uuid | FK |
| step_index | int | |
| step_type | enum | `ai_action` / `tool_call` / `user_decision` / `human_handoff` |
| decision_slot | enum | nullable，用户在此处做了哪类判断 |
| user_choice | jsonb | nullable |
| tool_name | text | nullable |
| input / output | jsonb | |
| latency_ms | int | |
| created_at | timestamptz | |

**这张表是整个系统的地基：**Skill 草稿、画像信号、审计日志、行为信号全部由它派生。任何执行都必须落 step，不允许只落 execution。

### feedbacks

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| execution_id | uuid | FK not null | |
| issue_type | enum | not null | `wrong_output` / `missing_boundary` / `unstable` / `dependency_broken` / `not_applicable_to_me` / `other` |
| description | text | nullable | |
| suggested_change | text | nullable | |
| adopted | bool | default false | |
| adopted_version_id | uuid | FK nullable | |
| created_at | timestamptz | not null | |

### version_candidates

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| skill_id | uuid | FK not null | |
| trigger_rule | enum | not null | `repeated_failure` / `correction_rate_high` / `dependency_unhealthy` / `new_boundary_verified` / `manual` |
| evidence | jsonb | not null | 触发依据，含关联 feedback/execution id 列表 |
| status | enum | not null | `open` / `accepted` / `dismissed` / `expired` |
| resulting_version_id | uuid | FK nullable | |
| created_at | timestamptz | not null | |

### skill_scores

准入四层的物化结果，每层独立打分，便于解释与调试。

| 字段 | 类型 | 说明 |
|---|---|---|
| skill_id | uuid | PK |
| admission_passed | bool | 第一层：结构、依赖、权限、数据边界、适用范围 |
| admission_failures | jsonb | 未通过项列表 |
| offline_score | numeric(3,2) | 第二层：四类 eval 加权 |
| online_score | numeric(3,2) | 第三层：采用、修正、失败、复用、留存 |
| maintenance_score | numeric(3,2) | 第四层：版本活跃度、依赖健康、问题响应 |
| quality_score | numeric(3,2) | 综合，见 F8 公式 |
| is_candidate_eligible | bool | 是否进入排序候选集 |
| computed_at | timestamptz | |

### paths / path_nodes / path_edges / path_follows

| paths | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| goal_label | text | 目标节点名，如 `产研实习` |
| owner_user_id | uuid | nullable，个人真实路径则非空；空表示聚合图谱 |

| path_nodes | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| path_id | uuid | FK |
| label | text | |
| stage | enum | |
| skill_ids | uuid[] | 该节点代表性 Skill |
| experience_ids | uuid[] | 真实案例 |
| typical_duration_days | int | nullable |

| path_edges | 类型 | 说明 |
|---|---|---|
| from_node_id / to_node_id | uuid | FK |
| walked_count | int | 有多少人真实走过这条边，用于分支权重 |

| path_follows | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| follower_user_id | uuid | FK |
| source_path_id | uuid | FK |
| source_owner_user_id | uuid | nullable，构成前路关系与后继者统计 |
| tailored_nodes | jsonb | 裁剪后的个人版本 |
| status | enum | `active` / `completed` / `dropped` |
| created_at | timestamptz | |

### companion_groups / companion_members

| companion_groups | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| path_node_id | uuid | FK |
| time_window_id | uuid | FK |
| department | text | nullable，同院系优先成组 |
| cohort | text | nullable |

| companion_members | 类型 | 说明 |
|---|---|---|
| group_id / user_id | uuid | FK |
| joined_at | timestamptz | |

**成组规则：**同 `path_node_id` + 同 `time_window_id` + 同 `department` 优先；人数不足 3 时放宽到同校，再放宽到同 cohort。上限 30 人，超出则分组。

### handovers

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| skill_id | uuid | FK |
| from_user_id / to_user_id | uuid | FK；to 为空表示转平台代管 |
| reason | enum | `graduation` / `inactive` / `voluntary` / `admin` |
| status | enum | `nominated` / `accepted` / `declined` / `expired` / `archived` |
| nominated_at / resolved_at | timestamptz | |

### pricing_signals / attributions

| pricing_signals | 类型 | 说明 |
|---|---|---|
| skill_id | uuid | PK |
| call_count / effective_count / composed_count | int | |
| correction_rate / abandon_rate | numeric(3,2) | |
| maintenance_decay | numeric(3,2) | 长期不维护的衰减系数 |
| substitutability | numeric(3,2) | 同任务候选集内可替代程度 |
| suggested_price_index | numeric(6,2) | 见 F15 公式，仅展示不结算 |

| attributions | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| skill_id | uuid | FK |
| user_id | uuid | FK |
| kind | enum | `origin` / `decision_contribution` / `composition` / `maintenance` |
| weight | numeric(5,4) | 归属权重 |
| evidence | jsonb | 依据（被采纳判断 id、组合调用次数等） |

### description_corpus

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| utterance | text | 用户原话 |
| source | enum | `site_search` / `goal_input` / `external_comment` / `eval_failure` |
| task_intent | enum | nullable，标注后填 |
| mapped_skill_id | uuid | nullable |
| used_in_eval | bool | 是否已作为可发现性测试用例 |
| created_at | timestamptz | |

### time_windows

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| label | text | 如 `2026秋开题` |
| window_type | enum | `thesis_proposal` / `autumn_recruit` / `spring_recruit` / `postgrad` / `defense` / `final_exam` |
| starts_at / ends_at | date | |
| target_department | text | nullable |
| primary_task_intents | enum[] | 该窗口的主任务 |

## 4.3 枚举定义

| 枚举 | 取值 |
|---|---|
| grade_stage | `freshman` `sophomore` `junior` `senior` `master` `phd` `other` |
| task_intent | 允许执行：`thesis_topic` `resume_rewrite` `resume_jd_align` `report_structure` `mock_interview` `interview_review` `project_convergence` `literature_review` `content_script`；禁止执行：`emotional_support` `life_decision` `zero_sum_competition` `realtime_fact` `resource_dependent` |
| skill_status | `draft` `insight_only` `gated` `published` `needs_review` `deprecated` `archived` |
| execution_status | `running` `completed` `abandoned` `failed` `handed_off` |
| proof_type | `platform_trace` `artifact_upload` `self_report` |
| decision_slot | `when_to_check` `when_to_probe` `when_to_use_tool` `when_to_switch` |
| skill_file_slot | `skill_md` `references` `scripts` `assets` `gotchas` `evals` |
| eval_type | `discoverability` `completion` `stability` `boundary_stop` |
| growth_state | `learned` `did` `succeeded` `taught`（对应学过/做过/做成过/教会过） |


# 5. 状态机

## 5.1 Skill 生命周期

| 当前状态 | 事件 | 目标状态 | 守卫条件 |
|---|---|---|---|
| — | 从执行发起固化 | `draft` | execution.status = completed |
| `draft` | 蒸馏度评估 | `insight_only` | distillation_score < 0.75 或 boundary 未填 |
| `draft` | 蒸馏度评估 | `gated` | distillation_score ≥ 0.75 且 boundary 完整 |
| `insight_only` | 补充材料后重评 | `gated` | 同上 |
| `gated` | 发布前四问全部通过 | `published` | 四类 eval 均达阈值（见 F6.2）且准入检查通过 |
| `gated` | 任一项未通过 | `gated` | 停留，返回未通过项 |
| `published` | 版本候选被接受 | `published` | 生成新 skill_version，current_version_id 前移 |
| `published` | when_to_switch 判断全部失效 或 依赖健康度转红 | `needs_review` | 自动触发 |
| `needs_review` | 维护者修复并重跑 eval | `published` | 同发布条件 |
| `needs_review` | 30 天无响应 | `deprecated` | 定时任务 |
| `published` / `needs_review` | 移交无人接受 | `archived` | 见 5.4 |
| `deprecated` / `archived` | 管理员恢复 | `needs_review` | 仅管理员 |

**约束：**只有 `published` 状态进入排序候选集。`deprecated` 与 `archived` 不可被调用，但溯源页面仍可访问（证据不删除）。

## 5.2 Execution 生命周期

| 当前状态 | 事件 | 目标状态 | 副作用 |
|---|---|---|---|
| — | 用户开始任务 | `running` | 创建 execution + step 0 |
| `running` | 完成标准满足或用户点完成 | `completed` | 计算 completion_signal；触发固化邀请（若为本人首次做成） |
| `running` | 触碰边界条件 | `handed_off` | 写 human_handoff step，提示人工接管，不计为失败 |
| `running` | 用户离开超 30 分钟无操作 | `abandoned` | 记录 abandoned_at_step；计入 abandon_rate |
| `running` | 系统错误 | `failed` | 记录错误；不计入 abandon_rate |
| `completed` | 7 天后定时任务 | `completed` | 回填 reused_within_7d |

**重要区分：**`abandoned` 是产品信号（用户主动放弃，说明 Skill 不好或不匹配），`failed` 是工程信号（系统问题）。两者不可混算，否则质量分会被系统故障污染。

## 5.3 版本候选流程

| 状态 | 事件 | 目标 | 说明 |
|---|---|---|---|
| — | 触发规则命中 | `open` | 系统自动创建，通知维护者 |
| `open` | 维护者接受 | `accepted` | 打开草稿编辑，生成新版本 |
| `open` | 维护者驳回 | `dismissed` | 需填原因，写回 evidence |
| `open` | 14 天无处理 | `expired` | 计入 maintenance_score 下降 |

## 5.4 移交流程

| 状态 | 事件 | 目标 | 说明 |
|---|---|---|---|
| — | 创作者标记毕业 / 连续 30 天未响应问题 | `nominated` | 系统按 F14 规则提名后继者 |
| `nominated` | 后继者接受 | `accepted` | maintainer_user_id 变更，creator 保留署名 |
| `nominated` | 后继者拒绝 | `nominated` | 顺延到下一候选，最多 3 轮 |
| `nominated` | 3 轮无人接受或 14 天超时 | `archived` | Skill 转 `archived`，从候选集移除，溯源仍可查 |

# 6. 功能模块

每个模块给出：目标、用户故事、流程、规则、边界与异常、验收标准、埋点。

## F1 目标输入与任务识别（P0）

**目标：**用户说一句人话，系统判断这是什么任务、能不能做、下一步是什么。

**用户故事：**作为一个大三学生，我输入"我的选题被导师退了两次，说范围太大"，我希望立刻知道我现在的问题是什么、下一步做什么，而不是看到一堆 Skill 分类。

**流程：**

1. 用户在首页输入自然语言（10–200 字），可附材料。
2. 调用 P1 Prompt，返回 `task_intent`、`四筛判定`、`current_position`、`gap`、`next_step`、`confidence`。
3. 若 `task_intent` 属禁止执行五类 → 走 1.5 拒绝策略，流程终止。
4. 若四筛未全过 → 展示"这类问题我们不用 Skill 解决"的说明 + 可做的替代动作。
5. 若通过 → 生成任务卡，提供两个入口：立即开始（进 F4 工作台）、先看别人怎么走（进 F2）。

**规则：**

| 规则 | 值 |
|---|---|
| 输入长度 | 10–500 字，少于 10 字提示补充 |
| 置信度阈值 | `confidence < 0.6` 时不直接进任务，先反问一个澄清问题（最多一轮） |
| 澄清轮数上限 | 1 轮。第二轮仍不明确则默认按最相近 intent 走，并允许用户手动改 |
| 材料上传 | 支持 pdf / docx / md / txt / 图片，单文件 ≤ 20MB |

**边界与异常：**

- 识别失败（模型异常）→ **兜底**：降级为任务选择列表，让用户从九类允许 intent 中手选。
- 输入含明显情绪表达但同时含具体任务（如"烦死了，选题还没定"）→ 优先按任务处理，回应中不忽略情绪但不展开。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 用户已登录 | 输入"选题被退了两次，说范围太大" | 返回 `task_intent=thesis_topic`，四筛全过，给出 next_step，且 2 秒内首屏可见 |
| 2 | 用户已登录 | 输入"我最近很焦虑，不知道该干什么" | 不创建任务、不创建 experience，返回支持性回应与资源入口 |
| 3 | 用户已登录 | 输入"我该不该考研" | 返回多条真实路径分支与代价说明，不出现任何建议性措辞 |
| 4 | 模型服务不可用 | 输入任意目标 | 展示任务手选列表，不报错页 |

**埋点：**`goal_input_submitted`（含字数、是否带附件）、`intent_classified`（intent、confidence）、`intent_rejected`（拒绝类型）、`task_card_created`、`clarify_asked`。

## F2 Growth Graph 与跟走（P1）

**目标：**让用户看到到达同一目标的多条真实路线，并把其中一段变成自己的。

**流程：**

1. 用户进入成长地图，系统按 `goal_label` 拉取聚合 path，展示节点、分支、每条边的 `walked_count`。
2. 用户点开节点 → 展示所需能力、代表 Skill、真实案例数、已走过的人数、正在走的人数。
3. 点击"跟走这段路线" → 调用路径裁剪：按 `user_profile_signals` 与 `grade_stage` 移除已完成节点、替换不适配 Skill、重排顺序。
4. 生成 `path_follows` 记录，并把首个节点的 next_step 推到首页。

**规则：**

- 分支不得少于 2 条。若某目标只有 1 条真实路线，标注"目前只有一条记录在案的路线，样本不足"，不伪造分支。
- 裁剪结果必须显式告知用户："这是根据你现在的情况裁剪的第一版，随着你在这里做事会越来越准。"（对应画像冷启动的诚实性）
- `walked_count < 3` 的边标注为"少数样本"。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 存在 3 条到达 `产研实习` 的路径 | 打开成长地图 | 显示 3 条分支及各自 walked_count，样本不足的边有标注 |
| 2 | 用户为大二 | 跟走一条大四学生的路线 | 裁剪后节点数减少，且出现"第一版会比较粗"的说明 |
| 3 | 某目标仅 1 条路径 | 打开地图 | 显示样本不足提示，不出现编造的第二条分支 |

**埋点：**`graph_viewed`、`node_expanded`、`follow_started`、`follow_tailored`（裁剪前后节点数）。

## F3 同行组（P1）

**目标：**补齐"伙伴"，用同行降低放弃率。

**规则：**见 4.2 `companion_groups` 成组规则。展示文案形如"本院系有 9 人正在走这一段，其中 5 人和你同一周开题"。

**约束：**

- 只展示聚合人数与进度分布，不展示个人进度明细，不展示谁放弃了。
- 用户可关闭同行展示（隐私开关），关闭后不再参与成组统计。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 同节点同窗口同院系有 9 人 | 打开节点 | 显示 9 人及同周人数 |
| 2 | 同条件仅 2 人 | 打开节点 | 放宽到同校统计，并标注统计范围 |
| 3 | 用户关闭同行展示 | 打开节点 | 不显示同行信息，且该用户不出现在他人统计中 |

## F4 任务工作台（P0，最高优先级）

**目标：**让"做事"发生在平台内。这是全系统可信度、证据、行为信号的唯一来源。

**用户故事：**作为一个要改选题的学生，我希望有个地方能把材料放进来、和 AI 一起一步步把选题收窄，并且在关键的地方它会停下来问我，而不是直接甩给我一段结果。

**布局：**左侧任务上下文与材料；中间协作执行区；右侧实时执行轨迹；底部完成后的固化入口。

**流程：**

1. 创建 execution（status=running），写 step 0（含 user_context 画像快照）。
2. 若有 skill_version_id，按其 `workflow` 逐步执行；否则由 P1 生成临时步骤序列。
3. 每一步：
   - `ai_action`：AI 产出内容，用户可接受/编辑/拒绝，编辑量记入 `correction_ratio`。
   - `tool_call`：执行确定性操作（检索、查重、格式校验），必须展示调用了什么、返回了什么。
   - `user_decision`：**关键判断停顿点。**系统显式停下，展示当前信号与两到三个可选做法，记录 `decision_slot` 与 `user_choice`。
   - `human_handoff`：触碰边界，提示需要人来判断，给出建议的下一步。
4. 完成标准满足 → 提示完成，计算 `completion_signal`。
5. 若该用户此任务为首次做成 → 底部出现"把这次的方法固化下来"，进入 F5。

**规则：**

| 规则 | 值 |
|---|---|
| 关键判断停顿点数量 | 每次执行至少 1 个、最多 5 个。0 个则该次执行不可用于固化（没有判断就没有可蒸馏内容） |
| 自动保存 | 每步结束即持久化，断线可恢复 |
| 修正量计算 | 用户编辑后文本与 AI 输出的编辑距离 / AI 输出长度 |
| 放弃判定 | 30 分钟无操作，或用户显式退出并选择"先不做了" |
| 单步时延预算 | p95 ≤ 8s 首字节；超时展示进度而非空白 |

**边界与异常：**

- 工具调用失败 → 该步降级为 `ai_action` 并明确标注"未能验证，以下为模型判断"，不静默跳过。
- 模型输出不符合 schema → 重试 1 次，仍失败则 **兜底**：展示上一步结果与手动继续入口。
- 用户上传材料解析失败 → 允许粘贴纯文本继续。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 已创建选题任务 | 走完全流程 | executions.status=completed，execution_steps ≥ 5 条，其中 `user_decision` ≥ 1 |
| 2 | 执行中 | 触发查重工具失败 | 该步标注"未能验证"，流程继续，step 记录 tool 失败 |
| 3 | 执行中 | 关闭页面 10 分钟后返回 | 从上一步恢复，无数据丢失 |
| 4 | 执行中 | 静置 31 分钟 | status=abandoned，记录 abandoned_at_step |
| 5 | 完成执行 | 查看轨迹面板 | 每一步的输入、输出、工具、耗时可见 |
| 6 | 一次执行 0 个判断停顿 | 尝试固化 | 固化入口不可用，提示原因 |

**埋点：**`execution_started`、`step_completed`（step_type、latency）、`decision_pause_shown`、`decision_made`（slot、choice）、`tool_called`（name、success）、`handoff_triggered`、`execution_completed`（completion_signal）、`execution_abandoned`（step）。

## F5 Skill Creator（P0）

**目标：**把一次执行变成一个结构化、可安装、有证据的 Skill 草稿，用户只做确认。

### F5.1 轨迹抽取

输入 `execution_id`，调用 P2 Prompt，从 `execution_steps` 中抽取：候选判断（按四槽分类）、流程步骤、遇到的错误、触碰的边界、用到的工具与模板。

**规则：**抽取结果必须逐条携带 `source_step_index`，前端在每条候选旁展示"来自第 N 步"，用户可点击回看原始上下文。**没有来源的候选判断一律丢弃，不允许模型凭空补充。**

### F5.2 四槽确认界面

四个槽位分区展示，每槽内是候选判断卡片，用户可采纳、编辑、删除、补充。

| 槽位 | 界面提示语 |
|---|---|
| when_to_check | 在哪一步你会停下来回头验证？ |
| when_to_probe | 什么情况下你会要求补充信息而不是直接动手？ |
| when_to_use_tool | 哪一步必须查、必须跑，不能靠判断？ |
| when_to_switch | 什么现象一出现，你就知道当前这条路走不通？ |

每条判断必须填 `trigger_signal`、`judgment`、`scope` 三项才可采纳；`counter_example` 选填。

### F5.3 蒸馏度算法

六个维度，每个维度取值 0 / 0.5 / 1，加权求和。

| 维度 | 权重 | 计分规则 |
|---|---|---|
| 真实任务 real_task | 0.15 | 1：来源 execution 存在且 steps ≥ 5；0.5：有产物上传但无平台轨迹；0：仅自述 |
| 明确结果 outcome | 0.15 | 1：`done_criteria_met=true` 且 artifact_delta > 0；0.5：有产物但未达完成标准；0：无 |
| 核心流程 workflow | 0.15 | 1：有序步骤 ≥ 3 且每步有输入输出；0.5：步骤 2 条；0：≤1 |
| 关键判断 decisions | 0.25 | 已填槽位数 / 4（每槽至少 1 条完整判断） |
| 失败案例 failures | 0.15 | 1：gotchas ≥ 1 条且含触发条件与后果；0.5：有错误描述但无触发条件；0：无 |
| 适用边界 boundary | 0.15 | 1：`not_applicable` ≥ 1 且 `handoff_trigger` 已定义；否则 0 |

**发布门槛（三条同时满足）：**

1. `distillation_score ≥ 0.75`
2. `decisions ≥ 0.5`（至少填满两个槽位）
3. `boundary = 1`（硬性，安全项不允许折中）

不满足 → 状态转 `insight_only`，把缺口写进 `insights.missing_for_skill`，界面话术为"这次先存成经验笔记，还缺 X 和 Y"，**不得使用"失败""不合格"字样**。

界面实时显示六项状态（✓ / △ / ✕）与当前总分，AI 针对最低项继续追问。

### F5.4 文件夹生成

调用 P4 Prompt，按 4.2 `skill_files` 的六个 slot 生成文件，产出可下载的 zip，同时入库。

| slot | 生成规则 |
|---|---|
| skill_md | 目标、完成标准、有序流程（每步标注对应 decision）、边界与人工接管、契约摘要 |
| references | 来源材料摘要、评判标准细则（不含原始隐私材料） |
| scripts | 从 `tool_call` step 抽出的确定性操作，生成可执行脚本或调用声明 |
| assets | 执行中用到或产出的模板 |
| gotchas | 每条一个文件：触发条件、错误表现、后果、如何避免 |
| evals | 四类测试用例，可发现性用例取自 description_corpus |

**约束：**`skill_md` 必须且仅一个；`evals` 至少一个；生成后必须能通过一次"自安装自调用"校验（加载文件夹并执行一条 eval），否则不允许提交发布。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 一次含 3 个判断的完成执行 | 发起固化 | 四槽预填 3 条候选，每条显示来源步号 |
| 2 | 用户删除全部 failures | 查看蒸馏度 | failures 项显示 ✕，总分下降 0.15，若低于 0.75 则发布按钮禁用 |
| 3 | boundary 未填 | 蒸馏度为 0.8 | 仍不允许发布，提示边界为硬性要求 |
| 4 | 蒸馏度 0.6 | 提交 | 生成 insight，界面无"失败/不合格"字样，列出具体缺口 |
| 5 | 通过门槛 | 生成文件夹 | 产出 zip，含 skill_md 与至少一个 evals 文件，自安装校验通过 |
| 6 | 模型生成的判断无来源步号 | 抽取完成 | 该条不出现在候选中 |

**埋点：**`creator_opened`、`decision_slot_filled`（slot）、`distillation_score_changed`、`downgraded_to_insight`（缺口项）、`skill_folder_generated`、`self_install_check`（结果）。

## F6 发布前四问 / Eval 引擎（P0）

**目标：**发布是门禁，不是按钮。

### F6.1 四类测试

| eval_type | 测什么 | 用例来源 | 判定 |
|---|---|---|---|
| discoverability | description 写得对不对（**V0.1 遗漏项**） | description_corpus 中同 intent 的真实用户原话 ≥ 10 条 | 对每条原话跑召回，目标 Skill 出现在前 5 名的比例 |
| completion | 拿到任务能否做完 | 旧问题回放：来源 experience 的原始输入 | 完成标准满足比例 |
| stability | 换输入是否稳定 | 换学科、换材料质量、换年级各 ≥ 2 例 | 完成标准满足比例 |
| boundary_stop | 遇到边界是否知道停 | 故意超出适用范围的输入 ≥ 3 例 | 正确触发 handoff 的比例 |

### F6.2 阈值

| eval_type | 通过阈值 | 说明 |
|---|---|---|
| discoverability | recall@5 ≥ 0.80 | 未过则返回未召回的原话，引导改 description |
| completion | pass_rate ≥ 0.80 | |
| stability | pass_rate ≥ 0.70 | 允许比 completion 低，但不得低于 0.70 |
| boundary_stop | pass_rate = 1.00 | **硬性 100%。**该停不停是安全问题，不接受任何折中 |

四项全过 + 准入检查通过（F7 第一层）→ 允许发布。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | gated 状态 Skill，description 写得像官话 | 跑四问 | discoverability 未过，返回具体未召回原话列表 |
| 2 | 同上，改 description 后重跑 | recall@5 达 0.85 | 该项通过，其余项状态保留不需重跑 |
| 3 | 边界测试 3 例中 1 例未触发 handoff | 尝试发布 | 拒绝发布，明确提示"边界停机必须 100%" |
| 4 | 四项全过但缺 evals 文件 | 尝试发布 | 拒绝，提示 evals 必须入库 |

**埋点：**`eval_run_started`、`eval_run_finished`（type、pass_rate）、`publish_blocked`（原因）、`skill_published`。

## F7 准入四层与质量分（P0）

**目标：**目录会变成噪声，所以先决定谁有资格进候选集。

| 层 | 检查项 | 降权/过滤信号 | 执行时机 |
|---|---|---|---|
| 准入检查 | 结构完整性、依赖可解析、权限声明、数据边界、适用范围 | 缺少必要文件、依赖失效、权限过大、边界模糊 | 发布前 + 每日巡检 |
| 离线评测 | 四类 eval | 误触发、结果漂移、只对单一样例有效、成本失控 | 发布前 + 版本变更时 |
| 在线证据 | 采用、人工修正、失败、复用、留存 | 大量试用后放弃、频繁人工重做、失败率持续升高 | 每小时聚合 |
| 维护状态 | 版本活跃度、依赖健康、问题响应、回滚记录 | 长期未维护、旧版本失效、问题无人处理 | 每日 |

**权限过大的判定：**声明的 permission 中存在执行中从未实际使用的项，或包含 `write_external` / `send_message` 类不可逆操作但 `handoff_trigger` 未覆盖。

**质量分公式：**

```
quality_score = 0.40 * offline_score
              + 0.35 * online_score
              + 0.25 * maintenance_score

offline_score = 0.30*discoverability + 0.30*completion
              + 0.25*stability + 0.15*boundary_stop

online_score  = 0.35*adoption_rate
              + 0.25*(1 - correction_rate)
              + 0.25*(1 - abandon_rate)
              + 0.15*reuse_rate_7d

maintenance_score = 0.40*version_activity      # 90 天内是否有版本或候选处理
                  + 0.35*dependency_health     # 依赖可用比例
                  + 0.25*issue_response        # 版本候选未过期比例

is_candidate_eligible = admission_passed
                        AND status = 'published'
                        AND quality_score >= 0.50
```

**冷启动规则：**在线证据不足（call_count < 10）时，`online_score` 取 `offline_score * 0.8` 作为先验，并在 Trust Card 标注"线上证据样本不足"。不允许因为没数据就给高分。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | Skill 依赖的检索工具下线 | 每日巡检 | admission_passed=false，从候选集移除，通知维护者 |
| 2 | Skill 声明了发送邮件权限但从未使用 | 巡检 | 标记权限过大，quality_score 降权并提示收敛权限 |
| 3 | Skill 调用 5 次 | 计算 online_score | 使用先验值，Trust Card 出现样本不足标注 |
| 4 | 90 天无版本活动且有 2 个过期候选 | 计算 | maintenance_score 显著下降，排序位次后移 |

**埋点：**`admission_check_run`（passed、failures）、`score_recomputed`（各层分值）、`skill_delisted`（原因）。

## F8 Router 与排序（P0）

**目标：**用户描述目标，系统给出能力，并说明为什么是它。

**两段式：**先用 F7 的 `is_candidate_eligible` 做硬过滤，再对候选集排序。

**排序公式：**

```
rank_score = 0.30 * quality_score
           + 0.35 * task_fit
           + 0.25 * user_fit
           - 0.10 * risk_penalty

task_fit  = 0.6 * description_similarity   # 第一跳只看 name + description
          + 0.4 * intent_exact_match

user_fit  = 0.4 * stage_match              # grade_stage 与 stage_fit 交集
          + 0.3 * constraint_match         # 时间/资源约束是否满足
          + 0.3 * prerequisite_satisfied   # 前置 Skill 是否已具备

risk_penalty = 0.5 * permission_breadth    # 权限范围归一化
             + 0.3 * irreversible_ops
             + 0.2 * cost_over_budget
```

**排序解释（强制）：**每个结果必须附一句解释，模板由 P6 生成，内容取 `rank_score` 中贡献最大的两项与被降权的一项。同时对**未被推荐的高热度项**给出说明，这是平台辨别力的可见形式。

**规则：**

- 返回上限 5 个，超过则折叠。
- 任何界面不得展示 `rank_score` 数值本身，只展示解释。
- 若候选集为空 → 展示"这个任务目前还没有可信的能力单元"，并提供裸任务执行入口（进 F4，无 Skill 执行）。这条路径同时是新需求发现来源，写入 `description_corpus`。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 存在高热度无证据的 A 与低热度有证据的 B | 搜索"把科研经历改成产研简历" | B 排在 A 之前，且显示为什么选 B、为什么没选 A |
| 2 | 用户为大一 | 请求简历任务 | 优先返回 stage_fit 含 freshman 的 Skill |
| 3 | 某 Skill 需要发送邮件权限 | 排序 | risk_penalty 生效，位次后移，解释中提及权限 |
| 4 | 候选集为空 | 提交目标 | 不返回勉强匹配项，提供裸任务入口，原话入语料库 |
| 5 | 任意排序结果 | 查看界面 | 不出现任何数字评分 |

**埋点：**`route_requested`、`candidates_filtered`（过滤前后数量）、`ranking_returned`（top5 的 skill_id 与解释）、`explanation_expanded`、`empty_candidate_set`。


## F9 Skill 组合（P1）

**目标：**复杂目标由能力组合完成，而不是让用户自己挑齐。

**流程：**目标 → 拆解为有序子任务 → 每个子任务走 F8 排序取第一名 → 校验 `prerequisite` 关系与上下文传递 → 生成组合计划供用户确认。

**规则：**

| 规则 | 值 |
|---|---|
| 组合长度上限 | 6 个 Skill。超过说明目标应拆成路径节点而非一次组合 |
| 上下文传递 | 前一个 Skill 的 output 必须能满足后一个的 input 契约，否则插入一步用户确认 |
| 前置校验 | 存在未满足的 `prerequisite` 时，自动前插该 Skill 或提示跳过风险 |
| 失败处理 | 任一环失败则暂停整条链，保留已完成产物，不静默继续 |
| 成本预算 | 组合总 `cost_budget_ms` 超过 120 秒时提示分次执行 |

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 目标"拿到产研实习" | 请求组合 | 返回 ≤6 步有序计划，每步含所选 Skill 与理由 |
| 2 | 第 2 步的 input 契约不满足第 1 步 output | 生成计划 | 自动插入用户确认步骤 |
| 3 | 第 3 步执行失败 | 继续 | 链条暂停，前两步产物保留可导出 |

## F10 Trust Card 与判断级溯源（P0）

**目标：**用户托付的是一项工作，所以要看的是证据，不是分数。

**分区与字段：**

| 分区 | 内容 | 数据来源 |
|---|---|---|
| 它做什么 | 一句话任务、完成标准、为什么推荐给你 | skill_versions + F8 解释 |
| 流程 | 有序步骤，每个岔路口可点开 | workflow + skill_version_decisions |
| 证据 | 来源 experience 摘要、四类 eval 通过率、过程性完成信号 | eval_runs + pricing_signals |
| 边界 | 不适用条件、人工接管点、降级路径、gotchas 摘要 | boundary + gotchas 文件 |
| 授权与安全 | 读什么数据、做什么操作、最小权限声明、敏感操作确认说明 | contract.permissions |
| 运行 | 成本与时延、稳定性、失败类型分布、人工修正率 | executions 聚合 |
| 维护 | 当前维护者、是否已移交、最近更新、依赖健康、版本变化摘要 | skills + skill_scores |

**判断级溯源交互：**点击流程中任一岔路口 → 浮层展示该 `decision` 的 `trigger_signal` / `judgment` / `scope` / `counter_example` / `verified_by_count` / 来源 experience 的**脱敏摘要**（不展示原始材料）。

**硬性约束：**

- 全页不得出现综合评分、星级、排行位次。
- 每个数字必须可点击到来源（如"修正率 18%"→ 该指标的统计口径与样本量）。
- 样本量 < 10 时，所有比率标注"样本不足"，不显示两位小数。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 任意已发布 Skill | 打开 Trust Card | 页面无任何综合评分或星级 |
| 2 | 流程含 4 个判断 | 点击第 3 个岔路口 | 浮层显示触发信号、判断、适用场景、来源摘要 |
| 3 | 来源 experience 为私有 | 查看溯源 | 只见脱敏摘要，不见原始上传材料 |
| 4 | 调用次数为 6 | 查看修正率 | 显示"样本不足"，不显示精确百分比 |
| 5 | Skill 已移交 | 查看维护分区 | 显示当前维护者与原创作者署名 |

**埋点：**`trust_card_viewed`、`decision_trace_opened`（decision_id）、`metric_source_opened`、`boundary_section_read`。

## F11 授权、最小权限与审计（P0）

**目标：**满足委托五要件：身份可识别、授权可确认、行为可审计、结果可追溯、责任边界清楚。

| 要件 | 实现 |
|---|---|
| 身份可识别 | 每个 Skill 必须有非匿名 creator 与 maintainer，界面可见 |
| 授权可确认 | 执行前展示权限清单与将执行的操作；敏感操作逐次二次确认 |
| 行为可审计 | 完整 `execution_steps` 日志，用户可回看每步输入输出与工具调用 |
| 结果可追溯 | 判断级溯源 + 版本记录 |
| 责任边界清楚 | 不适用条件、handoff 触发点、降级路径必须非空 |

**敏感操作定义（需逐次确认）：**对外发送（邮件、消息）、写入用户平台外账号、删除或覆盖用户原始材料、产生费用的调用、任何不可逆操作。

**最小权限执行：**运行时只授予 `contract.permissions` 声明的权限；执行结束统计实际使用集，未使用项进入 F7 的"权限过大"判定。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | Skill 声明读取上传文件 | 开始执行 | 执行前展示权限清单，用户确认后才开始 |
| 2 | Skill 尝试发送邮件 | 执行到该步 | 弹出二次确认，拒绝则走降级路径而非失败 |
| 3 | 执行完成 | 打开审计日志 | 每步的工具、输入、输出、耗时可见 |
| 4 | Skill 未声明的权限 | 运行时尝试使用 | 被拒绝并记录异常，执行转 handed_off |
| 5 | boundary 为空 | 尝试发布 | 被拒绝（与 F5.3 门槛一致） |

## F12 执行反馈与版本升级（P0）

**目标：**闭环由行为信号自动驱动，不依赖用户回来填表。

**信号采集（全自动）：**

| 信号 | 计算 | 频率 |
|---|---|---|
| adoption_rate | completed 且 exported / 总调用 | 每小时 |
| correction_rate | 平均 correction_ratio | 每小时 |
| abandon_rate | abandoned / 总调用（不含 failed） | 每小时 |
| reuse_rate_7d | 7 日内二次调用用户占比 | 每日 |
| failure_type_dist | feedbacks 按 issue_type 分布 | 每小时 |

**版本候选触发规则：**

| trigger_rule | 条件 |
|---|---|
| repeated_failure | 同一 `issue_type` 在 14 天内累积 ≥ 3 次，且来自 ≥ 3 个不同用户 |
| correction_rate_high | 连续 14 天 correction_rate > 0.40 |
| dependency_unhealthy | dependency_health < 0.8 |
| new_boundary_verified | 同一新边界被 ≥ 2 次独立执行验证 |
| manual | 维护者主动发起 |

**关键规则：单条负反馈不触发版本变更**，只进观察池。触发必须是重复性或被独立验证，避免版本被个例带偏。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 1 位用户提 1 条反馈 | 等待 | 不创建版本候选，进观察池 |
| 2 | 3 位用户 14 天内提同类反馈 3 条 | 聚合任务运行 | 创建 open 候选，通知维护者，evidence 含 3 个 feedback_id |
| 3 | 维护者接受候选 | 编辑并发布 | 生成新版本，changelog 非空，原候选转 accepted，关联 feedbacks.adopted=true |
| 4 | 候选 14 天无处理 | 定时任务 | 转 expired，maintenance_score 下降 |
| 5 | 依赖健康度 0.7 | 巡检 | 创建 dependency_unhealthy 候选且 Skill 转 needs_review |

**埋点：**`signal_aggregated`、`version_candidate_created`（rule）、`version_candidate_resolved`（结果）、`version_published`（from→to）。

## F13 个人成长主页（P1）

**模块：**当前位置、成长路线、代表性 Skill、成长状态、影响力、失败与复盘。

**成长状态判定（`growth_state`）：**

| 状态 | 判定条件 |
|---|---|
| learned | 调用过相关 Skill |
| did | 有 completed execution |
| succeeded | completed 且 `completion_signal.exported=true` |
| taught | 是某 published Skill 的 creator 或 maintainer，且该 Skill 被他人成功调用 ≥ 1 次 |

**影响力指标：**帮助人数（不同调用者数）、有效完成数、被组合次数、有效贡献数（被采纳的判断/反馈数）、后继者数（`path_follows` 中 source_owner 为本人的活跃数）、已接手维护数。

**硬性约束：**不展示粉丝数、不展示成长分数、不展示与他人的名次比较。默认全部不公开，逐项开启。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 新用户 | 打开自己主页 | 全部模块默认不公开，有逐项开启开关 |
| 2 | 用户有 1 个被他人成功调用的 Skill | 查看成长状态 | 该领域显示 `taught` |
| 3 | 任意主页 | 查看 | 无粉丝数、无分数、无排名 |

## F14 毕业移交（P2）

**后继者提名规则（按顺序打分取前 3）：**

```
successor_score = 0.40 * (该 Skill 的成功调用次数)
                + 0.30 * (被采纳的反馈或判断数)
                + 0.20 * (自身 growth_state 达 succeeded 或 taught)
                + 0.10 * (同院系同方向)
```

**流程见 5.4。**规则：每轮候选有 7 天响应期，最多 3 轮；无人接受则转 `archived`，Skill 从候选集移除但溯源页保留。原创作者的 `creator_user_id` 与 attributions 中的 `origin` 归属永不变更。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 创作者标记毕业 | 系统运行 | 生成 nominated 记录，含前 3 候选与打分依据 |
| 2 | 第一候选拒绝 | 7 天内 | 顺延第二候选 |
| 3 | 3 轮无人接受 | 超时 | Skill 转 archived，不可调用，溯源可访问 |
| 4 | 移交完成 | 查看 Trust Card | 显示新维护者 + 原创作者署名 |

## F15 定价信号与贡献归属（P2，数据结构 P1）

**定价指数（仅展示，不结算）：**

```
suggested_price_index =
    base
  * (1 + 0.5*normalize(call_count) + 0.3*normalize(composed_count))
  * effective_rate                    // = effective_count / max(call_count, 1)
  * (1 - 0.4*correction_rate)
  * (1 - 0.3*abandon_rate)
  * maintenance_decay
  * (1 + 0.2*(1 - substitutability))

maintenance_decay = max(0.5, 1 - 0.1 * floor(days_since_update / 90))
```

**归属权重分配：**

| kind | 权重规则 |
|---|---|
| origin | 固定 0.50，永久归创作者 |
| decision_contribution | 共享 0.20，按各人被采纳判断数占比分配 |
| composition | 共享 0.15，被组合方按被调用次数占比分配 |
| maintenance | 共享 0.15，按维护期内推动的版本数占比分配 |

**约束：**48 小时与冷启动期只记录与展示，不做任何形式的结算或提现。界面文案必须写明"当前不涉及金钱结算"。

## F16 运营后台（P1）

**目标：**支撑 0→1 阶段的运营重模式。

| 功能 | 说明 | 优先级 |
|---|---|---|
| 种子蒸馏工作台 | 运营代替用户走 F4+F5，产出 `origin=ops_seed` 的 Skill；必须补一次真实执行才能发布 | P1 |
| description 语料管理 | 导入、标注 intent、指派到 Skill、标记为 eval 用例 | P1 |
| 准入复核队列 | 查看 admission 未通过项，人工判定或退回维护者 | P1 |
| 时间窗口配置 | 维护 `time_windows`，按院系配置推送 | P1 |
| 需求缺口看板 | 展示 `empty_candidate_set` 事件聚合的高频未满足原话，直接指向下一批要做的 Skill | P1 |
| 强制下架 | 管理员操作，需填原因，记录审计 | P2 |

**约束：**运营创建的 Skill 与用户创建的走**完全相同**的发布门禁，不设特权通道。这是平台可信度的底线。

# 7. AI 能力规格

所有 Prompt 遵循同一契约：输入结构化、输出严格 JSON、schema 校验失败重试 1 次、二次失败走兜底。所有 Prompt 禁止编造来源。

## P1 目标识别与四筛判定

**输入：**`utterance`、`attachments_summary[]`、`user_profile_snapshot`。

**输出 schema：**

```json
{
  "task_intent": "thesis_topic",
  "confidence": 0.86,
  "sieve": {
    "amortizable": true, "testable": true,
    "transferable": true, "short_loop": true,
    "reason_if_false": null
  },
  "current_position": "选题已有初稿但范围过大",
  "gap": ["缺少可支撑性判断", "缺少收窄方法"],
  "next_step": "先盘清手上能拿到的材料，再决定问题写多大",
  "clarify_question": null
}
```

**约束：**`task_intent` 只能取 4.3 枚举值；命中禁止五类时其余字段一律置空；`confidence < 0.6` 必须给出 `clarify_question`。

**兜底：**返回 `task_intent=null`，前端展示手选列表。

## P2 轨迹 → 四类判断抽取

**输入：**`execution_steps[]`（含 step_index、type、input、output、user_choice）。

**输出 schema：**

```json
{
  "decisions": [{
    "slot": "when_to_switch",
    "trigger_signal": "一句话说不清研究问题",
    "judgment": "立刻收窄到单一现象",
    "scope": "文献综述阶段之前",
    "source_step_index": 4,
    "confidence": 0.8
  }],
  "workflow": [{"step_index":1,"title":"...","io":"..."}],
  "gotchas": [{"trigger":"...","symptom":"...","consequence":"..."}],
  "boundary_hints": ["..."],
  "tools_used": ["plagiarism_check"]
}
```

**硬性约束：**每条 decision 必须有 `source_step_index` 且该 step 真实存在；缺失或越界的条目由后端直接丢弃，不进入界面。**禁止模型补充轨迹中不存在的判断。**

## P3 蒸馏度评估

**输入：**当前草稿六维度的原始材料。**输出：**六项分值 + 最低项 + 针对最低项的一个追问。

**约束：**分值必须可由 F5.3 规则复算（模型只做归类判断，不做加权计算，加权在后端做）。这样保证分数可解释、可测试。

## P4 Skill 文件夹生成

**输入：**确认后的 decisions、workflow、gotchas、boundary、contract、来源摘要。

**输出：**每个 slot 的文件内容数组。

**约束：**`description` 必须从 `description_corpus` 提供的候选原话中改写，禁止使用书面官话；`references` 不得包含原始隐私材料；`scripts` 只允许生成确定性操作，涉及判断的步骤必须留在 `skill_md`。

## P5 description 生成与召回测试

**输入：**任务定义 + 同 intent 的 ≥10 条真实原话。**输出：**候选 description ×3 + 每条的预期召回覆盖。

**约束：**召回测试由检索层执行，不由模型自评。

## P6 排序解释生成

**输入：**`rank_score` 各分项、被降权项、以及被跳过的高热度候选。

**输出：**≤40 字的推荐理由 + ≤40 字的"为什么没选另一个"。

**约束：**禁止出现数字评分；必须提到具体证据类型（如"有科研转产研样例与失败案例"）。

# 8. 接口契约

统一前缀 `/api/v1`。认证用 Bearer token。错误体统一为 `{"error":{"code","message","details"}}`。

| 方法 | 路径 | 说明 | 优先级 |
|---|---|---|---|
| POST | `/goals/interpret` | F1 目标识别，返回任务卡或拒绝策略 | P0 |
| POST | `/executions` | 创建执行 | P0 |
| GET | `/executions/{id}` | 含 steps 与 trace | P0 |
| POST | `/executions/{id}/steps` | 追加一步（含 user_decision） | P0 |
| POST | `/executions/{id}/complete` | 完成并计算 completion_signal | P0 |
| POST | `/executions/{id}/abandon` | 显式放弃 | P0 |
| POST | `/executions/{id}/distill` | 发起固化，返回草稿与候选判断 | P0 |
| PATCH | `/skill-drafts/{id}` | 更新四槽与草稿字段，返回蒸馏度 | P0 |
| POST | `/skill-drafts/{id}/generate-folder` | 生成六 slot 文件与 zip | P0 |
| POST | `/skill-versions/{id}/evals/run` | 跑发布前四问 | P0 |
| POST | `/skill-versions/{id}/publish` | 门禁校验后发布 | P0 |
| POST | `/route` | 两段式路由，返回 ≤5 结果与解释 | P0 |
| GET | `/skills/{id}/trust-card` | Trust Card 全量 | P0 |
| GET | `/decisions/{id}/trace` | 判断级溯源（脱敏） | P0 |
| POST | `/executions/{id}/feedback` | 提交反馈 | P0 |
| GET | `/skills/{id}/version-candidates` | 候选列表 | P0 |
| POST | `/version-candidates/{id}/accept` | 接受候选 | P0 |
| POST | `/compositions/plan` | 组合计划 | P1 |
| GET | `/paths?goal=` | Growth Graph | P1 |
| POST | `/paths/{id}/follow` | 跟走并裁剪 | P1 |
| GET | `/path-nodes/{id}/companions` | 同行聚合 | P1 |
| GET | `/users/{id}/profile` | 成长主页（按可见性过滤） | P1 |
| POST | `/skills/{id}/handover` | 发起移交 | P2 |
| POST | `/handovers/{id}/accept` | 接受移交 | P2 |
| GET | `/ops/corpus` `/ops/gaps` `/ops/admissions` | 运营后台 | P1 |

**关键接口示例：`POST /route`**

请求：

```json
{
  "utterance": "把科研经历改成产研岗位简历",
  "task_intent": "resume_rewrite",
  "user_context": {"grade_stage":"senior","constraints":{"time_budget_h":3}}
}
```

响应：

```json
{
  "results": [{
    "skill_id": "...", "name": "科研经历转产研简历改写",
    "version": "1.2",
    "why_this": "有科研转产研样例、基线对比和失败案例",
    "why_not_alternative": "另一个热度更高，但没有任务测试和边界说明",
    "evidence": {"eval_pass": {"completion":0.86,"stability":0.74},
                 "sample_size": 42, "sample_sufficient": true},
    "risk": {"permissions":["read_upload"],"irreversible":false}
  }],
  "filtered_out_count": 7,
  "empty_reason": null
}
```

**约束：**响应中不得包含 `rank_score` 或任何综合分数字段。


# 9. 指标与埋点

## 9.1 北极星指标

**每周被固化并通过发布门禁的 Skill 数 × 该批 Skill 的有效完成率。**

选它的理由：它同时约束供给的量与质。只涨数量会被有效完成率拉下来，只保质量会被数量拉下来。单看调用量会鼓励做热点，单看 Skill 总数会鼓励灌水。

## 9.2 分层指标

| 层 | 指标 | 定义 | 目标（首个时间窗口） |
|---|---|---|---|
| 需求 | 任务识别准确率 | 人工抽检 100 条的 intent 正确率 | ≥ 0.90 |
| 需求 | 伪需求正确拦截率 | 五类禁止 intent 被正确拒绝的比例 | 1.00（硬性） |
| 执行 | 执行完成率 | completed / (completed+abandoned) | ≥ 0.55 |
| 执行 | 判断停顿触发率 | 含 ≥1 个 user_decision 的执行占比 | ≥ 0.90 |
| 供给 | 固化转化率 | 发起固化 / 首次做成的执行数 | ≥ 0.35 |
| 供给 | 门禁通过率 | published / gated | 0.50–0.80（过高说明门槛太松） |
| 供给 | 降级为 Insight 占比 | insight_only / draft | 观察项，不设目标 |
| 信任 | 溯源下钻率 | 打开 Trust Card 后点开判断的比例 | ≥ 0.25 |
| 流通 | 首位命中率 | 用户采用排序第 1 名的比例 | ≥ 0.60 |
| 流通 | 空候选集率 | empty_candidate_set / route 请求 | ≤ 0.20 且持续下降 |
| 闭环 | 版本升级周期 | 候选创建到新版本发布的中位天数 | ≤ 7 |
| 闭环 | 候选过期率 | expired / created | ≤ 0.20 |
| 增长 | 院系渗透率 | 某院系某届参与人数 / 该届人数 | 首个院系 ≥ 0.20 |
| 增长 | 窗口内任务密度 | 窗口期内人均完成任务数 | ≥ 1.5 |

## 9.3 埋点事件表

| 事件 | 关键属性 |
|---|---|
| goal_input_submitted | char_count, has_attachment |
| intent_classified | task_intent, confidence, sieve_passed |
| intent_rejected | reject_type |
| execution_started | skill_version_id, has_skill |
| step_completed | step_type, latency_ms, tool_name, success |
| decision_pause_shown / decision_made | slot, options_count, choice_index |
| handoff_triggered | boundary_rule |
| execution_completed | exported, artifact_delta, correction_ratio, duration |
| execution_abandoned | abandoned_at_step, elapsed_ms |
| creator_opened | execution_id, candidate_decision_count |
| decision_slot_filled | slot, source_step_index |
| distillation_score_changed | score, lowest_dimension |
| downgraded_to_insight | missing_dimensions |
| skill_folder_generated | slot_counts, zip_bytes |
| self_install_check | passed, error |
| eval_run_finished | eval_type, pass_rate, blocked |
| skill_published | skill_id, version, distillation_score |
| route_requested | task_intent |
| candidates_filtered | before, after, filter_reasons |
| ranking_returned | top5_skill_ids, has_explanation |
| explanation_expanded | position |
| empty_candidate_set | utterance_hash, task_intent |
| trust_card_viewed | skill_id |
| decision_trace_opened | decision_id |
| version_candidate_created | trigger_rule, evidence_count |
| version_published | from_version, to_version, trigger_rule |
| handover_nominated / handover_resolved | reason, result, round |

**约束：**任何埋点不得记录用户上传材料的原文、成绩、求职结果与情绪表达内容。`empty_candidate_set` 记录原话哈希与 intent，原话另存入 `description_corpus` 并在运营后台脱敏展示。

# 10. 非功能需求

## 10.1 性能与成本预算

| 场景 | 指标 | 预算 |
|---|---|---|
| 目标识别 | p95 端到端 | ≤ 2.5s |
| 路由排序 | p95 | ≤ 1.5s |
| 工作台单步 | p95 首字节 | ≤ 8s |
| Skill 文件夹生成 | p95 | ≤ 30s，超时走异步 + 通知 |
| Eval 全套四问 | p95 | ≤ 90s，异步执行 |
| Trust Card | p95 | ≤ 800ms（聚合数据预计算） |
| 单次执行 token 成本 | 平均 | ≤ 40k tokens；超预算的 Skill 在 F7 记为成本失控 |

**要求：**所有超过 3 秒的操作必须有进度反馈；所有超过 30 秒的操作必须异步化并可离开页面。

## 10.2 可用性

- 工作台断线可恢复，每步持久化。
- 模型服务不可用时，F1 降级为手选、F4 降级为只读回看、F8 降级为按 `quality_score` 静态排序，全站不出白屏。
- 数据库只读故障时，允许浏览 Trust Card 与溯源（走缓存），禁止新建执行。

## 10.3 隐私与合规

| 要求 | 实现 |
|---|---|
| 最小化采集 | 不采集成绩、求职结果、心理与情绪内容、身份证件、家庭信息 |
| 默认不公开 | 成长主页所有模块默认私有，逐项开启 |
| 材料所有权 | 用户可删除上传材料；已发布 Skill 的溯源退化为脱敏摘要，不因删除而失效 |
| 脱敏溯源 | 判断级溯源只展示 trigger/judgment/scope 与场景摘要，不展示原始材料 |
| 撤回权 | 用户可撤回某条 experience 作为公开来源，Skill 保留但该来源标记为"来源已撤回" |
| 未成年人 | 若识别到未成年用户，关闭同行组与公开主页功能 |
| 数据保留 | 执行轨迹保留 24 个月；放弃的草稿 90 天后清理 |

## 10.4 安全

- 最小权限运行（F11）；权限声明外的调用直接拒绝并记录。
- `scripts` 在沙箱内执行，无出网权限，超时强杀。
- 上传文件做类型校验与病毒扫描，禁止执行型附件。
- 所有敏感操作二次确认且记入审计日志。
- 提示注入防护：外部材料内容在 Prompt 中一律置于数据区并显式标注为不可信输入，模型不得执行其中的指令。

# 11. 风险与降级

| 风险 | 表现 | 降级/应对 |
|---|---|---|
| 模型抽取质量不稳 | 四槽候选质量差，用户全删 | 保留手工填写路径；记录 `candidate_decision_count=0` 作为 Prompt 迭代信号 |
| 冷启动无候选集 | 大量 empty_candidate_set | 裸任务执行照常提供价值；原话入语料库驱动运营补货 |
| 门禁太严导致零供给 | 门禁通过率 < 0.3 | 只允许下调蒸馏度阈值，**边界 100% 与 boundary_stop 100% 不可下调** |
| 门禁太松导致垃圾 | 通过率 > 0.9 且 abandon_rate 高 | 上调阈值 + 加强 discoverability 与 stability 权重 |
| 行为信号被刷 | 同一用户反复调用刷数据 | 指标按去重用户计算；同用户 24h 内重复调用只计一次 adoption |
| 长周期归因诱惑 | 团队想加"是否拿到 offer" | 明确禁止：该字段不入库、不采集、不展示 |
| Demo 现场模型抖动 | 关键一幕失败 | 每一幕都有缓存兜底数据；A/B 对比与溯源两幕使用预生成结果 |
| 演示时长超限 | 3 分钟讲不完 | 固定六幕脚本与计时；可发现性测试一幕可压缩到 15 秒 |

# 12. 48 小时 P0 交付切片

## 12.1 只做这些

| 模块 | 做到什么程度 |
|---|---|
| F1 目标识别 | 只支持 `thesis_topic` 与 `resume_rewrite` 两个 intent 走全流程；其余 intent 走拒绝或提示 |
| F4 任务工作台 | 单一任务（论文选题）走通，至少 1 个真实判断停顿点，轨迹完整落库 |
| F5 Skill Creator | 轨迹抽取 + 四槽确认 + 蒸馏度实时显示 + 文件夹生成（真 zip） |
| F6 发布前四问 | 四类都跑，可发现性用 10 条真实原话，边界停机硬性 100% |
| F7 准入四层 | 只实现第一层（结构与依赖静态检查）+ 第二层（eval 结果），三四层用预置数据 |
| F8 Router | 两段式过滤 + 排序 + 解释；A/B 对比场景可用预置候选 |
| F10 Trust Card | 全分区展示 + 判断级溯源下钻 |
| F12 闭环 | 反馈提交 + 一条 repeated_failure 规则 + 手动接受候选生成 V1.1 |

## 12.2 用预置数据但必须真实结构

| 项 | 做法 |
|---|---|
| online_score / maintenance_score | 预置，但走真实字段与公式，界面标注样本不足 |
| Growth Graph | 3 条真实分支的静态数据 |
| 同行组 | 静态人数，成组规则不实现 |
| 个人主页 | 降级为 Trust Card 内的"来源人物"浮层 |
| 定价与归属 | 只落库不展示，或展示但标注"不涉及结算" |

## 12.3 排期

| 时段 | 交付物 | 验收 |
|---|---|---|
| 0–4h | Schema 落地（users/executions/execution_steps/skills/skill_versions/decisions/skill_files/evals）+ P1/P2 Prompt 骨架 + Demo 脚本定稿 | 能手工插一条完整执行并查询出来 |
| 4–12h | F4 工作台走通选题任务，判断停顿点可交互，轨迹落库 | 验收 F4 的 1、3、5 三条 |
| 12–24h | F5 全流程：抽取→四槽→蒸馏度→zip；自安装校验 | 验收 F5 的 1、2、4、5 四条 |
| 24–36h | F6 四问 + F7 前两层 + F10 Trust Card 与溯源 | 验收 F6 的 1、3 和 F10 的 1、2 |
| 36–44h | F8 排序与 A/B 解释 + F12 反馈到 V1.1 + 视觉统一 | 验收 F8 的 1、5 和 F12 的 3 |
| 44–48h | 锁版、缓存兜底、录备份 Demo、连续演练 5 次 | 每幕都有兜底数据，计时 ≤ 3 分钟 |

## 12.4 P0 验收总清单

以下每条必须为真才算完成。

1. 一次真实选题任务能在平台内走完，`execution_steps` ≥ 5 且含 ≥ 1 个 `user_decision`。
2. 该次执行能自动生成 Skill 草稿，四槽候选均带 `source_step_index`。
3. 蒸馏度六项实时显示，删掉失败案例会看到分数下降。
4. 蒸馏度不足时降级为 Insight，界面无"失败/不合格"字样，列出具体缺口。
5. 生成的 zip 含 `SKILL.md` 与至少一个 `evals` 文件，且能通过自安装自调用校验。
6. 可发现性测试能当场失败一次，改 `description` 后重跑通过。
7. 边界停机测试未满 100% 时，发布被拒绝。
8. 路由结果把有证据的低热度 Skill 排在高热度无证据之前，并给出两句解释。
9. Trust Card 全页无任何综合评分或星级。
10. 点击流程岔路口能看到该条判断的触发信号、判断、适用场景与来源摘要。
11. 提交 3 条同类反馈后生成版本候选，接受后产生 V1.1 且 changelog 非空。
12. 输入情绪类或人生抉择类表述，系统不创建任务、不创建 experience。

# 附录 A. 验收标准索引

| 模块 | 验收条数 | 关键硬性项 |
|---|---|---|
| F1 目标识别 | 4 | 伪需求拦截率 100% |
| F2 Growth Graph | 3 | 不伪造分支 |
| F3 同行组 | 3 | 不展示个人进度 |
| F4 工作台 | 6 | 0 判断不可固化 |
| F5 Creator | 6 | 无来源的判断必须丢弃；boundary 硬性 |
| F6 四问 | 4 | boundary_stop 必须 100% |
| F7 准入 | 4 | 无数据不给高分 |
| F8 Router | 5 | 界面不出现数字评分 |
| F9 组合 | 3 | 失败不静默继续 |
| F10 Trust Card | 5 | 无综合评分；溯源脱敏 |
| F11 授权审计 | 5 | 未声明权限直接拒绝 |
| F12 闭环 | 5 | 单条反馈不触发版本 |
| F13 主页 | 3 | 默认全部不公开 |
| F14 移交 | 4 | creator 署名永不变更 |

# 附录 B. 不可妥协清单

以下十条是产品主张的载体。任何一条被砍，产品就退化成普通的 AI 工具或经验社区，评审时也会立刻失去差异化。**若工期紧张，宁可砍模块，不可砍这十条。**

1. 做事发生在平台内，执行必须落 `execution_steps`。
2. Skill 草稿由轨迹生成，用户只确认，不撰写。
3. 每条判断必须有来源步号，无来源即丢弃。
4. 适用边界是发布的硬性门槛，不接受折中。
5. 边界停机测试必须 100% 通过。
6. 可发现性测试必须存在且可失败。
7. 任何信任展示不出现综合评分或星级。
8. 判断级溯源必须可下钻，且脱敏。
9. 单条负反馈不触发版本变更。
10. 五类伪需求一律不进任务流、不落 Experience。

# 附录 C. 与产品方案 V0.2 的对应关系

| 方案章节 | 本 PRD 对应 |
|---|---|
| 第 2 章 需求判别 | 1.4 Out of Scope、1.5 拒绝策略、F1 四筛判定 |
| 第 5.3 毕业移交 | 5.4 状态机、F14 |
| 第 6.3 同行者 | F3、companion_groups |
| 第 7.2 两条生产路径 | F4 + F5，`skills.origin` 字段 |
| 第 7.3 四类关键判断 | `decision_slot` 枚举、F5.2 四槽界面、P2 Prompt |
| 第 7.4 判断级溯源 | `decisions` 表、`skill_version_decisions`、F10 |
| 第 7.5 蒸馏度 | F5.3 算法与门槛 |
| 第 7.6 发布前四问 | F6、`eval_type` 枚举 |
| 第 8.2 文件夹分工 | `skill_files.slot`、F5.4 |
| 第 8.3 渐进披露 | F8 `task_fit` 只用 name+description |
| 第 8.4 语料来源 | `description_corpus`、F16 |
| 第 9.2 五维度取数 | `completion_signal`、F12 信号采集 |
| 第 9.3 委托五要件 | F11 |
| 第 10.2 准入四层 | F7 |
| 第 10.3 行为信号替代成功率 | 10.3 禁止采集 offer 结果、F12 |
| 第 10.4 A/B 辨别力 | F8 验收标准第 1 条 |
| 第 11.2 闭环三缺口 | 5.3、F12 触发规则 |
| 第 12.3 定价信号 | F15 |
| 第 13 章 引爆与增长 | F16 运营后台、`time_windows`、9.2 增长指标 |
