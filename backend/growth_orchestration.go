// F17 编排态：对于保研、考研、出国、求职季这类长周期方向性需求，
// 交付的不是一份产物，而是一份「知道接下来几周该做什么」的编排。
//
// 三条硬约束（PRD 不可妥协清单 11–14）：
//  1. 没有真实 Path 作为来源就不生成——orchestration_items.source_path_id 为 NOT NULL。
//  2. 任何界面与响应都不得出现成功率、通过率这类结果预测数字，只给绝对人数与真实分叉。
//  3. 不可控项必须独立分组，不允许伪装成待办。
//  4. retrospective 来源的 Path 不给耗时分布与卡点统计，且必须标注来自回忆整理。
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

// ---------- 结构 ----------

type pathNodeLite struct {
	ID           int64  `json:"id"`
	NodeIndex    int    `json:"node_index"`
	Label        string `json:"label"`
	TaskIntent   string `json:"task_intent,omitempty"`
	WeekOffset   int    `json:"week_offset"`
	Controllable bool   `json:"controllable"`
}

type sourcePath struct {
	ID          int64          `json:"id"`
	GoalLabel   string         `json:"goal_label"`
	Provenance  string         `json:"provenance"`
	WalkedCount int            `json:"walked_count"`
	Branch      json.RawMessage `json:"branch_summary"`
	Nodes       []pathNodeLite `json:"nodes"`
}

// provenanceNote retrospective 必须显式告知用户信息的局限
func provenanceNote(p string) string {
	if p == ProvenanceRetrospective {
		return "这条路是学长回忆整理的，顺序可信，时间估算仅供参考。"
	}
	return ""
}

// ---------- F17.2 第一步：前置检查 ----------

