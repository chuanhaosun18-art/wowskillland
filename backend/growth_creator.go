// F5 Skill Creator：把一次真实执行变成结构化、可安装、有证据的 Skill 草稿。
// 主路径是「先做一遍，再固化」——用户只做确认，不做撰写。
// 硬约束：抽取出的每条判断必须携带 source_step_index，无来源的一律丢弃（禁止模型凭空补充）。
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---------- 蒸馏度 ----------

// distillDetail 六个维度的得分明细
type distillDetail struct {
	RealTask  float64 `json:"real_task"`
	Outcome   float64 `json:"outcome"`
	Workflow  float64 `json:"workflow"`
	Decisions float64 `json:"decisions"`
	Failures  float64 `json:"failures"`
	Boundary  float64 `json:"boundary"`
	// ProofType 决定总分上限。轨迹补录封顶 0.85——能发布但拿不到满分，
	// 这样既承认用户会在平台外做事，又保住了「进工作台更可信」的激励结构（v1.2 第 2 条）。
	ProofType string  `json:"proof_type"`
	Cap       float64 `json:"cap"`
}

// 权重（PRD F5.3）
const (
	wRealTask  = 0.15
	wOutcome   = 0.15
	wWorkflow  = 0.15
	wDecisions = 0.25
	wFailures  = 0.15
	wBoundary  = 0.15
)

func (d distillDetail) total() float64 {
	raw := clamp01(d.RealTask*wRealTask + d.Outcome*wOutcome + d.Workflow*wWorkflow +
		d.Decisions*wDecisions + d.Failures*wFailures + d.Boundary*wBoundary)
	if d.ProofType == ProofArtifactUpload && raw > BackfillScoreCap {
		return BackfillScoreCap
	}
	return raw
}

// capNote 封顶时给界面的说明，避免用户以为是自己填得不够
func (d distillDetail) capNote() string {
	if d.ProofType == ProofArtifactUpload {
		return fmt.Sprintf("这是补录的经历，蒸馏度上限 %.2f。想拿满分就在工作台里做一次——有执行轨迹的证据更硬。",
			BackfillScoreCap)
	}
	return ""
}

// publishable 三条同时满足才允许发布；边界是硬性项，不接受折中
func (d distillDetail) publishable() (bool, []string) {
	missing := []string{}
	if d.total() < DistillationThreshold {
		missing = append(missing, fmt.Sprintf("蒸馏度 %.2f，还差 %.2f 到发布线", d.total(), DistillationThreshold-d.total()))
	}
	if d.Decisions < DecisionsMinScore {
		missing = append(missing, "关键判断至少要填满两个槽位")
	}
	if d.Boundary < 1 {
		missing = append(missing, "适用边界是硬性要求：必须写清不适用条件和什么时候交回给人")
	}
	return len(missing) == 0, missing
}

// lowest 返回当前最弱的维度，供 AI 针对缺口继续追问
func (d distillDetail) lowest() string {
	items := []struct {
		Key string
		Val float64
	}{
		{"real_task", d.RealTask}, {"outcome", d.Outcome}, {"workflow", d.Workflow},
		{"decisions", d.Decisions}, {"failures", d.Failures}, {"boundary", d.Boundary},
	}
	lowKey, lowVal := items[0].Key, items[0].Val
	for _, it := range items[1:] {
		if it.Val < lowVal {
			lowKey, lowVal = it.Key, it.Val
		}
	}
	return lowKey
}

var dimensionLabels = map[string]string{
	"real_task": "真实任务", "outcome": "明确结果", "workflow": "核心流程",
	"decisions": "关键判断", "failures": "失败案例", "boundary": "适用边界",
}

// computeDistill 依据草稿现状打分。模型只做归类，加权一律在后端算，保证可解释可测试。
func computeDistill(exec *Execution, v *SkillVersion, decisions []Decision) distillDetail {
	var d distillDetail
	d.ProofType = v.ProofType
	if d.ProofType == "" {
		d.ProofType = ProofPlatformTrace
	}
	d.Cap = 1
	if d.ProofType == ProofArtifactUpload {
		d.Cap = BackfillScoreCap
	}

	// 真实任务：平台内轨迹 ≥5 步得满分；补录只给 0.5
	if d.ProofType == ProofArtifactUpload {
		d.RealTask = 0.5
	} else if exec != nil && len(exec.Steps) >= 5 {
		d.RealTask = 1
	} else if exec != nil && len(exec.Steps) > 0 {
		d.RealTask = 0.5
	}

	// 明确结果：完成标准达成且产物有变化
	if exec != nil {
		var sig struct {
			Exported      bool    `json:"exported"`
			ArtifactDelta float64 `json:"artifact_delta"`
		}
		json.Unmarshal(exec.CompletionSignal, &sig)
		switch {
		case exec.Status == ExecCompleted && sig.ArtifactDelta > 0 && sig.Exported:
			d.Outcome = 1
		case exec.Status == ExecCompleted && sig.ArtifactDelta > 0:
			d.Outcome = 0.5
		}
	}

	// 核心流程：有序步骤数
	var steps []map[string]interface{}
	json.Unmarshal([]byte(v.Workflow), &steps)
	switch {
	case len(steps) >= 3:
		d.Workflow = 1
	case len(steps) == 2:
		d.Workflow = 0.5
	}

	// 关键判断：已填满的槽位数 / 4
	filled := map[string]bool{}
	for _, dec := range decisions {
		if dec.InvalidatedAt != nil {
			continue
		}
		if strings.TrimSpace(dec.TriggerSignal) != "" && strings.TrimSpace(dec.Judgment) != "" &&
			strings.TrimSpace(dec.Scope) != "" {
			filled[dec.Slot] = true
		}
	}
	d.Decisions = clamp01(float64(len(filled)) / 4.0)

	// 失败案例：gotchas 每条需含触发条件与后果
	var gotchas []struct {
		Trigger     string `json:"trigger"`
		Symptom     string `json:"symptom"`
		Consequence string `json:"consequence"`
	}
	json.Unmarshal([]byte(v.Gotchas), &gotchas)
	for _, g := range gotchas {
		if strings.TrimSpace(g.Trigger) != "" && strings.TrimSpace(g.Consequence) != "" {
			d.Failures = 1
			break
		}
		if strings.TrimSpace(g.Symptom) != "" {
			d.Failures = 0.5
		}
	}

	// 适用边界：不适用条件 + 人工接管触发点都要有
	var b struct {
		NotApplicable  []string `json:"not_applicable"`
		HandoffTrigger []string `json:"handoff_trigger"`
		FallbackPath   string   `json:"fallback_path"`
	}
	json.Unmarshal([]byte(v.Boundary), &b)
	if len(b.NotApplicable) > 0 && len(b.HandoffTrigger) > 0 {
		d.Boundary = 1
	}
	return d
}

