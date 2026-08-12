// 迷茫期路由器 · S3 微尝试 + 验证级反馈 + 许愿池
//
// 主张：选卡即装载成陪跑 Agent——卡就是 Skill 包（script/done_criteria/boundary/feeling），
// 不重新生成人格，只把卡的内容装进陪跑对话。
// 验证级反馈：完成时回答两问（发生了什么 / 有没有卡上没写的情况），verdict 成为信任证据。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// attemptLite attempts 表一行（S3/S4 展示）
type attemptLite struct {
	ID         int64  `json:"id"`
	CardID     int64  `json:"card_id"`
	CardTitle  string `json:"card_title"`
	MomentID   *int64 `json:"moment_id"`
	Status     string `json:"status"`
	Verdict    string `json:"verdict"`
	CounterExample string `json:"counter_example"`
	CreatedAt  string `json:"created_at"`
	FinishedAt *string `json:"finished_at"`
}

// coachSystemPrompt S3 陪跑 Agent：只陪跑，不测评不建议不承诺
const coachSystemPrompt = `你是一个「陪跑教练」，在陪一个人走完一张「第一步卡」。

铁律：
1. 只围绕卡上的 script（第一步做什么）陪跑，不扩展成新任务。
2. 不测评、不建议、不承诺：不评价他行不行、不替他选路、不承诺结果。
3. 对方卡住时，只提醒卡上的 done_criteria（做到什么程度算完成）和 boundary（什么情况下别照做）。
4. 每次回复 ≤ 120 字，口语化，像一个走过这条路的人。
5. 当对方说已经做到 done_criteria，引导他确认完成；当他遇到卡上没写的情况，鼓励他把情况记下来——这会让这张卡变得更好。`

