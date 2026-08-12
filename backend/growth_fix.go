package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---------- 失败原因反馈 + 反向指导（门禁开药方） ----------

// gateFixContext 汇总一次门禁的全部失败上下文，供生成修复建议
type gateFixContext struct {
	SkillID   int64
	VersionID int64
	Goal      string
	Workflow  string
	Boundary  string
	DoneCrit  string
	Gotchas   string
	Decisions []Decision
	AdmFails  []string
	EvalFails []gin.H // [{eval_type, input, reason}]
	Distill   float64
	StillMiss []string
}

// collectGateFixContext 组装修复建议的上下文（草稿 + 准入失败 + 四问失败用例）
func collectGateFixContext(skillID, versionID int64) *gateFixContext {
	exec, ver, decisions, err := loadDraftParts(versionID)
	if err != nil {
		return nil
	}
	ctx := &gateFixContext{
		SkillID: skillID, VersionID: versionID,
		Goal: ver.Goal, Workflow: ver.Workflow, Boundary: ver.Boundary,
		DoneCrit: ver.DoneCriteria, Gotchas: ver.Gotchas, Decisions: decisions,
	}
	adm := runAdmissionCheck(skillID, ver)
	ctx.AdmFails = adm.Failures
	detail := computeDistill(exec, ver, decisions)
	ctx.Distill = detail.total()
	_, ctx.StillMiss = detail.publishable()

	for _, t := range []string{EvalDiscoverability, EvalCompletion, EvalStability, EvalBoundaryStop} {
		var raw string
		if err := db.QueryRow(`SELECT COALESCE(detail,'') FROM eval_runs
			WHERE version_id = ? AND eval_type = ? ORDER BY id DESC LIMIT 1`,
			versionID, t).Scan(&raw); err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		var cases []evalCaseResult
		if json.Unmarshal([]byte(raw), &cases) != nil {
			continue
		}
		for _, cs := range cases {
			if !cs.Passed {
				ctx.EvalFails = append(ctx.EvalFails, gin.H{
					"eval_type": t, "input": cs.Input, "reason": cs.Reason,
				})
			}
		}
	}
	return ctx
}

// fixSuggestion 结构化修复建议：诊断 + 可写回草稿的起草内容
type fixSuggestion struct {
	Diagnosis []struct {
		Item string `json:"item"`
		Why  string `json:"why"`
		How  string `json:"how"`
	} `json:"diagnosis"`
	Draft struct {
		Goal         *string  `json:"goal,omitempty"`
		DoneCriteria []string `json:"done_criteria,omitempty"`
		Gotchas      []string `json:"gotchas,omitempty"`
		Boundary     *struct {
			NotApplicable  []string `json:"not_applicable"`
			HandoffTrigger []string `json:"handoff_trigger"`
			FallbackPath   string   `json:"fallback_path"`
		} `json:"boundary,omitempty"`
		Judgments []struct {
			Slot          string `json:"slot"`
			TriggerSignal string `json:"trigger_signal"`
			Judgment      string `json:"judgment"`
			Scope         string `json:"scope"`
		} `json:"judgments,omitempty"`
	} `json:"draft"`
}

const fixCoachPrompt = `你是 Skill 质量教练。下面是一个 Skill 当前的定义、它没通过的门禁检查、以及失败的测试用例原因。
你的任务不是判分，而是给出「为什么会失败」和「具体怎么改」，并直接起草可以写回 Creator 草稿的内容。

【Skill 定义】
目标：%s
流程：%s
边界：%s
完成标准：%s
高频错误：%s
已填关键判断：
%s

【准入检查失败】
%s

【四问失败用例（类型/输入/判定原因）】
%s

【蒸馏度】%.2f（发布线 0.75），缺口：%s

输出严格 JSON，不要 markdown 代码块，格式如下：
{
  "diagnosis": [
    {"item": "失败点名称（如：准入-边界模糊）", "why": "为什么会失败", "how": "具体怎么改"}
  ],
  "draft": {
    "goal": "目标不清才写，否则省略",
    "done_criteria": ["可判断的完成标准，每条都能被验证，至少 2 条"],
    "boundary": {"not_applicable": ["明确不适用的情况"], "handoff_trigger": ["出现什么现象就交回给人"]},
    "gotchas": ["这条 Skill 最容易犯的错误，让执行时警醒"],
    "judgments": [{"slot": "when_to_check", "trigger_signal": "触发信号", "judgment": "该做什么判断", "scope": "适用场景"}]
  }
}

规则：
1. judgments 的 slot 只能是 when_to_check / when_to_probe / when_to_use_tool / when_to_switch 之一。
2. draft 里没有把握的字段就省略，不要虚构这条 Skill 不存在的环节。
3. draft 必须与上面四问的失败原因一一对应：边界失败就补 boundary，完成失败就补 done_criteria 与 judgments，流程断点就补 judgments。
4. 不要给违反伦理或危险的建议。`

