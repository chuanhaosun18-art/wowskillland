// F4 任务工作台：让「做事」发生在平台内。
// 这是全系统可信度、证据与行为信号的唯一来源——任何执行都必须落 execution_steps。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 创建与读取 ----------

// createExecution POST /api/growth/executions
func createExecution(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		TaskIntent string `json:"task_intent"`
		TaskTitle  string `json:"task_title"`
		Material   string `json:"material"` // 用户贴进来的原始材料（选题草稿 / 简历文本）
		Goal       string `json:"goal"`
		SkillID    int64  `json:"skill_id"` // 装载的后端 skill id（真实 skill 包；0 = 未关联）
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if _, ok := AllowedIntents[body.TaskIntent]; !ok {
		// 伪需求或未知类型一律拒绝入库，这是硬约束
		c.JSON(http.StatusBadRequest, gin.H{"error": "该任务类型不允许创建执行记录"})
		return
	}

	// 画像快照：执行时的用户状态，用于后续归因（不是问卷，是执行副产品的起点）
	snapshot := gin.H{}
	if u, err := getUserByID(uid); err == nil {
		snapshot = gin.H{"school": u.School, "major": u.Major, "grade": u.Grade, "ai_level": u.AILevel}
	}
	input := gin.H{"goal": body.Goal, "material": body.Material}

	// skill_id 关联真实 skill 包：非 0 时存入 skill_version_id，陪跑 Agent 会探索该包内容
	var svID interface{}
	if body.SkillID > 0 {
		svID = body.SkillID
	}
	res, err := db.Exec(`INSERT INTO executions (user_id, task_intent, task_title, user_context, input, status, skill_version_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uid, body.TaskIntent, strings.TrimSpace(body.TaskTitle), jsonOrEmpty(snapshot), jsonOrEmpty(input), ExecRunning, svID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	execID, _ := res.LastInsertId()

	// step 0：记录起点。没有 step 的 execution 视为无效
	insertStep(execID, 0, StepAIAction, "任务开始", "", "", "", nil, body.Material, "已接收任务上下文", 0)

	exec, err := loadExecution(execID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": exec})
}

// getExecution GET /api/growth/executions/:id
func getExecution(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	exec, err := loadExecution(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	// 执行日志仅本人可见（PRD 2.3 数据可见性）
	if exec.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "执行日志仅本人可见"})
		return
	}
	autoAbandonIfIdle(exec)
	c.JSON(http.StatusOK, gin.H{"data": exec, "can_distill": canDistill(exec)})
}

// listMyExecutions GET /api/growth/executions
func listMyExecutions(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT id, task_intent, task_title, status, started_at
		FROM executions WHERE user_id = ? ORDER BY id DESC LIMIT 50`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var intent, title, status string
		var started time.Time
		if rows.Scan(&id, &intent, &title, &status, &started) == nil {
			out = append(out, gin.H{
				"id": id, "task_intent": intent, "task_label": AllowedIntents[intent],
				"task_title": title, "status": status, "started_at": started,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// ---------- 推进执行 ----------

// advanceStep LLM 决定的下一步
type advanceStep struct {
	StepType     string   `json:"step_type"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	DecisionSlot string   `json:"decision_slot"`
	Options      []string `json:"options"`
	ToolName     string   `json:"tool_name"`
	ToolInput    string   `json:"tool_input"`
	Done         bool     `json:"done"`
	DoneReason   string   `json:"done_reason"`
}

const workbenchSystemPrompt = `你是大学生成长平台的任务协作教练。你不是聊天机器人，你在带用户一步一步把一件真实的事做完。

你每次只输出「下一步」，三种类型之一：
- ai_action：你直接产出内容（例如给出收窄后的三个候选问题），继续推进。信息不足、任务描述模糊、需要用户补充时，也用 ai_action 直接向用户提问澄清（问题要具体、一次只问最关键的一两个），然后继续对话。
- tool_call：这一步必须查、必须验证，不能靠判断。可用工具：topic_similarity_check（检查选题是否已被大量做过）、jd_keyword_extract（从 JD 里提取要求关键词）。
- human_handoff：仅当任务彻底超出平台适用范围（例如用户需要的是导师的学术判断，或涉及不可控的线下资源）才交回给人，并在交回前说明原因与建议。

硬性要求：
1. 完成标准满足时输出 done=true 并说明依据；不要为了凑轮次继续推进。
2. 信息不足时用 ai_action 向用户提问，绝不 human_handoff 终结任务。
3. 每一步的 content 都要具体到用户今天能动手，禁止「制定计划」「深入思考」这类空话。
4. 如果上下文里有【已装载 Skill 包内容】，用户问到「这个 skill 怎么用 / 它要求什么 / 按它做某一步」时，必须基于包内真实内容（SKILL.md 工作流、scripts 脚本、references 资料）回答，不要编造包内不存在的依赖、命令或步骤。
5. 如果【Skill 包说明】显示「未关联可探索的 zip 包」，说明这个 Skill 只有装载卡材料：基于【用户材料】回答即可，不得要求用户去本地磁盘打开 SKILL.md、scripts/、references/ 等文件，不得臆测包内文件结构。
6. 用中文。

严格只输出 JSON，不要 markdown 代码块：
{"step_type":"","title":"","content":"","decision_slot":"","options":[],"tool_name":"","tool_input":"","done":false,"done_reason":""}`

// advanceExecution POST /api/growth/executions/:id/advance
// 前端每次调用推进一步；LLM 决定下一步类型，遇到关键判断会停下来等用户选。
func advanceExecution(c *gin.Context) {
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
	if exec.Status != ExecRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "该执行已结束，状态：" + exec.Status})
		return
	}

	// 用户在判断点之后的回复：回写为最近一个未选择判断点的选择。
	// 之前用户输入从未进入上下文，导致 LLM 反复追问同一个判断点（死循环）。
	var body struct {
		UserInput string `json:"user_input"`
	}
	c.ShouldBindJSON(&body)
	if input := strings.TrimSpace(body.UserInput); input != "" {
		if recordPendingDecision(id, input) {
			log.Printf("execution %d: 用户输入已记录为判断点选择", id)
			// 刷新内存中的步骤，让 LLM 真正看到用户刚做出的选择
			if refreshed, err := loadExecution(id); err == nil {
				exec = refreshed
			}
		}
	}

	start := time.Now()
	next, err := askNextStep(exec, strings.TrimSpace(body.UserInput))
	if err != nil {
		// 兜底：不推进，但把上一步结果和手动继续入口交回前端（PRD F4 异常处理）
		log.Printf("advance execution %d failed: %v", id, err)
		c.JSON(http.StatusOK, gin.H{
			"mode":    "degraded",
			"message": "这一步没能自动生成，你可以手动写下这一步做了什么，我继续往下走",
			"steps":   exec.Steps,
		})
		return
	}
	latency := int(time.Since(start).Milliseconds())

	nextIdx := len(exec.Steps)

	// 工具调用：确定性操作，必须展示调用了什么、返回了什么；失败要显式标注而不是静默跳过
	if next.StepType == StepToolCall {
		out, ok := runTool(next.ToolName, next.ToolInput, exec)
		insertStep(id, nextIdx, StepToolCall, next.Title, "", "", next.ToolName, &ok, next.ToolInput, out, latency)
		touchExecution(id)
		resp := gin.H{"mode": "tool", "tool_name": next.ToolName, "tool_ok": ok, "output": out, "title": next.Title}
		if !ok {
			resp["warning"] = "未能验证，以下内容仅为模型判断"
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// 判断点（user_decision）已停用：不再停下来等用户选择，直接按普通 AI 步骤落库推进
	if next.StepType == StepUserDecision {
		insertStep(id, nextIdx, StepAIAction, next.Title, "", "", "", nil, "", next.Content, latency)
		touchExecution(id)
		c.JSON(http.StatusOK, gin.H{
			"mode":        "action",
			"step_index":  nextIdx,
			"title":       next.Title,
			"content":     next.Content,
			"done":        next.Done,
			"done_reason": next.DoneReason,
		})
		return
	}

	// 交回人处理：不计为失败；仅提示人工介入，任务保持进行中，用户仍可继续对话（无限轮）
	if next.StepType == StepHumanHandoff {
		insertStep(id, nextIdx, StepHumanHandoff, next.Title, "", "", "", nil, "", next.Content, latency)
		touchExecution(id)
		c.JSON(http.StatusOK, gin.H{"mode": "handoff", "title": next.Title, "content": next.Content})
		return
	}

	// 普通 AI 步骤
	insertStep(id, nextIdx, StepAIAction, next.Title, "", "", "", nil, "", next.Content, latency)
	touchExecution(id)
	c.JSON(http.StatusOK, gin.H{
		"mode":        "action",
		"step_index":  nextIdx,
		"title":       next.Title,
		"content":     next.Content,
		"done":        next.Done,
		"done_reason": next.DoneReason,
	})
}

// askNextStep 组装历史轨迹并让 LLM 给出下一步
// userInput 为用户本轮输入：必须进入上下文，否则 LLM 看不到用户说了什么，会答非所问
func askNextStep(exec *Execution, userInput string) (*advanceStep, error) {
	var sb strings.Builder
	var input struct {
		Goal     string `json:"goal"`
		Material string `json:"material"`
	}
	json.Unmarshal(exec.Input, &input)

	sb.WriteString(fmt.Sprintf("【任务类型】%s\n【任务标题】%s\n【用户目标】%s\n",
		AllowedIntents[exec.TaskIntent], exec.TaskTitle, input.Goal))
	if strings.TrimSpace(input.Material) != "" {
		m := input.Material
		if len([]rune(m)) > 2000 {
			m = string([]rune(m)[:2000]) + "…（已截断）"
		}
		sb.WriteString("【用户材料】\n" + m + "\n")
	}
	// 关联真实 skill 包时，探索 zip 内完整内容（SKILL.md / references / scripts / gotchas / evals），
	// 让 Agent 能准确回答「这个 skill 怎么用」，而不是凭 material 断章取义。
	// 未关联 zip 包（如演示 skill）时明确告知，避免 Agent 要求用户去本地磁盘找文件。
	if exec.SkillVersionID != nil {
		if pkg := exploreSkillPackage(*exec.SkillVersionID); pkg != "" {
			sb.WriteString("\n【已装载 Skill 包内容（来自真实 zip 包）】\n" + pkg + "\n")
		} else {
			sb.WriteString("\n【Skill 包说明】当前 Skill 关联了 zip 包但读取失败，请基于【用户材料】回答。\n")
		}
	} else {
		sb.WriteString("\n【Skill 包说明】当前 Skill 未关联可探索的 zip 包，只有装载卡材料（见【用户材料】）。" +
			"不得要求用户去本地磁盘打开 SKILL.md、scripts/、references/ 等文件——你只能基于【用户材料】回答；" +
			"材料中没有的文件结构信息，不要臆测也不要向用户索要。\n")
	}
	sb.WriteString("\n【已完成的步骤】\n")
	for _, s := range exec.Steps {
		switch s.StepType {
		case StepUserDecision:
			// 历史遗留的判断点步骤仍展示（含用户当时的选向），帮助模型理解上下文
			choice := "（未记录）"
			if len(s.UserChoice) > 0 && string(s.UserChoice) != "" {
				var cj struct {
					Choice string `json:"choice"`
				}
				if json.Unmarshal(s.UserChoice, &cj) == nil && cj.Choice != "" {
					choice = cj.Choice
				} else {
					choice = string(s.UserChoice)
				}
			}
			sb.WriteString(fmt.Sprintf("%d. [历史判断点:%s] %s → 用户选择：%s\n", s.StepIndex, s.DecisionSlot, s.Title, choice))
		case StepToolCall:
			ok := "成功"
			if s.ToolOK != nil && !*s.ToolOK {
				ok = "失败（未能验证）"
			}
			sb.WriteString(fmt.Sprintf("%d. [工具:%s %s] %s\n", s.StepIndex, s.ToolName, ok, truncate(s.Output, 200)))
		default:
			sb.WriteString(fmt.Sprintf("%d. %s：%s\n", s.StepIndex, s.Title, truncate(s.Output, 300)))
		}
	}
	if strings.TrimSpace(userInput) != "" {
		sb.WriteString("\n【用户本轮输入】\n" + userInput + "\n")
	}
	sb.WriteString("\n请根据任务进度和用户本轮输入，给出下一步。")

	msgs := []chatMsg{
		{Role: "system", Content: workbenchSystemPrompt},
		{Role: "user", Content: sb.String()},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := callGuideDeepSeek(context.Background(), msgs)
		if err != nil {
			lastErr = err
			continue
		}
		var step advanceStep
		if err := json.Unmarshal([]byte(extractJSON(raw)), &step); err != nil {
			lastErr = err
			continue
		}
		if step.StepType == "" {
			lastErr = fmt.Errorf("模型未给出 step_type")
			continue
		}
		return &step, nil
	}
	return nil, lastErr
}

// ---------- 记录用户动作 ----------

// recordDecision POST /api/growth/executions/:id/decide
// 用户在关键判断点做出选择。这一步产生的是最有价值的数据。
func recordDecision(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		StepIndex int    `json:"step_index"`
		Choice    string `json:"choice"`
		Note      string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Choice) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "choice is required"})
		return
	}
	var owner int64
	if err := db.QueryRow(`SELECT user_id FROM executions WHERE id = ?`, id).Scan(&owner); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	choice := gin.H{"choice": body.Choice, "note": body.Note}
	if _, err := db.Exec(`UPDATE execution_steps SET user_choice = ? WHERE execution_id = ? AND step_index = ?`,
		jsonOrEmpty(choice), id, body.StepIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	touchExecution(id)
	c.JSON(http.StatusOK, gin.H{"message": "recorded"})
}

// recordPendingDecision 把用户的最新输入回写为最近一个未选择判断点的选择。
// 返回 true 表示找到了待选择的判断点并已记录。
func recordPendingDecision(execID int64, input string) bool {
	var stepIdx int
	err := db.QueryRow(`SELECT step_index FROM execution_steps
		WHERE execution_id = ? AND step_type = ? AND (user_choice IS NULL OR user_choice = '')
		ORDER BY step_index DESC LIMIT 1`, execID, StepUserDecision).Scan(&stepIdx)
	if err != nil {
		return false
	}
	choice := gin.H{"choice": input}
	if _, err := db.Exec(`UPDATE execution_steps SET user_choice = ? WHERE execution_id = ? AND step_index = ?`,
		jsonOrEmpty(choice), execID, stepIdx); err != nil {
		log.Printf("record pending decision exec %d step %d: %v", execID, stepIdx, err)
		return false
	}
	return true
}

// recordEdit POST /api/growth/executions/:id/edit
// 用户改写了某一步的 AI 输出：记录人工修正率，这是替代「成功率」的行为信号之一。
func recordEdit(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		StepIndex    int    `json:"step_index"`
		EditedOutput string `json:"edited_output"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var owner int64
	var original string
	if err := db.QueryRow(`SELECT e.user_id, s.output FROM executions e
		JOIN execution_steps s ON s.execution_id = e.id AND s.step_index = ?
		WHERE e.id = ?`, body.StepIndex, id).Scan(&owner, &original); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "step not found"})
		return
	}

	ratio := editRatio(original, body.EditedOutput)
	db.Exec(`UPDATE execution_steps SET output = ? WHERE execution_id = ? AND step_index = ?`,
		body.EditedOutput, id, body.StepIndex)
	// 取全执行的最大修正率作为该次执行的 correction_ratio
	db.Exec(`UPDATE executions SET correction_ratio = MAX(correction_ratio, ?) WHERE id = ?`, ratio, id)
	touchExecution(id)
	c.JSON(http.StatusOK, gin.H{"correction_ratio": ratio})
}

// ---------- 结束执行 ----------

// completeExecution POST /api/growth/executions/:id/complete
// 计算 completion_signal：用「是否把产物用出去了」替代「是否最终成功」。
func completeExecution(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Exported      bool   `json:"exported"` // 是否导出/提交了产物
		FinalArtifact string `json:"final_artifact"`
	}
	c.ShouldBindJSON(&body)

	exec, err := loadExecution(id)
	if err != nil || exec.UserID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	if exec.Status != ExecRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "该执行已结束"})
		return
	}

	var input struct {
		Material string `json:"material"`
	}
	json.Unmarshal(exec.Input, &input)
	delta := artifactDelta(input.Material, body.FinalArtifact)

	signal := gin.H{
		"exported":         body.Exported,
		"artifact_delta":   delta,
		"manual_rework":    exec.CorrectionRatio > 0.5,
		"reused_within_7d": false, // 由定时任务回填
	}
	db.Exec(`UPDATE executions SET status = ?, output = ?, completion_signal = ?, ended_at = CURRENT_TIMESTAMP WHERE id = ?`,
		ExecCompleted, body.FinalArtifact, jsonOrEmpty(signal), id)

	// 完成后若该次执行含关键判断，则可以固化
	exec, _ = loadExecution(id)
	resp := gin.H{
		"data":              exec,
		"completion_signal": signal,
		"can_distill":       canDistill(exec),
		"distill_hint":      distillHint(exec),
	}
	// 反向通道：告诉用户这件事在哪条长路上的第几周。
	// 这是任务态用户知道编排态存在的唯一入口——没有它，用户做完一件事就走了（v1.2 第 3 条）。
	if s := suggestOrchestration(exec.TaskIntent); s != nil {
		resp["orchestration_suggestion"] = s
	}
	c.JSON(http.StatusOK, resp)
}

