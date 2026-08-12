// 迷茫期路由器 · A2 迷茫访谈 + A3 假设生成
//
// 硬约束（答辩即卖点）：
//  1. 访谈 ≤5 轮：只问经历（试过什么/卡在哪/手头有什么/每周多少时间），绝不问评价（哪个好/该不该选）。
//  2. 假设必须附访谈原话作为依据（evidence_quote）；找不到原话的假设直接丢弃——和 A5「无来源即丢弃」同一纪律。
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

// CrossroadInterviewMaxRound 访谈轮数上限（产品硬约束 ≤5）
const CrossroadInterviewMaxRound = 5

// crossroadTurn 访谈中的一轮 {q, a}
type crossroadTurn struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// crossroadInterviewSnapshot 访谈快照（前端 S2 展示）
type crossroadInterviewSnapshot struct {
	MomentID   int64           `json:"moment_id"`
	RawText    string          `json:"raw_text"`
	Turns      []crossroadTurn `json:"turns"`
	RoundCount int             `json:"round_count"`
	Ready      bool            `json:"ready"`
}

// interviewSystemPrompt A2 迷茫访谈 prompt：只问经历，不问评价
const interviewSystemPrompt = `你在做一场「迷茫访谈」。对方刚说过他此刻的「不知道」。

铁律：
1. 只问经历：过去试过什么、现在走到哪一步、卡在哪、手头有什么材料、每周能投入几个半天。绝不问「你觉得哪个好」「你更倾向哪个」「你更合适哪条路」这类评价题。不测评、不建议、不承诺。
2. 每次只问 1 个问题，一句话问完，口语化。
3. 对方已经说过的信息不要重复问。
4. 当你已经能大致勾勒出「他在哪、卡在哪、手头有什么」时，输出 {"done": true}，不要再问。
5. 只输出 JSON：{"question": "下一问", "done": false} 或 {"done": true}。
6. 不允许输出 JSON 以外的任何内容。`

// nextInterviewTurn A2 访谈推进（无状态轮询：客户端把上一轮的问题和回答带回，服务端追加归档）
// POST /api/crossroad/interviews/:momentId/turn   body {question, answer}
func nextInterviewTurn(c *gin.Context) {
	uid := c.GetInt64("userID")
	momentID := c.Param("momentId")

	var body struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	_ = c.ShouldBindJSON(&body)
	answer := strings.TrimSpace(body.Answer)

	// 属主校验 + 取原始陈述
	var owner int64
	var rawText string
	if err := db.QueryRow(`SELECT user_id, raw_text FROM moments WHERE id = ?`, momentID).
		Scan(&owner, &rawText); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}

	// 读访谈快照（首次则空）
	var turnsJSON, contextJSON string
	var roundCount, ready int
	err := db.QueryRow(`SELECT context, turns, round_count, ready FROM interviews WHERE moment_id = ?`, momentID).
		Scan(&contextJSON, &turnsJSON, &roundCount, &ready)
	if err != nil {
		contextJSON = "{}"
		turnsJSON = "[]"
	}
	var turns []crossroadTurn
	_ = json.Unmarshal([]byte(turnsJSON), &turns)

	// 到达上限：不再问，直接 ready
	if roundCount >= CrossroadInterviewMaxRound {
		saveInterview(momentID, contextJSON, turns, roundCount, true)
		c.JSON(http.StatusOK, gin.H{
			"ready":       true,
			"round_limit": true,
			"message":     "问到这里就够了。可以生成方向假设了。",
			"snapshot":    snapshotOf(momentID, rawText, turns, roundCount, true),
		})
		return
	}

	// 有回答则归档一轮（首轮 answer 为空，直接开问）
	if answer != "" {
		turns = append(turns, crossroadTurn{Q: body.Question, A: answer})
		roundCount++
	}

	// 组装 LLM 输入：原始陈述 + 完整轮次
	var sb strings.Builder
	sb.WriteString("【他说的不知道】" + rawText + "\n")
	if len(turns) > 0 {
		sb.WriteString("【访谈记录】\n")
		for i, t := range turns {
			sb.WriteString(fmt.Sprintf("%d. 问：%s\n   答：%s\n", i+1, t.Q, t.A))
		}
	}
	sb.WriteString(fmt.Sprintf("【已进行 %d/%d 轮】", roundCount, CrossroadInterviewMaxRound))

	content, lerr := callGuideDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: interviewSystemPrompt},
		{Role: "user", Content: sb.String()},
	})
	var res struct {
		Question string `json:"question"`
		Done     bool   `json:"done"`
	}
	parseOK := lerr == nil && json.Unmarshal([]byte(extractJSONObject(content)), &res) == nil

	// 到上限或 LLM 判定完成：ready
	if res.Done || roundCount >= CrossroadInterviewMaxRound {
		saveInterview(momentID, contextJSON, turns, roundCount, true)
		c.JSON(http.StatusOK, gin.H{
			"ready":       true,
			"message":     "访谈够了，去生成方向假设吧。",
			"snapshot":    snapshotOf(momentID, rawText, turns, roundCount, true),
			"degraded":    !parseOK,
		})
		return
	}

	// 兜底问题（LLM 失败时退化为固定四问）
	if !parseOK || strings.TrimSpace(res.Question) == "" {
		res.Question = fallbackInterviewQuestion(roundCount)
	}

	saveInterview(momentID, contextJSON, turns, roundCount, false)
	c.JSON(http.StatusOK, gin.H{
		"ready":      false,
		"question":   res.Question,
		"round":      roundCount + 1,
		"snapshot":   snapshotOf(momentID, rawText, turns, roundCount, false),
		"degraded":   !parseOK,
	})
}