// ---------- 轨迹抽取（P2） ----------

type extractResult struct {
	Decisions []struct {
		Slot            string `json:"slot"`
		TriggerSignal   string `json:"trigger_signal"`
		Judgment        string `json:"judgment"`
		Scope           string `json:"scope"`
		CounterExample  string `json:"counter_example"`
		SourceStepIndex int    `json:"source_step_index"`
	} `json:"decisions"`
	Workflow []struct {
		Title string `json:"title"`
		IO    string `json:"io"`
	} `json:"workflow"`
	Gotchas []struct {
		Trigger     string `json:"trigger"`
		Symptom     string `json:"symptom"`
		Consequence string `json:"consequence"`
	} `json:"gotchas"`
	BoundaryHints []string `json:"boundary_hints"`
	ToolsUsed     []string `json:"tools_used"`
	Goal          string   `json:"goal"`
	DoneCriteria  []string `json:"done_criteria"`
	SuggestedName string   `json:"suggested_name"`
}

const extractSystemPrompt = `你是经验蒸馏器。输入是一次真实任务的完整执行轨迹，你要从里面抽出可以被别人复用的方法。

你抽的不是「做了什么」，而是那些会改变结果的判断。四个槽位：
when_to_check（在哪一步停下来回头验证）
when_to_probe（什么情况下要求补充信息而不是直接动手）
when_to_use_tool（哪一步必须查必须跑，不能靠判断）
when_to_switch（什么现象一出现就知道当前路走不通）

铁律：
1. 每条判断必须填 source_step_index，指向轨迹里真实存在的那一步编号。
2. 绝对禁止补充轨迹里没有发生过的判断。宁可少抽，不许编。轨迹里只有两个判断就只输出两个。
3. trigger_signal 写「出现什么信号」，judgment 写「就要怎么做」，scope 写「在什么场景下成立」。三者都要具体。
4. gotchas 是这次真实踩到或差点踩到的坑，要有触发条件和后果。没有就给空数组。
5. suggested_name 用中文，是一个能被别人搜到的任务名，不要起花名。

严格只输出 JSON，不要 markdown 代码块：
{"suggested_name":"","goal":"","done_criteria":[],"decisions":[{"slot":"","trigger_signal":"","judgment":"","scope":"","counter_example":"","source_step_index":0}],"workflow":[{"title":"","io":""}],"gotchas":[{"trigger":"","symptom":"","consequence":""}],"boundary_hints":[],"tools_used":[]}`

// distillExecution POST /api/growth/executions/:id/distill
// 从执行轨迹生成 Skill 草稿：创建 skills(draft) + skill_versions(1.0) + 候选 decisions
func distillExecution(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	exec, err := loadExecution(id)
	if err != nil || exec.UserID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if !canDistill(exec) {
		c.JSON(http.StatusConflict, gin.H{"error": distillHint(exec)})
		return
	}

	// 已经固化过就直接返回原草稿，避免重复创建
	var existingVer int64
	if err := db.QueryRow(`SELECT id FROM skill_versions WHERE source_execution_id = ? ORDER BY id LIMIT 1`, id).
		Scan(&existingVer); err == nil && existingVer > 0 {
		respondDraft(c, existingVer)
		return
	}

	res, err := extractFromTrace(exec)
	if err != nil {
		log.Printf("distill extract failed exec=%d: %v", id, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "抽取失败：" + err.Error()})
		return
	}

	// 丢弃没有合法来源步号的判断（硬约束）
	maxIdx := len(exec.Steps) - 1
	kept := 0
	dropped := 0

	name := strings.TrimSpace(res.SuggestedName)
	if name == "" {
		name = AllowedIntents[exec.TaskIntent]
	}

	skillRes, err := db.Exec(`INSERT INTO skills (owner_id, name, description, category, tags, version,
		status, task_intent, origin, maintainer_id) VALUES (?, ?, ?, ?, '[]', '1.0', ?, ?, ?, ?)`,
		uid, name, "", AllowedIntents[exec.TaskIntent], SkillStatusDraft, exec.TaskIntent, OriginRouteOne, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	skillID, _ := skillRes.LastInsertId()

	workflow := []map[string]interface{}{}
	for i, w := range res.Workflow {
		workflow = append(workflow, map[string]interface{}{"index": i + 1, "title": w.Title, "io": w.IO})
	}
	boundary := map[string]interface{}{
		"not_applicable":  res.BoundaryHints,
		"handoff_trigger": []string{},
		"fallback_path":   "",
	}
	contract := map[string]interface{}{
		"input":       "任务材料（选题草稿 / 简历文本）",
		"output":      "可提交的产物",
		"permissions": []string{"read_upload"},
		"tools":       res.ToolsUsed,
	}

	verRes, err := db.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal, done_criteria,
		workflow, boundary, contract, gotchas, source_execution_id) VALUES (?, '1.0', ?, ?, ?, ?, ?, ?, ?, ?)`,
		skillID, "", res.Goal, jsonOrEmpty(res.DoneCriteria), jsonOrEmpty(workflow),
		jsonOrEmpty(boundary), jsonOrEmpty(contract), jsonOrEmpty(res.Gotchas), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	versionID, _ := verRes.LastInsertId()
	db.Exec(`UPDATE skills SET current_version_id = ? WHERE id = ?`, versionID, skillID)

	for _, d := range res.Decisions {
		if d.SourceStepIndex < 0 || d.SourceStepIndex > maxIdx {
			dropped++
			continue
		}
		if !isValidSlot(d.Slot) || strings.TrimSpace(d.Judgment) == "" {
			dropped++
			continue
		}
		db.Exec(`INSERT INTO decisions (experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
			counter_example, source_step_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, skillID, d.Slot, d.TriggerSignal, d.Judgment, d.Scope, d.CounterExample, d.SourceStepIndex)
		kept++
	}
	log.Printf("distill exec=%d skill=%d decisions kept=%d dropped=%d", id, skillID, kept, dropped)

	respondDraftWithStats(c, versionID, kept, dropped)
}

