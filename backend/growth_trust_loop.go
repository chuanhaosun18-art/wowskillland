// F10 Trust Card 与判断级溯源 + F12 反馈闭环与版本升级
// 硬约束一：Trust Card 全页不出现综合评分、星级、排行位次。
// 硬约束二：单条负反馈不触发版本变更，只进观察池。
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---------- F10 Trust Card ----------

// getTrustCard GET /api/growth/skills/:id/trust-card
func getTrustCard(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var name, status, origin, taskIntent string
	var ownerID, maintainerID, versionID sql.NullInt64
	var updatedAt string
	if err := db.QueryRow(`SELECT name, COALESCE(status,''), COALESCE(origin,''), COALESCE(task_intent,''),
		owner_id, maintainer_id, current_version_id, updated_at FROM skills WHERE id = ?`, skillID).
		Scan(&name, &status, &origin, &taskIntent, &ownerID, &maintainerID, &versionID, &updatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	out := gin.H{
		"skill_id":    skillID,
		"name":        name,
		"status":      status,
		"task_intent": taskIntent,
		"task_label":  AllowedIntents[taskIntent],
		// 明确告知这里不给分数，避免前端习惯性去找 rating 字段
		"no_composite_score": true,
		"score_policy":       "我们不给综合评分。一个总分没法告诉你它在什么情况下有效、在哪里会失败。",
	}

	var ver *SkillVersion
	if versionID.Valid && versionID.Int64 > 0 {
		ver, _ = loadSkillVersion(versionID.Int64)
	}

	// 分区一：它做什么
	if ver != nil {
		var criteria []string
		json.Unmarshal([]byte(ver.DoneCriteria), &criteria)
		out["what_it_does"] = gin.H{
			"goal":          ver.Goal,
			"description":   ver.Description,
			"done_criteria": criteria,
			"version":       ver.Version,
		}
	}

	// 分区二：流程（每个岔路口可下钻）
	decisions := loadDecisions(skillID)
	decByStep := map[int][]gin.H{}
	for _, d := range decisions {
		if d.InvalidatedAt != nil {
			continue
		}
		decByStep[d.SourceStepIndex] = append(decByStep[d.SourceStepIndex], gin.H{
			"decision_id": d.ID,
			"slot":        d.Slot,
			"slot_prompt": slotPrompt(d.Slot),
			"summary":     d.Judgment,
		})
	}
	steps := []gin.H{}
	if ver != nil {
		var wf []struct {
			Index int    `json:"index"`
			Title string `json:"title"`
			IO    string `json:"io"`
		}
		json.Unmarshal([]byte(ver.Workflow), &wf)
		for _, s := range wf {
			steps = append(steps, gin.H{"index": s.Index, "title": s.Title, "io": s.IO})
		}
	}
	// 判断按槽位分组给前端，界面上每一条都可点开
	slotGroups := []gin.H{}
	for _, sd := range DecisionSlots {
		items := []gin.H{}
		for _, d := range decisions {
			if d.Slot != sd.Slot || d.InvalidatedAt != nil {
				continue
			}
			items = append(items, gin.H{
				"decision_id":       d.ID,
				"trigger_signal":    d.TriggerSignal,
				"judgment":          d.Judgment,
				"scope":             d.Scope,
				"source_step_index": d.SourceStepIndex,
				"verified_by_count": d.VerifiedByCount,
			})
		}
		if len(items) > 0 {
			slotGroups = append(slotGroups, gin.H{"slot": sd.Slot, "prompt": sd.Prompt, "decisions": items})
		}
	}
	out["workflow"] = gin.H{"steps": steps, "decision_slots": slotGroups}

	// 分区三：证据
	evidence := gin.H{}
	evals := []gin.H{}
	if ver != nil {
		for _, t := range []string{EvalDiscoverability, EvalCompletion, EvalStability, EvalBoundaryStop} {
			var rate, threshold float64
			var passedInt int
			if err := db.QueryRow(`SELECT pass_rate, threshold, passed FROM eval_runs
				WHERE version_id = ? AND eval_type = ? ORDER BY id DESC LIMIT 1`, ver.ID, t).
				Scan(&rate, &threshold, &passedInt); err == nil {
				evals = append(evals, gin.H{"eval_type": t, "label": evalLabel(t),
					"pass_rate": rate, "threshold": threshold, "passed": passedInt == 1})
			}
		}
		evidence["distillation_score"] = ver.DistillationScore
	}
	evidence["evals"] = evals
	evidence["traceable_decisions"] = len(decisions)

	// 来源执行（脱敏：只给步数，不给材料）
	if ver != nil {
		var srcExec sql.NullInt64
		db.QueryRow(`SELECT source_execution_id FROM skill_versions WHERE id = ?`, ver.ID).Scan(&srcExec)
		// 补录来源必须明确标注，不能和平台内轨迹混为一谈（v1.2 硬约束 15）
		if ver.ProofType == ProofArtifactUpload {
			evidence["source"] = gin.H{
				"kind": ProofArtifactUpload,
				"note": "来源为补录，无执行轨迹。判断由创作者自述，蒸馏度上限 0.85。",
			}
		} else if srcExec.Valid && srcExec.Int64 > 0 {
			var stepCount int
			db.QueryRow(`SELECT COUNT(*) FROM execution_steps WHERE execution_id = ?`, srcExec.Int64).Scan(&stepCount)
			evidence["source"] = gin.H{
				"kind":       ProofPlatformTrace,
				"step_count": stepCount,
				"note":       "来自一次平台内真实执行，原始材料不公开",
			}
		} else {
			evidence["source"] = gin.H{"kind": ProofSelfReport, "note": "尚无平台内执行轨迹作为根"}
		}
	}
	out["evidence"] = evidence

	// 分区四：边界
	if ver != nil {
		var b struct {
			NotApplicable  []string `json:"not_applicable"`
			HandoffTrigger []string `json:"handoff_trigger"`
			FallbackPath   string   `json:"fallback_path"`
		}
		json.Unmarshal([]byte(ver.Boundary), &b)
		gotchas := []gin.H{}
		rows, err := db.Query(`SELECT file_path FROM skill_files WHERE skill_id = ? AND file_path LIKE '%/gotchas/%'`, skillID)
		if err == nil {
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					gotchas = append(gotchas, gin.H{"path": p})
				}
			}
			rows.Close()
		}
		out["boundary"] = gin.H{
			"not_applicable":  b.NotApplicable,
			"handoff_trigger": b.HandoffTrigger,
			"fallback_path":   b.FallbackPath,
			"gotchas":         gotchas,
		}
	}

	// 分区五：授权与安全（委托五要件里最容易被忽略的一半）
	if ver != nil {
		var ct struct {
			Input       string   `json:"input"`
			Output      string   `json:"output"`
			Permissions []string `json:"permissions"`
			SideEffects []string `json:"side_effects"`
		}
		json.Unmarshal([]byte(ver.Contract), &ct)
		sensitive := []string{}
		for _, p := range ct.Permissions {
			if isIrreversiblePermission(p) {
				sensitive = append(sensitive, p)
			}
		}
		out["authorization"] = gin.H{
			"input":               ct.Input,
			"output":              ct.Output,
			"permissions":         ct.Permissions,
			"side_effects":        ct.SideEffects,
			"sensitive_ops":       sensitive,
			"needs_confirmation":  len(sensitive) > 0,
			"least_privilege_note": "运行时只授予这里声明的权限；声明外的调用会被直接拒绝并记录。",
		}
	}

	// 分区六：运行（样本不足必须标注，且不给两位小数）
	out["runtime"] = runtimeStats(skillID, versionID)

	// 分区七：维护
	maint := gin.H{"updated_at": updatedAt}
	if ownerID.Valid {
		if u, err := getUserByID(ownerID.Int64); err == nil {
			maint["creator"] = u.Username
			// 创作者可点进成长身份：前路关系的入口——你走过我正在走的路
			maint["creator_user_id"] = ownerID.Int64
		}
	}
	if maintainerID.Valid {
		if u, err := getUserByID(maintainerID.Int64); err == nil {
			maint["maintainer"] = u.Username
			maint["maintainer_user_id"] = maintainerID.Int64
		}
		if ownerID.Valid && maintainerID.Int64 != ownerID.Int64 {
			maint["handed_over"] = true
		}
	}
	var sc SkillScore
	var admFail string
	var admPassed, sampleOK, eligible int
	if err := db.QueryRow(`SELECT admission_passed, admission_failures, offline_score, online_score,
		maintenance_score, sample_sufficient, is_candidate_eligible FROM skill_scores WHERE skill_id = ?`, skillID).
		Scan(&admPassed, &admFail, &sc.OfflineScore, &sc.OnlineScore, &sc.MaintenanceScore,
			&sampleOK, &eligible); err == nil {
		maint["admission_passed"] = admPassed == 1
		maint["admission_failures"] = json.RawMessage(admFail)
		maint["maintenance_health"] = sc.MaintenanceScore
		maint["in_candidate_pool"] = eligible == 1
	}
	if ver != nil {
		maint["changelog"] = ver.Changelog
	}
	out["maintenance"] = maint

	// 版本历史
	versions := []gin.H{}
	rows, err := db.Query(`SELECT version, COALESCE(changelog,''), published_at FROM skill_versions
		WHERE skill_id = ? ORDER BY id`, skillID)
	if err == nil {
		for rows.Next() {
			var v, cl string
			var pub sql.NullTime
			if rows.Scan(&v, &cl, &pub) == nil {
				versions = append(versions, gin.H{"version": v, "changelog": cl, "published_at": nullTime(pub)})
			}
		}
		rows.Close()
	}
	out["versions"] = versions

	c.JSON(http.StatusOK, out)
}

