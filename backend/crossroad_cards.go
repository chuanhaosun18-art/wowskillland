// 迷茫期路由器 · A4 卡片匹配 + A5 学长录入
//
// 纪律：
//  A4 是规则匹配不是 AI——按关键词重叠筛卡，宁缺毋滥；匹配不到就进许愿池。
//  A5 学长口述 → LLM 抽取四槽（trigger_context/script/done_criteria/boundary）+ 感受 + 原文存档；
//     无来源即丢弃（source_transcript 必存）；boundary 为空只能存为草稿，不能直接供给。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// crossroadCard 第一步卡（Skill 包的简化形态）
type crossroadCard struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	CreatorID        *int64 `json:"creator_id"`
	TriggerContext   string `json:"trigger_context"`
	Script           string `json:"script"`
	DoneCriteria     string `json:"done_criteria"`
	DecisionPoints   string `json:"decision_points"`
	Boundary         string `json:"boundary"`
	Feeling          string `json:"feeling"`
	SourceTranscript string `json:"source_transcript"`
	Status           string `json:"status"`
	VariantOfCardID  *int64 `json:"variant_of_card_id"`
	VerifiedYes      int    `json:"verified_yes"`
	VerificationCount int   `json:"verification_count"`
}

// ---------- A4 卡片匹配（规则过滤，非 AI） ----------

