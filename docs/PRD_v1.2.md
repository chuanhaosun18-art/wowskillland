---
title: "SkillX WowLand 产品需求文档（PRD）"
subtitle: "大学生成长复利系统 · 完整产品范围 · 工程可实施版"
---

**版本 PRD v1.2 · 2026-08-12 · 对应产品方案 V0.2（方案侧待回改，见附录 C1）**

# v1.2 修订说明：补七个站不住的地方
# 目录

v1.1 修订说明：从两态到三态　·　0. 文档说明　·　1. 产品定位与范围（含三态路由）　·　2. 角色与权限矩阵　·　3. 术语与核心概念　·　4. 领域模型与数据表 schema　·　5. 状态机（Skill / Execution / 版本候选 / 编排 / 移交）　·　6. 功能模块 F1–F17（F17 编排态）　·　7. AI 能力规格 P1–P7　·　8. 接口契约　·　9. 指标与埋点　·　10. 非功能需求　·　11. 风险与降级　·　12. 48 小时 P0 交付切片　·　附录 A 验收标准索引　·　附录 B 不可妥协清单（13 条）　·　附录 C 与方案 V0.2 的对应关系与回改清单


v1.1 引入编排态之后，有七处经不起追问。这一版逐条补掉，其中前三条改的是地基。

| # | 问题 | 为什么必须改 | v1.2 的处理 | 位置 |
|---|---|---|---|---|
| 1 | 编排态的冷启动矛盾 | Skill 能靠运营几十分钟陪跑一遍来冷启动，但 Path 跨越数月。早期 Path 只能来自回溯访谈——而回溯正是方案里批评过的"依赖回忆、细节流失"。编排态在早期不得不违反自己的核心原则 | Path 引入可信度分级：`observed`（平台内真实走完）与 `retrospective`（学长回忆整理）。retrospective 只给节点顺序，**不给耗时分布与卡点统计**，界面必须显式标注 | 4.2 `paths`、4.3、F17.3、F17.4 |
| 2 | 最大的未验证假设 | "用户本来就要做这件事，固化是副产品"成立的前提是做这件事发生在平台内。但改简历用 Word、改选题在纸上跟导师聊，工具习惯很强。不进工作台就什么都拿不到，对早期用户太硬 | 新增轨迹补录路径：平台外做完可以上传前后版本 + 回答四槽问题，**蒸馏度上限封在 0.85**，能发布但拿不到满分 | F5.5 新增 |
| 3 | 漏斗断点 | v1.1 只有"编排项 → 任务态"单向通道。任务态用户永远不知道编排态存在，做完一件事就走了。这是把一次性用户变成长期用户的唯一入口 | 反向通道：任务完成后，若该 intent 命中某条 Path 的节点，提示"这件事在这条路上属于第 N 周，要不要看完整编排" | F17.6 改为双向 |
| 4 | 部分 intent 撑不起硬约束 | 模拟面试的本质是练习不是决策，一次模拟面试里没有"什么时候切换策略"。但 F4 规定 0 个关键判断不可固化，这类任务会永远卡住且用户不知道为什么 | task_intent 分层为**可生产**与**只消费**。只消费类不要求关键判断、不提示固化、不进 Creator | 4.3、F4、F5 |
| 5 | 反馈闭环在早期永远不触发 | 版本候选要求 14 天内 ≥3 次且 ≥3 个用户。冷启动期一个 Skill 总共可能只有 5 次调用，V1.0→V1.1 在真实早期是纸面的 | 冷启动规则：调用量 <20 时降为 ≥2 次且 ≥2 个用户，并允许创作者提交一次自己的真实执行作为独立验证 | F12 |
| 6 | 砍掉分数之后用户怎么选 | 砍综合分是对的，但五个候选各有一堆维度，用户只能靠"系统替我排的序"，而对系统排序的信任本来就不高 | 每个结果增加 `choose_if`：一句"选它取决于什么"（材料已齐选 A，还没盘清选 B）。把选择权还给用户且不引入分数 | F8、P6 |
| 7 | "你怎么证明有用" | 坚持不要结果指标是对的，但缺一个不越界的答法 | 用**是否回来**作为唯一诚实的有效性证明：编排的三周复核留存 + Skill 的 7 日复用率。没用的东西人不会回来第三次 | 9.4 新增 |

**版本 PRD v1.1 · 2026-08-12 · 对应产品方案 V0.2**

**读者：**前端、后端、AI 工程、设计、运营。本文档的目标是让实现者不需要回头问"这里到底要什么"。

# v1.1 修订说明：从两态到三态

## 为什么改

v1.0 把保研、考研、人生方向这类需求整类判为"伪需求"并拒绝，这是错的。错误不在于"要不要拒绝"，而在于**把"结果不可控"当成了"整件事不能做"**。

以保研为例：名额确实不可控，但保研准备的时间线高度可复用——每年几十万人走同一条路，什么时候联系导师、什么时候材料要齐、什么时候出结果，这些全是纯粹的继承性知识。v1.0 用"是否拿到名额"作为完成标准，所以判它不可测；如果完成标准是"接下来八周每周该做什么、关键节点有没有漏"，它是完全可测的。

