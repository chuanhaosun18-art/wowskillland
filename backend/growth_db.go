// 成长闭环数据层：任务执行 → 关键判断 → Skill 版本 → 门禁 → 准入评分 → 反馈升级
// 说明：本文件只做「增量」迁移，不修改也不删除既有的 skills / skill_files / skill_reviews / skill_issues 表。
// 与 PRD 的偏差：PRD 规定主键为 uuid v7，此处沿用既有代码库的 INTEGER AUTOINCREMENT，保持风格一致。
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// ---------- 枚举常量（禁止在业务代码里写字面量魔法字符串） ----------

// 允许进入任务流的 task_intent
const (
	IntentThesisTopic       = "thesis_topic"
	IntentResumeRewrite     = "resume_rewrite"
	IntentResumeJDAlign     = "resume_jd_align"
	IntentReportStructure   = "report_structure"
	IntentMockInterview     = "mock_interview"
	IntentInterviewReview   = "interview_review"
	IntentProjectConverge   = "project_convergence"
	IntentLiteratureReview  = "literature_review"
	IntentContentScript     = "content_script"
)

// 禁止进入任务流的 task_intent（伪需求五类）
const (
	IntentEmotionalSupport = "emotional_support"
	IntentLifeDecision     = "life_decision"
	IntentZeroSum          = "zero_sum_competition"
	IntentRealtimeFact     = "realtime_fact"
	IntentResourceDep      = "resource_dependent"
)

// AllowedIntents 允许执行的任务类型
var AllowedIntents = map[string]string{
	IntentThesisTopic:      "论文选题打磨与收敛",
	IntentResumeRewrite:    "把科研经历改成产研岗位简历",
	IntentResumeJDAlign:    "简历与具体 JD 对齐",
	IntentReportStructure:  "组会汇报与答辩陈述结构",
	IntentMockInterview:    "模拟面试",
	IntentInterviewReview:  "面试复盘",
	IntentProjectConverge:  "项目与竞赛方案收敛",
	IntentLiteratureReview: "文献综述入门与检索策略",
	IntentContentScript:    "内容脚本与选题结构",
}

// ProductiveIntents 可生产类：判断密集，可固化为 Skill。
// 只消费类（模拟面试、面试复盘、内容脚本）本质是练习，不产出方法——
// 对它们要求「至少一个关键判断」会让用户永远卡住且不知道为什么（PRD v1.2 第 4 条）。
var ProductiveIntents = map[string]bool{
	IntentThesisTopic:      true,
	IntentResumeRewrite:    true,
	IntentResumeJDAlign:    true,
	IntentReportStructure:  true,
	IntentProjectConverge:  true,
	IntentLiteratureReview: true,
}

// isProductive 该 intent 是否可固化为 Skill
func isProductive(intent string) bool {
	return ProductiveIntents[intent]
}

// ---------- 编排态（PRD v1.1 F17 / v1.2 修订） ----------

// 编排类 intent
const (
	IntentDirectionCommitted = "direction_committed" // 已决定方向，要编排
)

// OrchestrationIntents 可编排的方向
var OrchestrationIntents = map[string]string{
	"postgrad_recommend": "保研准备",
	"postgrad_exam":      "考研准备",
	"study_abroad":       "出国申请",
	"job_season":         "求职季",
	"research_entry":     "进组做科研",
	"competition_season": "竞赛季",
}

// Path 可信度分级（v1.2 第 1 条）。
// 决定这条 Path 能对外给什么信息——回忆整理来的不给耗时分布与卡点统计。
const (
	ProvenanceObserved      = "observed"
	ProvenanceRetrospective = "retrospective"
)

// 编排与编排项状态
const (
	OrchDrafting  = "drafting"
	OrchActive    = "active"
	OrchPaused    = "paused"
	OrchCompleted = "completed"
	OrchAbandoned = "abandoned"
)

const (
	ItemTodo    = "todo"
	ItemDone    = "done"
	ItemSkipped = "skipped"
	ItemExpired = "expired"
)

// 证据类型（v1.2：轨迹补录）
const (
	ProofPlatformTrace  = "platform_trace"
	ProofArtifactUpload = "artifact_upload"
	ProofSelfReport     = "self_report"
)