// fallbackInterviewQuestion 兜底：不依赖 LLM 的固定四问
func fallbackInterviewQuestion(round int) string {
	qs := []string{
		"这件事你之前自己试过什么吗？试到哪一步了？",
		"现在具体卡在哪一步？缺什么？",
		"手头已经有什么材料、资源或线索？",
		"接下来这段时间，你每周大概能投入几个半天？",
	}
	if round < len(qs) {
		return qs[round]
	}
	return "还有没有别的你试过但没说的？"
}

// getInterviewSnapshot GET /api/crossroad/interviews/:momentId
func getInterviewSnapshot(c *gin.Context) {
	uid := c.GetInt64("userID")
	momentID := c.Param("momentId")
	var owner int64
	var rawText string
	if err := db.QueryRow(`SELECT user_id, raw_text FROM moments WHERE id = ?`, momentID).
		Scan(&owner, &rawText); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}
	var turnsJSON string
	var roundCount, ready int
	_ = db.QueryRow(`SELECT turns, round_count, ready FROM interviews WHERE moment_id = ?`, momentID).
		Scan(&turnsJSON, &roundCount, &ready)
	var turns []crossroadTurn
	_ = json.Unmarshal([]byte(turnsJSON), &turns)
	c.JSON(http.StatusOK, gin.H{"data": snapshotOf(momentID, rawText, turns, roundCount, ready == 1)})
}

// snapshotOf 组装访谈快照
func snapshotOf(momentID string, rawText string, turns []crossroadTurn, roundCount int, ready bool) crossroadInterviewSnapshot {
	return crossroadInterviewSnapshot{
		MomentID:   int64FromString(momentID),
		RawText:    rawText,
		Turns:      turns,
		RoundCount: roundCount,
		Ready:      ready,
	}
}