**而这恰好是"让上一位大学生的终点成为下一位的起点"最成立的场景。**把它排除掉，等于把产品主张最锋利的一块亲手扔了。

## 改了什么

| 变化 | v1.0 的问题 | v1.1 的处理 |
|---|---|---|
| 引入第三态：编排态 | 只有"任务态"和"拒绝态"两个出口，长周期方向性需求无处可去 | 新增编排态。交付物不是产物而是一份带时间的编排；完成标准定义在过程节奏上，不在结果上 |
| 重切五类 intent | 五类一律拦死 | 情绪类仍全拦；"该不该"仍不给建议但"决定之后怎么排"进编排态；名额竞争、资源依赖进编排态；实时信息从独立需求降为编排的输入字段 |
| 明确编排的供给物是 Path | Path 只是"可视化的图谱"，没有交付形态 | Path（多次执行串成的路径）成为可交付的编排源。Skill 是一次任务里的方法，Path 是多个任务的顺序与节奏 |
| 四筛的正确用法 | 用"最终结果"套四筛，导致误杀 | 明确：完成标准与短链路都判在**过程编排**上，不判在结果上 |
| 新增硬约束 | — | 没有真实 Path 作为来源的编排不允许生成；编排界面不得出现任何形式的成功率 |
| 修订不可妥协清单第 10 条 | "五类伪需求一律不进任务流、不落 Experience" | 改为"五类不进任务流；其中三类可进编排流并落 Path；情绪类与该不该类仍全拦" |

## 这次改动没有放松的地方

编排态不是"什么都能答"的后门。以下四条与 v1.0 完全一致，且在 F17 里被写成硬约束：

- 不承诺结果，不出现任何形式的成功率或"通过率"。
- 不回答"该不该"（是否考研、是否出国、是否换专业）。
- 情绪与心理类需求仍然全拦，不进任何流。
- 不可控部分必须显式标注为不可控，不允许用模糊表述掩盖。

一句话概括这次修订：**决策本身我们不做，决策之后的编排我们做，而且只用别人真走过的路来做。**

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
| **编排** | 上下文访谈、基于真实 Path 生成带时间的编排、周复核 | P0（薄版本） |
| **编排供给** | 从多次 execution 汇聚为 Path、Path 的分叉与代价呈现 | P1 |

## 1.4 Out of Scope（明确不做）

| 不做 | 原因 |
|---|---|
| 支付、结算、提现、分润发放 | 冷启动期分不出有意义的金额，且引入合规成本。只做定价信号与归属记录 |
| 关注、粉丝、私信、评论区 | 本产品的关系是"跟走"与"同行"，不是内容社交。加了会把产品拖回内容平台 |
| Skill 综合评分与排行榜 | 违反核心信任主张（见 1.2） |
| 情绪陪伴、心理咨询类能力 | 需求真实但形态不匹配，且有伦理与安全风险。见 1.5，三态里唯一全拦的一类 |
| **"该不该"型抉择建议**（是否考研/出国/换专业） | 这类问题没有"做对了"的标准，给建议等于算命。注意：**只排除"该不该"，不排除"已经决定之后怎么排"**——后者进编排态 |
| **任何形式的结果承诺与成功率** | 保研名额、评奖、录取由分配规则与他人表现决定，无法归因。编排态可以给时间线，但一个百分比都不许出现 |
| 把实时信息做成 Skill | Skill 保存行动方法而非事实，过期的 Skill 比没有更危险。它在 v1.1 里降级为编排的输入字段（见 F17），不作为独立需求 |
| 无来源的编排 | 纯 LLM 生成的时间线看起来像样但不可信，是同类产品的通用坟场。没有真实 Path 作为来源就不生成 |
| 多端适配、国际化、暗色模式 | 非机制必要 |

## 1.5 三态路由：任务态 / 编排态 / 拒绝态

用户说一句话，系统只有三个出口。**没有第四个出口，也不允许"先聊着看看"。**

| | 任务态 task | 编排态 orchestration | 拒绝态 rejected |
|---|---|---|---|
| 用户实际要什么 | 把一件事做完 | 知道接下来几周到几个月该做什么 | — |
| 交付物 | 一份产物（选题、简历、汇报稿） | 一份带时间的编排（有序、有截止日、可勾选） | 一段说明 + 替代出口 |
| 完成标准 | 产物达到 done_criteria | 编排被采纳 + 周节奏被跟上 + 关键节点未过期 | 无 |
| 反馈周期 | 一次会话到一周 | 每周复核一次 | — |
| 供给物 | **Skill**（一次任务里的方法） | **Path**（多个任务的顺序与节奏） | — |
| 能承诺什么 | 结果可改善 | **只承诺编排，绝不承诺结果** | — |
| 落库 | experiences + executions | orchestrations + path_follows | **什么都不落** |
| 承载模块 | F4 工作台 + F5 Creator | F17 编排 | F1 内联返回 |

### 1.5.1 五类原"伪需求"的重新映射