// runtimeStats 行为信号聚合。样本不足时标注，不显示精确比率。
func runtimeStats(skillID int64, versionID sql.NullInt64) gin.H {
	var total int
	db.QueryRow(`SELECT COUNT(*) FROM executions WHERE skill_version_id IN
		(SELECT id FROM skill_versions WHERE skill_id = ?)`, skillID).Scan(&total)

	out := gin.H{"sample_size": total, "sample_sufficient": total >= OnlineEvidenceMinCall}
	if total < OnlineEvidenceMinCall {
		out["note"] = "样本不足，以下数字仅供参考，不作为判断依据"
	}
	if total == 0 {
		return out
	}

	var abandoned, exported int
	var avgCorrection, avgLatency sql.NullFloat64
	db.QueryRow(`SELECT
		SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
		SUM(CASE WHEN completion_signal LIKE '%"exported":true%' THEN 1 ELSE 0 END),
		AVG(correction_ratio) FROM executions
		WHERE skill_version_id IN (SELECT id FROM skill_versions WHERE skill_id = ?)`,
		ExecAbandoned, skillID).Scan(&abandoned, &exported, &avgCorrection)
	db.QueryRow(`SELECT AVG(s.latency_ms) FROM execution_steps s
		JOIN executions e ON e.id = s.execution_id
		WHERE e.skill_version_id IN (SELECT id FROM skill_versions WHERE skill_id = ?)`, skillID).Scan(&avgLatency)

	// 用行为信号替代成果信号：是否用出去、是否放弃、是否人工重做
	round := func(v float64) float64 {
		if total < OnlineEvidenceMinCall {
			// 样本不足时只给粗粒度
			return float64(int(v*10)) / 10
		}
		return float64(int(v*100)) / 100
	}
	out["adoption_rate"] = round(float64(exported) / float64(total))
	out["abandon_rate"] = round(float64(abandoned) / float64(total))
	if avgCorrection.Valid {
		out["correction_rate"] = round(avgCorrection.Float64)
	}
	if avgLatency.Valid {
		out["avg_step_latency_ms"] = int(avgLatency.Float64)
	}

	// 失败类型分布
	dist := []gin.H{}
	rows, err := db.Query(`SELECT issue_type, COUNT(*) FROM exec_feedbacks WHERE skill_id = ?
		GROUP BY issue_type ORDER BY COUNT(*) DESC`, skillID)
	if err == nil {
		for rows.Next() {
			var t string
			var n int
			if rows.Scan(&t, &n) == nil {
				dist = append(dist, gin.H{"issue_type": t, "count": n})
			}
		}
		rows.Close()
	}
	out["failure_types"] = dist
	return out
}