// probeOrchestration POST /api/growth/orchestrations/probe
// 编排态的第一道门：没有可用的来源 Path 就不进访谈、不生成编排。
func probeOrchestration(c *gin.Context) {
	var body struct {
		Utterance           string `json:"utterance"`
		OrchestrationIntent string `json:"orchestration_intent"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	intent := strings.TrimSpace(body.OrchestrationIntent)
	if _, ok := OrchestrationIntents[intent]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知的编排方向"})
		return
	}

	paths := loadSourcePaths(intent)
	if len(paths) == 0 {
		// 关键一幕：没人走过就不编。原话入语料库，成为供给缺口信号。
		if strings.TrimSpace(body.Utterance) != "" {
			db.Exec(`INSERT INTO description_corpus (utterance, source, task_intent)
				VALUES (?, 'orch_no_source_path', ?)`, body.Utterance, intent)
		}
		log.Printf("orchestration probe: no source path for intent=%s", intent)
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"message":   "这条路目前还没有人在这里走完过。我不会凭空给你排一份——那种时间表看着像样，但对你没用。",
			"options": []gin.H{
				{"label": "看看相邻方向有没有人走过", "action": "browse_paths"},
				{"label": "我自己开始走，做完第一件事", "action": "create_execution"},
			},
		})
		return
	}

	total := 0
	for _, p := range paths {
		total += p.WalkedCount
	}
	c.JSON(http.StatusOK, gin.H{
		"available":         true,
		"orchestration_intent": intent,
		"label":             OrchestrationIntents[intent],
		"source_paths":      paths,
		"walked_total":      total,
		"provenance_note":   provenanceNote(paths[0].Provenance),
		"next":              "开始上下文访谈",
	})
}

// loadSourcePaths 取可用的来源 Path（节点数达标才算可用）
func loadSourcePaths(intent string) []sourcePath {
	rows, err := db.Query(`SELECT id, goal_label, provenance, walked_count, COALESCE(branch_summary,'{}')
		FROM paths WHERE orchestration_intent = ? ORDER BY walked_count DESC`, intent)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// 单连接模式下循环体内不能再发起 db 查询（会死锁并永久占用唯一连接），
	// 所以先把所有 path 的节点一次性预取到内存 map。
	nodesByPath := map[int64][]pathNodeLite{}
	nr, err := db.Query(`SELECT path_id, id, node_index, label, COALESCE(task_intent,''), week_offset, controllable
		FROM path_nodes ORDER BY path_id, node_index`)
	if err == nil {
		for nr.Next() {
			var pathID int64
			var n pathNodeLite
			var ctrl int
			if nr.Scan(&pathID, &n.ID, &n.NodeIndex, &n.Label, &n.TaskIntent, &n.WeekOffset, &ctrl) == nil {
				n.Controllable = ctrl == 1
				nodesByPath[pathID] = append(nodesByPath[pathID], n)
			}
		}
		nr.Close()
	}

	out := []sourcePath{}
	for rows.Next() {
		var p sourcePath
		var branch string
		if rows.Scan(&p.ID, &p.GoalLabel, &p.Provenance, &p.WalkedCount, &branch) != nil {
			continue
		}
		p.Branch = rawOrDefault(branch, "{}")
		p.Nodes = nodesByPath[p.ID]
		if len(p.Nodes) < PathMinNodesForOrch {
			continue // 节点太少撑不起编排
		}
		out = append(out, p)
	}
	return out
}

// loadPathNodes 读节点。retrospective 不返回耗时与卡点（v1.2 第 1 条）。
func loadPathNodes(pathID int64, provenance string) []pathNodeLite {
	rows, err := db.Query(`SELECT id, node_index, label, COALESCE(task_intent,''), week_offset, controllable
		FROM path_nodes WHERE path_id = ? ORDER BY node_index`, pathID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []pathNodeLite{}
	for rows.Next() {
		var n pathNodeLite
		var ctrl int
		if rows.Scan(&n.ID, &n.NodeIndex, &n.Label, &n.TaskIntent, &n.WeekOffset, &ctrl) != nil {
			continue
		}
		n.Controllable = ctrl == 1
		out = append(out, n)
	}
	return out
}

// ---------- F17.2 第二步：上下文访谈（P7 阶段一） ----------

const orchInterviewPrompt = `你在帮一个大学生把长周期目标排成可执行的编排。现在是采集上下文的阶段，不是聊天。

你必须采齐这四项才能开始编排：
- target：具体目标（哪一类、什么时间点，例如「今年夏令营，本校计算机」）
- current_progress：当前进度（材料到什么程度、关键结果出没出）
- weekly_hours：每周大概能投入多少时间
- hard_constraints：硬约束（绩点排名、语言成绩、实验室情况等）

规则：
1. 每轮最多问 2 个问题，问完就停。
2. 已经采到的字段不要重复问。
3. 禁止询问情绪状态、心理状况、家庭情况。
4. 禁止在这个阶段给任何建议或判断，你只负责问。
5. 四项采齐时 ready_to_generate 置 true 且 missing 为空数组。

严格只输出 JSON，不要 markdown 代码块：
{"questions":[],"collected":{"target":"","current_progress":"","weekly_hours":"","hard_constraints":""},"missing":[],"ready_to_generate":false}`

type interviewResult struct {
	Questions       []string          `json:"questions"`
	Collected       map[string]string `json:"collected"`
	Missing         []string          `json:"missing"`
	ReadyToGenerate bool              `json:"ready_to_generate"`
}

// interviewOrchestration POST /api/growth/orchestrations/interview
func interviewOrchestration(c *gin.Context) {
	var body struct {
		OrchestrationIntent string            `json:"orchestration_intent"`
		Round               int               `json:"round"`
		Answer              string            `json:"answer"`
		Collected           map[string]string `json:"collected"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Round >= OrchInterviewMaxRound {
		// 到上限就别再问了，问下去会变成问卷
		c.JSON(http.StatusOK, gin.H{
			"ready_to_generate": len(missingFields(body.Collected)) == 0,
			"collected":         body.Collected,
			"missing":           missingFields(body.Collected),
			"message":           "问到这里就够了。缺的部分我会在编排里标注为待确认。",
			"round_limit_hit":   true,
		})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【编排方向】%s\n【已采集】%s\n【本轮用户回答】%s\n【剩余轮数】%d\n",
		OrchestrationIntents[body.OrchestrationIntent], jsonOrEmpty(body.Collected),
		body.Answer, OrchInterviewMaxRound-body.Round))

	msgs := []chatMsg{
		{Role: "system", Content: orchInterviewPrompt},
		{Role: "user", Content: sb.String()},
	}
	raw, err := callGuideDeepSeek(context.Background(), msgs)
	if err != nil {
		// 兜底：退化为固定四问，不阻塞流程
		c.JSON(http.StatusOK, gin.H{
			"questions": fixedInterviewQuestions(body.Collected),
			"collected": body.Collected,
			"missing":   missingFields(body.Collected),
			"ready_to_generate": false,
			"degraded":  true,
		})
		return
	}
	var res interviewResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"questions": fixedInterviewQuestions(body.Collected),
			"collected": body.Collected,
			"missing":   missingFields(body.Collected),
			"ready_to_generate": false,
			"degraded":  true,
		})
		return
	}
	// 以后端判定为准，不信模型自称的 ready
	merged := mergeCollected(body.Collected, res.Collected)
	miss := missingFields(merged)
	c.JSON(http.StatusOK, gin.H{
		"questions":         res.Questions,
		"collected":         merged,
		"missing":           miss,
		"ready_to_generate": len(miss) == 0,
		"round":             body.Round + 1,
	})
}