v1.0 把这五类一律拦死。v1.1 按"结果不可控"与"过程不可编排"分开判，结果只有一类保持全拦。

| intent | v1.0 | v1.1 归属 | 系统行为 |
|---|---|---|---|
| `emotional_support` | 拒绝 | **仍然全拦** | 不进任何流。一段不做评判、不追问、不展开的回应 + 校内心理支持入口。不落任何记录 |
| `life_decision` | 拒绝 | **拆开** | "该不该考研"→ 拒绝态，不给建议，只展示真实分支与各自代价；"我决定考研了，接下来六个月怎么排"→ 编排态 |
| `zero_sum_competition` | 拒绝 | **编排态** | 给完整准备时间线，但硬性标注名额不可控，禁止任何成功率或通过率数字 |
| `resource_dependent` | 拒绝 | **编排态（部分）** | 可转移部分进编排（联系导师的时机、顺序、邮件写法）；不可转移部分（对方是否回应）显式标注为不可控 |
| `realtime_fact` | 拒绝 | **降为编排输入** | 不再是独立需求。"今年夏令营什么时候开"作为编排项的 `deadline_source` 字段存在，标注时效与来源 |

### 1.5.2 判定顺序（实现必须按此顺序）

1. 命中 `emotional_support` → 拒绝态，立即返回，**不再做任何后续判断**。
2. 命中"该不该"型措辞（是否 / 该不该 / 值不值得 + 抉择对象）→ 拒绝态的分支展示。
3. 命中编排三类，或用户明确表达了"接下来几周 / 几个月"的时间跨度 → 编排态。
4. 其余走四筛；四筛全过 → 任务态；不全过 → 说明为什么不做成 Skill。

**边界情况：**同一句话里既有情绪又有具体任务（如"烦死了，选题还没定"）仍按 v1.0 规则优先走任务态，回应中不忽略情绪但不展开。既有抉择又有编排诉求（如"我到底该不该保研，如果保研要怎么准备"）→ 抉择部分不答，编排部分进编排态，并在回应里说清这个切分。

### 1.5.3 四筛的正确用法（v1.0 的误用来源）

四把筛子本身没错，错在把它们套在**最终结果**上。v1.1 明确：

| 筛子 | 判在哪里（正确） | 判在哪里（v1.0 的错误） |
|---|---|---|
| 可摊销 | 这条路每年有多少人走 | — |
| 可测试 | **过程编排**是否有可判断的完成标准（本周三件事做了没、材料是否齐） | 最终结果是否可判断（是否拿到名额） |
| 可转移 | 时间线与顺序能否脱离具体某个人 | 结果能否复制 |
| 短链路 | **编排的复核节奏**是否够短（每周可验证） | 从开始到拿到结果的总周期 |

按修正后的判法，保研准备四筛全过；"该不该考研"仍然在"可测试"上不过关，因为它连过程都无法定义。这个判法能自动区分两者，不需要额外规则。

**硬性约束（替代 v1.0 的原约束）：**

- `emotional_support` 与"该不该"型抉择：不允许创建 `experiences`、`executions`、`orchestrations` 中的任何记录。
- 编排三类：不允许进入 Skill Creator（它们不产出 Skill），但允许创建 `orchestrations` 与 `path_follows`。
- 任何态下都不允许输出成功率、通过率、录取率这类数字。

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
| Path | 多次执行串成的顺序与节奏，编排态的供给物 | `paths` + `path_nodes` |
| 编排 | 从一条或多条真实 Path 适配到个人上下文后产生的带时间的行动序列 | `orchestrations` + `orchestration_items` |
| 周复核 | 每周一次的编排勾选，产生"节奏是否跟上"这个行为信号 | `orchestration_reviews` |
| 不可控项 | 编排里结果不由用户努力决定的部分，必须显式标注 | `orchestration_items.controllable=false` |

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
| orchestration_intent | enum | nullable，该 Path 可供哪类编排消费 |
| provenance | enum | **v1.2 新增**，`observed` / `retrospective`。决定这条 Path 能对外给什么信息 |
| walked_count | int | 走过的人数 |
| branch_summary | jsonb | 真实分叉与各自去向人数，含"停下"这一支 |

**provenance 决定的信息披露差异（v1.2 硬性规则）：**

| 能给什么 | observed | retrospective |
|---|---|---|
| 节点顺序 | ✓ | ✓ |
| 每个节点的典型耗时 | ✓ | ✕（回忆不可靠） |
| 最常卡在哪一步 | ✓ | ✕ |
| 真实分叉与人数 | ✓ | ✓（但标注为口述） |
| 界面标注 | 无需特别说明 | **必须显示"这条路是学长回忆整理的，顺序可信，时间估算仅供参考"** |

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
| task_intent | **任务态·可生产**（判断密集，可固化为 Skill）：`thesis_topic` `resume_rewrite` `resume_jd_align` `report_structure` `project_convergence` `literature_review`；**任务态·只消费**（练习型，不要求关键判断、不进 Creator）：`mock_interview` `interview_review` `content_script`；**编排态**：`zero_sum_competition` `resource_dependent` `direction_committed`；**拒绝态（全拦）**：`emotional_support` `life_decision_undecided`；**编排输入（非独立 intent）**：`realtime_fact` |
| path_provenance | `observed`（平台内真实走完，给完整统计）`retrospective`（回忆整理，只给顺序不给耗时与卡点） |
| proof_type | `platform_trace`（平台内轨迹，蒸馏度无上限）`artifact_upload`（轨迹补录，蒸馏度封顶 0.85）`self_report`（仅自述，只能落 Insight） |
| orchestration_intent | `postgrad_recommend`（保研准备）`postgrad_exam`（考研准备）`study_abroad`（出国申请）`job_season`（求职季）`research_entry`（进组做科研）`competition_season`（竞赛季） |
| orchestration_status | `drafting` `active` `paused` `completed` `abandoned` |
| item_status | `todo` `done` `skipped` `expired` |
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