// getDecisionTrace GET /api/growth/decisions/:id/trace
// 判断级溯源：只给判断与场景摘要，不给原始材料。
func getDecisionTrace(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var d Decision
	var sid sql.NullInt64
	var inval sql.NullTime
	if err := db.QueryRow(`SELECT id, experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
		counter_example, source_step_index, verified_by_count, invalidated_at, created_at
		FROM decisions WHERE id = ?`, did).
		Scan(&d.ID, &d.ExperienceExecID, &sid, &d.Slot, &d.TriggerSignal, &d.Judgment, &d.Scope,
			&d.CounterExample, &d.SourceStepIndex, &d.VerifiedByCount, &inval, &d.CreatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "decision not found"})
		return
	}
	d.InvalidatedAt = nullTime(inval)

	// 来源那一步的脱敏摘要
	source := gin.H{"step_index": d.SourceStepIndex}
	var stepType, title, output string
	if err := db.QueryRow(`SELECT step_type, title, output FROM execution_steps
		WHERE execution_id = ? AND step_index = ?`, d.ExperienceExecID, d.SourceStepIndex).
		Scan(&stepType, &title, &output); err == nil {
		source["step_type"] = stepType
		source["title"] = title
		// 脱敏：只给一句摘要，不给原文
		source["situation_summary"] = truncate(output, 120)
	}
	var taskIntent string
	db.QueryRow(`SELECT task_intent FROM executions WHERE id = ?`, d.ExperienceExecID).Scan(&taskIntent)
	source["task_label"] = AllowedIntents[taskIntent]

	c.JSON(http.StatusOK, gin.H{
		"decision": gin.H{
			"id":                d.ID,
			"slot":              d.Slot,
			"slot_prompt":       slotPrompt(d.Slot),
			"trigger_signal":    d.TriggerSignal,
			"judgment":          d.Judgment,
			"scope":             d.Scope,
			"counter_example":   d.CounterExample,
			"verified_by_count": d.VerifiedByCount,
			"invalidated":       d.InvalidatedAt != nil,
		},
		"source":       source,
		"privacy_note": "出于隐私要求，这里只展示判断与场景摘要，不展示来源者的原始材料。",
	})
}