// startAttempt S3 选卡装载：创建微尝试
// POST /api/crossroad/attempts   body {card_id, moment_id?}
func startAttempt(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		CardID   int64 `json:"card_id"`
		MomentID *int64 `json:"moment_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.CardID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	// 卡必须已发布
	var status string
	if err := db.QueryRow(`SELECT status FROM cards WHERE id = ?`, body.CardID).Scan(&status); err != nil || status != "published" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "这张卡还不能被装载"})
		return
	}
	// 已有该卡的 running 尝试则直接返回
	var exist int64
	_ = db.QueryRow(`SELECT id FROM attempts WHERE user_id = ? AND card_id = ? AND status = 'running' LIMIT 1`,
		uid, body.CardID).Scan(&exist)
	if exist > 0 {
		c.JSON(http.StatusOK, gin.H{"attempt_id": exist, "already": true})
		return
	}
	res, err := db.Exec(`INSERT INTO attempts (card_id, user_id, moment_id, status, flow_moments) VALUES (?, ?, ?, 'running', '[]')`,
		body.CardID, uid, body.MomentID)
	if err != nil {
		log.Printf("crossroad attempt: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "装载失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"attempt_id": id, "already": false})
}

// coachTurn 陪跑对话一轮：把卡内容装进 system prompt，对话追加到 flow_moments
// POST /api/crossroad/attempts/:id/chat   body {message}
func coachTurn(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("id")
	var body struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "说点什么吧"})
		return
	}

	// 校验归属 + 取卡内容
	var owner int64
	var cardID int64
	var status string
	var flowJSON string
	if err := db.QueryRow(`SELECT user_id, card_id, status, flow_moments FROM attempts WHERE id = ?`, id).
		Scan(&owner, &cardID, &status, &flowJSON); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "尝试不存在"})
		return
	}
	if status != AttemptRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "这条尝试已结束"})
		return
	}

	var cardTitle, script, doneCriteria, boundary, feeling string
	_ = db.QueryRow(`SELECT title, script, done_criteria, boundary, feeling FROM cards WHERE id = ?`, cardID).
		Scan(&cardTitle, &script, &doneCriteria, &boundary, &feeling)

	// 组装轮次
	var turns []crossroadTurn
	_ = json.Unmarshal([]byte(flowJSON), &turns)
	sys := coachSystemPrompt + fmt.Sprintf(`
【第一步卡】%s
第一步做什么：%s
做到什么程度算完成：%s
什么情况下别照做：%s
他当时的感受：%s`, cardTitle, script, doneCriteria, boundary, feeling)

	msgs := []chatMsg{{Role: "system", Content: sys}}
	for _, t := range turns {
		msgs = append(msgs, chatMsg{Role: "user", Content: t.Q})
		msgs = append(msgs, chatMsg{Role: "assistant", Content: t.A})
	}
	msgs = append(msgs, chatMsg{Role: "user", Content: msg})

	reply, err := callGuideDeepSeek(context.Background(), msgs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "陪跑失败：" + err.Error()})
		return
	}
	reply = strings.TrimSpace(reply)

	turns = append(turns, crossroadTurn{Q: msg, A: reply})
	tj, _ := json.Marshal(turns)
	_, _ = db.Exec(`UPDATE attempts SET flow_moments = ? WHERE id = ?`, string(tj), id)

	c.JSON(http.StatusOK, gin.H{"reply": reply, "card_title": cardTitle, "done_criteria": doneCriteria})
}

// completeAttempt 验证级反馈：回答两问 + verdict
// POST /api/crossroad/attempts/:id/complete   body {verdict, happened, unexpected}
// verdict ∈ done / partial / not_done
func completeAttempt(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("id")
	var body struct {
		Verdict    string `json:"verdict"`
		Happened   string `json:"happened"`   // 第一问：走完这一步发生了什么
		Unexpected string `json:"unexpected"` // 第二问：有没有卡上没写的情况
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	verdict := strings.TrimSpace(body.Verdict)
	if verdict != "done" && verdict != "partial" && verdict != "not_done" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "verdict 必须是 done / partial / not_done"})
		return
	}

	var owner int64
	var cardID int64
	var status string
	if err := db.QueryRow(`SELECT user_id, card_id, status FROM attempts WHERE id = ?`, id).
		Scan(&owner, &cardID, &status); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "尝试不存在"})
		return
	}
	if status != AttemptRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "这条尝试已结束"})
		return
	}

	// 完成尝试
	_, err := db.Exec(`UPDATE attempts SET status = 'finished', verdict = ?, counter_example = ?, finished_at = CURRENT_TIMESTAMP WHERE id = ?`,
		verdict, strings.TrimSpace(body.Unexpected), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}

	// 更新卡验证计数（信任证据）
	_, _ = db.Exec(`UPDATE cards SET verification_count = verification_count + 1, verified_yes = verified_yes + 1 WHERE id = ? AND ? = 'done'`, cardID, verdict)
	if verdict != "done" {
		_, _ = db.Exec(`UPDATE cards SET verification_count = verification_count + 1 WHERE id = ?`, cardID)
	}
	// 有反例则记入 escape_moments
	if strings.TrimSpace(body.Unexpected) != "" {
		var escJSON string
		_ = db.QueryRow(`SELECT escape_moments FROM attempts WHERE id = ?`, id).Scan(&escJSON)
		var esc []string
		_ = json.Unmarshal([]byte(escJSON), &esc)
		esc = append(esc, body.Unexpected)
		ej, _ := json.Marshal(esc)
		_, _ = db.Exec(`UPDATE attempts SET escape_moments = ? WHERE id = ?`, string(ej), id)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "verdict": verdict})
}

// abandonAttempt 放弃也是合法终态
// POST /api/crossroad/attempts/:id/abandon
func abandonAttempt(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("id")
	var owner int64
	var status string
	if err := db.QueryRow(`SELECT user_id, status FROM attempts WHERE id = ?`, id).
		Scan(&owner, &status); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "尝试不存在"})
		return
	}
	if status != AttemptRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "这条尝试已结束"})
		return
	}
	_, _ = db.Exec(`UPDATE attempts SET status = 'abandoned', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// listMyAttempts 我的微尝试
// GET /api/crossroad/attempts
func listMyAttempts(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT a.id, a.card_id, c.title, a.moment_id, a.status, a.verdict, a.counter_example, a.created_at, a.finished_at
		FROM attempts a LEFT JOIN cards c ON c.id = a.card_id
		WHERE a.user_id = ? ORDER BY a.id DESC LIMIT 100`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []attemptLite{}
	for rows.Next() {
		var a attemptLite
		var momentID *int64
		var finishedAt *string
		if rows.Scan(&a.ID, &a.CardID, &a.CardTitle, &momentID, &a.Status, &a.Verdict, &a.CounterExample, &a.CreatedAt, &finishedAt) == nil {
			a.MomentID = momentID
			a.FinishedAt = finishedAt
			out = append(out, a)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// getAttempt GET /api/crossroad/attempts/:id  含陪跑对话
func getAttempt(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("id")
	var owner int64
	var a attemptLite
	var momentID *int64
	var finishedAt *string
	var flowJSON, escJSON string
	err := db.QueryRow(`SELECT user_id, id, card_id, moment_id, status, verdict, counter_example, flow_moments, escape_moments, created_at, finished_at FROM attempts WHERE id = ?`, id).
		Scan(&owner, &a.ID, &a.CardID, &momentID, &a.Status, &a.Verdict, &a.CounterExample, &flowJSON, &escJSON, &a.CreatedAt, &finishedAt)
	if err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "尝试不存在"})
		return
	}
	a.MomentID = momentID
	a.FinishedAt = finishedAt
	_ = db.QueryRow(`SELECT title FROM cards WHERE id = ?`, a.CardID).Scan(&a.CardTitle)
	var turns []crossroadTurn
	_ = json.Unmarshal([]byte(flowJSON), &turns)
	var esc []string
	_ = json.Unmarshal([]byte(escJSON), &esc)
	c.JSON(http.StatusOK, gin.H{"data": a, "turns": turns, "escape_moments": esc})
}

// ---------- 许愿池（没有匹配卡时的出口，反向指导供给） ----------

// addWish POST /api/crossroad/wishes   body {moment_id, direction_label}
func addWish(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		MomentID       int64  `json:"moment_id"`
		DirectionLabel string `json:"direction_label"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.MomentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	label := strings.TrimSpace(body.DirectionLabel)
	if label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "说说你想要什么方向的第一步"})
		return
	}
	_, err := db.Exec(`INSERT INTO wishes (moment_id, user_id, direction_label) VALUES (?, ?, ?)`,
		body.MomentID, uid, label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "许愿失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "愿望已放入许愿池。有人走过这条路时会回来找你。"})
}

// listWishes GET /api/crossroad/wishes  方向标签聚合（供供给方看缺口）
func listWishes(c *gin.Context) {
	rows, err := db.Query(`SELECT id, moment_id, user_id, direction_label, fulfilled, created_at FROM wishes ORDER BY id DESC LIMIT 100`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, momentID, userID int64
		var label string
		var fulfilled int
		var createdAt string
		if rows.Scan(&id, &momentID, &userID, &label, &fulfilled, &createdAt) == nil {
			out = append(out, gin.H{"id": id, "moment_id": momentID, "user_id": userID, "direction_label": label, "fulfilled": fulfilled == 1, "created_at": createdAt})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