## 5.4 编排生命周期（v1.1 新增）

| 当前状态 | 事件 | 目标状态 | 守卫条件 |
|---|---|---|---|
| — | 编排态识别通过，进入上下文访谈 | `drafting` | 至少命中一条真实 Path 作为来源；否则不创建 |
| `drafting` | 用户采纳编排 | `active` | 编排至少 3 项且每项有截止日；不可控项已标注 |
| `drafting` | 用户放弃 | `abandoned` | 记录放弃时缺哪些上下文，作为访谈改进信号 |
| `active` | 周复核提交 | `active` | 计算本周完成比例；过期项自动转 `expired` |
| `active` | 全部关键项完成，或用户标记结束 | `completed` | — |
| `active` | 连续 3 周未复核 | `paused` | 不算失败。发一次提醒后不再打扰 |
| `paused` | 用户回来复核 | `active` | 回来时先问"情况有变化吗"，允许重排 |
| `active` / `paused` | 来源 Path 出现被验证的新分叉 | `active` | 生成"编排更新建议"，用户确认后并入，不自动改 |

**约束：**编排永远不会因为"没拿到结果"而转 `failed`——它没有 `failed` 状态。结果不由编排负责，这是设计选择而不是遗漏。

## 5.5 移交流程

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
| 关键判断停顿点数量 | **仅对可生产类 intent 生效**：至少 1 个、最多 5 个，0 个则不可固化。只消费类（模拟面试、面试复盘、内容脚本）不要求判断停顿，也不提示固化——它们本质是练习，不产出方法（v1.2 修正） |
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

### F5.3b 轨迹补录：承认用户在平台外做事（v1.2 新增）

"用户本来就要做这件事，固化是副产品"这句话成立的前提是**做这件事发生在平台内**。但改简历用 Word、改选题在纸上跟导师聊，工具习惯很强。如果不进工作台就什么都拿不到，早期用户会直接流失。

所以给一条降级路径，但**不给平权**。

| | 平台内执行 | 轨迹补录 |
|---|---|---|
| 入口 | F4 工作台 | 个人中心「我在别处做完了一件事」 |
| 提供什么 | 自动轨迹 | 手工上传前后两个版本 + 回答四槽问题 |
| `proof_type` | `platform_trace` | `artifact_upload` |
| 蒸馏度真实任务维度 | 1.0 | 0.5 |
| **蒸馏度总分上限** | 无上限 | **封顶 0.85** |
| 能否发布 | 能 | 能（0.85 > 0.75 发布线） |
| Trust Card 标注 | "来自一次平台内真实执行" | **"来源为补录，无执行轨迹"** |
| 判断的 source_step_index | 指向真实步号 | 指向用户自述的阶段序号，并标注为补录 |

**为什么封顶而不是禁止：**封顶保住了激励结构——想拿满分就进工作台。禁止会把早期用户全挡在外面，而封顶只是让他们拿不到最高分。

**验收标准：**

| # | Given | When | Then |
|---|---|---|---|
| 1 | 用户上传前后两版简历并填满四槽 | 计算蒸馏度 | 总分不超过 0.85，即使六维全满 |
| 2 | 同上 | 发布 | 允许发布（超过 0.75 线） |
| 3 | 同上 | 查看 Trust Card 证据分区 | 显示"来源为补录，无执行轨迹" |
| 4 | 只上传产物但四槽全空 | 提交 | 落为 Insight，不允许成为 Skill |

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

**选择建议 `choose_if`（v1.2 新增，强制）：**砍掉综合分之后产生了一个真实的可用性问题——五个候选各有一堆证据维度，用户只能依赖"系统替我排的序"，而用户对系统排序的信任本来就不高。

所以每个结果额外给一句"选它取决于什么"，用**用户自己能判断的条件**表述：

> A：如果你手上材料已经比较全，选这个——它从第一步就开始做取舍。
> B：如果你连有哪些材料都还没盘清，选这个——它前两步专门做盘点。

这把选择权还给了用户，又没有引入任何分数。**约束：**`choose_if` 必须是用户能自我判断的条件（材料齐不齐、有没有实习经历、时间够不够），不能是平台内部指标（质量分高不高、样本多不多）。

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