// extractFromTrace 调用 LLM 从轨迹抽取
func extractFromTrace(exec *Execution) (*extractResult, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【任务类型】%s\n【任务标题】%s\n\n【完整执行轨迹】\n",
		AllowedIntents[exec.TaskIntent], exec.TaskTitle))
	for _, s := range exec.Steps {
		switch s.StepType {
		case StepUserDecision:
			choice := ""
			if len(s.UserChoice) > 0 {
				choice = string(s.UserChoice)
			}
			sb.WriteString(fmt.Sprintf("步骤 %d【关键判断 slot=%s】\n  当时的信号：%s\n  用户选择：%s\n",
				s.StepIndex, s.DecisionSlot, truncate(s.Input, 500), choice))
		case StepToolCall:
			ok := "成功"
			if s.ToolOK != nil && !*s.ToolOK {
				ok = "失败"
			}
			sb.WriteString(fmt.Sprintf("步骤 %d【工具 %s %s】输入：%s\n  返回：%s\n",
				s.StepIndex, s.ToolName, ok, truncate(s.Input, 200), truncate(s.Output, 400)))
		case StepHumanHandoff:
			sb.WriteString(fmt.Sprintf("步骤 %d【交回给人】%s\n", s.StepIndex, truncate(s.Output, 300)))
		default:
			sb.WriteString(fmt.Sprintf("步骤 %d【%s】%s\n", s.StepIndex, s.Title, truncate(s.Output, 500)))
		}
	}
	sb.WriteString(fmt.Sprintf("\n【合法的 source_step_index 范围】0 到 %d\n", len(exec.Steps)-1))

	msgs := []chatMsg{
		{Role: "system", Content: extractSystemPrompt},
		{Role: "user", Content: sb.String()},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := callGuideDeepSeek(context.Background(), msgs)
		if err != nil {
			lastErr = err
			continue
		}
		var res extractResult
		if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
			lastErr = err
			continue
		}
		return &res, nil
	}
	return nil, lastErr
}

// ---------- F5.3b 轨迹补录（v1.2 新增） ----------

