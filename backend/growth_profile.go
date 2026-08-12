// F13 个人成长主页与成长身份（PRD 第 6 章 F13）
//
// 核心设计：成长路径不是用户自填的标签，而是从真实执行里派生出来的。
// 每一个节点都对应一次 execution，每一条判断都对应一次真实岔路口。
// 硬性约束：不展示粉丝数、不展示成长分数、不展示与他人的名次比较；默认全部不公开。
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 成长状态四阶（PRD growth_state 枚举）：学过 → 做过 → 做成过 → 教会过
const (
	GrowthLearned   = "learned"
	GrowthDid       = "did"
	GrowthSucceeded = "succeeded"
	GrowthTaught    = "taught"
)

var growthStateLabels = map[string]string{
	GrowthLearned:   "学过",
	GrowthDid:       "做过",
	GrowthSucceeded: "做成过",
	GrowthTaught:    "教会过",
}

// growthStateRank 用于取某个方向上的最高阶
var growthStateRank = map[string]int{
	"":              0,
	GrowthLearned:   1,
	GrowthDid:       2,
	GrowthSucceeded: 3,
	GrowthTaught:    4,
}

// 可见性开关的键。默认全部关闭——个人成长数据默认私有，逐项开启。
var visibilityKeys = []string{"timeline", "states", "assets", "influence", "failures"}