**冷启动例外（v1.2 新增）：**上面的门槛在早期永远不会满足——冷启动期一个 Skill 总共可能只有 5 次调用，`V1.0 → V1.1` 就成了纸面能力。因此：

| 条件 | repeated_failure 门槛 |
|---|---|
| 累计调用量 ≥ 20 | 14 天内 ≥3 次且 ≥3 个不同用户（标准规则） |
| 累计调用量 < 20 | 14 天内 **≥2 次且 ≥2 个不同用户** |
| 任意调用量 | 创作者本人提交一次真实执行并复现该问题，可作为**一个独立验证**计入 |

最后一条是关键：它让创作者能主动验证问题而不是干等用户凑数，同时"独立验证"的要求仍然拦住了凭一句抱怨就改版本。降低门槛的同时没有降低证据要求。

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

## F17 编排态（v1.1 新增，P0 薄版本）

**目标：**对于保研、考研、出国、求职季这类长周期方向性需求，交付的不是一份产物，而是一份**知道接下来几周该做什么**的编排——而且它必须来自别人真走过的路，不是模型编的。

**用户故事：**作为一个已经决定保研的大三学生，我不需要谁告诉我该不该保研，我需要知道接下来八周每周该干什么、哪些材料什么时候必须齐、什么时候该联系导师。我想看到走过这条路的人当时是怎么排的，以及他们在哪里踩了坑。

### F17.1 为什么它不是任务态

| | 任务态 | 编排态 |
|---|---|---|
| 一次交互产出 | 一份产物 | 一份序列 |
| 中间有没有 AI 执行 | 有，AI 帮你做 | **没有，AI 只帮你排。做的动作发生在平台外** |
| 证据从哪来 | 平台内执行轨迹 | 别人走完这条路留下的 Path |
| 失败是什么 | 产物不达标 | 节奏跟不上、关键节点过期 |

**这条区别很重要：**编排态不承载执行，所以它不产生 `execution_steps`，也就不产生 Skill。它消费的是别人执行完之后汇聚出来的 Path。**编排态是 Path 的消费端，任务态是 Path 的生产端。**

### F17.2 流程

1. **识别**（F1 判定为编排态）→ 立刻检查是否存在可用的来源 Path。
   - 没有任何来源 Path → **不进入访谈，不生成编排**。返回："这条路目前还没有人在这里走完过，我不会凭空给你排一份。" 并给出两个出口：看看相邻方向的编排、或者自己开始走（把原话写进语料库）。
2. **上下文访谈**（P7）→ 采集编排必需的个人上下文。**目的明确，不是闲聊。**
   - 必采：目标（哪一类、什么时间点）、当前进度、每周可投入时间、硬约束（绩点、语言成绩、实验室情况）。
   - 访谈轮数上限 5 轮。每轮只问 1–2 个问题，问完即止。
   - 缺任何一项必采字段就不生成——缺上下文的编排必然是废纸。
3. **生成编排**（P7 第二阶段）→ 从来源 Path 适配到个人上下文，产出有序项。
   - 每一项必须带：标题、为什么现在做、截止日、是否可控、来源（出自哪条 Path 的哪个节点）。
   - 不可控项单独成组，不混在待办里。
4. **采纳**→ 用户可删项、改期、加项。采纳后转 `active`，进入周复核。
5. **周复核**→ 每周一次勾选。产出三个信号：本周完成比例、过期项数、是否主动重排。
6. **回流**→ 复核数据回到来源 Path，让"这条路一般要多久""哪一步最容易卡"这类信息变准。

### F17.3 硬性规则

| 规则 | 值 | 为什么 |
|---|---|---|
| 无来源 Path 不生成 | 至少 1 条 `paths` 记录，且该 Path 至少含 3 个已完成节点 | 纯 LLM 编的时间线是同类产品的通用坟场：第一次惊艳，第二次发现它不了解你 |
| 每项必须可溯源 | 每个 `orchestration_item` 必须有 `source_path_node_id` | 与"没有来源步号的判断要丢弃"是同一条原则 |
| 不可控项必须标注 | `controllable=false` 的项独立分组，文案模板固定 | 不允许用模糊表述把不可控的事说得像能努力得来 |
| 禁止任何成功率 | 接口响应与界面均不得出现百分比形式的结果预测 | 结果由分配规则与他人表现决定，给数字就是骗人 |
| 分叉必须如实呈现 | 展示来源 Path 上真实发生过的分叉与各自去向 | 见 F17.4 |
| 编排项数量 | 3–40 项，跨度 2–26 周 | 少于 3 项不构成编排；超过 26 周不可信 |
| 访谈轮数 | ≤5 轮 | 超过就变成问卷，用户会走 |
| 没有 failed 状态 | 见 5.4 | 结果不由编排负责 |

### F17.4 信任怎么建（不能复用 Trust Card 的做法）

Skill 的信任来自四类 eval，编排没有 eval——它无法在发布前跑测试。所以编排的信任来自**如实呈现分叉**：

> 这条编排来自 12 个人走过的路。其中 7 个进了夏令营，3 个中途转向考研，2 个停下了。转向考研的 3 个人，都是在第 5 周确认绩点排名之后做的决定。