// abandonExecution POST /api/growth/executions/:id/abandon
// 放弃是产品信号（说明 Skill 不好或不匹配），与 failed（系统故障）严格区分。
func abandonExecution(c *gin.Context) {
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
	step := len(exec.Steps)
	db.Exec(`UPDATE executions SET status = ?, abandoned_at_step = ?, ended_at = CURRENT_TIMESTAMP WHERE id = ?`,
		ExecAbandoned, step, id)
	c.JSON(http.StatusOK, gin.H{"message": "abandoned", "abandoned_at_step": step})
}

// ---------- 工具 ----------

// runTool 执行确定性操作。返回 (输出, 是否成功)。
// 失败时不静默跳过，由调用方标注「未能验证」。
func runTool(name, input string, exec *Execution) (string, bool) {
	switch name {
	case "topic_similarity_check":
		// 用站内语料做一次朴素相似度检查：真实项目里应替换为学术检索 API
		rows, err := db.Query(`SELECT utterance FROM description_corpus WHERE task_intent = ? LIMIT 50`, exec.TaskIntent)
		if err != nil {
			return "检索不可用", false
		}
		defer rows.Close()
		hits := []string{}
		key := keyTerms(input)
		for rows.Next() {
			var u string
			if rows.Scan(&u) != nil {
				continue
			}
			if overlapScore(key, keyTerms(u)) >= 0.34 {
				hits = append(hits, u)
			}
		}
		if len(hits) == 0 {
			return "站内没有检索到高度相似的表述，说明这个切入角度还比较少见（注意：这只是站内语料，不等于学术查重）。", true
		}
		return "站内检索到相似表述 " + strconv.Itoa(len(hits)) + " 条，说明这是高频问题：\n- " +
			strings.Join(hits[:min(len(hits), 3)], "\n- ") +
			"\n（这只是站内语料，不等于学术查重）", true
	case "jd_keyword_extract":
		terms := keyTerms(input)
		if len(terms) == 0 {
			return "没有可提取的关键词", false
		}
		return "从 JD 中提取到要求关键词：" + strings.Join(terms[:min(len(terms), 12)], "、"), true
	}
	return "未知工具：" + name, false
}