// backfillExecution POST /api/growth/backfill
//
// 承认一件事：用户会在平台外做事。改简历用 Word、改选题在纸上跟导师聊，工具习惯很强。
// 如果不进工作台就什么都拿不到，早期用户会直接流失。
// 所以给降级路径，但不给平权——蒸馏度封顶 0.85，想拿满分就进工作台。
func backfillExecution(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		TaskIntent string `json:"task_intent"`
		TaskTitle  string `json:"task_title"`
		Before     string `json:"before"` // 做之前的产物
		After      string `json:"after"`  // 做之后的产物
		Decisions  []struct {
			Slot          string `json:"slot"`
			TriggerSignal string `json:"trigger_signal"`
			Judgment      string `json:"judgment"`
			Scope         string `json:"scope"`
			StageIndex    int    `json:"stage_index"` // 用户自述的阶段序号
		} `json:"decisions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !isProductive(body.TaskIntent) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "这类任务不产出方法，不需要补录"})
		return
	}
	if strings.TrimSpace(body.After) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "至少要有做完之后的产物"})
		return
	}

	// 建一条已完成的执行记录，标记为补录来源
	snapshot := gin.H{}
	if u, err := getUserByID(uid); err == nil {
		snapshot = gin.H{"school": u.School, "major": u.Major, "grade": u.Grade}
	}
	input := gin.H{"goal": body.TaskTitle, "material": body.Before}
	signal := gin.H{
		"exported":       true,
		"artifact_delta": artifactDelta(body.Before, body.After),
		"manual_rework":  false,
		"backfilled":     true,
	}
	res, err := db.Exec(`INSERT INTO executions (user_id, task_intent, task_title, user_context,
		input, output, status, completion_signal, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		uid, body.TaskIntent, strings.TrimSpace(body.TaskTitle), jsonOrEmpty(snapshot),
		jsonOrEmpty(input), body.After, ExecCompleted, jsonOrEmpty(signal))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	execID, _ := res.LastInsertId()

	// 用自述阶段生成占位轨迹。标题里写明是补录，审计时一眼能看出来。
	insertStep(execID, 0, StepAIAction, "补录：做之前", "", "", "", nil, body.Before, body.Before, 0)
	insertStep(execID, 1, StepAIAction, "补录：做之后", "", "", "", nil, "", body.After, 0)

	// 建 Skill 与版本，proof_type 标为补录
	name := strings.TrimSpace(body.TaskTitle)
	if name == "" {
		name = AllowedIntents[body.TaskIntent]
	}
	skillRes, err := db.Exec(`INSERT INTO skills (owner_id, name, description, category, tags, version,
		status, task_intent, origin, maintainer_id) VALUES (?, ?, '', ?, '[]', '1.0', ?, ?, ?, ?)`,
		uid, name, AllowedIntents[body.TaskIntent], SkillStatusDraft, body.TaskIntent, OriginRouteTwo, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	skillID, _ := skillRes.LastInsertId()

	boundary := map[string]interface{}{
		"not_applicable": []string{}, "handoff_trigger": []string{}, "fallback_path": "",
	}
	contract := map[string]interface{}{
		"input": "任务材料", "output": "可提交的产物", "permissions": []string{"read_upload"},
	}
	verRes, err := db.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal,
		done_criteria, workflow, boundary, contract, gotchas, source_execution_id, proof_type)
		VALUES (?, '1.0', '', ?, '[]', '[]', ?, ?, '[]', ?, ?)`,
		skillID, body.TaskTitle, jsonOrEmpty(boundary), jsonOrEmpty(contract), execID, ProofArtifactUpload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	versionID, _ := verRes.LastInsertId()
	db.Exec(`UPDATE skills SET current_version_id = ? WHERE id = ?`, versionID, skillID)

	// 用户自己填的判断：source_step_index 指向自述阶段序号，并且仍然要求三项齐全
	kept := 0
	for _, d := range body.Decisions {
		if !isValidSlot(d.Slot) {
			continue
		}
		if strings.TrimSpace(d.TriggerSignal) == "" || strings.TrimSpace(d.Judgment) == "" ||
			strings.TrimSpace(d.Scope) == "" {
			continue
		}
		idx := d.StageIndex
		if idx < 0 || idx > 1 {
			idx = 1
		}
		db.Exec(`INSERT INTO decisions (experience_exec_id, skill_id, slot, trigger_signal, judgment,
			scope, source_step_index) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			execID, skillID, d.Slot, d.TriggerSignal, d.Judgment, d.Scope, idx)
		kept++
	}

	// 四槽全空就只落 Insight，不允许成为 Skill
	if kept == 0 {
		db.Exec(`INSERT INTO insights (execution_id, user_id, claim, why, missing_for_skill)
			VALUES (?, ?, ?, ?, ?)`,
			execID, uid, name, "补录了产物但没有说清关键判断",
			jsonOrEmpty([]string{"至少填一条关键判断才能成为可复用的方法"}))
		db.Exec(`UPDATE skills SET status = ? WHERE id = ?`, SkillStatusInsightOnly, skillID)
		c.JSON(http.StatusOK, gin.H{
			"message":       "先存成经验笔记",
			"still_missing": []string{"至少填一条关键判断（出现什么信号、就要怎么做、在什么场景成立）"},
			"execution_id":  execID,
		})
		return
	}

	log.Printf("backfill: exec=%d skill=%d decisions=%d", execID, skillID, kept)
	respondDraftWithStats(c, versionID, kept, 0)
}

// ---------- 草稿读写 ----------

// getDraft GET /api/growth/drafts/:versionID
func getDraft(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	respondDraft(c, vid)
}