这段话里没有成功率，但它给了用户真正需要的东西：**这条路的真实分布，以及分叉发生在哪一步。**这也是"不承诺结果"和"有用"能同时成立的唯一方式。

界面必须同时显示：走过的人数、各分支去向与人数、分叉最常发生的节点、以及"停下"这一支（不允许隐藏）。

### F17.5 编排专属数据表（扩展 §4.2）

**orchestrations**

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| user_id | uuid | FK not null | |
| orchestration_intent | enum | not null | 见 4.3 |
| goal_label | text | not null | 用户自己的话，如"保研到本校计算机" |
| context | jsonb | not null | 访谈采集的必采字段快照 |
| horizon_weeks | int | not null | 2–26 |
| status | enum | not null | 见 4.3 `orchestration_status` |
| adopted_at | timestamptz | nullable | 采纳时间，未采纳则为空 |
| last_review_at | timestamptz | nullable | 最近一次周复核 |
| created_at | timestamptz | not null | |

**orchestration_items**

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | uuid | PK | |
| orchestration_id | uuid | FK not null | |
| week_index | int | not null | 第几周，1 起 |
| title | text | not null | 一句能今天动手的事 |
| why_now | text | not null | 为什么这一步在这一周 |
| due_date | date | nullable | 有硬截止的必填 |
| deadline_source | text | nullable | 截止日的依据与时效（承载原 `realtime_fact`） |
| controllable | bool | not null default true | false 的项独立分组 |
| source_path_id | uuid | FK not null | **必填，无来源不允许入库** |
| source_path_node_id | uuid | FK nullable | 精确到节点更好 |
| linked_skill_id | uuid | FK nullable | 这一项如果有对应 Skill，可一键进任务态 |
| status | enum | not null | 见 4.3 `item_status` |
| done_at | timestamptz | nullable | |

**约束：**`source_path_id` 为 NOT NULL 是这张表最重要的一条——它在数据库层面保证了编排不可能凭空生成。

**orchestration_reviews**

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid | PK |
| orchestration_id | uuid | FK |
| week_index | int | 复核的是第几周 |
| done_count / total_count | int | 本周完成比例的分子分母 |
| expired_count | int | 本周过期项数 |
| replanned | bool | 用户是否在本次复核里重排 |
| note | text | 用户自己写的一句话（选填） |
| reviewed_at | timestamptz | |

### F17.6 编排与任务态的双向通道（v1.2 修订）

**正向：编排 → 任务。**编排项里带 `linked_skill_id` 的，点进去直接进 F4 工作台做那一件事。编排告诉你这周该改简历，任务态帮你把简历改完，改完之后固化成的 Skill 又能被下一份编排引用。

**反向：任务 → 编排（v1.2 新增）。**v1.1 只做了正向，结果是任务态用户永远不知道编排态存在——做完一件事就走了。这是漏斗上的一个洞，而它恰好是把一次性用户变成长期用户的唯一入口。

触发规则：一次执行 `completed` 之后，若该 `task_intent` 命中任意一条 Path 的某个节点，提示一句：

> 你刚做完的这件事，在「保研准备」这条路上属于第 3 周的动作。这条路还有 5 个节点在后面，要不要看完整编排？

| 规则 | 值 |
|---|---|
| 触发时机 | execution 转 completed 之后，与固化提示并列展示 |
| 匹配依据 | `path_nodes` 中存在与该 task_intent 相同的节点 |
| 频次上限 | 同一用户同一 orchestration_intent 只提示 2 次，拒绝两次后不再提示 |
| 不得阻塞 | 提示以卡片形式出现，不弹窗、不打断固化流程 |

**为什么不自动生成编排：**因为编排必须有跨任务的 Path 作为来源，而且必须经过上下文访谈。反向通道只负责"让用户知道编排存在"，生成仍然走 F17.2 的完整流程。

### F17.7 验收标准

| # | Given | When | Then |
|---|---|---|---|
| 1 | 库里没有任何保研相关 Path | 输入"我决定保研了，接下来怎么准备" | 不生成编排，明确说明还没人走完过这条路，给出两个替代出口，且不创建 `orchestrations` 记录 |
| 2 | 存在 1 条含 5 个已完成节点的保研 Path | 同上 | 进入访谈，最多 5 轮内采齐必采字段 |
| 3 | 访谈只采到目标，未采到每周可投入时间 | 请求生成 | 拒绝生成并指出缺哪一项 |
| 4 | 上下文采齐 | 生成编排 | 返回 3–40 项，每项含 why_now 与 source_path_node_id，不可控项独立分组 |
| 5 | 任意编排 | 查看界面与接口响应 | 不出现任何百分比形式的结果预测 |
| 6 | 来源 Path 有 3 人转向考研 | 查看编排的来源说明 | 如实显示分叉去向与人数，含"停下"这一支 |
| 7 | 编排处于 active | 提交周复核 | 记录完成比例与过期项，`last_review_at` 更新 |
| 8 | 连续 3 周未复核 | 定时任务 | 转 `paused`，只提醒一次，不重复打扰 |
| 9 | 某编排项带 linked_skill_id | 点击该项 | 跳到 F4 工作台并预填任务上下文 |
| 10 | 输入"我到底该不该保研" | 提交 | 走拒绝态分支展示，不创建编排、不给建议 |
| 11 | 输入"我最近很崩溃" | 提交 | 全拦，不创建任何记录（与 v1.0 一致） |