// matchCards 用假设 + 原始陈述做关键词规则匹配
// POST /api/crossroad/moments/:momentId/match
func matchCards(c *gin.Context) {
	uid := c.GetInt64("userID")
	momentID := c.Param("momentId")

	var owner int64
	var rawText, grade, major string
	if err := db.QueryRow(`SELECT user_id, raw_text FROM moments WHERE id = ?`, momentID).
		Scan(&owner, &rawText); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}
	_ = db.QueryRow(`SELECT COALESCE(grade,''), COALESCE(major,'') FROM users WHERE id = ?`, uid).Scan(&grade, &major)

	// 取该记录的假设作为匹配输入
	hyps := listHypotheses(momentID)
	hypLabels := []string{}
	for _, h := range hyps {
		if lbl, ok := h["label"].(string); ok && lbl != "" {
			hypLabels = append(hypLabels, lbl)
		}
	}

	// 规则过滤：全部已发布卡
	rows, err := db.Query(`SELECT id, title, creator_id, trigger_context, script, done_criteria, decision_points, boundary, feeling, source_transcript, status, variant_of_card_id, verified_yes, verification_count
		FROM cards WHERE status = 'published' ORDER BY verified_yes DESC, verification_count DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()

	rawTerms := keyTerms(rawText)
	var ranked []gin.H
	for rows.Next() {
		var card crossroadCard
		var creatorID, variantID *int64
		if rows.Scan(&card.ID, &card.Title, &creatorID, &card.TriggerContext, &card.Script,
			&card.DoneCriteria, &card.DecisionPoints, &card.Boundary, &card.Feeling,
			&card.SourceTranscript, &card.Status, &variantID, &card.VerifiedYes, &card.VerificationCount) != nil {
			continue
		}
		card.CreatorID = creatorID
		card.VariantOfCardID = variantID

		// 触发条件得分：假设 vs trigger_context 的最大重叠 + 原始陈述兜底
		triggerTerms := keyTerms(card.TriggerContext)
		score := overlapScore(rawTerms, triggerTerms)
		for _, lbl := range hypLabels {
			s := overlapScore(keyTerms(lbl), triggerTerms)
			if s > score {
				score = s
			}
		}
		if score <= 0 {
			continue // 规则过滤：零重叠直接不要
		}
		// 边界冲突过滤：用户处境与 boundary 明显冲突则剔除
		if boundaryConflicts(card.Boundary, grade, major, rawText) {
			continue
		}
		ranked = append(ranked, gin.H{
			"card":    card,
			"score":   score,
			"match_by": "rule",
		})
	}

	// 按分数降序，取前 8
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i]["score"].(float64) > ranked[j]["score"].(float64)
	})
	if len(ranked) > 8 {
		ranked = ranked[:8]
	}

	if len(ranked) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data":    []gin.H{},
			"message": "暂时没有匹配的第一步卡。可以把它放进许愿池，我们会找走过的人来补。",
			"wish":    true,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ranked})
}

// boundaryConflicts 规则判断用户处境是否踩中卡的边界
func boundaryConflicts(boundary, grade, major, rawText string) bool {
	b := strings.TrimSpace(boundary)
	if b == "" {
		return false
	}
	// 只处理明确否定/限定的边界
	if !strings.Contains(b, "不") && !strings.Contains(b, "仅") && !strings.Contains(b, "限") {
		return false
	}
	userCtx := grade + " " + major + " " + rawText
	// 用户处境与边界文本重叠度 ≥ 0.5 视为冲突
	return overlapScore(keyTerms(userCtx), keyTerms(b)) >= 0.5
}

// ---------- A5 学长录入（四槽抽取） ----------

// seniorTranscribeSystemPrompt A5 抽取 prompt：只抽原话里有的，无来源即丢弃
const seniorTranscribeSystemPrompt = `你在把一段学长的口述，整理成一张「第一步卡」。

这张卡要被后来的学弟学妹用，只允许写他真实做过的：
1. trigger_context：什么处境下适用这张卡（基于原话里的真实情况）。
2. script：第一步做什么，具体动作，今天就能动手。
3. done_criteria：做到什么程度算这一步完成。
4. boundary：什么时候不要用这张卡（原话里没提到边界就写「暂无」，这张卡只能存草稿不能发布）。
5. feeling：他当时做这一步时的真实感受（原话里有才写，没有就留空）。
6. title：卡片标题，一句话。

纪律：
- 每一项都必须是原话里出现过的内容，不允许补充、不允许编造。
- script、done_criteria、boundary 尽量直接摘录原话中的原句（允许去掉口语句尾词），不要概括改写。
- 原话没有的信息一律留空或写「暂无」。
- 只输出 JSON，不要 markdown 代码块：
{"title":"","trigger_context":"","script":"","done_criteria":"","boundary":"","feeling":""}`

// transcribeSenior A5 学长录入
// POST /api/crossroad/seniors/transcribe   body {story_text}
func transcribeSenior(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		StoryText string `json:"story_text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	story := strings.TrimSpace(body.StoryText)
	if len([]rune(story)) < 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "口述太短，多讲几句细节吧（至少 30 字）"})
		return
	}

	content, err := callGuideDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: seniorTranscribeSystemPrompt},
		{Role: "user", Content: "学长的口述：\n" + story},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "抽取失败：" + err.Error()})
		return
	}
	var card crossroadCard
	if json.Unmarshal([]byte(extractJSONObject(content)), &card) != nil || strings.TrimSpace(card.Title) == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "抽取异常，请重试"})
		return
	}

	// 无来源即丢弃：所有字段必须能在原话里找到依据（feeling 允许为空）
	if !transcriptSupports(story, card) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "口述里找不到足够的依据来支撑这张卡。请讲得更具体：当时是什么情况、你做了什么、做到什么程度、什么情况下别照做。",
			"dropped": true,
		})
		return
	}

	// boundary 为空 → 只能存草稿，不能直接供给（产品纪律）
	status := "published"
	if strings.TrimSpace(card.Boundary) == "" || card.Boundary == "暂无" {
		status = "draft"
	}

	dpJSON, _ := json.Marshal([]string{})
	res, err := db.Exec(`INSERT INTO cards
		(title, creator_id, trigger_context, script, done_criteria, decision_points, boundary, feeling, source_transcript, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		card.Title, uid, card.TriggerContext, card.Script, card.DoneCriteria, string(dpJSON),
		card.Boundary, card.Feeling, story, status)
	if err != nil {
		log.Printf("crossroad transcribe: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	id, _ := res.LastInsertId()
	card.ID = id
	card.SourceTranscript = story
	card.Status = status

	msg := "卡已收录。"
	if status == "draft" {
		msg = "卡已存为草稿：边界（什么时候别照做）没写清，补上边界后才能被匹配供给。"
	}
	c.JSON(http.StatusOK, gin.H{"card": card, "status": status, "message": msg})
}

// transcriptSupports 校验抽取结果都有原话依据（feeling 允许为空）。
// 依据判定两级：逐字命中 或 关键词重合度 ≥0.6（容忍 LLM 摘录时的少量改写）。
func transcriptSupports(story string, card crossroadCard) bool {
	fields := []string{card.TriggerContext, card.Script, card.DoneCriteria, card.Title}
	for _, f := range fields {
		if strings.TrimSpace(f) == "" || f == "暂无" {
			return false
		}
	}
	hasEvidence := func(f string) bool {
		f = strings.TrimSpace(f)
		if f == "" || f == "暂无" {
			return true // boundary 允许留空（走草稿）；其他字段在上面的非空校验已拦
		}
		if strings.Contains(story, f) {
			return true
		}
		return overlapScore(keyTerms(story), keyTerms(f)) >= 0.6
	}
	// script / done_criteria 至少一段有依据；boundary 若有内容也必须有依据
	found := 0
	for _, f := range []string{card.Script, card.DoneCriteria} {
		if hasEvidence(f) {
			found++
		}
	}
	if found < 1 {
		return false
	}
	b := strings.TrimSpace(card.Boundary)
	return b == "" || b == "暂无" || hasEvidence(b)
}

// patchCard PATCH /api/crossroad/cards/:id  补边界 / 改名等（属主或管理员）
func patchCard(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("id")
	var body struct {
		Boundary *string `json:"boundary"`
		Title    *string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	var owner int64
	if err := db.QueryRow(`SELECT creator_id FROM cards WHERE id = ?`, id).Scan(&owner); err != nil || owner != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "卡不存在或无权修改"})
		return
	}
	var sets []string
	var args []interface{}
	if body.Boundary != nil && strings.TrimSpace(*body.Boundary) != "" {
		sets = append(sets, "boundary = ?")
		args = append(args, strings.TrimSpace(*body.Boundary))
		sets = append(sets, "status = 'published'")
	}
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		sets = append(sets, "title = ?")
		args = append(args, strings.TrimSpace(*body.Title))
	}
	if len(sets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可更新的内容"})
		return
	}
	args = append(args, id)
	if _, err := db.Exec(fmt.Sprintf(`UPDATE cards SET %s WHERE id = ?`, strings.Join(sets, ", ")), args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// listCards GET /api/crossroad/cards?status=published&mine=1
func listCards(c *gin.Context) {
	status := c.DefaultQuery("status", "published")
	uid := c.GetInt64("userID")
	mine := c.Query("mine")

	query := `SELECT id, title, creator_id, trigger_context, script, done_criteria, decision_points, boundary, feeling, source_transcript, status, variant_of_card_id, verified_yes, verification_count
		FROM cards WHERE status = ?`
	args := []interface{}{status}
	if mine == "1" {
		query += ` AND creator_id = ?`
		args = append(args, uid)
	}
	query += ` ORDER BY verified_yes DESC, verification_count DESC, id DESC LIMIT 50`

	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []crossroadCard{}
	for rows.Next() {
		var card crossroadCard
		var creatorID, variantID *int64
		if rows.Scan(&card.ID, &card.Title, &creatorID, &card.TriggerContext, &card.Script,
			&card.DoneCriteria, &card.DecisionPoints, &card.Boundary, &card.Feeling,
			&card.SourceTranscript, &card.Status, &variantID, &card.VerifiedYes, &card.VerificationCount) == nil {
			card.CreatorID = creatorID
			card.VariantOfCardID = variantID
			out = append(out, card)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// getCard GET /api/crossroad/cards/:id
func getCard(c *gin.Context) {
	id := c.Param("id")
	var card crossroadCard
	var creatorID, variantID *int64
	err := db.QueryRow(`SELECT id, title, creator_id, trigger_context, script, done_criteria, decision_points, boundary, feeling, source_transcript, status, variant_of_card_id, verified_yes, verification_count
		FROM cards WHERE id = ?`, id).
		Scan(&card.ID, &card.Title, &creatorID, &card.TriggerContext, &card.Script,
			&card.DoneCriteria, &card.DecisionPoints, &card.Boundary, &card.Feeling,
			&card.SourceTranscript, &card.Status, &variantID, &card.VerifiedYes, &card.VerificationCount)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "卡不存在"})
		return
	}
	card.CreatorID = creatorID
	card.VariantOfCardID = variantID
	c.JSON(http.StatusOK, gin.H{"data": card})
}