// 编排相关阈值
const (
	OrchMinItems          = 3
	OrchMaxItems          = 40
	OrchMinWeeks          = 2
	OrchMaxWeeks          = 26
	OrchInterviewMaxRound = 5
	OrchPauseAfterWeeks   = 3    // 连续几周未复核转 paused
	PathMinNodesForOrch   = 3    // 一条 Path 至少几个节点才够做编排来源
	BackfillScoreCap      = 0.85 // 轨迹补录的蒸馏度上限（v1.2 第 2 条）
	ColdStartCallCount    = 20   // 低于此调用量走冷启动反馈门槛（v1.2 第 5 条）
)

// RejectedIntents 拒绝态（v1.2 后只剩两类真正全拦）
//
// v1.1 之前这里有五类。修正的原因是：把「结果不可控」当成了「整件事不能做」。
// 保研的名额不可控，但保研准备的时间线高度可复用——那部分进编排态（见 growth_orchestration.go）。
var RejectedIntents = map[string]string{
	IntentEmotionalSupport: "情绪与心理类需求真实但不适合拆成流程与完成标准，我们不做成 Skill。",
	IntentLifeDecision:     "这类选择没有可判断的完成标准，给建议等于算命。我们只展示别人真实走过的分支与各自代价。",
	IntentRealtimeFact:     "Skill 保存的是行动方法而不是事实，做成 Skill 第二天就会过期。它在编排里作为截止日依据存在。",
}

// OrchestrationRouteIntents 编排态：不进任务流、不产出 Skill，但给带时间的编排
var OrchestrationRouteIntents = map[string]string{
	IntentZeroSum:            "postgrad_recommend",
	IntentResourceDep:        "research_entry",
	IntentDirectionCommitted: "postgrad_recommend",
}

// decision 四个槽位
const (
	SlotWhenToCheck   = "when_to_check"
	SlotWhenToProbe   = "when_to_probe"
	SlotWhenToUseTool = "when_to_use_tool"
	SlotWhenToSwitch  = "when_to_switch"
)

// DecisionSlots 四槽定义（顺序即界面顺序）
var DecisionSlots = []struct {
	Slot   string
	Prompt string
}{
	{SlotWhenToCheck, "在哪一步你会停下来回头验证？"},
	{SlotWhenToProbe, "什么情况下你会要求补充信息而不是直接动手？"},
	{SlotWhenToUseTool, "哪一步必须查、必须跑，不能靠判断？"},
	{SlotWhenToSwitch, "什么现象一出现，你就知道当前这条路走不通？"},
}

// skill 生命周期状态
const (
	SkillStatusDraft       = "draft"
	SkillStatusInsightOnly = "insight_only"
	SkillStatusGated       = "gated"
	SkillStatusPublished   = "published"
	SkillStatusNeedsReview = "needs_review"
	SkillStatusDeprecated  = "deprecated"
	SkillStatusArchived    = "archived"
)

// execution 状态
const (
	ExecRunning   = "running"
	ExecCompleted = "completed"
	ExecAbandoned = "abandoned"
	ExecFailed    = "failed"
	ExecHandedOff = "handed_off"
)

// step 类型
const (
	StepAIAction     = "ai_action"
	StepToolCall     = "tool_call"
	StepUserDecision = "user_decision"
	StepHumanHandoff = "human_handoff"
)

// eval 类型
const (
	EvalDiscoverability = "discoverability"
	EvalCompletion      = "completion"
	EvalRobustness      = "robustness"
	EvalStability       = "stability"
	EvalBoundaryStop    = "boundary_stop"
	EvalPrudence        = "prudence" // 审慎度测试（经验型：信息不足/对抗诱导）
)

// skill 来源
const (
	OriginRouteOne   = "route_one_execution" // 先做一遍再固化（主路径）
	OriginRouteTwo   = "route_two_import"    // AI 引导对话生成（辅助，须补一次真实执行）
	OriginRouteUpload = "route_upload"       // 网页直接上传 Skill 包（先过四问门禁再上架）
	OriginOpsSeed    = "ops_seed"            // 运营种子蒸馏
)