// generateFixSuggestion 用 LLM 生成修复建议；失败时由调用方走规则兜底
func generateFixSuggestion(ctx *gateFixContext) (*fixSuggestion, error) {
	decLines := []string{}
	for _, d := range ctx.Decisions {
		if d.InvalidatedAt != nil {
			continue
		}
		decLines = append(decLines, fmt.Sprintf("- [%s] 当 %s 时，%s（适用：%s）",
			d.Slot, d.TriggerSignal, d.Judgment, d.Scope))
	}
	if len(decLines) == 0 {
		decLines = []string{"（无）"}
	}
	admBlock := "（无）"
	if len(ctx.AdmFails) > 0 {
		admBlock = "· " + strings.Join(ctx.AdmFails, "\n· ")
	}
	evalBlock := "（无）"
	if len(ctx.EvalFails) > 0 {
		lines := []string{}
		for _, f := range ctx.EvalFails {
			lines = append(lines, fmt.Sprintf("[%s] 输入：%s\n  判定原因：%s",
				f["eval_type"], f["input"], f["reason"]))
		}
		evalBlock = strings.Join(lines, "\n")
	}

	prompt := fmt.Sprintf(fixCoachPrompt,
		ctx.Goal, ctx.Workflow, ctx.Boundary, ctx.DoneCrit, ctx.Gotchas,
		strings.Join(decLines, "\n"), admBlock, evalBlock, ctx.Distill,
		strings.Join(ctx.StillMiss, "；"))

	msgs := []chatMsg{
		{Role: "system", Content: "你输出严格 JSON。"},
		{Role: "user", Content: prompt},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := callDeepSeek(context.Background(), msgs)
		if err != nil {
			lastErr = err
			continue
		}
		txt := strings.TrimSpace(raw)
		txt = strings.TrimPrefix(txt, "```json")
		txt = strings.TrimPrefix(txt, "```")
		txt = strings.TrimSuffix(txt, "```")
		if i := strings.Index(txt, "{"); i >= 0 {
			if j := strings.LastIndex(txt, "}"); j > i {
				txt = txt[i : j+1]
			}
		}
		var sug fixSuggestion
		if err := json.Unmarshal([]byte(txt), &sug); err != nil {
			lastErr = err
			continue
		}
		if len(sug.Diagnosis) == 0 {
			lastErr = fmt.Errorf("建议为空")
			continue
		}
		return &sug, nil
	}
	return nil, lastErr
}

// fallbackFixSuggestion 模型不可用时的规则化建议，保证闭环不断
func fallbackFixSuggestion(ctx *gateFixContext) *fixSuggestion {
	sug := &fixSuggestion{}
	for _, f := range ctx.AdmFails {
		switch {
		case strings.Contains(f, "SKILL.md"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"Skill 包里缺少唯一入口文件 SKILL.md，系统无法定位它的说明与启动方式。",
				"在包的根目录放一个 SKILL.md，说明这个 Skill 干什么、怎么被唤起。"))
		case strings.Contains(f, "evals"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"没有测试文件，系统不知道什么输入算合格，四问的完成度/稳定性无从判定。",
				"在包里补 evals/ 目录，至少写一个测试：给一条真实输入和期望产出。"))
		case strings.Contains(f, "边界模糊"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"不适用条件或人工接管触发点为空：它不知道什么时候该拒绝。",
				"在 Creator 的适用边界里写清：哪些情况不适用；出现什么信号就交回给人。"))
		case strings.Contains(f, "完成标准"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"没有可判断的完成标准，模型无法验证任务是否做完。",
				"写 2-3 条能被验证的标准，例如「输出包含 A/B/C 三个部分且总字数 ≥500」。"))
		case strings.Contains(f, "适用范围不明"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"目标为空，测试不知道它到底帮谁完成什么。",
				"在 Creator 里写清目标：它帮谁、在什么场景下、完成什么。"))
		case strings.Contains(f, "权限过大"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"声明了不可逆操作却没有人工接管覆盖。",
				"为每个不可逆权限补一条 handoff_trigger，出现相关操作前先交回给人确认。"))
		case strings.Contains(f, "依赖失效"):
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f,
				"声明的前置 Skill 不存在或已下线。",
				"在契约里修正或移除失效的前置 Skill。"))
		default:
			sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(f, "准入检查未通过。", "按上面提示补全对应字段。"))
		}
	}
	if ctx.Distill < DistillationThreshold {
		need := DistillationThreshold - ctx.Distill
		sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(
			fmt.Sprintf("蒸馏度 %.2f 未达发布线（还差 %.2f）", ctx.Distill, need),
			"蒸馏度看的是六维证据：真实任务、明确结果、核心流程、关键判断、失败案例、适用边界。",
			"关键判断至少填满两个槽位；把边界与失败案例补进 Creator。"))
	}
	for _, f := range ctx.EvalFails {
		et, _ := f["eval_type"].(string)
		sug.Diagnosis = append(sug.Diagnosis, fixDiagnosis(
			fmt.Sprintf("四问-%s 用例未通过：%s", et, f["input"]),
			fmt.Sprintf("模型判定：%s", f["reason"]),
			"在 Creator 里补全目标、流程、边界、关键判断后重跑该类型测试。"+evalFixHint(et)))
	}
	return sug
}