// updateDraft PATCH /api/growth/drafts/:versionID
// 用户确认或补充草稿字段后重算蒸馏度
func updateDraft(c *gin.Context) {
	uid := c.GetInt64("userID")
	vid, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	skillID, ownerID, err := versionOwner(vid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	if ownerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅创作者可编辑"})
		return
	}

	var body struct {
		Name         *string       `json:"name"`
		Description  *string       `json:"description"`
		Goal         *string       `json:"goal"`
		DoneCriteria []string      `json:"done_criteria"`
		Workflow     []interface{} `json:"workflow"`
		Gotchas      []interface{} `json:"gotchas"`
		Boundary     *struct {
			NotApplicable  []string `json:"not_applicable"`
			HandoffTrigger []string `json:"handoff_trigger"`
			FallbackPath   string   `json:"fallback_path"`
		} `json:"boundary"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	if body.Name != nil {
		db.Exec(`UPDATE skills SET name = ? WHERE id = ?`, strings.TrimSpace(*body.Name), skillID)
	}
	if body.Description != nil {
		db.Exec(`UPDATE skill_versions SET description = ? WHERE id = ?`, strings.TrimSpace(*body.Description), vid)
		db.Exec(`UPDATE skills SET description = ? WHERE id = ?`, strings.TrimSpace(*body.Description), skillID)
	}
	if body.Goal != nil {
		db.Exec(`UPDATE skill_versions SET goal = ? WHERE id = ?`, strings.TrimSpace(*body.Goal), vid)
	}
	if body.DoneCriteria != nil {
		db.Exec(`UPDATE skill_versions SET done_criteria = ? WHERE id = ?`, jsonOrEmpty(body.DoneCriteria), vid)
	}
	if body.Workflow != nil {
		db.Exec(`UPDATE skill_versions SET workflow = ? WHERE id = ?`, jsonOrEmpty(body.Workflow), vid)
	}
	if body.Gotchas != nil {
		db.Exec(`UPDATE skill_versions SET gotchas = ? WHERE id = ?`, jsonOrEmpty(body.Gotchas), vid)
	}
	if body.Boundary != nil {
		db.Exec(`UPDATE skill_versions SET boundary = ? WHERE id = ?`, jsonOrEmpty(body.Boundary), vid)
	}
	respondDraft(c, vid)
}

// upsertDecision POST /api/growth/drafts/:versionID/decisions
// 用户手工补一条判断。source_step_index 仍然必填——手工补的也要指回真实那一步。
func upsertDecision(c *gin.Context) {
	uid := c.GetInt64("userID")
	vid, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	skillID, ownerID, err := versionOwner(vid)
	if err != nil || ownerID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	var body struct {
		ID              *int64 `json:"id"`
		Slot            string `json:"slot"`
		TriggerSignal   string `json:"trigger_signal"`
		Judgment        string `json:"judgment"`
		Scope           string `json:"scope"`
		CounterExample  string `json:"counter_example"`
		SourceStepIndex int    `json:"source_step_index"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !isValidSlot(body.Slot) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法的判断槽位"})
		return
	}
	// 三项都填才算一条完整判断（PRD F5.2）
	if strings.TrimSpace(body.TriggerSignal) == "" || strings.TrimSpace(body.Judgment) == "" ||
		strings.TrimSpace(body.Scope) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "触发信号、判断、适用场景三项都要填"})
		return
	}

	var execID int64
	db.QueryRow(`SELECT COALESCE(source_execution_id, 0) FROM skill_versions WHERE id = ?`, vid).Scan(&execID)

	if body.ID != nil && *body.ID > 0 {
		db.Exec(`UPDATE decisions SET slot=?, trigger_signal=?, judgment=?, scope=?, counter_example=?, source_step_index=?
			WHERE id = ? AND skill_id = ?`,
			body.Slot, body.TriggerSignal, body.Judgment, body.Scope, body.CounterExample, body.SourceStepIndex,
			*body.ID, skillID)
	} else {
		db.Exec(`INSERT INTO decisions (experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
			counter_example, source_step_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			execID, skillID, body.Slot, body.TriggerSignal, body.Judgment, body.Scope,
			body.CounterExample, body.SourceStepIndex)
	}
	respondDraft(c, vid)
}

// deleteDecision DELETE /api/growth/decisions/:id
func deleteDecision(c *gin.Context) {
	uid := c.GetInt64("userID")
	did, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var skillID int64
	var ownerID sql.NullInt64
	if err := db.QueryRow(`SELECT d.skill_id, s.owner_id FROM decisions d
		JOIN skills s ON s.id = d.skill_id WHERE d.id = ?`, did).Scan(&skillID, &ownerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "decision not found"})
		return
	}
	if !ownerID.Valid || ownerID.Int64 != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅创作者可删除"})
		return
	}
	db.Exec(`DELETE FROM decisions WHERE id = ?`, did)

	var vid int64
	db.QueryRow(`SELECT COALESCE(current_version_id,0) FROM skills WHERE id = ?`, skillID).Scan(&vid)
	respondDraft(c, vid)
}

// downgradeToInsight POST /api/growth/drafts/:versionID/downgrade
// 蒸馏度不足时的正确出口：存成经验笔记，而不是判定失败。
func downgradeToInsight(c *gin.Context) {
	uid := c.GetInt64("userID")
	vid, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	skillID, ownerID, err := versionOwner(vid)
	if err != nil || ownerID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	exec, ver, decisions, err := loadDraftParts(vid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	detail := computeDistill(exec, ver, decisions)
	_, missing := detail.publishable()

	var execID int64
	if exec != nil {
		execID = exec.ID
	}
	claim := ver.Goal
	if strings.TrimSpace(claim) == "" {
		claim = "这次任务里有值得留下的东西，但还不足以成为可复用的方法"
	}
	db.Exec(`INSERT INTO insights (execution_id, user_id, claim, why, missing_for_skill) VALUES (?, ?, ?, ?, ?)`,
		execID, uid, claim, "关键判断或边界尚未说清", jsonOrEmpty(missing))
	db.Exec(`UPDATE skills SET status = ? WHERE id = ?`, SkillStatusInsightOnly, skillID)

	c.JSON(http.StatusOK, gin.H{
		// 话术要求：不得出现「失败」「不合格」字样
		"message": "这次先存成经验笔记",
		"still_missing": missing,
		"hint":    "补上这几项之后随时可以再来一次，笔记不会丢",
	})
}

// ---------- 文件夹生成（F5.4） ----------

// generateFolder POST /api/growth/drafts/:versionID/generate-folder
// 按材料给出的分工生成六个 slot，打成可安装的 zip，并做一次自安装自调用校验。
func generateFolder(c *gin.Context) {
	uid := c.GetInt64("userID")
	vid, err := strconv.ParseInt(c.Param("versionID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version id"})
		return
	}
	skillID, ownerID, err := versionOwner(vid)
	if err != nil || ownerID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	exec, ver, decisions, err := loadDraftParts(vid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	detail := computeDistill(exec, ver, decisions)
	ok, missing := detail.publishable()
	if !ok {
		c.JSON(http.StatusConflict, gin.H{
			"error":         "还不能生成可发布的 Skill 包",
			"still_missing": missing,
			"suggestion":    "可以先存成经验笔记",
		})
		return
	}

	var skillName string
	db.QueryRow(`SELECT name FROM skills WHERE id = ?`, skillID).Scan(&skillName)
	root := sanitizeKebab(skillName)

	files := buildSkillFiles(root, skillName, ver, decisions, exec)

	zipBytes, err := buildSkillZip(files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打包失败：" + err.Error()})
		return
	}

	// 自安装自调用校验：能不能从 zip 里找回 SKILL.md 与至少一个 evals 文件
	installOK, installErr := selfInstallCheck(files)
	if !installOK {
		c.JSON(http.StatusConflict, gin.H{"error": "自安装校验未通过：" + installErr})
		return
	}

	// 落盘：archives/<skillID>.zip + files/<skillID>/，并登记文件清单
	archivePath := filepath.Join(ArchiveDir, fmt.Sprintf("%d.zip", skillID))
	if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入 zip 失败：" + err.Error()})
		return
	}
	skillDir := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	os.RemoveAll(skillDir)
	os.MkdirAll(skillDir, 0o755)
	for _, f := range files {
		p := filepath.Join(skillDir, filepath.FromSlash(f.Path))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f.Content), 0o644)
	}
	db.Exec(`DELETE FROM skill_files WHERE skill_id = ?`, skillID)
	var totalSize int64
	if indexed, err := indexFiles(skillID, skillDir); err == nil {
		for _, f := range indexed {
			db.Exec(`INSERT INTO skill_files (skill_id, file_path, size, sha256) VALUES (?, ?, ?, ?)`,
				skillID, f.FilePath, f.Size, f.SHA256)
			totalSize += f.Size
		}
		db.Exec(`UPDATE skills SET file_count = ?, total_size = ?, archive_path = ?, status = ? WHERE id = ?`,
			len(indexed), totalSize, archivePath, SkillStatusGated, skillID)
	}
	db.Exec(`UPDATE skill_versions SET distillation_score = ?, distillation_detail = ? WHERE id = ?`,
		detail.total(), jsonOrEmpty(detail), vid)

	// 生成 evals 用例入库（供 F6 四问使用）
	seedEvalCases(skillID, vid, ver, decisions, exec)

	paths := []string{}
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	c.JSON(http.StatusOK, gin.H{
		"skill_id":            skillID,
		"version_id":          vid,
		"files":               paths,
		"distillation_score":  detail.total(),
		"self_install_check":  true,
		"zip_base64":          base64.StdEncoding.EncodeToString(zipBytes),
		"next":                "跑发布前四问",
	})
}

// buildSkillFiles 按材料原文的分工生成六个 slot
func buildSkillFiles(root, skillName string, ver *SkillVersion, decisions []Decision, exec *Execution) []generatedFile {
	files := []generatedFile{}

	// 1) SKILL.md：核心步骤 + 关键岔路口
	var md strings.Builder
	md.WriteString("---\n")
	md.WriteString("name: " + root + "\n")
	desc := ver.Description
	if strings.TrimSpace(desc) == "" {
		desc = ver.Goal
	}
	md.WriteString("description: " + strings.ReplaceAll(desc, "\n", " ") + "\n")
	md.WriteString("---\n\n")
	md.WriteString("# " + skillName + "\n\n")
	md.WriteString("## 这个 Skill 解决什么\n\n" + ver.Goal + "\n\n")

	var criteria []string
	json.Unmarshal([]byte(ver.DoneCriteria), &criteria)
	if len(criteria) > 0 {
		md.WriteString("## 什么算做完\n\n")
		for _, ct := range criteria {
			md.WriteString("- " + ct + "\n")
		}
		md.WriteString("\n")
	}

	var steps []struct {
		Index int    `json:"index"`
		Title string `json:"title"`
		IO    string `json:"io"`
	}
	json.Unmarshal([]byte(ver.Workflow), &steps)
	md.WriteString("## 流程\n\n")
	for _, s := range steps {
		md.WriteString(fmt.Sprintf("%d. **%s** — %s\n", s.Index, s.Title, s.IO))
	}
	md.WriteString("\n")

	md.WriteString("## 关键岔路口\n\n")
	md.WriteString("以下每条判断都来自一次真实执行，标注了它出自哪一步。\n\n")
	for _, slotDef := range DecisionSlots {
		group := []Decision{}
		for _, d := range decisions {
			if d.Slot == slotDef.Slot && d.InvalidatedAt == nil {
				group = append(group, d)
			}
		}
		if len(group) == 0 {
			continue
		}
		md.WriteString("### " + slotDef.Prompt + "\n\n")
		for _, d := range group {
			md.WriteString(fmt.Sprintf("- 当**%s**时，%s（适用：%s；来源：执行第 %d 步）\n",
				d.TriggerSignal, d.Judgment, d.Scope, d.SourceStepIndex))
			if strings.TrimSpace(d.CounterExample) != "" {
				md.WriteString("  - 反例：" + d.CounterExample + "\n")
			}
		}
		md.WriteString("\n")
	}

	var boundary struct {
		NotApplicable  []string `json:"not_applicable"`
		HandoffTrigger []string `json:"handoff_trigger"`
		FallbackPath   string   `json:"fallback_path"`
	}
	json.Unmarshal([]byte(ver.Boundary), &boundary)
	md.WriteString("## 边界与人工接管\n\n")
	md.WriteString("**不适用于：**\n")
	for _, x := range boundary.NotApplicable {
		md.WriteString("- " + x + "\n")
	}
	md.WriteString("\n**出现以下情况必须交回给人：**\n")
	for _, x := range boundary.HandoffTrigger {
		md.WriteString("- " + x + "\n")
	}
	if strings.TrimSpace(boundary.FallbackPath) != "" {
		md.WriteString("\n**降级路径：**" + boundary.FallbackPath + "\n")
	}
	md.WriteString("\n> 高频错误见 `gotchas/`，测试用例见 `evals/`，来源与评判细则见 `references/`。\n")
	files = append(files, generatedFile{Path: root + "/SKILL.md", Content: md.String()})

	// 2) references/：来源与评判细则（脱敏，不含原始隐私材料）
	var ref strings.Builder
	ref.WriteString("# 来源与评判细则\n\n")
	ref.WriteString("本 Skill 由一次真实任务执行固化而来。\n\n")
	if exec != nil {
		ref.WriteString(fmt.Sprintf("- 任务类型：%s\n- 执行步数：%d\n- 关键判断数：%d\n",
			AllowedIntents[exec.TaskIntent], len(exec.Steps), len(decisions)))
		ref.WriteString("\n> 出于隐私要求，原始材料不随包分发，这里只保留判断与场景摘要。\n")
	}
	files = append(files, generatedFile{Path: root + "/references/source-and-criteria.md", Content: ref.String()})

	// 3) scripts/：确定性操作（只放不该靠模型判断的部分）
	var contract struct {
		Tools []string `json:"tools"`
	}
	json.Unmarshal([]byte(ver.Contract), &contract)
	var script strings.Builder
	script.WriteString("# 确定性操作清单\n\n")
	script.WriteString("以下步骤必须查、必须跑，不能靠判断：\n\n")
	if len(contract.Tools) == 0 {
		script.WriteString("- （本次执行未使用确定性工具）\n")
	}
	for _, t := range contract.Tools {
		script.WriteString("- " + t + "\n")
	}
	files = append(files, generatedFile{Path: root + "/scripts/deterministic-checks.md", Content: script.String()})

	// 4) assets/：模板
	var asset strings.Builder
	asset.WriteString("# 产物模板\n\n")
	for _, s := range steps {
		asset.WriteString("## " + s.Title + "\n\n（在此填写）\n\n")
	}
	files = append(files, generatedFile{Path: root + "/assets/worksheet.md", Content: asset.String()})

	// 5) gotchas/：每条一个文件
	var gotchas []struct {
		Trigger     string `json:"trigger"`
		Symptom     string `json:"symptom"`
		Consequence string `json:"consequence"`
	}
	json.Unmarshal([]byte(ver.Gotchas), &gotchas)
	if len(gotchas) == 0 {
		files = append(files, generatedFile{
			Path:    root + "/gotchas/README.md",
			Content: "# 高频错误\n\n本版本尚未记录高频错误。\n",
		})
	}
	for i, g := range gotchas {
		var gb strings.Builder
		gb.WriteString(fmt.Sprintf("# 坑 %d\n\n", i+1))
		gb.WriteString("**触发条件：**" + g.Trigger + "\n\n")
		gb.WriteString("**表现：**" + g.Symptom + "\n\n")
		gb.WriteString("**后果：**" + g.Consequence + "\n")
		files = append(files, generatedFile{
			Path:    fmt.Sprintf("%s/gotchas/gotcha-%d.md", root, i+1),
			Content: gb.String(),
		})
	}

	// 6) evals/：四类测试用例
	var ev strings.Builder
	ev.WriteString("# 测试用例\n\n")
	ev.WriteString("## 可发现性（该出现时能否被找到）\n\n")
	for _, u := range corpusFor(ver.SkillID, 10) {
		ev.WriteString("- " + u + "\n")
	}
	ev.WriteString("\n## 完成（旧问题回放）\n\n")
	if exec != nil {
		ev.WriteString("- 回放来源执行的原始输入\n")
	}
	ev.WriteString("\n## 稳定（换输入）\n\n- 换学科\n- 换材料质量\n- 换年级\n")
	ev.WriteString("\n## 边界停机（该停时是否停）\n\n")
	for _, x := range boundary.HandoffTrigger {
		ev.WriteString("- " + x + "\n")
	}
	files = append(files, generatedFile{Path: root + "/evals/cases.md", Content: ev.String()})

	return files
}

// selfInstallCheck 自安装自调用校验：SKILL.md 必须且仅一个，evals 至少一个
func selfInstallCheck(files []generatedFile) (bool, string) {
	skillMD := 0
	evals := 0
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f.Path))
		if base == "skill.md" {
			skillMD++
			if !strings.HasPrefix(strings.TrimSpace(f.Content), "---") {
				return false, "SKILL.md 缺少 frontmatter"
			}
		}
		if strings.Contains(f.Path, "/evals/") {
			evals++
		}
	}
	if skillMD != 1 {
		return false, fmt.Sprintf("SKILL.md 必须且只能有一个，当前 %d 个", skillMD)
	}
	if evals < 1 {
		return false, "evals 至少要有一个测试文件"
	}
	return true, ""
}

// corpusFor 取该 intent 下的真实用户原话，作为可发现性测试用例。
// task_intent 为空时返回空：空 intent 在语料库里是无关杂料（论文/考研等演示数据），
// 绝不能当成本 skill 的测试输入，否则会出现「保研 skill 拿论文选题测召回」的误判。
func corpusFor(skillID int64, limit int) []string {
	var intent string
	db.QueryRow(`SELECT COALESCE(task_intent,'') FROM skills WHERE id = ?`, skillID).Scan(&intent)
	if strings.TrimSpace(intent) == "" {
		return nil
	}
	rows, err := db.Query(`SELECT utterance FROM description_corpus
		WHERE task_intent = ? ORDER BY id LIMIT ?`, intent, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			out = append(out, u)
		}
	}
	return out
}

// ---------- 共用读取 ----------

func versionOwner(versionID int64) (skillID int64, ownerID int64, err error) {
	var owner sql.NullInt64
	err = db.QueryRow(`SELECT v.skill_id, s.owner_id FROM skill_versions v
		JOIN skills s ON s.id = v.skill_id WHERE v.id = ?`, versionID).Scan(&skillID, &owner)
	if err != nil {
		return 0, 0, err
	}
	if owner.Valid {
		ownerID = owner.Int64
	}
	return skillID, ownerID, nil
}

func loadSkillVersion(versionID int64) (*SkillVersion, error) {
	var v SkillVersion
	var published sql.NullTime
	err := db.QueryRow(`SELECT id, skill_id, version, description, goal, done_criteria, workflow,
		boundary, contract, gotchas, distillation_score, distillation_detail,
		COALESCE(proof_type,'platform_trace'), changelog, published_at, created_at
		FROM skill_versions WHERE id = ?`, versionID).
		Scan(&v.ID, &v.SkillID, &v.Version, &v.Description, &v.Goal, &v.DoneCriteria, &v.Workflow,
			&v.Boundary, &v.Contract, &v.Gotchas, &v.DistillationScore, &v.DistillationDetail,
			&v.ProofType, &v.Changelog, &published, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	v.PublishedAt = nullTime(published)
	return &v, nil
}

func loadDecisions(skillID int64) []Decision {
	rows, err := db.Query(`SELECT id, experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
		counter_example, source_step_index, verified_by_count, invalidated_at, created_at
		FROM decisions WHERE skill_id = ? ORDER BY slot, id`, skillID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Decision{}
	for rows.Next() {
		var d Decision
		var sid sql.NullInt64
		var inval sql.NullTime
		if err := rows.Scan(&d.ID, &d.ExperienceExecID, &sid, &d.Slot, &d.TriggerSignal, &d.Judgment,
			&d.Scope, &d.CounterExample, &d.SourceStepIndex, &d.VerifiedByCount, &inval, &d.CreatedAt); err != nil {
			continue
		}
		if sid.Valid {
			v := sid.Int64
			d.SkillID = &v
		}
		d.InvalidatedAt = nullTime(inval)
		out = append(out, d)
	}
	return out
}

// loadDraftParts 一次性取出草稿三件套
func loadDraftParts(versionID int64) (*Execution, *SkillVersion, []Decision, error) {
	ver, err := loadSkillVersion(versionID)
	if err != nil {
		return nil, nil, nil, err
	}
	decisions := loadDecisions(ver.SkillID)
	var execID sql.NullInt64
	db.QueryRow(`SELECT source_execution_id FROM skill_versions WHERE id = ?`, versionID).Scan(&execID)
	var exec *Execution
	if execID.Valid && execID.Int64 > 0 {
		exec, _ = loadExecution(execID.Int64)
	}
	return exec, ver, decisions, nil
}

func respondDraft(c *gin.Context, versionID int64) {
	respondDraftWithStats(c, versionID, -1, -1)
}

// respondDraftWithStats 返回草稿全貌 + 蒸馏度六项 + 缺口
func respondDraftWithStats(c *gin.Context, versionID int64, kept, dropped int) {
	exec, ver, decisions, err := loadDraftParts(versionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	detail := computeDistill(exec, ver, decisions)
	publishable, missing := detail.publishable()

	// 四槽分组，未填的槽位也要返回，界面才能显示提示语
	slots := []gin.H{}
	for _, sd := range DecisionSlots {
		items := []Decision{}
		for _, d := range decisions {
			if d.Slot == sd.Slot && d.InvalidatedAt == nil {
				items = append(items, d)
			}
		}
		slots = append(slots, gin.H{
			"slot":      sd.Slot,
			"prompt":    sd.Prompt,
			"decisions": items,
			"filled":    len(items) > 0,
		})
	}

	var skillName, skillStatus string
	db.QueryRow(`SELECT name, COALESCE(status,'') FROM skills WHERE id = ?`, ver.SkillID).Scan(&skillName, &skillStatus)

	resp := gin.H{
		"version_id":   versionID,
		"skill_id":     ver.SkillID,
		"skill_name":   skillName,
		"skill_status": skillStatus,
		"version":      ver,
		"slots":        slots,
		"distillation": gin.H{
			"score":         detail.total(),
			"detail":        detail,
			"threshold":     DistillationThreshold,
			"publishable":   publishable,
			"still_missing": missing,
			"lowest":        detail.lowest(),
			"lowest_label":  dimensionLabels[detail.lowest()],
			"labels":        dimensionLabels,
			"proof_type":    detail.ProofType,
			"cap":           detail.Cap,
			"cap_note":      detail.capNote(),
		},
		"corpus_candidates": corpusFor(ver.SkillID, 10),
	}
	if exec != nil {
		resp["source_execution"] = gin.H{"id": exec.ID, "step_count": len(exec.Steps)}
	}
	if kept >= 0 {
		resp["extract_stats"] = gin.H{
			"kept":    kept,
			"dropped": dropped,
			"note":    "没有来源步号的候选判断已被丢弃，不进入界面",
		}
	}
	c.JSON(http.StatusOK, resp)
}