var requiredContextFields = []string{"target", "current_progress", "weekly_hours", "hard_constraints"}

func missingFields(c map[string]string) []string {
	out := []string{}
	for _, f := range requiredContextFields {
		if c == nil || strings.TrimSpace(c[f]) == "" {
			out = append(out, f)
		}
	}
	return out
}

func mergeCollected(old, new map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range old {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	for k, v := range new {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

func fixedInterviewQuestions(collected map[string]string) []string {
	m := map[string]string{
		"target":           "你的具体目标是什么？（哪一类、什么时间点）",
		"current_progress": "现在进行到哪一步了？有哪些材料已经有了？",
		"weekly_hours":     "接下来这段时间，你每周大概能投入几个半天？",
		"hard_constraints": "有哪些是你改不了的条件？（比如绩点排名、语言成绩、实验室情况）",
	}
	out := []string{}
	for _, f := range missingFields(collected) {
		out = append(out, m[f])
		if len(out) >= 2 {
			break
		}
	}
	return out
}

// ---------- F17.2 第三步：生成编排（P7 阶段二） ----------

const orchGeneratePrompt = `你在把别人真实走过的路，适配成这个学生自己的编排。

铁律，违反任何一条这次生成就作废：
1. 每个 item 必须有 source_node_index，指向输入里真实存在的节点编号。宁可少排，不许编。
2. 禁止输出任何百分比。不许写成功率、通过率、录取率，也不许写「大概有七成机会」这类话。
3. 不可控的事必须放进 uncontrollable 数组，不许伪装成待办项。名额、排名、别人是否回复，都属于不可控。
4. why_now 必须解释「为什么这件事在这一周」，不能是任务标题的复述。
5. 不许回答「该不该」。即使用户在访谈里问了，也只排不评。
6. title 必须是今天就能动手的具体动作。

严格只输出 JSON，不要 markdown 代码块：
{"horizon_weeks":8,"items":[{"week_index":1,"title":"","why_now":"","due_date":"","controllable":true,"source_node_index":1,"linked_task_intent":""}],"uncontrollable":[{"title":"","note":""}]}`

type generatedOrch struct {
	HorizonWeeks int `json:"horizon_weeks"`
	Items        []struct {
		WeekIndex        int    `json:"week_index"`
		Title            string `json:"title"`
		WhyNow           string `json:"why_now"`
		DueDate          string `json:"due_date"`
		Controllable     bool   `json:"controllable"`
		SourceNodeIndex  int    `json:"source_node_index"`
		LinkedTaskIntent string `json:"linked_task_intent"`
	} `json:"items"`
	Uncontrollable []struct {
		Title string `json:"title"`
		Note  string `json:"note"`
	} `json:"uncontrollable"`
}

// createOrchestration POST /api/growth/orchestrations
func createOrchestration(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		OrchestrationIntent string            `json:"orchestration_intent"`
		GoalLabel           string            `json:"goal_label"`
		Context             map[string]string `json:"context"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if _, ok := OrchestrationIntents[body.OrchestrationIntent]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知的编排方向"})
		return
	}
	// 上下文没采齐就不生成——缺上下文的编排必然是废纸
	if miss := missingFields(body.Context); len(miss) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "还差一点上下文，补齐才能排",
			"missing": miss,
		})
		return
	}

	paths := loadSourcePaths(body.OrchestrationIntent)
	if len(paths) == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "这条路还没有人走完过，我不会凭空生成编排",
		})
		return
	}

	gen, err := generateOrchestration(body.OrchestrationIntent, body.Context, paths)
	if err != nil {
		// 兜底：不给半成品编排，直接把来源 Path 的原始顺序摊开
		log.Printf("orchestration generate failed: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"mode":    "raw_path",
			"message": "这是别人走过的原始顺序，我没能帮你适配到你的情况。",
			"paths":   paths,
		})
		return
	}

	horizon := gen.HorizonWeeks
	if horizon < OrchMinWeeks {
		horizon = OrchMinWeeks
	}
	if horizon > OrchMaxWeeks {
		horizon = OrchMaxWeeks
	}

	pathIDs := []int64{}
	for _, p := range paths {
		pathIDs = append(pathIDs, p.ID)
	}
	branchText := branchSummaryText(paths)

	res, err := db.Exec(`INSERT INTO orchestrations (user_id, orchestration_intent, goal_label,
		context, horizon_weeks, status, branch_summary, source_path_ids)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, body.OrchestrationIntent, strings.TrimSpace(body.GoalLabel),
		jsonOrEmpty(body.Context), horizon, OrchDrafting, branchText, jsonOrEmpty(pathIDs))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	orchID, _ := res.LastInsertId()

	// 建立 node_index → node id 的映射，用于校验来源真实存在
	nodeByIndex := map[int]pathNodeLite{}
	primary := paths[0]
	for _, n := range primary.Nodes {
		nodeByIndex[n.NodeIndex] = n
	}

	kept, dropped := 0, 0
	for _, it := range gen.Items {
		node, ok := nodeByIndex[it.SourceNodeIndex]
		if !ok {
			dropped++ // 无来源节点，丢弃（硬约束）
			continue
		}
		if strings.TrimSpace(it.Title) == "" {
			dropped++
			continue
		}
		week := it.WeekIndex
		if week < 1 {
			week = 1
		}
		if week > horizon {
			week = horizon
		}
		ctrl := 1
		if !it.Controllable || !node.Controllable {
			ctrl = 0
		}
		linked := it.LinkedTaskIntent
		if _, ok := AllowedIntents[linked]; !ok {
			linked = node.TaskIntent
		}
		if _, ok := AllowedIntents[linked]; !ok {
			linked = ""
		}
		db.Exec(`INSERT INTO orchestration_items (orchestration_id, week_index, title, why_now,
			due_date, controllable, source_path_id, source_path_node_id, linked_task_intent, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			orchID, week, strings.TrimSpace(it.Title), strings.TrimSpace(it.WhyNow),
			strings.TrimSpace(it.DueDate), ctrl, primary.ID, node.ID, linked, ItemTodo)
		kept++
	}

	// 不可控项：来自模型的 + Path 上本来就标了不可控的节点，合并去重后独立存
	for _, u := range gen.Uncontrollable {
		if strings.TrimSpace(u.Title) == "" {
			continue
		}
		db.Exec(`INSERT INTO orchestration_items (orchestration_id, week_index, title, why_now,
			controllable, source_path_id, status) VALUES (?, ?, ?, ?, 0, ?, ?)`,
			orchID, 0, strings.TrimSpace(u.Title), strings.TrimSpace(u.Note), primary.ID, ItemTodo)
	}

	if kept < OrchMinItems {
		// 有效项太少不构成编排，直接作废，不留半成品
		db.Exec(`DELETE FROM orchestration_items WHERE orchestration_id = ?`, orchID)
		db.Exec(`DELETE FROM orchestrations WHERE id = ?`, orchID)
		c.JSON(http.StatusOK, gin.H{
			"mode":    "raw_path",
			"message": "适配出来的有效步骤太少，我不给你一个凑数的编排。这是别人走过的原始顺序。",
			"paths":   paths,
		})
		return
	}
	log.Printf("orchestration %d created: kept=%d dropped=%d", orchID, kept, dropped)
	respondOrchestration(c, orchID, gin.H{"kept": kept, "dropped": dropped,
		"note": "没有来源节点的步骤已被丢弃，不会出现在编排里"})
}

// generateOrchestration 调 LLM 做适配
func generateOrchestration(intent string, ctx map[string]string, paths []sourcePath) (*generatedOrch, error) {
	p := paths[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【编排方向】%s\n【我的上下文】\n", OrchestrationIntents[intent]))
	for _, f := range requiredContextFields {
		sb.WriteString("- " + f + "：" + ctx[f] + "\n")
	}
	sb.WriteString(fmt.Sprintf("\n【来源 Path】%s（%d 人走过，来源类型 %s）\n合法的 source_node_index 为 1 到 %d：\n",
		p.GoalLabel, p.WalkedCount, p.Provenance, len(p.Nodes)))
	for _, n := range p.Nodes {
		ctrlText := "可控"
		if !n.Controllable {
			ctrlText = "不可控"
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s（原路径约在第 %d 周）\n",
			n.NodeIndex, ctrlText, n.Label, n.WeekOffset))
	}
	if p.Provenance == ProvenanceRetrospective {
		sb.WriteString("\n注意：这条 Path 来自回忆整理，周次是粗略估计。你可以调整顺序间距，但不要编造精确耗时。\n")
	}

	msgs := []chatMsg{
		{Role: "system", Content: orchGeneratePrompt},
		{Role: "user", Content: sb.String()},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := callGuideDeepSeek(context.Background(), msgs)
		if err != nil {
			lastErr = err
			continue
		}
		var g generatedOrch
		if err := json.Unmarshal([]byte(extractJSON(raw)), &g); err != nil {
			lastErr = err
			continue
		}
		if len(g.Items) == 0 {
			lastErr = fmt.Errorf("模型未给出任何步骤")
			continue
		}
		return &g, nil
	}
	return nil, lastErr
}

// branchSummaryText 如实呈现分叉。只给绝对人数，不给比率。
func branchSummaryText(paths []sourcePath) string {
	p := paths[0]
	var b struct {
		Walked   int `json:"walked"`
		Branches []struct {
			Label string `json:"label"`
			Count int    `json:"count"`
			Note  string `json:"note"`
		} `json:"branches"`
		Note string `json:"note"`
	}
	json.Unmarshal(p.Branch, &b)
	if len(b.Branches) == 0 {
		return fmt.Sprintf("这条编排来自 %d 个人走过的路。", p.WalkedCount)
	}
	parts := []string{}
	for _, br := range b.Branches {
		s := fmt.Sprintf("%d 个%s", br.Count, br.Label)
		if strings.TrimSpace(br.Note) != "" {
			s += "（" + br.Note + "）"
		}
		parts = append(parts, s)
	}
	out := fmt.Sprintf("这条编排来自 %d 个人走过的路：%s。", b.Walked, strings.Join(parts, "，"))
	if strings.TrimSpace(b.Note) != "" {
		out += b.Note + "。"
	}
	return out
}

// ---------- 读取与采纳 ----------

// getOrchestration GET /api/growth/orchestrations/:id
func getOrchestration(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	uid := c.GetInt64("userID")
	var owner int64
	if err := db.QueryRow(`SELECT user_id FROM orchestrations WHERE id = ?`, id).Scan(&owner); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "orchestration not found"})
		return
	}
	if owner != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "编排仅本人可见"})
		return
	}
	autoPauseIfStale(id)
	respondOrchestration(c, id, nil)
}

// listMyOrchestrations GET /api/growth/orchestrations
func listMyOrchestrations(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT id, orchestration_intent, goal_label, status, horizon_weeks, created_at
		FROM orchestrations WHERE user_id = ? ORDER BY id DESC`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var intent, goal, status string
		var weeks int
		var at time.Time
		if rows.Scan(&id, &intent, &goal, &status, &weeks, &at) == nil {
			out = append(out, gin.H{
				"id": id, "orchestration_intent": intent,
				"label": OrchestrationIntents[intent], "goal_label": goal,
				"status": status, "horizon_weeks": weeks, "created_at": at,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// adoptOrchestration POST /api/growth/orchestrations/:id/adopt
func adoptOrchestration(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var owner int64
	var status string
	if err := db.QueryRow(`SELECT user_id, status FROM orchestrations WHERE id = ?`, id).
		Scan(&owner, &status); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "orchestration not found"})
		return
	}
	if status != OrchDrafting {
		c.JSON(http.StatusConflict, gin.H{"error": "该编排已不是草稿状态"})
		return
	}
	// 采纳前校验：可控项数量与不可控项是否已标注
	var controllableCount int
	db.QueryRow(`SELECT COUNT(*) FROM orchestration_items WHERE orchestration_id = ? AND controllable = 1`, id).
		Scan(&controllableCount)
	if controllableCount < OrchMinItems {
		c.JSON(http.StatusConflict, gin.H{"error": "可执行步骤不足，不能采纳"})
		return
	}
	db.Exec(`UPDATE orchestrations SET status = ?, adopted_at = CURRENT_TIMESTAMP WHERE id = ?`, OrchActive, id)
	respondOrchestration(c, id, nil)
}

// updateOrchItem PATCH /api/growth/orchestrations/:id/items/:itemId
func updateOrchItem(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err1 := strconv.ParseInt(c.Param("id"), 10, 64)
	itemID, err2 := strconv.ParseInt(c.Param("itemId"), 10, 64)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Status    string `json:"status"`
		DueDate   string `json:"due_date"`
		WeekIndex *int   `json:"week_index"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var owner int64
	if err := db.QueryRow(`SELECT user_id FROM orchestrations WHERE id = ?`, id).Scan(&owner); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "orchestration not found"})
		return
	}
	if body.Status != "" {
		if !validItemStatus(body.Status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的状态"})
			return
		}
		if body.Status == ItemDone {
			db.Exec(`UPDATE orchestration_items SET status = ?, done_at = CURRENT_TIMESTAMP
				WHERE id = ? AND orchestration_id = ?`, ItemDone, itemID, id)
		} else {
			db.Exec(`UPDATE orchestration_items SET status = ?, done_at = NULL
				WHERE id = ? AND orchestration_id = ?`, body.Status, itemID, id)
		}
	}
	if body.DueDate != "" {
		db.Exec(`UPDATE orchestration_items SET due_date = ? WHERE id = ? AND orchestration_id = ?`,
			body.DueDate, itemID, id)
	}
	if body.WeekIndex != nil {
		db.Exec(`UPDATE orchestration_items SET week_index = ? WHERE id = ? AND orchestration_id = ?`,
			*body.WeekIndex, itemID, id)
	}
	respondOrchestration(c, id, nil)
}

func validItemStatus(s string) bool {
	switch s {
	case ItemTodo, ItemDone, ItemSkipped, ItemExpired:
		return true
	}
	return false
}

// ---------- 周复核 ----------

// reviewOrchestration POST /api/growth/orchestrations/:id/reviews
// 周复核产出的是「节奏有没有跟上」这个行为信号——编排唯一诚实的有效性证据。
func reviewOrchestration(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		WeekIndex int    `json:"week_index"`
		Replanned bool   `json:"replanned"`
		Note      string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var owner int64
	var status string
	if err := db.QueryRow(`SELECT user_id, status FROM orchestrations WHERE id = ?`, id).
		Scan(&owner, &status); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "orchestration not found"})
		return
	}

	// 过期判定：有截止日且已过，仍未完成的，转 expired
	today := time.Now().Format("2006-01-02")
	db.Exec(`UPDATE orchestration_items SET status = ?
		WHERE orchestration_id = ? AND status = ? AND due_date != '' AND due_date < ?`,
		ItemExpired, id, ItemTodo, today)

	// 注意：SUM 在零行时返回 NULL，直接扫进 int 会报错，所以必须套 COALESCE。
	// 这里的 WHERE 带 week_index 过滤，完全可能匹配到零行。
	var done, total, expired int
	db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
		COUNT(*),
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0)
		FROM orchestration_items
		WHERE orchestration_id = ? AND week_index = ? AND controllable = 1`,
		ItemDone, ItemExpired, id, body.WeekIndex).Scan(&done, &total, &expired)

	replan := 0
	if body.Replanned {
		replan = 1
	}
	db.Exec(`INSERT INTO orchestration_reviews (orchestration_id, week_index, done_count,
		total_count, expired_count, replanned, note) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, body.WeekIndex, done, total, expired, replan, strings.TrimSpace(body.Note))
	db.Exec(`UPDATE orchestrations SET last_review_at = CURRENT_TIMESTAMP, status = ? WHERE id = ?`,
		OrchActive, id)

	// 全部可控项做完则视为完成
	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM orchestration_items
		WHERE orchestration_id = ? AND controllable = 1 AND status IN (?, ?)`,
		id, ItemTodo, ItemExpired).Scan(&remaining)
	if remaining == 0 {
		db.Exec(`UPDATE orchestrations SET status = ? WHERE id = ?`, OrchCompleted, id)
	}

	rate := 0.0
	if total > 0 {
		rate = float64(done) / float64(total)
	}
	c.JSON(http.StatusOK, gin.H{
		"week_index":    body.WeekIndex,
		"done_count":    done,
		"total_count":   total,
		"expired_count": expired,
		"done_rate":     rate,
		"encourage":     reviewEncourage(rate, expired),
	})
}

// reviewEncourage 复核后的一句话。刻意不夸张、不制造焦虑，也不给结果承诺。
func reviewEncourage(rate float64, expired int) string {
	switch {
	case expired > 0:
		return fmt.Sprintf("有 %d 项过了截止日。过期不等于失败，但要决定是补上还是删掉——留在列表里最消耗人。", expired)
	case rate >= 0.8:
		return "这周跟得很稳。下周照这个节奏就行。"
	case rate >= 0.4:
		return "完成了一半多。剩下的要么这周补，要么直接往后挪，不要挂着。"
	default:
		return "这周没怎么推进。如果是计划排得太满，现在就改——编排是给你用的，不是用来让你有负担的。"
	}
}

// autoPauseIfStale 连续数周未复核则转 paused。不算失败，只提醒一次。
func autoPauseIfStale(id int64) {
	var status string
	var last sql.NullTime
	var adopted sql.NullTime
	if err := db.QueryRow(`SELECT status, last_review_at, adopted_at FROM orchestrations WHERE id = ?`, id).
		Scan(&status, &last, &adopted); err != nil {
		return
	}
	if status != OrchActive {
		return
	}
	ref := last
	if !ref.Valid {
		ref = adopted
	}
	if !ref.Valid {
		return
	}
	if time.Since(ref.Time) > time.Duration(OrchPauseAfterWeeks)*7*24*time.Hour {
		db.Exec(`UPDATE orchestrations SET status = ? WHERE id = ?`, OrchPaused, id)
	}
}

// ---------- 反向通道：任务 → 编排（v1.2 第 3 条） ----------

// suggestOrchestration 一次执行完成后，看它在哪条 Path 的哪个节点上。
// 这是让任务态用户知道编排态存在的唯一入口，也是漏斗上原来那个洞。
func suggestOrchestration(taskIntent string) gin.H {
	if taskIntent == "" {
		return nil
	}
	var pathID, nodeID int64
	var goalLabel, intent, nodeLabel string
	var weekOffset, walked int
	err := db.QueryRow(`SELECT p.id, p.goal_label, p.orchestration_intent, p.walked_count,
		n.id, n.label, n.week_offset
		FROM path_nodes n JOIN paths p ON p.id = n.path_id
		WHERE n.task_intent = ? AND p.orchestration_intent != ''
		ORDER BY p.walked_count DESC LIMIT 1`, taskIntent).
		Scan(&pathID, &goalLabel, &intent, &walked, &nodeID, &nodeLabel, &weekOffset)
	if err != nil {
		return nil
	}
	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM path_nodes WHERE path_id = ? AND node_index >
		(SELECT node_index FROM path_nodes WHERE id = ?)`, pathID, nodeID).Scan(&remaining)

	return gin.H{
		"orchestration_intent": intent,
		"label":                OrchestrationIntents[intent],
		"message": fmt.Sprintf("你刚做完的这件事，在「%s」这条路上大约是第 %d 周的动作。这条路后面还有 %d 个节点。",
			OrchestrationIntents[intent], weekOffset, remaining),
		"walked_count": walked,
		"cta":          "看完整编排",
	}
}

// ---------- 响应组装 ----------

func respondOrchestration(c *gin.Context, id int64, extra gin.H) {
	var o struct {
		ID           int64
		Intent       string
		GoalLabel    string
		Context      string
		Horizon      int
		Status       string
		Branch       string
		SourcePathIDs string
	}
	var adopted, lastReview sql.NullTime
	if err := db.QueryRow(`SELECT id, orchestration_intent, goal_label, context, horizon_weeks,
		status, COALESCE(branch_summary,''), COALESCE(source_path_ids,'[]'), adopted_at, last_review_at
		FROM orchestrations WHERE id = ?`, id).
		Scan(&o.ID, &o.Intent, &o.GoalLabel, &o.Context, &o.Horizon, &o.Status,
			&o.Branch, &o.SourcePathIDs, &adopted, &lastReview); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "orchestration not found"})
		return
	}

	// 可控项按周分组；不可控项独立成组（硬约束：不许混在待办里）
	weeks := map[int][]gin.H{}
	uncontrollable := []gin.H{}
	rows, err := db.Query(`SELECT id, week_index, title, why_now, COALESCE(due_date,''),
		COALESCE(deadline_source,''), controllable, source_path_node_id,
		COALESCE(linked_task_intent,''), status FROM orchestration_items
		WHERE orchestration_id = ? ORDER BY week_index, id`, id)
	if err == nil {
		for rows.Next() {
			var itemID int64
			var week, ctrl int
			var title, why, due, dsrc, linked, status string
			var nodeID sql.NullInt64
			if rows.Scan(&itemID, &week, &title, &why, &due, &dsrc, &ctrl, &nodeID, &linked, &status) != nil {
				continue
			}
			item := gin.H{
				"id": itemID, "title": title, "why_now": why, "due_date": due,
				"deadline_source": dsrc, "status": status,
				"linked_task_intent": linked,
			}
			if linked != "" {
				item["linked_task_label"] = AllowedIntents[linked]
			}
			if nodeID.Valid {
				item["source_path_node_id"] = nodeID.Int64
			}
			if ctrl == 1 {
				item["week_index"] = week
				weeks[week] = append(weeks[week], item)
			} else {
				uncontrollable = append(uncontrollable, item)
			}
		}
		rows.Close()
	}

	weekList := []gin.H{}
	for w := 1; w <= o.Horizon; w++ {
		if items, ok := weeks[w]; ok {
			weekList = append(weekList, gin.H{"week_index": w, "items": items})
		}
	}

	reviews := []gin.H{}
	r2, err := db.Query(`SELECT week_index, done_count, total_count, expired_count, replanned, reviewed_at
		FROM orchestration_reviews WHERE orchestration_id = ? ORDER BY id`, id)
	if err == nil {
		for r2.Next() {
			var w, d, t, e, rp int
			var at time.Time
			if r2.Scan(&w, &d, &t, &e, &rp, &at) == nil {
				reviews = append(reviews, gin.H{"week_index": w, "done_count": d,
					"total_count": t, "expired_count": e, "replanned": rp == 1, "reviewed_at": at})
			}
		}
		r2.Close()
	}

	// 来源 Path 的可信度必须一起返回，界面要标注
	var pathIDs []int64
	json.Unmarshal([]byte(o.SourcePathIDs), &pathIDs)
	sources := []gin.H{}
	for _, pid := range pathIDs {
		var goal, prov string
		var walked int
		if db.QueryRow(`SELECT goal_label, provenance, walked_count FROM paths WHERE id = ?`, pid).
			Scan(&goal, &prov, &walked) == nil {
			sources = append(sources, gin.H{
				"path_id": pid, "goal_label": goal, "provenance": prov,
				"walked_count": walked, "provenance_note": provenanceNote(prov),
			})
		}
	}

	out := gin.H{
		"id":                   o.ID,
		"orchestration_intent": o.Intent,
		"label":                OrchestrationIntents[o.Intent],
		"goal_label":           o.GoalLabel,
		"context":              rawOrDefault(o.Context, "{}"),
		"horizon_weeks":        o.Horizon,
		"status":               o.Status,
		"branch_summary":       o.Branch,
		"weeks":                weekList,
		"uncontrollable":       uncontrollable,
		"reviews":              reviews,
		"source_paths":         sources,
		"adopted_at":           nullTime(adopted),
		"last_review_at":       nullTime(lastReview),
		// 明确告诉前端这里不给结果预测，别去找成功率字段
		"no_outcome_promise": true,
		"promise_note":       "这份编排只承诺顺序和节奏，不承诺结果。名额与他人表现不由你的准备决定。",
	}
	for k, v := range extra {
		out[k] = v
	}
	c.JSON(http.StatusOK, out)
}