// 发布门禁阈值（PRD F5.3 / F6.2）
const (
	DistillationThreshold = 0.75 // 蒸馏度总分下限
	DecisionsMinScore     = 0.5  // 判断维度下限（至少填满两个槽位）
	RecallAt5Threshold    = 0.80 // 可发现性
	CompletionThreshold   = 0.80
	StabilityThreshold    = 0.70
	BoundaryStopThreshold = 1.00 // 硬性，不可下调
	AbandonIdleMinutes    = 30   // 静置多久判定放弃
	OnlineEvidenceMinCall = 10   // 少于此调用量走冷启动先验
)

// ---------- 结构体 ----------

// Execution 一次真实任务执行
type Execution struct {
	ID              int64           `json:"id"`
	UserID          int64           `json:"user_id"`
	SkillVersionID  *int64          `json:"skill_version_id"`
	TaskIntent      string          `json:"task_intent"`
	TaskTitle       string          `json:"task_title"`
	UserContext     json.RawMessage `json:"user_context"`
	Input           json.RawMessage `json:"input"`
	Output          json.RawMessage `json:"output,omitempty"`
	Status          string          `json:"status"`
	CompletionSignal json.RawMessage `json:"completion_signal,omitempty"`
	CorrectionRatio float64         `json:"correction_ratio"`
	AbandonedAtStep *int            `json:"abandoned_at_step,omitempty"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	Steps           []ExecutionStep `json:"steps,omitempty"`
}

// ExecutionStep 执行轨迹的一步。这张表是整个系统的地基：
// Skill 草稿、画像信号、审计日志、行为信号全部由它派生。
type ExecutionStep struct {
	ID           int64           `json:"id"`
	ExecutionID  int64           `json:"execution_id"`
	StepIndex    int             `json:"step_index"`
	StepType     string          `json:"step_type"`
	Title        string          `json:"title"`
	DecisionSlot string          `json:"decision_slot,omitempty"`
	UserChoice   json.RawMessage `json:"user_choice,omitempty"`
	ToolName     string          `json:"tool_name,omitempty"`
	ToolOK       *bool           `json:"tool_ok,omitempty"`
	Input        string          `json:"input,omitempty"`
	Output       string          `json:"output,omitempty"`
	LatencyMS    int             `json:"latency_ms"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Decision 会改变结果的单条关键判断，最小可溯源单位
type Decision struct {
	ID              int64      `json:"id"`
	ExperienceExecID int64     `json:"experience_exec_id"` // 来源执行，不可为空
	SkillID         *int64     `json:"skill_id,omitempty"`
	Slot            string     `json:"slot"`
	TriggerSignal   string     `json:"trigger_signal"`
	Judgment        string     `json:"judgment"`
	Scope           string     `json:"scope"`
	CounterExample  string     `json:"counter_example,omitempty"`
	SourceStepIndex int        `json:"source_step_index"`
	VerifiedByCount int        `json:"verified_by_count"`
	InvalidatedAt   *time.Time `json:"invalidated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// SkillVersion 一个 Skill 的所有可执行内容都在版本上
type SkillVersion struct {
	ID                 int64      `json:"id"`
	SkillID            int64      `json:"skill_id"`
	Version            string     `json:"version"`
	Description        string     `json:"description"`
	Goal               string     `json:"goal"`
	DoneCriteria       string     `json:"done_criteria"`
	Workflow           string     `json:"workflow"`
	Boundary           string     `json:"boundary"`
	Contract           string     `json:"contract"`
	Gotchas            string     `json:"gotchas"`
	DistillationScore  float64    `json:"distillation_score"`
	DistillationDetail string     `json:"distillation_detail"`
	ProofType          string     `json:"proof_type"`
	Changelog          string     `json:"changelog,omitempty"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// EvalRun 一次发布前测试的结果
type EvalRun struct {
	ID           int64     `json:"id"`
	SkillID      int64     `json:"skill_id"`
	VersionID    int64     `json:"version_id"`
	EvalType     string    `json:"eval_type"`
	PassedCount  int       `json:"passed_count"`
	TotalCount   int       `json:"total_count"`
	PassRate     float64   `json:"pass_rate"`
	Threshold    float64   `json:"threshold"`
	Passed       bool      `json:"passed"`
	Detail       string    `json:"detail"`
	RanAt        time.Time `json:"ran_at"`
}

// SkillScore 准入四层的物化结果
type SkillScore struct {
	SkillID          int64     `json:"skill_id"`
	AdmissionPassed  bool      `json:"admission_passed"`
	AdmissionFailures string   `json:"admission_failures"`
	OfflineScore     float64   `json:"offline_score"`
	OnlineScore      float64   `json:"online_score"`
	MaintenanceScore float64   `json:"maintenance_score"`
	QualityScore     float64   `json:"quality_score"`
	SampleSufficient bool      `json:"sample_sufficient"`
	CandidateEligible bool     `json:"is_candidate_eligible"`
	ComputedAt       time.Time `json:"computed_at"`
}

// ---------- 迁移 ----------

// initGrowthSchema 建成长闭环相关表；由 initDB 末尾调用
func initGrowthSchema() {
	schema := `
CREATE TABLE IF NOT EXISTS executions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  skill_version_id INTEGER,
  task_intent TEXT NOT NULL,
  task_title TEXT DEFAULT '',
  user_context TEXT DEFAULT '{}',
  input TEXT DEFAULT '{}',
  output TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'running',
  completion_signal TEXT DEFAULT '',
  correction_ratio REAL DEFAULT 0,
  abandoned_at_step INTEGER,
  started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  ended_at DATETIME,
  last_active_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS execution_steps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  execution_id INTEGER NOT NULL,
  step_index INTEGER NOT NULL,
  step_type TEXT NOT NULL,
  title TEXT DEFAULT '',
  decision_slot TEXT DEFAULT '',
  user_choice TEXT DEFAULT '',
  tool_name TEXT DEFAULT '',
  tool_ok INTEGER,
  input TEXT DEFAULT '',
  output TEXT DEFAULT '',
  latency_ms INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS decisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  experience_exec_id INTEGER NOT NULL,
  skill_id INTEGER,
  slot TEXT NOT NULL,
  trigger_signal TEXT NOT NULL,
  judgment TEXT NOT NULL,
  scope TEXT NOT NULL,
  counter_example TEXT DEFAULT '',
  source_step_index INTEGER NOT NULL,
  verified_by_count INTEGER DEFAULT 0,
  invalidated_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS insights (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  execution_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  claim TEXT NOT NULL,
  why TEXT DEFAULT '',
  missing_for_skill TEXT DEFAULT '[]',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skill_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  version TEXT NOT NULL,
  description TEXT DEFAULT '',
  goal TEXT DEFAULT '',
  done_criteria TEXT DEFAULT '[]',
  workflow TEXT DEFAULT '[]',
  boundary TEXT DEFAULT '{}',
  contract TEXT DEFAULT '{}',
  gotchas TEXT DEFAULT '[]',
  distillation_score REAL DEFAULT 0,
  distillation_detail TEXT DEFAULT '{}',
  changelog TEXT DEFAULT '',
  source_execution_id INTEGER,
  published_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(skill_id, version)
);

CREATE TABLE IF NOT EXISTS skill_evals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  version_id INTEGER NOT NULL,
  eval_type TEXT NOT NULL,
  input TEXT NOT NULL,
  expected TEXT DEFAULT '',
  is_replay INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS eval_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  version_id INTEGER NOT NULL,
  eval_type TEXT NOT NULL,
  passed_count INTEGER DEFAULT 0,
  total_count INTEGER DEFAULT 0,
  pass_rate REAL DEFAULT 0,
  threshold REAL DEFAULT 0,
  passed INTEGER DEFAULT 0,
  detail TEXT DEFAULT '[]',
  ran_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS exec_feedbacks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  execution_id INTEGER NOT NULL,
  skill_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  issue_type TEXT NOT NULL,
  description TEXT DEFAULT '',
  suggested_change TEXT DEFAULT '',
  adopted INTEGER DEFAULT 0,
  adopted_version_id INTEGER,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS exec_closings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  execution_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  skill_id INTEGER NOT NULL,
  q1 TEXT DEFAULT '',
  q2 TEXT DEFAULT '',
  verdict TEXT NOT NULL DEFAULT '成立',
  verdict_reason TEXT DEFAULT '',
  improvement TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS version_candidates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  trigger_rule TEXT NOT NULL,
  evidence TEXT DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'open',
  resulting_version_id INTEGER,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS skill_scores (
  skill_id INTEGER PRIMARY KEY,
  admission_passed INTEGER DEFAULT 0,
  admission_failures TEXT DEFAULT '[]',
  offline_score REAL DEFAULT 0,
  online_score REAL DEFAULT 0,
  maintenance_score REAL DEFAULT 0,
  quality_score REAL DEFAULT 0,
  sample_sufficient INTEGER DEFAULT 0,
  is_candidate_eligible INTEGER DEFAULT 0,
  computed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS description_corpus (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  utterance TEXT NOT NULL,
  source TEXT NOT NULL,
  task_intent TEXT DEFAULT '',
  mapped_skill_id INTEGER,
  used_in_eval INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- ---------- 编排态（F17）----------
-- Path：多次执行串成的顺序与节奏，编排态的供给物。
-- provenance 决定它能对外给什么信息：retrospective 只给顺序，不给耗时与卡点。
CREATE TABLE IF NOT EXISTS paths (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  goal_label TEXT NOT NULL,
  orchestration_intent TEXT DEFAULT '',
  owner_user_id INTEGER,
  provenance TEXT NOT NULL DEFAULT 'retrospective',
  walked_count INTEGER DEFAULT 0,
  branch_summary TEXT DEFAULT '{}',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS path_nodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path_id INTEGER NOT NULL,
  node_index INTEGER NOT NULL,
  label TEXT NOT NULL,
  task_intent TEXT DEFAULT '',
  week_offset INTEGER DEFAULT 0,
  typical_duration_days INTEGER,
  common_blocker TEXT DEFAULT '',
  controllable INTEGER DEFAULT 1,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orchestrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  orchestration_intent TEXT NOT NULL,
  goal_label TEXT NOT NULL,
  context TEXT DEFAULT '{}',
  horizon_weeks INTEGER NOT NULL DEFAULT 8,
  status TEXT NOT NULL DEFAULT 'drafting',
  branch_summary TEXT DEFAULT '',
  source_path_ids TEXT DEFAULT '[]',
  adopted_at DATETIME,
  last_review_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- source_path_id NOT NULL 是这张表最重要的一条约束：
-- 它在数据库层面保证编排不可能凭空生成（PRD 不可妥协清单第 11 条）。
CREATE TABLE IF NOT EXISTS orchestration_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  orchestration_id INTEGER NOT NULL,
  week_index INTEGER NOT NULL,
  title TEXT NOT NULL,
  why_now TEXT DEFAULT '',
  due_date TEXT DEFAULT '',
  deadline_source TEXT DEFAULT '',
  controllable INTEGER NOT NULL DEFAULT 1,
  source_path_id INTEGER NOT NULL,
  source_path_node_id INTEGER,
  linked_task_intent TEXT DEFAULT '',
  status TEXT NOT NULL DEFAULT 'todo',
  done_at DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orchestration_reviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  orchestration_id INTEGER NOT NULL,
  week_index INTEGER NOT NULL,
  done_count INTEGER DEFAULT 0,
  total_count INTEGER DEFAULT 0,
  expired_count INTEGER DEFAULT 0,
  replanned INTEGER DEFAULT 0,
  note TEXT DEFAULT '',
  reviewed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_paths_intent ON paths(orchestration_intent);
CREATE INDEX IF NOT EXISTS idx_pnodes_path ON path_nodes(path_id, node_index);
CREATE INDEX IF NOT EXISTS idx_orch_user ON orchestrations(user_id, status);
CREATE INDEX IF NOT EXISTS idx_oitems_orch ON orchestration_items(orchestration_id, week_index);
CREATE INDEX IF NOT EXISTS idx_orev_orch ON orchestration_reviews(orchestration_id);

CREATE INDEX IF NOT EXISTS idx_exec_user ON executions(user_id);
CREATE INDEX IF NOT EXISTS idx_exec_skill ON executions(skill_version_id);
CREATE INDEX IF NOT EXISTS idx_steps_exec ON execution_steps(execution_id, step_index);
CREATE INDEX IF NOT EXISTS idx_dec_exec ON decisions(experience_exec_id);
CREATE INDEX IF NOT EXISTS idx_dec_skill ON decisions(skill_id);
CREATE INDEX IF NOT EXISTS idx_ver_skill ON skill_versions(skill_id);
CREATE INDEX IF NOT EXISTS idx_evals_ver ON skill_evals(version_id);
CREATE INDEX IF NOT EXISTS idx_runs_ver ON eval_runs(version_id);
CREATE INDEX IF NOT EXISTS idx_fb_skill ON exec_feedbacks(skill_id);
CREATE INDEX IF NOT EXISTS idx_cand_skill ON version_candidates(skill_id, status);
CREATE INDEX IF NOT EXISTS idx_corpus_intent ON description_corpus(task_intent);
`
	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("init growth schema failed: %v", err)
	}

	// skills 表增量补列（老库兼容；列已存在时忽略）
	growthMigrations := []string{
		"ALTER TABLE skills ADD COLUMN status TEXT DEFAULT 'published'",
		"ALTER TABLE skills ADD COLUMN task_intent TEXT DEFAULT ''",
		"ALTER TABLE skills ADD COLUMN origin TEXT DEFAULT 'route_two_import'",
		"ALTER TABLE skills ADD COLUMN maintainer_id INTEGER",
		"ALTER TABLE skills ADD COLUMN current_version_id INTEGER",
		"ALTER TABLE skills ADD COLUMN quality_score REAL DEFAULT 0",
		// 个人成长主页的可见性开关（JSON）。默认空 = 全部不公开。
		"ALTER TABLE users ADD COLUMN profile_visibility TEXT DEFAULT ''",
		// v1.2：证据来源类型。artifact_upload（轨迹补录）的蒸馏度封顶 0.85。
		"ALTER TABLE skill_versions ADD COLUMN proof_type TEXT DEFAULT 'platform_trace'",
	}
	for _, m := range growthMigrations {
		if _, err := db.Exec(m); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				log.Printf("growth migrate warning: %v", err)
			}
		}
	}
	// exec_closings 表增量补列（verdict 反例说明，老库兼容）
	checkColumn("exec_closings", "verdict_reason", "ALTER TABLE exec_closings ADD COLUMN verdict_reason TEXT DEFAULT ''")

	seedCorpus()
	seedPaths()
	log.Println("growth schema initialized")
}