func fixDiagnosis(item, why, how string) struct {
	Item string `json:"item"`
	Why  string `json:"why"`
	How  string `json:"how"`
} {
	return struct {
		Item string `json:"item"`
		Why  string `json:"why"`
		How  string `json:"how"`
	}{item, why, how}
}

func evalFixHint(evalType string) string {
	switch evalType {
	case EvalDiscoverability:
		return "可发现性：把用户原话里的说法搬进 description。"
	case EvalCompletion:
		return "完成度：补清晰可验证的完成标准，并补关键判断消除流程断点。"
	case EvalStability:
		return "稳定性：补充边界与降级路径，让变体输入有明确应对。"
	case EvalBoundaryStop:
		return "边界停机是安全项必须 100%：把该输入写进不适用条件，并写明交回信号。"
	}
	return ""
}

// gateFixSuggestion POST /api/growth/skills/:id/gate-fix-suggestion
func gateFixSuggestion(c *gin.Context) {
	uid := c.GetInt64("userID")
	skillID := mustInt64(c.Param("id"))
	var owner sql.NullInt64
	var versionID sql.NullInt64
	if err := db.QueryRow(`SELECT owner_id, current_version_id FROM skills WHERE id = ?`, skillID).
		Scan(&owner, &versionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	if !owner.Valid || owner.Int64 != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅创作者可生成建议"})
		return
	}
	if !versionID.Valid {
		c.JSON(http.StatusConflict, gin.H{"error": "还没有可测试的版本"})
		return
	}
	ctx := collectGateFixContext(skillID, versionID.Int64)
	if ctx == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	// 没有失败就不需要开药方
	if len(ctx.AdmFails) == 0 && len(ctx.EvalFails) == 0 && ctx.Distill >= DistillationThreshold {
		c.JSON(http.StatusOK, gin.H{"suggestion": gin.H{
			"diagnosis": []gin.H{{"item": "无失败项", "why": "四问、准入、蒸馏度均已达标", "how": "可以直接发布，或继续打磨"}},
			"draft":     nil,
		}})
		return
	}
	sug, err := generateFixSuggestion(ctx)
	if err != nil {
		sug = fallbackFixSuggestion(ctx)
	}
	c.JSON(http.StatusOK, gin.H{"suggestion": sug})
}

// fixDraftPatch 建议中可写回草稿的部分
type fixDraftPatch struct {
	Goal         *string   `json:"goal"`
	DoneCriteria []string  `json:"done_criteria"`
	Gotchas      []string  `json:"gotchas"`
	Boundary     *struct {
		NotApplicable  []string `json:"not_applicable"`
		HandoffTrigger []string `json:"handoff_trigger"`
		FallbackPath   string   `json:"fallback_path"`
	} `json:"boundary"`
	Judgments []struct {
		Slot          string `json:"slot"`
		TriggerSignal string `json:"trigger_signal"`
		Judgment      string `json:"judgment"`
		Scope         string `json:"scope"`
	} `json:"judgments"`
}