### F17.8 埋点

`orch_intent_detected`（intent、是否有来源 Path）、`orch_no_source_path`（原话哈希、intent，这是最重要的供给缺口信号）、`orch_interview_round`（轮次、本轮采到的字段）、`orch_generated`（项数、周跨度、来源 Path 数）、`orch_adopted`（删改项数）、`orch_review_submitted`（week_index、完成比例、过期项数、是否重排）、`orch_item_to_task`（item_id、skill_id）、`orch_paused`、`orch_completed`。

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

## P7 编排：上下文访谈与编排生成（v1.1 新增）

分两阶段，共用一个 Prompt 文件但调用时机不同。

**阶段一：上下文访谈**

输入：`orchestration_intent`、`goal_label`、已采集字段、剩余轮数。

输出 schema：

```json
{
  "questions": ["接下来这段时间你每周大概能投入几个半天？"],
  "collected": {"target": "本校计算机保研", "current_progress": "绩点排名未出"},
  "missing": ["weekly_hours", "hard_constraints"],
  "ready_to_generate": false
}
```

约束：每轮 `questions` 最多 2 条；`ready_to_generate=true` 时 `missing` 必须为空；禁止询问情绪状态、家庭情况、心理状况。

**阶段二：编排生成**

输入：`context`（采齐的上下文）、`source_paths[]`（每条含节点序列、典型耗时、真实分叉与人数）。

输出 schema：

```json
{
  "horizon_weeks": 8,
  "items": [{
    "week_index": 1,
    "title": "把三段科研经历各写成一句可验证的结果",
    "why_now": "夏令营材料在第 3 周就要投，第 1 周不动手后面会被压死",
    "due_date": "2026-03-15",
    "deadline_source": null,
    "controllable": true,
    "source_path_node_id": "…",
    "linked_skill_intent": "resume_rewrite"
  }],
  "uncontrollable": [{
    "title": "绩点排名结果",
    "note": "由全院排名决定，不由你的准备决定。第 5 周出，出来之后可能需要重排。"
  }],
  "branch_summary": "12 人走过：7 进夏令营，3 转考研，2 停下。转向多发生在第 5 周确认排名之后。"
}
```

**铁律：**

1. 每个 item 必须有 `source_path_node_id`，且必须指向输入里真实存在的节点。缺失或越界的 item 由后端直接丢弃。
2. **禁止输出任何百分比形式的结果预测。**`branch_summary` 只能给绝对人数，不能给比率。
3. 不可控的事必须放进 `uncontrollable`，不允许出现在 `items` 里伪装成待办。
4. `why_now` 必须解释"为什么是这一周"，不能写成任务描述的复述。
5. 不允许出现"该不该"的判断，即使用户在访谈里问了。

**兜底：**生成失败或 schema 校验两次不通过 → 不返回半成品编排，而是把来源 Path 的原始节点序列直接展示给用户，标注"这是别人走过的原始顺序，我没能帮你适配到你的情况"。半成品编排比没有编排更有害。

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
| POST | `/orchestrations/probe` | 编排态前置检查：有没有可用的来源 Path | P0 |
| POST | `/orchestrations/interview` | 上下文访谈一轮 | P0 |
| POST | `/orchestrations` | 生成并落库编排（drafting） | P0 |
| GET | `/orchestrations/{id}` | 编排全量（含不可控项与分叉说明） | P0 |
| POST | `/orchestrations/{id}/adopt` | 采纳，转 active | P0 |
| PATCH | `/orchestrations/{id}/items/{itemId}` | 改期、勾选、跳过 | P0 |
| POST | `/orchestrations/{id}/reviews` | 提交周复核 | P0 |
| GET | `/orchestrations/mine` | 我的编排列表 | P0 |
| POST | `/orchestrations/{id}/items/{itemId}/to-task` | 从编排项跳进任务态并预填上下文 | P1 |

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
| 编排 | 编排采纳率 | adopted / generated | ≥ 0.60 |
| 编排 | 周节奏跟上率 | 复核中本周完成比例的中位数 | ≥ 0.50 |
| 编排 | 关键节点过期率 | expired 项 / 有截止日的项 | ≤ 0.15 |
| 编排 | 复核留存 | 连续复核 ≥ 3 周的编排占比 | ≥ 0.40 |
| 编排 | 无来源拦截率 | `orch_no_source_path` / 编排态请求 | 观察项；持续偏高说明该补 Path 供给 |
| 编排 | 编排转任务率 | 从编排项进入工作台的次数 / 活跃编排数 | ≥ 0.30，体现三态贯通 |

**关于编排指标的一条纪律：**这里没有、也不许加"目标达成率"。编排不承诺结果，一旦把结果指标放进考核，产品会立刻开始优化"让用户觉得能成"，那是骗人的开始。