// seedPaths 预置一条保研 Path 作为编排态的来源。
//
// 这里必须诚实：它的 provenance 是 retrospective —— 学长回忆整理的，不是平台内观测到的。
// Path 跨越数月，冷启动期不可能有 observed 数据，这是编排态无法回避的矛盾（PRD v1.2 第 1 条）。
// 因此 retrospective 只给节点顺序，不给耗时分布与卡点统计，界面上必须标注来源。
func seedPaths() {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM paths`).Scan(&n); err != nil || n > 0 {
		return
	}
	branch := map[string]interface{}{
		"walked": 12,
		"branches": []map[string]interface{}{
			{"label": "进入夏令营", "count": 7},
			{"label": "转向考研", "count": 3, "note": "多在第 5 周确认绩点排名之后"},
			{"label": "中途停下", "count": 2},
		},
		"note": "以上为口述统计，来自 12 位学长的回忆整理",
	}
	res, err := db.Exec(`INSERT INTO paths (goal_label, orchestration_intent, provenance, walked_count, branch_summary)
		VALUES (?, 'postgrad_recommend', ?, 12, ?)`,
		"保研到本校或同层次院校", ProvenanceRetrospective, jsonOrEmpty(branch))
	if err != nil {
		log.Printf("seed path failed: %v", err)
		return
	}
	pathID, _ := res.LastInsertId()

	nodes := []struct {
		Label        string
		Intent       string
		WeekOffset   int
		Controllable bool
	}{
		{"盘清手上能拿出来的东西：课程排名、科研经历、竞赛、语言成绩", "", 1, true},
		{"把三段经历各写成一句可验证的结果", IntentResumeRewrite, 1, true},
		{"确定目标院校与方向清单，分成冲稳保三档", "", 2, true},
		{"写第一封联系导师的邮件并发出", IntentResourceDep, 3, true},
		{"准备夏令营申请材料并投递", "", 3, true},
		{"绩点排名结果公布", "", 5, false},
		{"根据排名重排目标清单", "", 5, true},
		{"准备面试：自我介绍与科研经历问答", IntentMockInterview, 6, true},
		{"夏令营面试", "", 7, false},
		{"面试复盘，为下一轮调整", IntentInterviewReview, 8, true},
	}
	for i, nd := range nodes {
		ctrl := 1
		if !nd.Controllable {
			ctrl = 0
		}
		db.Exec(`INSERT INTO path_nodes (path_id, node_index, label, task_intent, week_offset, controllable)
			VALUES (?, ?, ?, ?, ?, ?)`,
			pathID, i+1, nd.Label, nd.Intent, nd.WeekOffset, ctrl)
	}
	log.Printf("seeded retrospective path %d with %d nodes", pathID, len(nodes))
}

// seedCorpus 预置一批真实用户表达，供可发现性测试使用。
// 这些原话是「description 必须用用户真实表达」的语料来源，不是文案。
func seedCorpus() {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM description_corpus`).Scan(&n); err != nil || n > 0 {
		return
	}
	seeds := []struct {
		Utterance string
		Intent    string
	}{
		{"我的选题被导师退了两次，说范围太大", IntentThesisTopic},
		{"开题报告改到第四版还是被打回来，不知道问题在哪", IntentThesisTopic},
		{"想研究大模型但不知道从哪切进去", IntentThesisTopic},
		{"选题一句话说不清楚，导师每次都问我到底要解决什么", IntentThesisTopic},
		{"我的研究问题是不是太宽了，手上数据根本不够", IntentThesisTopic},
		{"选题查了发现别人做过了，要不要换", IntentThesisTopic},
		{"导师让我把题目收窄，可我不知道砍哪一块", IntentThesisTopic},
		{"开题下周就要交，现在只有一个方向没有问题", IntentThesisTopic},
		{"论文选题怎么定才不会中途做不下去", IntentThesisTopic},
		{"我想做的题目太大了，怎么缩小范围", IntentThesisTopic},
		{"科研经历怎么写进产品岗的简历", IntentResumeRewrite},
		{"我只有实验室经历，投产研岗简历怎么写", IntentResumeRewrite},
		{"简历上全是论文和实验，HR 说看不懂", IntentResumeRewrite},
		{"怎么把课题经历翻译成产品语言", IntentResumeRewrite},
		{"我的项目没有数据结果，简历怎么量化", IntentResumeRewrite},
		{"投产品经理岗，科研背景是劣势吗，简历怎么改", IntentResumeRewrite},
		{"把科研经历改成产研岗位简历", IntentResumeRewrite},
		{"实验室做的东西怎么写成业务成果", IntentResumeRewrite},
		{"简历改了很多版还是没面试邀约", IntentResumeRewrite},
		{"科研转产研，简历第一段该写什么", IntentResumeRewrite},
	}
	for _, s := range seeds {
		db.Exec(`INSERT INTO description_corpus (utterance, source, task_intent) VALUES (?, 'ops_seed', ?)`,
			s.Utterance, s.Intent)
	}
	log.Printf("seeded %d description corpus utterances", len(seeds))
}

// ---------- 小工具 ----------

// jsonOrEmpty 把任意值序列化为字符串，失败返回空 JSON 对象
func jsonOrEmpty(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// rawOrDefault 空字符串时返回给定默认 JSON，避免前端拿到 null
func rawOrDefault(s, def string) json.RawMessage {
	if strings.TrimSpace(s) == "" {
		return json.RawMessage(def)
	}
	if !json.Valid([]byte(s)) {
		return json.RawMessage(def)
	}
	return json.RawMessage(s)
}

// nullTime 读取可空时间列
func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// clamp01 把分值限制在 [0,1]
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