// ---------- F12 反馈闭环 ----------

// submitExecFeedback POST /api/growth/executions/:id/feedback
func submitExecFeedback(c *gin.Context) {
	uid := c.GetInt64("userID")
	execID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		IssueType       string `json:"issue_type"`
		Description     string `json:"description"`
		SuggestedChange string `json:"suggested_change"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !isValidIssueType(body.IssueType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法的反馈类型"})
		return
	}

	var owner int64
	var versionID sql.NullInt64
	if err := db.QueryRow(`SELECT user_id, skill_version_id FROM executions WHERE id = ?`, execID).
		Scan(&owner, &versionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if owner != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "只能对自己的执行提交反馈"})
		return
	}
	if !versionID.Valid {
		c.JSON(http.StatusConflict, gin.H{"error": "这次执行没有使用任何 Skill，无法反馈"})
		return
	}
	var skillID int64
	db.QueryRow(`SELECT skill_id FROM skill_versions WHERE id = ?`, versionID.Int64).Scan(&skillID)

	db.Exec(`INSERT INTO exec_feedbacks (execution_id, skill_id, user_id, issue_type, description, suggested_change)
		VALUES (?, ?, ?, ?, ?, ?)`,
		execID, skillID, uid, body.IssueType, body.Description, body.SuggestedChange)

	created, rule, evidenceCount := checkVersionTriggers(skillID)
	recomputeSkillScore(skillID)

	resp := gin.H{"message": "已收到"}
	if created {
		resp["version_candidate_created"] = true
		resp["trigger_rule"] = rule
		resp["evidence_count"] = evidenceCount
		resp["note"] = "同类问题已重复出现，已经生成版本候选并通知维护者"
	} else {
		// 这句话是刻意的：单条反馈不动版本，避免版本被个例带偏
		resp["note"] = "先进观察池。单独一条反馈不会改动版本——要等同类问题重复出现，或者被另一次独立执行验证。"
	}
	c.JSON(http.StatusOK, resp)
}

func isValidIssueType(t string) bool {
	switch t {
	case "wrong_output", "missing_boundary", "unstable", "dependency_broken", "not_applicable_to_me", "other":
		return true
	}
	return false
}

// checkVersionTriggers 版本候选触发规则。
// repeated_failure：同一 issue_type 在 14 天内 ≥3 次，且来自 ≥3 个不同用户。
func checkVersionTriggers(skillID int64) (bool, string, int) {
	// 单连接模式下，先取冷启动门槛（rows 打开前查库是安全的）
	var callCount int
	db.QueryRow(`SELECT COUNT(*) FROM executions WHERE skill_version_id IN
		(SELECT id FROM skill_versions WHERE skill_id = ?)`, skillID).Scan(&callCount)
	needCount, needUsers := 3, 3
	if callCount < ColdStartCallCount {
		needCount, needUsers = 2, 2
	}

	// 聚合数据先全部读进内存，关闭 rows 之后才能继续查库/写库
	type fbAgg struct {
		issueType string
		count     int
		users     int
	}
	aggs := []fbAgg{}
	rows, err := db.Query(`SELECT issue_type, COUNT(*) AS c, COUNT(DISTINCT user_id) AS u
		FROM exec_feedbacks
		WHERE skill_id = ? AND adopted = 0
		  AND created_at >= datetime('now','-14 days')
		GROUP BY issue_type`, skillID)
	if err != nil {
		return false, "", 0
	}
	for rows.Next() {
		var a fbAgg
		if rows.Scan(&a.issueType, &a.count, &a.users) != nil {
			continue
		}
		aggs = append(aggs, a)
	}
	rows.Close()

	for _, a := range aggs {
		if a.count >= needCount && a.users >= needUsers {
			// 已有 open 候选则不重复创建
			var existing int
			db.QueryRow(`SELECT COUNT(*) FROM version_candidates WHERE skill_id = ? AND status = 'open'`, skillID).
				Scan(&existing)
			if existing > 0 {
				return false, "", 0
			}
			ids := []int64{}
			r2, err := db.Query(`SELECT id FROM exec_feedbacks WHERE skill_id = ? AND issue_type = ?
				AND adopted = 0 AND created_at >= datetime('now','-14 days')`, skillID, a.issueType)
			if err == nil {
				for r2.Next() {
					var id int64
					if r2.Scan(&id) == nil {
						ids = append(ids, id)
					}
				}
				r2.Close()
			}
			evidence := gin.H{
				"issue_type":   a.issueType,
				"count":        a.count,
				"unique_users": a.users,
				"feedback_ids": ids,
			}
			db.Exec(`INSERT INTO version_candidates (skill_id, trigger_rule, evidence, status)
				VALUES (?, 'repeated_failure', ?, 'open')`, skillID, jsonOrEmpty(evidence))
			db.Exec(`UPDATE skills SET status = ? WHERE id = ? AND COALESCE(status,'') = ?`,
				SkillStatusNeedsReview, skillID, SkillStatusPublished)
			return true, "repeated_failure", a.count
		}
	}
	return false, "", 0
}

// listVersionCandidates GET /api/growth/skills/:id/version-candidates
func listVersionCandidates(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := db.Query(`SELECT id, trigger_rule, evidence, status, created_at
		FROM version_candidates WHERE skill_id = ? ORDER BY id DESC`, skillID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var rule, evidence, status, createdAt string
		if rows.Scan(&id, &rule, &evidence, &status, &createdAt) == nil {
			out = append(out, gin.H{
				"id": id, "trigger_rule": rule, "evidence": json.RawMessage(evidence),
				"status": status, "created_at": createdAt,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// acceptVersionCandidate POST /api/growth/version-candidates/:id/accept
// 接受候选 → 生成新版本（V1.0 → V1.1），changelog 必填。
func acceptVersionCandidate(c *gin.Context) {
	uid := c.GetInt64("userID")
	candID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Changelog     string `json:"changelog"`
		NewBoundary   []string `json:"new_boundary"`   // 新增的不适用条件
		NewHandoff    []string `json:"new_handoff"`    // 新增的人工接管触发点
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Changelog) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "changelog 必填：要说清这一版改了什么、为什么"})
		return
	}

	var skillID int64
	var status string
	if err := db.QueryRow(`SELECT skill_id, status FROM version_candidates WHERE id = ?`, candID).
		Scan(&skillID, &status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "candidate not found"})
		return
	}
	if status != "open" {
		c.JSON(http.StatusConflict, gin.H{"error": "该候选已处理，状态：" + status})
		return
	}
	var owner sql.NullInt64
	db.QueryRow(`SELECT COALESCE(maintainer_id, owner_id) FROM skills WHERE id = ?`, skillID).Scan(&owner)
	if !owner.Valid || owner.Int64 != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅维护者可处理版本候选"})
		return
	}

	// 以当前版本为基础生成新版本
	var curID sql.NullInt64
	db.QueryRow(`SELECT current_version_id FROM skills WHERE id = ?`, skillID).Scan(&curID)
	if !curID.Valid {
		c.JSON(http.StatusConflict, gin.H{"error": "没有可继承的版本"})
		return
	}
	cur, err := loadSkillVersion(curID.Int64)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 把新边界并进去——这是最典型的一次版本升级：一条新边界被验证后进入下一版
	var b struct {
		NotApplicable  []string `json:"not_applicable"`
		HandoffTrigger []string `json:"handoff_trigger"`
		FallbackPath   string   `json:"fallback_path"`
	}
	json.Unmarshal([]byte(cur.Boundary), &b)
	b.NotApplicable = append(b.NotApplicable, body.NewBoundary...)
	b.HandoffTrigger = append(b.HandoffTrigger, body.NewHandoff...)

	newVersion := bumpVersion(cur.Version)
	var srcExec sql.NullInt64
	db.QueryRow(`SELECT source_execution_id FROM skill_versions WHERE id = ?`, cur.ID).Scan(&srcExec)
	var srcVal interface{}
	if srcExec.Valid {
		srcVal = srcExec.Int64
	}

	res, err := db.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal, done_criteria,
		workflow, boundary, contract, gotchas, distillation_score, distillation_detail, changelog,
		source_execution_id, published_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		skillID, newVersion, cur.Description, cur.Goal, cur.DoneCriteria, cur.Workflow,
		jsonOrEmpty(b), cur.Contract, cur.Gotchas, cur.DistillationScore, cur.DistillationDetail,
		strings.TrimSpace(body.Changelog), srcVal)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	newVerID, _ := res.LastInsertId()

	db.Exec(`UPDATE skills SET current_version_id = ?, version = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newVerID, newVersion, SkillStatusPublished, skillID)
	db.Exec(`UPDATE version_candidates SET status = 'accepted', resulting_version_id = ?, resolved_at = CURRENT_TIMESTAMP
		WHERE id = ?`, newVerID, candID)

	// 关联反馈标记为已采纳（有效贡献的依据）
	var evidence string
	db.QueryRow(`SELECT evidence FROM version_candidates WHERE id = ?`, candID).Scan(&evidence)
	var ev struct {
		FeedbackIDs []int64 `json:"feedback_ids"`
	}
	json.Unmarshal([]byte(evidence), &ev)
	for _, fid := range ev.FeedbackIDs {
		db.Exec(`UPDATE exec_feedbacks SET adopted = 1, adopted_version_id = ? WHERE id = ?`, newVerID, fid)
	}

	// 新版本要重新跑门禁再对外，这里先把评分刷新
	recomputeSkillScore(skillID)

	c.JSON(http.StatusOK, gin.H{
		"message":        "已升级",
		"from_version":   cur.Version,
		"to_version":     newVersion,
		"new_version_id": newVerID,
		"changelog":      strings.TrimSpace(body.Changelog),
		"reminder":       "新版本建议重跑一次发布前四问，尤其是边界停机。",
	})
}

// bumpVersion 1.0 → 1.1；无法解析时退化为追加 .1
func bumpVersion(v string) string {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) >= 2 {
		major := parts[0]
		var minor int
		if _, err := fmt.Sscanf(parts[1], "%d", &minor); err == nil {
			return fmt.Sprintf("%s.%d", major, minor+1)
		}
	}
	if v == "" {
		return "1.1"
	}
	return v + ".1"
}
