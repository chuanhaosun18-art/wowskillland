// 迷茫期路由器（Crossroad）数据模型
//
// 产品主张：每一次「我不知道」，都有人真走过。
// 三条纪律：不测评、不建议、不承诺。
//
// 8 张表跑通全场：
//   users      处境字段（grade/major/weekly_hours）直接放用户表，匹配用
//   moments    每一次「我不知道」都是一条记录
//   interviews 迷茫访谈快照（≤5 轮，只采处境与行为信号）
//   hypotheses 方向假设（必须引用访谈原话作为依据）
//   cards      第一步卡 = Skill 包（boundary 为空不能发布，每字段可溯源到口述）
//   attempts   微尝试记录（放弃也是合法终态）
//   forks      路口分叉聚合（只存绝对人数与代价原话，不给百分比）
//   wishes     没有匹配卡时的许愿池，反向指导供给
package main

// 迷茫形态四态：探索型 / 抉择型 / 行动型 / 情绪类（拦截，不落记录）
const (
	MomentExplore = "explore" // 探索型：还没想清楚要什么
	MomentDecide  = "decide"  // 抉择型：在几个明确选项之间
	MomentAct     = "act"     // 行动型：已决定，需要的是怎么排
	MomentEmotion = "emotion" // 情绪类：拦截，给校内心理支持入口
)

// 微尝试状态
const (
	AttemptRunning  = "running"
	AttemptFinished = "finished"
	AttemptAbandoned = "abandoned"
)

// initCrossroadSchema 建迷茫期路由器相关表；由 initDB 末尾调用
func initCrossroadSchema() {
	schema := `
-- 处境字段补列：weekly_hours（每周自由时间）用于匹配
ALTER TABLE users ADD COLUMN weekly_hours INTEGER DEFAULT 0;
`

	// 幂等迁移：users 表补列
	db.Exec(schema)

	// 主表（IF NOT EXISTS 幂等）
	main := `
CREATE TABLE IF NOT EXISTS moments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  raw_text TEXT NOT NULL,
  routed_state TEXT NOT NULL,
  routed_reason TEXT DEFAULT '',
  context TEXT DEFAULT '{}',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS interviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  moment_id INTEGER NOT NULL,
  context TEXT DEFAULT '{}',
  turns TEXT DEFAULT '[]',
  round_count INTEGER DEFAULT 0,
  ready INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hypotheses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  moment_id INTEGER NOT NULL,
  label TEXT NOT NULL,
  evidence_quote TEXT NOT NULL,
  card_id INTEGER,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  creator_id INTEGER,
  trigger_context TEXT NOT NULL,
  script TEXT NOT NULL,
  done_criteria TEXT NOT NULL,
  decision_points TEXT DEFAULT '[]',
  boundary TEXT NOT NULL,
  feeling TEXT DEFAULT '',
  source_transcript TEXT DEFAULT '',
  variant_of_card_id INTEGER,
  verification_count INTEGER DEFAULT 0,
  verified_yes INTEGER DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'published',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  card_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  moment_id INTEGER,
  status TEXT NOT NULL DEFAULT 'running',
  flow_moments TEXT DEFAULT '[]',
  escape_moments TEXT DEFAULT '[]',
  verdict TEXT DEFAULT '',
  counter_example TEXT DEFAULT '',
  finished_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS forks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  junction_label TEXT NOT NULL,
  walked_count INTEGER DEFAULT 0,
  branches TEXT DEFAULT '[]',
  switch_node TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS wishes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  moment_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  direction_label TEXT NOT NULL,
  fulfilled INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
	// 幂等迁移：interviews.moment_id 必须唯一，upsert（ON CONFLICT）才生效。
	// 用 CREATE UNIQUE INDEX IF NOT EXISTS 保证老库升级后同样成立。
	db.Exec(main)
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_interviews_moment ON interviews(moment_id)`)
}