// int64FromString 路由参数安全转换
func int64FromString(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

// saveInterview 落库访谈快照（新增或更新）
func saveInterview(momentID string, contextJSON string, turns []crossroadTurn, roundCount int, ready bool) {
	tj, _ := json.Marshal(turns)
	readyInt := 0
	if ready {
		readyInt = 1
	}
	_, err := db.Exec(`INSERT INTO interviews (moment_id, context, turns, round_count, ready) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(moment_id) DO UPDATE SET turns=excluded.turns, round_count=excluded.round_count, ready=excluded.ready, updated_at=CURRENT_TIMESTAMP`,
		momentID, contextJSON, string(tj), roundCount, readyInt)
	if err != nil {
		log.Printf("crossroad interview save: %v", err)
	}
}

// ---------- A3 假设生成 ----------

// hypothesisSystemPrompt A3 方向假设 prompt：假设必须引用访谈原话
const hypothesisSystemPrompt = `你在把一段迷茫访谈记录，提炼成「方向假设」。

铁律：
1. 只允许产出假设，不允许产出建议。假设是「他可能想朝这个方向走」的判断，不是「你应该去做 X」的指导。
2. 每条假设必须附 evidence_quote：访谈记录里逐字出现的原话。找不到原话支撑的假设不写。
3. 最多 3 条。
4. 只输出 JSON 数组：[{"label": "假设一句话", "evidence_quote": "访谈原话"}]
5. 不允许输出 JSON 以外的任何内容。`

// generateHypotheses A3 假设生成（原话必须可溯源，否则丢弃）
// POST /api/crossroad/moments/:momentId/hypotheses
func generateHypotheses(c *gin.Context) {
	uid := c.GetInt64("userID")
	momentID := c.Param("momentId")

	var owner int64
	var rawText string
	if err := db.QueryRow(`SELECT user_id, raw_text FROM moments WHERE id = ?`, momentID).
		Scan(&owner, &rawText); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}

	// 取访谈记录
	var turnsJSON string
	_ = db.QueryRow(`SELECT turns FROM interviews WHERE moment_id = ?`, momentID).Scan(&turnsJSON)
	var turns []crossroadTurn
	_ = json.Unmarshal([]byte(turnsJSON), &turns)

	if len(turns) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "还没有访谈记录，先完成访谈再生成假设"})
		return
	}

	// 组装输入
	var sb strings.Builder
	sb.WriteString("【他说的不知道】" + rawText + "\n")
	sb.WriteString("【访谈记录】\n")
	for i, t := range turns {
		sb.WriteString(fmt.Sprintf("%d. 问：%s\n   答：%s\n", i+1, t.Q, t.A))
	}

	content, err := callGuideDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: hypothesisSystemPrompt},
		{Role: "user", Content: sb.String()},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "假设生成失败：" + err.Error()})
		return
	}

	var hyps []struct {
		Label         string `json:"label"`
		EvidenceQuote string `json:"evidence_quote"`
	}
	// 容忍 [ 前的杂讯
	body := extractJSONArray(content)
	if json.Unmarshal([]byte(body), &hyps) != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "假设生成异常，请重试"})
		return
	}

	// 纪律校验：evidence_quote 必须能在访谈原话里溯源（逐字或关键词重合 ≥0.6，容忍摘录时的少量改写）
	allAnswers := ""
	for _, t := range turns {
		allAnswers += t.A + "\n"
	}
	answerTerms := keyTerms(allAnswers)
	saved := 0
	for _, h := range hyps {
		label := strings.TrimSpace(h.Label)
		quote := strings.TrimSpace(h.EvidenceQuote)
		if label == "" || quote == "" {
			continue
		}
		hasSource := strings.Contains(allAnswers, quote) ||
			(overlapScore(answerTerms, keyTerms(quote)) >= 0.6)
		if !hasSource {
			continue // 无来源即丢弃（A3 同款纪律）
		}
		if _, err := db.Exec(`INSERT INTO hypotheses (moment_id, label, evidence_quote) VALUES (?, ?, ?)`,
			momentID, label, quote); err == nil {
			saved++
		}
	}

	if saved == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}, "dropped": len(hyps), "message": "没有能通过原话溯源校验的假设。访谈可能还太浅，再聊两轮试试。"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listHypotheses(momentID), "dropped": len(hyps) - saved})
}

// listHypotheses 某个迷茫记录的全部假设
func listHypotheses(momentID string) []gin.H {
	rows, err := db.Query(`SELECT id, label, evidence_quote, card_id FROM hypotheses WHERE moment_id = ? ORDER BY id`, momentID)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var label, quote string
		var cardID *int64
		if rows.Scan(&id, &label, &quote, &cardID) == nil {
			out = append(out, gin.H{"id": id, "label": label, "evidence_quote": quote, "card_id": cardID})
		}
	}
	return out
}

// extractJSONArray 从 LLM 输出里截取第一个 [...] 数组（容忍 markdown 代码块）
func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			return s[i : j+1]
		}
	}
	return s
}
