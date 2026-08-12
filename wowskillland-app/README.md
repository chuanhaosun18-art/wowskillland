# WowSkillLand

面向大学生迷茫期的 Skill / Agent 平台。一句话定位：**每一次「我不知道」，都有人真走过。**

三条纪律贯穿全部界面：**不测评、不建议、不承诺。**

产品设计全文见 [`docs/WowSkillLand_产品设计.md`](../docs/WowSkillLand_产品设计.md)，可浏览版见 [`docs/迷茫期路由器_产品设计.html`](../docs/迷茫期路由器_产品设计.html)。

## 它解决什么

大学生成长里的经验散落在学长口述、社群帖子和通用 AI 里：搜得到共鸣，落不到下一步。WowSkillLand 把这些经验收成可装载的 Skill——匹配的不是「最佳答案」，而是**处境和你最像、且真的走过来了的人**。

供给分三层：

| 层 | 作用 |
|---|---|
| 故事层 | 学长口述全文，负责共鸣 |
| 卡片层 | 可执行的第一步（两周剧本 / 判断点 / 边界） |
| 分布层 | 路口绝对人数与代价原话，校准幸存者偏差 |

## 用户怎么走

唯一入口是首页输入框：「说说你现在的不知道」。路由器分成四个出口，判错可以一键改道。

```
一句话
  ├─ 探索型 → 轻访谈 → 匹配第一步卡 → 装载陪跑
  ├─ 抉择型 → 路口页（只给人数和代价，不给建议）
  ├─ 行动型 → 编排（只用别人走过的 Path；没人走过就拒绝生成）
  └─ 情绪类 → 拦截，给校心理支持入口，不落任何记录
```

四个路口：大0 本省/外省、大二 转专业、大三 保研/就业/考研、大四 工作/读研。

## 怎么跑

需要 Go 1.22+、Python 3（或任意静态服务器）、DeepSeek API Key。

```bash
# 1. 后端
cd backend
export SKILLHUB_DATA="$PWD/../.data"
export DEEPSEEK_API_KEY="sk-..."
go run .                 # http://localhost:8080

# 2. 前端
cd ../wowskillland-app
python3 -m http.server 5500
# 打开 http://localhost:5500
```

手机直接访问同一地址即可（底栏导航）。登录 / 注册走后端 JWT；登录后首页输入会打 DeepSeek 三态路由。

## 前后端怎么接

前端只调 `js/api.js` 里的 `WowAPI.*`。`WowConfig.USE_MOCK = false` 时走真实后端。

| 前端 | 后端 | 说明 |
|---|---|---|
| 登录 / 注册 | `POST /api/auth/login` `register` | JWT + AI 问卷 |
| 意图路由 | `POST /api/growth/goals/interpret` | 探索 / 抉择 / 编排 / 情绪 |
| 陪跑对话 | `POST /api/growth/executions` + `advance` | 把卡的剧本、判断点、边界注入 material |
| 学长抽取 | `POST /api/growth/backfill` | 口述补录，蒸馏度封顶 0.85 |
| 编排探测 | `POST /api/growth/orch-probe` | 无来源 Path 则拒绝生成 |
| Skill 列表 | `GET /api/skills` | 后端库为空时仍显示演示卡 |

## 目录

```
wowskillland-app/
  index.html      单页入口（hash 路由，适配手机）
  css/app.css
  js/data.js      阶段 / 路口 / 演示 Skill
  js/real_skills.js   GitHub 开源 Skill
  js/api.js       后端适配层
  js/app.js       页面与交互
```

后端仍是本仓库的 Go + Gin + SQLite（`backend/`），成长闭环说明见 [`GROWTH_README.md`](../GROWTH_README.md)。