// insertStep 写一条轨迹。所有执行都必须走这里，不允许只落 execution。
func insertStep(execID int64, idx int, stepType, title, slot, choice, toolName string, toolOK *bool, input, output string, latency int) {
	var okVal interface{}
	if toolOK != nil {
		if *toolOK {
			okVal = 1
		} else {
			okVal = 0
		}
	}
	if _, err := db.Exec(`INSERT INTO execution_steps
		(execution_id, step_index, step_type, title, decision_slot, user_choice, tool_name, tool_ok, input, output, latency_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execID, idx, stepType, title, slot, choice, toolName, okVal, input, output, latency); err != nil {
		log.Printf("insert step failed exec=%d idx=%d: %v", execID, idx, err)
	}
}

func touchExecution(id int64) {
	db.Exec(`UPDATE executions SET last_active_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
}

// loadExecution 读执行及其全部轨迹
func loadExecution(id int64) (*Execution, error) {
	var e Execution
	var userCtx, input, output, signal string
	var svID sql.NullInt64
	var abandonedAt sql.NullInt64
	var endedAt sql.NullTime
	err := db.QueryRow(`SELECT id, user_id, skill_version_id, task_intent, task_title,
		user_context, input, output, status, completion_signal, correction_ratio,
		abandoned_at_step, started_at, ended_at FROM executions WHERE id = ?`, id).
		Scan(&e.ID, &e.UserID, &svID, &e.TaskIntent, &e.TaskTitle,
			&userCtx, &input, &output, &e.Status, &signal, &e.CorrectionRatio,
			&abandonedAt, &e.StartedAt, &endedAt)
	if err != nil {
		return nil, err
	}
	if svID.Valid {
		v := svID.Int64
		e.SkillVersionID = &v
	}
	if abandonedAt.Valid {
		v := int(abandonedAt.Int64)
		e.AbandonedAtStep = &v
	}
	e.EndedAt = nullTime(endedAt)
	e.UserContext = rawOrDefault(userCtx, "{}")
	e.Input = rawOrDefault(input, "{}")
	if strings.TrimSpace(output) != "" {
		e.Output = json.RawMessage(jsonOrEmpty(output))
	}
	e.CompletionSignal = rawOrDefault(signal, "{}")

	rows, err := db.Query(`SELECT id, execution_id, step_index, step_type, title, decision_slot,
		user_choice, tool_name, tool_ok, input, output, latency_ms, created_at
		FROM execution_steps WHERE execution_id = ? ORDER BY step_index`, id)
	if err != nil {
		return &e, nil
	}
	defer rows.Close()
	for rows.Next() {
		var s ExecutionStep
		var choice, toolName string
		var toolOK sql.NullInt64
		if err := rows.Scan(&s.ID, &s.ExecutionID, &s.StepIndex, &s.StepType, &s.Title, &s.DecisionSlot,
			&choice, &toolName, &toolOK, &s.Input, &s.Output, &s.LatencyMS, &s.CreatedAt); err != nil {
			continue
		}
		s.ToolName = toolName
		if toolOK.Valid {
			b := toolOK.Int64 == 1
			s.ToolOK = &b
		}
		if strings.TrimSpace(choice) != "" && json.Valid([]byte(choice)) {
			s.UserChoice = json.RawMessage(choice)
		}
		e.Steps = append(e.Steps, s)
	}
	return &e, nil
}

// autoAbandonIfIdle 静置超时自动判定放弃（PRD F4 验收第 4 条）
func autoAbandonIfIdle(e *Execution) {
	if e.Status != ExecRunning {
		return
	}
	var last time.Time
	if err := db.QueryRow(`SELECT last_active_at FROM executions WHERE id = ?`, e.ID).Scan(&last); err != nil {
		return
	}
	if time.Since(last) > time.Duration(AbandonIdleMinutes)*time.Minute {
		step := len(e.Steps)
		db.Exec(`UPDATE executions SET status = ?, abandoned_at_step = ?, ended_at = CURRENT_TIMESTAMP WHERE id = ?`,
			ExecAbandoned, step, e.ID)
		e.Status = ExecAbandoned
		e.AbandonedAtStep = &step
	}
}

// canDistill 一次执行是否可以固化为 Skill。
//
// 硬约束：0 个关键判断的执行不可固化——没有判断就没有可蒸馏的内容。
// 但这条只对「可生产类」intent 生效：模拟面试、面试复盘、内容脚本本质是练习，
// 一次练习里没有「什么时候切换策略」这种判断，对它们要求判断会让用户永远卡住
// 且不知道自己做错了什么（PRD v1.2 第 4 条）。
func canDistill(e *Execution) bool {
	if e.Status != ExecCompleted {
		return false
	}
	if !isProductive(e.TaskIntent) {
		return false // 只消费类不产出 Skill，也不提示固化
	}
	return countDecidedSteps(e) >= 1
}

func distillHint(e *Execution) string {
	if e.Status != ExecCompleted {
		return "任务完成后才能固化"
	}
	if !isProductive(e.TaskIntent) {
		return "这类任务是练习，不产出方法——练完就好，不需要固化。"
	}
	if countDecidedSteps(e) < 1 {
		return "这次执行没有产生任何关键判断，没有可以沉淀的方法。下次遇到岔路口时停下来选一次，就能固化了。"
	}
	return ""
}

func countDecidedSteps(e *Execution) int {
	n := 0
	for _, s := range e.Steps {
		if s.StepType == StepUserDecision && len(s.UserChoice) > 0 {
			n++
		}
	}
	return n
}

func isValidSlot(s string) bool {
	for _, d := range DecisionSlots {
		if d.Slot == s {
			return true
		}
	}
	return false
}

func slotPrompt(s string) string {
	for _, d := range DecisionSlots {
		if d.Slot == s {
			return d.Prompt
		}
	}
	return ""
}

// editRatio 编辑距离 / 原文长度，用于人工修正率
func editRatio(original, edited string) float64 {
	a := []rune(original)
	b := []rune(edited)
	if len(a) == 0 {
		if len(b) == 0 {
			return 0
		}
		return 1
	}
	// 长文本截断，避免 O(n*m) 爆炸
	const maxLen = 1200
	if len(a) > maxLen {
		a = a[:maxLen]
	}
	if len(b) > maxLen {
		b = b[:maxLen]
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		copy(prev, cur)
	}
	return clamp01(float64(prev[len(b)]) / float64(len(a)))
}

// artifactDelta 产物相对初始版本的变化量
func artifactDelta(before, after string) float64 {
	if strings.TrimSpace(after) == "" {
		return 0
	}
	if strings.TrimSpace(before) == "" {
		return 1
	}
	return editRatio(before, after)
}

// keyTerms 朴素中文关键词切分：取 2 字滑窗 + 英文单词。够用于站内相似度，不追求分词准确。
func keyTerms(s string) []string {
	s = strings.ToLower(s)
	var latin []string
	var cjk []rune
	cur := strings.Builder{}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			cur.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			if cur.Len() > 0 {
				latin = append(latin, cur.String())
				cur.Reset()
			}
			cjk = append(cjk, r)
		default:
			if cur.Len() > 0 {
				latin = append(latin, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		latin = append(latin, cur.String())
	}
	terms := latin
	for i := 0; i+1 < len(cjk); i++ {
		terms = append(terms, string(cjk[i:i+2]))
	}
	return terms
}

// overlapScore 两组关键词的重合比例（以较短一组为分母）
func overlapScore(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, t := range b {
		set[t] = true
	}
	hit := 0
	seen := map[string]bool{}
	for _, t := range a {
		if seen[t] {
			continue
		}
		seen[t] = true
		if set[t] {
			hit++
		}
	}
	denom := len(seen)
	if denom == 0 {
		return 0
	}
	return float64(hit) / float64(denom)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