## 9.4 有效性怎么证明（v1.2 新增）

不要结果指标是对的，但必须有一个能当面回答"你凭什么说有用"的口径。答案是：**用"用户是否回来"作为唯一诚实的有效性证明。**

| 证据 | 指标 | 为什么它能证明有用 |
|---|---|---|
| 编排 | 连续复核 ≥3 周的编排占比 | 一份没用的时间表，人不会连续三周回来勾选 |
| Skill | 7 日复用率 | 一个没用的方法，人不会第二次主动调用 |
| 任务态 | 执行完成率（完成 / 完成+放弃） | 半路放弃是最直接的无用信号 |

对外口径固定为一句话：

> 我们不敢说你能保研成功，那不由我们决定。但我们能证明用过的人第三周还在用——没用的东西，人不会回来第三次。

**禁止的说法：**任何形式的"提升 X%"、"成功率"、"上岸率"，以及任何用个案（"某同学用了之后拿到了 offer"）代替群体行为信号的表述。个案不是证据。

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
| F17 编排（薄版本） | 只支持 `postgrad_recommend` 一个方向；来源 Path 用运营手工录入的 1–2 条；访谈固定 4 个问题不做动态追问；编排生成 + 采纳 + 一次周复核；无来源时的拦截必须真实生效 |

## 12.2 用预置数据但必须真实结构

| 项 | 做法 |
|---|---|
| online_score / maintenance_score | 预置，但走真实字段与公式，界面标注样本不足 |
| Growth Graph | 3 条真实分支的静态数据 |
| 编排的来源 Path | 运营手工录入 1–2 条保研 Path（含节点序列、典型耗时、真实分叉与人数）。**但"无来源就不生成"这条必须是真逻辑**——Demo 里要现场演一次输入没人走过的方向、系统拒绝生成 |
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
10. **（v1.1 修订）**情绪类与"该不该"型抉择不进任何流、不落任何记录；编排三类不进任务流、不产出 Skill，但可落 `orchestrations` 与 `path_follows`。
11. **（v1.1 新增）**没有真实 Path 作为来源的编排不允许生成。`orchestration_items.source_path_id` 在数据库层面为 NOT NULL。
12. **（v1.1 新增）**任何态、任何界面、任何接口响应都不得出现成功率、通过率、录取率这类结果预测数字。编排只给绝对人数与真实分叉。
13. **（v1.1 新增）**编排里不可控的部分必须独立分组并显式标注，不允许伪装成待办项。
14. **（v1.2 新增）**`retrospective` 来源的 Path 不允许给出耗时分布与卡点统计，界面必须标注它来自回忆整理。
15. **（v1.2 新增）**轨迹补录（`proof_type=artifact_upload`）的蒸馏度总分封顶 0.85，且 Trust Card 必须标注"来源为补录，无执行轨迹"。
16. **（v1.2 新增）**对外证明有效性只允许用行为信号（复核留存、复用率、完成率），禁止任何"提升 X%"式表述，也禁止用个案代替群体信号。

## 关于第 10 条的修订说明

v1.0 的第 10 条把两件不同的事混在了一起：「不适合 Skill 化」和「不做」。保研的名额不可控，但保研准备的时间线高度可复用；把整类拒绝掉，等于因为结果不可承诺就放弃了过程可编排。

这次修订**不是妥协，是纠错**。判断标准反而更严了：新增的第 11、12、13 条把"不承诺结果"从一句原则变成了三条可检查的约束，其中第 11 条直接落在数据库的 NOT NULL 上。

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
| 第 2 章 需求判别（v1.1 需回改方案） | 1.5 三态路由；方案 V0.2 的四象限需要更新：原"有意识地不做"象限要拆成编排态与拒绝态两块 |
| 第 6 章 Growth Graph（v1.1 升级） | Path 从"可视化图谱"升级为编排态的供给物，见 F17.1 |

## C1. v1.1 对产品方案 V0.2 的回改清单

PRD 改了，方案也要跟着改，否则两份文档会互相矛盾。以下是需要回改方案的地方：

| 方案位置 | 需要怎么改 |
|---|---|
| §2.4 明确排除五类伪需求 | 改成三类：仍全拦的（情绪）、拆开的（该不该 vs 决定后编排）、进编排态的（名额竞争、资源依赖）；实时信息改为编排输入 |
| §2.5 四象限 | 原"需求强度高 + Skill 适配度低 = 有意识地不做"象限要拆：编排态占大部分，只有情绪类留在不做 |
| §2.2 四把筛子 | 补一句：完成标准与短链路都判在过程编排上，不判在结果上 |
| §6 Growth Graph | 补 Path 作为可交付编排物的定位 |
| §17.2 关键表达 | 建议新增一句金句："决策本身我们不做，决策之后的编排我们做，而且只用别人真走过的路来做。" |
| §16.2 风险表 | 新增一行："编排变成算命 / 表现：出现成功率或该不该的建议 / 应对：三条硬约束 + 只给绝对人数与真实分叉" |