// pathNode 成长路径上的一个节点，一对一对应一次真实执行
type pathNode struct {
	ExecutionID   int64      `json:"execution_id"`
	TaskIntent    string     `json:"task_intent"`
	TaskLabel     string     `json:"task_label"`
	TaskTitle     string     `json:"task_title"`
	Status        string     `json:"status"`
	StatusLabel   string     `json:"status_label"`
	Exported      bool       `json:"exported"`
	StepCount     int        `json:"step_count"`
	DecisionCount int        `json:"decision_count"`
	SkillID       *int64     `json:"skill_id,omitempty"`
	SkillName     string     `json:"skill_name,omitempty"`
	SkillStatus   string     `json:"skill_status,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
}

// getMyGrowthProfile GET /api/growth/profile/me
func getMyGrowthProfile(c *gin.Context) {
	uid := c.GetInt64("userID")
	buildGrowthProfile(c, uid, true)
}

// getUserGrowthProfile GET /api/growth/profile/:id
// 看别人的成长身份，按其可见性设置过滤
func getUserGrowthProfile(c *gin.Context) {
	target, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	viewer := c.GetInt64("userID")
	buildGrowthProfile(c, target, viewer == target)
}

// buildGrowthProfile 组装成长身份。isSelf=false 时按可见性开关裁剪。
func buildGrowthProfile(c *gin.Context, uid int64, isSelf bool) {
	user, err := getUserByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	vis := loadVisibility(uid)

	out := gin.H{
		"user_id":  uid,
		"username": user.Username,
		"is_self":  isSelf,
		// 明确告诉前端这里没有分数也没有排名，避免又去找 rating
		"no_score_no_rank": true,
		"principle":        "这条路径不是填出来的，是你真实做过的事连成的。每个节点都能点回那次执行。",
	}

	// ---------- 当前位置 ----------
	// 不是标签堆叠，而是"最近在做什么 + 学业阶段"
	position := gin.H{"grade": user.Grade, "major": user.Major, "school": user.School}
	var lastIntent, lastTitle string
	var lastAt sql.NullTime
	if err := db.QueryRow(`SELECT task_intent, task_title, started_at FROM executions
		WHERE user_id = ? ORDER BY id DESC LIMIT 1`, uid).Scan(&lastIntent, &lastTitle, &lastAt); err == nil {
		position["recent_task"] = AllowedIntents[lastIntent]
		position["recent_title"] = lastTitle
		position["recent_at"] = nullTime(lastAt)
	} else {
		position["recent_task"] = ""
		position["empty_hint"] = "还没有走过任何一段路。去「我要成长」说一句你现在卡在哪，第一个节点就有了。"
	}
	out["current_position"] = position

	// ---------- 成长路线（时间线） ----------
	if isSelf || vis["timeline"] {
		out["timeline"] = loadTimeline(uid)
	} else {
		out["timeline_hidden"] = true
	}

	// ---------- 成长状态四阶 ----------
	if isSelf || vis["states"] {
		out["growth_states"] = computeGrowthStates(uid)
		out["state_labels"] = growthStateLabels
	} else {
		out["states_hidden"] = true
	}

	// ---------- 能力资产 ----------
	if isSelf || vis["assets"] {
		out["assets"] = loadCapabilityAssets(uid, isSelf)
	} else {
		out["assets_hidden"] = true
	}

	// ---------- 影响力 ----------
	if isSelf || vis["influence"] {
		out["influence"] = loadInfluence(uid)
	} else {
		out["influence_hidden"] = true
	}

	// ---------- 失败与复盘 ----------
	if isSelf || vis["failures"] {
		out["setbacks"] = loadSetbacks(uid)
	} else {
		out["setbacks_hidden"] = true
	}

	if isSelf {
		out["visibility"] = vis
		out["visibility_keys"] = visibilityKeys
	}
	c.JSON(http.StatusOK, out)
}

// loadTimeline 成长路线：真实执行按时间排列，每个节点带它产出了什么
func loadTimeline(uid int64) []pathNode {
	// 单连接模式（SetMaxOpenConns(1)）下，唯一的连接被 db.Query 返回的 rows 占用后，
	// 在 rows 关闭前再发起任何 db 查询都会永久死锁——不管在循环体内还是循环外。
	// 所以所有辅助查询必须先于主查询完成，结果存内存 map，主查询的循环体里只查 map。
	stepCounts := map[int64]int{}
	if scRows, err := db.Query(`SELECT execution_id, COUNT(*) FROM execution_steps GROUP BY execution_id`); err == nil {
		for scRows.Next() {
			var eid, n int64
			if scRows.Scan(&eid, &n) == nil {
				stepCounts[eid] = int(n)
			}
		}
		scRows.Close()
	}
	decisionCounts := map[int64]int{}
	if dcRows, err := db.Query(`SELECT execution_id, COUNT(*) FROM execution_steps
		WHERE step_type = ? AND user_choice != '' GROUP BY execution_id`, StepUserDecision); err == nil {
		for dcRows.Next() {
			var eid, n int64
			if dcRows.Scan(&eid, &n) == nil {
				decisionCounts[eid] = int(n)
			}
		}
		dcRows.Close()
	}
	// 执行固化成 Skill 的映射：source_execution_id -> 产出（ORDER BY v.id，最后一次覆盖即最新）
	type skillOut struct {
		id     int64
		name   string
		status string
	}
	skillByExec := map[int64]skillOut{}
	if skRows, err := db.Query(`SELECT v.source_execution_id, s.id, s.name, COALESCE(s.status,'')
		FROM skill_versions v JOIN skills s ON s.id = v.skill_id
		WHERE v.source_execution_id IS NOT NULL ORDER BY v.id`); err == nil {
		for skRows.Next() {
			var eid int64
			var so skillOut
			if skRows.Scan(&eid, &so.id, &so.name, &so.status) == nil {
				skillByExec[eid] = so
			}
		}
		skRows.Close()
	}

	// 所有辅助查询已完成，现在才打开主查询（占用唯一连接直到循环结束）。
	rows, err := db.Query(`SELECT e.id, e.task_intent, e.task_title, e.status,
		COALESCE(e.completion_signal,''), e.started_at, e.ended_at
		FROM executions e WHERE e.user_id = ? ORDER BY e.id`, uid)
	if err != nil {
		return []pathNode{}
	}
	defer rows.Close()

	nodes := []pathNode{}
	for rows.Next() {
		var n pathNode
		var signal string
		var endedAt sql.NullTime
		if err := rows.Scan(&n.ExecutionID, &n.TaskIntent, &n.TaskTitle, &n.Status,
			&signal, &n.StartedAt, &endedAt); err != nil {
			continue
		}
		n.TaskLabel = AllowedIntents[n.TaskIntent]
		n.StatusLabel = execStatusLabel(n.Status)
		n.EndedAt = nullTime(endedAt)

		var sig struct {
			Exported bool `json:"exported"`
		}
		if strings.TrimSpace(signal) != "" {
			json.Unmarshal([]byte(signal), &sig)
		}
		n.Exported = sig.Exported

		n.StepCount = stepCounts[n.ExecutionID]
		n.DecisionCount = decisionCounts[n.ExecutionID]

		// 这次执行有没有固化成 Skill —— 这是路径节点最有价值的产出
		if so, ok := skillByExec[n.ExecutionID]; ok {
			sid := so.id
			n.SkillID = &sid
			n.SkillName = so.name
			n.SkillStatus = so.status
		}
		nodes = append(nodes, n)
	}
	return nodes
}

func execStatusLabel(s string) string {
	switch s {
	case ExecRunning:
		return "进行中"
	case ExecCompleted:
		return "已完成"
	case ExecAbandoned:
		return "中途停下"
	case ExecHandedOff:
		return "交回给人"
	case ExecFailed:
		return "出错"
	}
	return s
}

// computeGrowthStates 按任务方向算成长状态四阶。
// 判定全部来自真实信号，没有任何自评：
//   learned   调用过该方向的 Skill
//   did       有已完成的执行
//   succeeded 完成且把产物用出去了（exported）
//   taught    自己产出的该方向 Skill 已发布，且被别人成功调用过
func computeGrowthStates(uid int64) []gin.H {
	state := map[string]string{}
	detail := map[string]gin.H{}

	bump := func(intent, s string) {
		if growthStateRank[s] > growthStateRank[state[intent]] {
			state[intent] = s
		}
	}

	// learned / did / succeeded：看自己的执行
	rows, err := db.Query(`SELECT task_intent, status, COALESCE(completion_signal,''),
		COALESCE(skill_version_id, 0) FROM executions WHERE user_id = ?`, uid)
	if err == nil {
		for rows.Next() {
			var intent, status, signal string
			var svID int64
			if rows.Scan(&intent, &status, &signal, &svID) != nil {
				continue
			}
			if _, ok := AllowedIntents[intent]; !ok {
				continue
			}
			if svID > 0 {
				bump(intent, GrowthLearned) // 用过别人的能力
			}
			if status == ExecCompleted {
				bump(intent, GrowthDid)
				var sig struct {
					Exported bool `json:"exported"`
				}
				json.Unmarshal([]byte(signal), &sig)
				if sig.Exported {
					bump(intent, GrowthSucceeded)
				}
			}
			d := detail[intent]
			if d == nil {
				d = gin.H{"executions": 0, "completed": 0}
			}
			d["executions"] = d["executions"].(int) + 1
			if status == ExecCompleted {
				d["completed"] = d["completed"].(int) + 1
			}
			detail[intent] = d
		}
		rows.Close()
	}

	// taught：自己发布的 Skill 被别人成功调用过
	r2, err := db.Query(`SELECT s.task_intent, COUNT(DISTINCT e.user_id)
		FROM skills s
		JOIN skill_versions v ON v.skill_id = s.id
		JOIN executions e ON e.skill_version_id = v.id AND e.user_id != ? AND e.status = ?
		WHERE COALESCE(s.maintainer_id, s.owner_id) = ? AND COALESCE(s.status,'') = ?
		GROUP BY s.task_intent`, uid, ExecCompleted, uid, SkillStatusPublished)
	if err == nil {
		for r2.Next() {
			var intent string
			var helped int
			if r2.Scan(&intent, &helped) != nil {
				continue
			}
			if helped > 0 {
				bump(intent, GrowthTaught)
				d := detail[intent]
				if d == nil {
					d = gin.H{"executions": 0, "completed": 0}
				}
				d["helped_others"] = helped
				detail[intent] = d
			}
		}
		r2.Close()
	}

	out := []gin.H{}
	for intent, s := range state {
		out = append(out, gin.H{
			"task_intent": intent,
			"task_label":  AllowedIntents[intent],
			"state":       s,
			"state_label": growthStateLabels[s],
			"rank":        growthStateRank[s],
			"detail":      detail[intent],
		})
	}
	return out
}

// loadCapabilityAssets 能力资产：我产出的 Skill 及其来源经历
func loadCapabilityAssets(uid int64, isSelf bool) []gin.H {
	// 别人看时只展示已发布的；自己看时草稿与经验笔记也要看得到
	statusFilter := `AND COALESCE(s.status,'') = '` + SkillStatusPublished + `'`
	if isSelf {
		statusFilter = ""
	}
	// 单连接模式下唯一连接被主查询 rows 占用后不能再查库，所以先把计数预取到内存 map，
	// 主查询必须在所有辅助查询完成之后再打开。
	decisionCounts := map[int64]int{}
	if dr, err := db.Query(`SELECT skill_id, COUNT(*) FROM decisions
		WHERE invalidated_at IS NULL GROUP BY skill_id`); err == nil {
		for dr.Next() {
			var sid int64
			var n int
			if dr.Scan(&sid, &n) == nil {
				decisionCounts[sid] = n
			}
		}
		dr.Close()
	}
	callCounts := map[int64]int{}
	if cr, err := db.Query(`SELECT v.skill_id, COUNT(*) FROM executions e
		JOIN skill_versions v ON v.id = e.skill_version_id GROUP BY v.skill_id`); err == nil {
		for cr.Next() {
			var sid int64
			var n int
			if cr.Scan(&sid, &n) == nil {
				callCounts[sid] = n
			}
		}
		cr.Close()
	}

	rows, err := db.Query(`SELECT s.id, s.name, COALESCE(s.status,''), COALESCE(s.task_intent,''),
		COALESCE(v.version,''), COALESCE(v.distillation_score,0), COALESCE(sc.quality_score,0)
		FROM skills s
		LEFT JOIN skill_versions v ON v.id = s.current_version_id
		LEFT JOIN skill_scores sc ON sc.skill_id = s.id
		WHERE COALESCE(s.maintainer_id, s.owner_id) = ? `+statusFilter+`
		ORDER BY s.id DESC`, uid)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var id int64
		var name, status, intent, version string
		var distill, quality float64
		if rows.Scan(&id, &name, &status, &intent, &version, &distill, &quality) != nil {
			continue
		}
		decisionCount := decisionCounts[id]
		callCount := callCounts[id]

		out = append(out, gin.H{
			"skill_id":            id,
			"name":                name,
			"status":              status,
			"status_label":        skillStatusLabel(status),
			"task_label":          AllowedIntents[intent],
			"version":             version,
			"distillation_score":  distill,
			"traceable_decisions": decisionCount,
			"call_count":          callCount,
		})
	}
	return out
}

func skillStatusLabel(s string) string {
	switch s {
	case SkillStatusDraft:
		return "草稿"
	case SkillStatusInsightOnly:
		return "经验笔记"
	case SkillStatusGated:
		return "待过门禁"
	case SkillStatusPublished:
		return "已发布"
	case SkillStatusNeedsReview:
		return "待复核"
	case SkillStatusDeprecated:
		return "已弃用"
	case SkillStatusArchived:
		return "仅存档"
	}
	return s
}

// loadInfluence 影响力：奖励贡献而不是粉丝。
// 这里没有"粉丝数"，只有"多少人因为你的方法真的把事做成了"。
func loadInfluence(uid int64) gin.H {
	var helpedPeople, effectiveCompletions int
	db.QueryRow(`SELECT COUNT(DISTINCT e.user_id) FROM executions e
		JOIN skill_versions v ON v.id = e.skill_version_id
		JOIN skills s ON s.id = v.skill_id
		WHERE COALESCE(s.maintainer_id, s.owner_id) = ? AND e.user_id != ?`, uid, uid).
		Scan(&helpedPeople)
	db.QueryRow(`SELECT COUNT(*) FROM executions e
		JOIN skill_versions v ON v.id = e.skill_version_id
		JOIN skills s ON s.id = v.skill_id
		WHERE COALESCE(s.maintainer_id, s.owner_id) = ? AND e.user_id != ?
		  AND e.status = ? AND e.completion_signal LIKE '%"exported":true%'`,
		uid, uid, ExecCompleted).Scan(&effectiveCompletions)

	// 有效贡献：我提的反馈被别人采纳进新版本
	var adoptedFeedback int
	db.QueryRow(`SELECT COUNT(*) FROM exec_feedbacks WHERE user_id = ? AND adopted = 1`, uid).
		Scan(&adoptedFeedback)

	// 后继者：用过我的方法，并且自己也做成过的人。
	// path_follows 表尚未建（Growth Graph 是下一阶段），这里用执行数据做等价近似。
	var successors int
	db.QueryRow(`SELECT COUNT(DISTINCT e.user_id) FROM executions e
		JOIN skill_versions v ON v.id = e.skill_version_id
		JOIN skills s ON s.id = v.skill_id
		WHERE COALESCE(s.maintainer_id, s.owner_id) = ? AND e.user_id != ?
		  AND e.status = ? AND e.completion_signal LIKE '%"exported":true%'`,
		uid, uid, ExecCompleted).Scan(&successors)

	// 已接手维护：别人创建、我在维护的
	var maintaining int
	db.QueryRow(`SELECT COUNT(*) FROM skills WHERE maintainer_id = ? AND owner_id IS NOT NULL
		AND owner_id != ?`, uid, uid).Scan(&maintaining)

	// 判断被别人的 Skill 复用（被组合的近似）
	var decisionsContributed int
	db.QueryRow(`SELECT COUNT(*) FROM decisions d
		JOIN executions e ON e.id = d.experience_exec_id
		WHERE e.user_id = ? AND d.invalidated_at IS NULL`, uid).Scan(&decisionsContributed)

	return gin.H{
		"helped_people":         helpedPeople,
		"effective_completions": effectiveCompletions,
		"successors":            successors,
		"adopted_feedback":      adoptedFeedback,
		"maintaining_others":    maintaining,
		"decisions_contributed": decisionsContributed,
		"note":                  "这里没有粉丝数。后继者的意思是：用了你的方法、并且自己真的把事做成了的人。",
		"successor_basis":       "当前用执行数据近似；成长路径图谱上线后会改用真实跟走关系。",
	}
}

// loadSetbacks 失败与复盘：中途停下的执行 + 存成经验笔记的缺口。
// 这一块是刻意保留的——它让成长身份可信，而不是一张成就墙。
func loadSetbacks(uid int64) gin.H {
	stops := []gin.H{}
	rows, err := db.Query(`SELECT id, task_intent, task_title, COALESCE(abandoned_at_step,0), started_at
		FROM executions WHERE user_id = ? AND status = ? ORDER BY id DESC LIMIT 10`, uid, ExecAbandoned)
	if err == nil {
		for rows.Next() {
			var id int64
			var intent, title string
			var step int
			var at time.Time
			if rows.Scan(&id, &intent, &title, &step, &at) == nil {
				stops = append(stops, gin.H{
					"execution_id": id, "task_label": AllowedIntents[intent],
					"task_title": title, "stopped_at_step": step, "started_at": at,
				})
			}
		}
		rows.Close()
	}

	notes := []gin.H{}
	r2, err := db.Query(`SELECT id, claim, COALESCE(why,''), COALESCE(missing_for_skill,'[]'), created_at
		FROM insights WHERE user_id = ? ORDER BY id DESC LIMIT 10`, uid)
	if err == nil {
		for r2.Next() {
			var id int64
			var claim, why, missing string
			var at time.Time
			if r2.Scan(&id, &claim, &why, &missing, &at) == nil {
				notes = append(notes, gin.H{
					"insight_id": id, "claim": claim, "why": why,
					"still_missing": rawOrDefault(missing, "[]"), "created_at": at,
				})
			}
		}
		r2.Close()
	}

	return gin.H{
		"stopped":  stops,
		"insights": notes,
		"note":     "中途停下不是污点。知道什么时候该停，本身就是判断力的一部分。",
	}
}

// ---------- 可见性 ----------

// loadVisibility 读可见性开关，默认全部关闭
func loadVisibility(uid int64) map[string]bool {
	vis := map[string]bool{}
	for _, k := range visibilityKeys {
		vis[k] = false
	}
	var raw sql.NullString
	if err := db.QueryRow(`SELECT profile_visibility FROM users WHERE id = ?`, uid).Scan(&raw); err != nil {
		return vis
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return vis
	}
	var saved map[string]bool
	if json.Unmarshal([]byte(raw.String), &saved) != nil {
		return vis
	}
	for k := range vis {
		if v, ok := saved[k]; ok {
			vis[k] = v
		}
	}
	return vis
}

// updateVisibility PATCH /api/growth/profile/visibility
func updateVisibility(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body map[string]bool
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	vis := loadVisibility(uid)
	for _, k := range visibilityKeys {
		if v, ok := body[k]; ok {
			vis[k] = v
		}
	}
	if _, err := db.Exec(`UPDATE users SET profile_visibility = ? WHERE id = ?`, jsonOrEmpty(vis), uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"visibility": vis})
}