// mergeUnique 合并并去重字符串列表（保留原有顺序，新值追加在后）
func mergeUnique(dst, src []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range append(append([]string{}, dst...), src...) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// gateApplyFix POST /api/growth/skills/:id/gate-apply-fix
// 把建议草稿写回 skill_versions 与 decisions，重播用例，返回最新草稿。
func gateApplyFix(c *gin.Context) {
	uid := c.GetInt64("userID")
	skillID := mustInt64(c.Param("id"))
	var owner sql.NullInt64
	var versionID sql.NullInt64
	if err := db.QueryRow(`SELECT owner_id, current_version_id FROM skills WHERE id = ?`, skillID).
		Scan(&owner, &versionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	if !owner.Valid || owner.Int64 != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅创作者可应用建议"})
		return
	}
	if !versionID.Valid {
		c.JSON(http.StatusConflict, gin.H{"error": "还没有可测试的版本"})
		return
	}

	var in struct {
		Fix fixDraftPatch `json:"fix"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	vid := versionID.Int64
	ver, err := loadSkillVersion(vid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}

	// 1) goal
	if in.Fix.Goal != nil && strings.TrimSpace(*in.Fix.Goal) != "" {
		db.Exec(`UPDATE skill_versions SET goal = ? WHERE id = ?`, strings.TrimSpace(*in.Fix.Goal), vid)
		ver.Goal = strings.TrimSpace(*in.Fix.Goal)
	}
	// 2) done_criteria / gotchas：有建议则整体覆盖
	if len(in.Fix.DoneCriteria) > 0 {
		db.Exec(`UPDATE skill_versions SET done_criteria = ? WHERE id = ?`, jsonOrEmpty(in.Fix.DoneCriteria), vid)
		ver.DoneCriteria = jsonOrEmpty(in.Fix.DoneCriteria)
	}
	if len(in.Fix.Gotchas) > 0 {
		db.Exec(`UPDATE skill_versions SET gotchas = ? WHERE id = ?`, jsonOrEmpty(in.Fix.Gotchas), vid)
		ver.Gotchas = jsonOrEmpty(in.Fix.Gotchas)
	}
	// 3) boundary：与已有内容合并去重，不覆盖人工已写的内容
	if in.Fix.Boundary != nil {
		var b struct {
			NotApplicable  []string `json:"not_applicable"`
			HandoffTrigger []string `json:"handoff_trigger"`
			FallbackPath   string   `json:"fallback_path"`
		}
		json.Unmarshal([]byte(ver.Boundary), &b)
		b.NotApplicable = mergeUnique(b.NotApplicable, in.Fix.Boundary.NotApplicable)
		b.HandoffTrigger = mergeUnique(b.HandoffTrigger, in.Fix.Boundary.HandoffTrigger)
		if strings.TrimSpace(in.Fix.Boundary.FallbackPath) != "" {
			b.FallbackPath = strings.TrimSpace(in.Fix.Boundary.FallbackPath)
		}
		db.Exec(`UPDATE skill_versions SET boundary = ? WHERE id = ?`, jsonOrEmpty(b), vid)
		ver.Boundary = jsonOrEmpty(b)
	}

	// 4) judgments：校验后追加（同槽位同触发信号已存在则跳过）
	var execID int64
	db.QueryRow(`SELECT COALESCE(source_execution_id, 0) FROM skill_versions WHERE id = ?`, vid).Scan(&execID)
	existing := loadDecisions(skillID)
	hasDup := func(slot, trigger string) bool {
		for _, d := range existing {
			if d.InvalidatedAt == nil && d.Slot == slot && d.TriggerSignal == trigger {
				return true
			}
		}
		return false
	}
	applied := 0
	for _, j := range in.Fix.Judgments {
		slot := strings.TrimSpace(j.Slot)
		trigger := strings.TrimSpace(j.TriggerSignal)
		judgment := strings.TrimSpace(j.Judgment)
		scope := strings.TrimSpace(j.Scope)
		if !isValidSlot(slot) || trigger == "" || judgment == "" || scope == "" {
			continue
		}
		if hasDup(slot, trigger) {
			continue
		}
		db.Exec(`INSERT INTO decisions (experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
			counter_example, source_step_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			execID, skillID, slot, trigger, judgment, scope, "", 0)
		applied++
	}

	// 5) 重播用例，让新补的边界/判断/完成标准进入测试
	exec, newVer, decisions, _ := loadDraftParts(vid)
	seedEvalCases(skillID, vid, newVer, decisions, exec)

	respondDraftWithStats(c, vid, -1, -1)
}
