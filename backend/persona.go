// 虚拟自己（Persona）：把用户与 LLM 的引导对话保留、蒸馏成"虚拟自己"，
// 其他用户可选择与真人在线聊天（见 chat.go），或与"虚拟自己"聊天（LLM 扮演）。
// 权限：用户可开关虚拟自己（默认开启）；访客可决定聊天记录是否对对方可见。
//
// 单连接死锁注意：SQLite SetMaxOpenConns(1) 下，任何 db.Query 返回的 rows 在 Close 前
// 都占用唯一连接，期间不得发起其他 db 调用。所有函数均遵循"先查完、关掉 rows、再查下一个"。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 建表 ----------

func initPersonaSchema() {
	schema := `
CREATE TABLE IF NOT EXISTS persona_conversations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  title TEXT DEFAULT '',
  messages TEXT DEFAULT '[]',
  status TEXT DEFAULT 'active',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS personas (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL UNIQUE,
  conversation_id INTEGER,
  persona_text TEXT DEFAULT '',
  chat_enabled INTEGER NOT NULL DEFAULT 1,
  chat_count INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS persona_chats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  persona_id INTEGER NOT NULL,
  visitor_id INTEGER NOT NULL,
  allow_owner_view INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS persona_chat_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  role TEXT NOT NULL,
  content TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_pconv_user ON persona_conversations(user_id);
CREATE INDEX IF NOT EXISTS idx_pchat_persona ON persona_chats(persona_id);
CREATE INDEX IF NOT EXISTS idx_pchat_visitor ON persona_chats(visitor_id);
CREATE INDEX IF NOT EXISTS idx_pmsg_chat ON persona_chat_messages(chat_id);
`
	if _, err := db.Exec(schema); err != nil {
		panic("init persona schema failed: " + err.Error())
	}
}

// ---------- 引导对话保存（guideChat 每轮自动调用） ----------

// saveGuideConversation 将引导对话消息保存到 persona_conversations：
// 1) 带有效 conversation_id 且归属正确 → 更新该条；
// 2) 否则更新该用户最近一条 active 对话；
// 3) 再无则新建。返回会话 id。
func saveGuideConversation(uid int64, convID int64, msgs []chatMsg) int64 {
	raw, _ := json.Marshal(msgs)
	if convID > 0 {
		var owner int64
		if err := db.QueryRow(`SELECT user_id FROM persona_conversations WHERE id = ?`, convID).Scan(&owner); err == nil && owner == uid {
			db.Exec(`UPDATE persona_conversations SET messages = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(raw), convID)
			return convID
		}
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM persona_conversations WHERE user_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1`, uid).Scan(&id); err == nil {
		db.Exec(`UPDATE persona_conversations SET messages = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(raw), id)
		return id
	}
	res, err := db.Exec(`INSERT INTO persona_conversations (user_id, messages) VALUES (?, ?)`, uid, string(raw))
	if err == nil {
		id, _ = res.LastInsertId()
	}
	return id
}

// saveConversation POST /api/persona/conversations（需登录）——前端主动保存对话（guideChat 已自动保存，此为兜底）
type saveConversationReq struct {
	ConversationID int64     `json:"conversation_id"`
	Title          string    `json:"title"`
	Messages       []chatMsg `json:"messages"`
}

func saveConversation(c *gin.Context) {
	uid := c.GetInt64("userID")
	var req saveConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages is required"})
		return
	}
	id := saveGuideConversation(uid, req.ConversationID, req.Messages)
	if req.Title != "" {
		db.Exec(`UPDATE persona_conversations SET title = ? WHERE id = ?`, req.Title, id)
	}
	c.JSON(http.StatusOK, gin.H{"conversation_id": id})
}

// ---------- 蒸馏 ----------

const distillSystemPrompt = `你是 SkillHub 平台的「虚拟自己」蒸馏器。你的任务是把用户的一段对话记录和他提供的个人画像信息，蒸馏成一份第一人称的"虚拟自己"人格设定文本，用于让 AI 模仿这位用户与别人聊天。

要求：
1. 全程用第一人称"我"书写，语气、口吻、用词习惯尽量贴近用户本人在对话中的说话方式。
2. 内容要覆盖：我是谁（学校/年级/专业/个人简介）、我擅长什么、我知道什么（对话中提到的知识、经验、方法）、我对相关话题的观点与立场、我的说话风格。
3. 只能基于给定的对话记录和画像信息，不得编造其中不存在的事实、经历或观点；信息不足的部分如实略过。
4. 总长度控制在 600 字以内。直接输出设定文本本身，不要任何前缀说明、不要标题。`

func aiLevelLabel(level string) string {
	switch level {
	case "never":
		return "没用过AI工具"
	case "beginner":
		return "AI初学者"
	case "intermediate":
		return "AI进阶使用者"
	case "advanced":
		return "AI高手"
	}
	return "未知"
}

// buildDialogText 将对话消息拼成"用户/AI"可读文本，最多取最近 max 条
func buildDialogText(msgs []chatMsg, max int) string {
	if len(msgs) > max {
		msgs = msgs[len(msgs)-max:]
	}
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "user":
			parts = append(parts, "用户："+m.Content)
		case "assistant":
			parts = append(parts, "AI："+m.Content)
		}
	}
	return strings.Join(parts, "\n")
}

// distillConversation POST /api/persona/conversations/:id/distill（需登录，仅属主）
// 把对话 + 用户画像蒸馏成 persona_text，upsert 到 personas 表
func distillConversation(c *gin.Context) {
	uid := c.GetInt64("userID")
	convID := parseID(c.Param("id"))
	if convID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
		return
	}
	var messagesRaw string
	if err := db.QueryRow(`SELECT messages FROM persona_conversations WHERE id = ? AND user_id = ?`, convID, uid).Scan(&messagesRaw); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	user, err := getUserByID(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	msgs := []chatMsg{}
	json.Unmarshal([]byte(messagesRaw), &msgs)
	if len(msgs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "对话为空，无法蒸馏"})
		return
	}
	profile := fmt.Sprintf("学校：%s；年级：%s；专业：%s；个人简介：%s；AI熟练度：%s",
		user.School, user.Grade, user.Major, user.Bio, aiLevelLabel(user.AILevel))
	llmMsgs := []chatMsg{
		{Role: "system", Content: distillSystemPrompt},
		{Role: "user", Content: "【个人画像】\n" + profile + "\n\n【对话记录】\n" + buildDialogText(msgs, 40)},
	}
	text, err := callGuideDeepSeek(context.Background(), llmMsgs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "蒸馏失败：" + err.Error()})
		return
	}
	if strings.TrimSpace(text) == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "蒸馏失败：模型返回为空"})
		return
	}
	text = strings.TrimSpace(text)
	db.Exec(`INSERT INTO personas (user_id, conversation_id, persona_text, chat_enabled, chat_count, updated_at)
		VALUES (?, ?, ?, 1, 0, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET conversation_id = excluded.conversation_id,
			persona_text = excluded.persona_text, updated_at = CURRENT_TIMESTAMP`, uid, convID, text)
	db.Exec(`UPDATE persona_conversations SET status = 'distilled', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, convID)
	c.JSON(http.StatusOK, gin.H{"persona_text": text})
}

// ---------- 查询 / 开关 ----------

type personaRow struct {
	id          int64
	userID      int64
	text        string
	chatEnabled int
	chatCount   int
}

func loadPersona(uid int64) *personaRow {
	var p personaRow
	err := db.QueryRow(`SELECT id, user_id, persona_text, chat_enabled, chat_count FROM personas WHERE user_id = ?`, uid).
		Scan(&p.id, &p.userID, &p.text, &p.chatEnabled, &p.chatCount)
	if err != nil {
		return nil
	}
	return &p
}

// getMyPersona GET /api/persona/me（需登录）
func getMyPersona(c *gin.Context) {
	uid := c.GetInt64("userID")
	p := loadPersona(uid)
	if p == nil {
		c.JSON(http.StatusOK, gin.H{"persona": gin.H{"has_persona": false, "chat_enabled": 1}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"persona": gin.H{
		"has_persona":  true,
		"chat_enabled": p.chatEnabled == 1,
		"persona_text": p.text,
		"chat_count":   p.chatCount,
	}})
}

// updateMyPersona PATCH /api/persona/me（需登录） body: {"chat_enabled": true|false}
func updateMyPersona(c *gin.Context) {
	uid := c.GetInt64("userID")
	var req struct {
		ChatEnabled *bool `json:"chat_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChatEnabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_enabled is required"})
		return
	}
	v := 0
	if *req.ChatEnabled {
		v = 1
	}
	db.Exec(`INSERT INTO personas (user_id, chat_enabled, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET chat_enabled = excluded.chat_enabled, updated_at = CURRENT_TIMESTAMP`, uid, v)
	c.JSON(http.StatusOK, gin.H{"chat_enabled": *req.ChatEnabled})
}

// getPublicPersona GET /api/persona/public/:userId（游客/登录均可）
// 关闭或未蒸馏时返回 chat_enabled=0；开启时返回摘要展示文本
func getPublicPersona(c *gin.Context) {
	uid, _ := c.Get("userID")
	targetID := parseID(c.Param("userId"))
	if targetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	p := loadPersona(targetID)
	if p == nil || p.chatEnabled == 0 || strings.TrimSpace(p.text) == "" {
		c.JSON(http.StatusOK, gin.H{"persona": gin.H{"has_persona": false, "chat_enabled": 0}})
		return
	}
	summary := p.text
	if r := []rune(summary); len(r) > 300 {
		summary = string(r[:300]) + "…"
	}
	isSelf := false
	if id, ok := uid.(int64); ok {
		isSelf = id == targetID
	}
	c.JSON(http.StatusOK, gin.H{"persona": gin.H{
		"has_persona":  true,
		"chat_enabled": 1,
		"summary":      summary,
		"chat_count":   p.chatCount,
		"is_self":      isSelf,
	}})
}

// ---------- 访客与虚拟自己聊天 ----------

// createPersonaChat POST /api/persona/public/:userId/chats（需登录）
// body: {"allow_owner_view": false} —— 访客决定本次聊天记录是否对对方可见
func createPersonaChat(c *gin.Context) {
	visitorID := c.GetInt64("userID")
	targetID := parseID(c.Param("userId"))
	if targetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if visitorID == targetID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能与自己聊天"})
		return
	}
	p := loadPersona(targetID)
	if p == nil || p.chatEnabled == 0 || strings.TrimSpace(p.text) == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "该用户未开启虚拟自己"})
		return
	}
	var req struct {
		AllowOwnerView bool `json:"allow_owner_view"`
	}
	c.ShouldBindJSON(&req)
	allow := 0
	if req.AllowOwnerView {
		allow = 1
	}
	res, err := db.Exec(`INSERT INTO persona_chats (persona_id, visitor_id, allow_owner_view) VALUES (?, ?, ?)`, p.id, visitorID, allow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	chatID, _ := res.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"chat_id": chatID, "allow_owner_view": req.AllowOwnerView})
}

type personaMsg struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// loadChatMessages 取某 persona 聊天的全部消息（rows 已关闭后才返回）
func loadChatMessages(chatID int64) []personaMsg {
	out := []personaMsg{}
	rows, err := db.Query(`SELECT id, role, content, created_at FROM persona_chat_messages WHERE chat_id = ? ORDER BY id`, chatID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var m personaMsg
		if rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt) == nil {
			out = append(out, m)
		}
	}
	return out
}

// buildPersonaSystemPrompt 组装 LLM 扮演用户身份的 system prompt
func buildPersonaSystemPrompt(owner *User, personaText string) string {
	return fmt.Sprintf(`你是「%s」的虚拟自己，由 %s 的真实对话记录和个人画像蒸馏而成。请完全以 %s 本人的身份、口吻和立场与访客交流。

【虚拟自己设定】
%s

要求：
1. 只依据上面的设定和蒸馏对话中的事实作答，不得编造设定之外的个人经历、身份或观点。
2. 保持自然口语化，像本人聊天一样，不要暴露你是 AI，不要自称"作为AI"。
3. 回答简洁，一般不超过 200 字；访客闲聊时轻松回应，访客请教时给出符合本人知识背景的实在回答。
4. 如果访客的问题超出设定范围，坦诚表示自己不太了解，而不是胡编。`, owner.Username, owner.Username, owner.Username, personaText)
}

// sendPersonaChatMessage POST /api/persona/chat/:chatId/messages（需登录，仅访客本人）
// body: {"content": "..."}  —— 保存并让 LLM 扮演虚拟自己回复
func sendPersonaChatMessage(c *gin.Context) {
	uid := c.GetInt64("userID")
	chatID := parseID(c.Param("chatId"))
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}
	var personaID, visitorID int64
	if err := db.QueryRow(`SELECT persona_id, visitor_id FROM persona_chats WHERE id = ?`, chatID).Scan(&personaID, &visitorID); err != nil || visitorID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限操作该聊天"})
		return
	}
	var ownerID int64
	var personaText string
	if err := db.QueryRow(`SELECT user_id, persona_text FROM personas WHERE id = ?`, personaID).Scan(&ownerID, &personaText); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "persona not found"})
		return
	}
	owner, err := getUserByID(ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 取历史消息（rows 已关闭），组装 LLM 消息
	hist := loadChatMessages(chatID)
	llmMsgs := []chatMsg{{Role: "system", Content: buildPersonaSystemPrompt(owner, personaText)}}
	for _, m := range hist {
		llmMsgs = append(llmMsgs, chatMsg{Role: m.Role, Content: m.Content})
	}
	llmMsgs = append(llmMsgs, chatMsg{Role: "user", Content: req.Content})

	// 先存用户消息，再调 LLM，最后存回复并计数
	db.Exec(`INSERT INTO persona_chat_messages (chat_id, role, content) VALUES (?, 'user', ?)`, chatID, req.Content)
	reply, err := callGuideDeepSeek(context.Background(), llmMsgs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "虚拟自己回复失败：" + err.Error()})
		return
	}
	reply = strings.TrimSpace(reply)
	db.Exec(`INSERT INTO persona_chat_messages (chat_id, role, content) VALUES (?, 'assistant', ?)`, chatID, reply)
	db.Exec(`UPDATE personas SET chat_count = chat_count + 1 WHERE id = ?`, personaID)
	c.JSON(http.StatusOK, gin.H{"reply": reply})
}

// getPersonaChatMessages GET /api/persona/chat/:chatId/messages（需登录）
// 访客本人可见全部；主人仅在对方允许（allow_owner_view=1）时可见
func getPersonaChatMessages(c *gin.Context) {
	uid := c.GetInt64("userID")
	chatID := parseID(c.Param("chatId"))
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	var personaID, visitorID, allow int64
	if err := db.QueryRow(`SELECT persona_id, visitor_id, allow_owner_view FROM persona_chats WHERE id = ?`, chatID).
		Scan(&personaID, &visitorID, &allow); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	if visitorID != uid {
		var ownerID int64
		if err := db.QueryRow(`SELECT user_id FROM personas WHERE id = ?`, personaID).Scan(&ownerID); err != nil || ownerID != uid || allow == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限查看该聊天"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": loadChatMessages(chatID), "allow_owner_view": allow == 1})
}

// listMyPersonaChats GET /api/persona/me/chats（需登录，主人视角）
// 只返回对方允许查看的聊天，附带访客用户名与最近一条消息
func listMyPersonaChats(c *gin.Context) {
	uid := c.GetInt64("userID")
	var personaID int64
	if err := db.QueryRow(`SELECT id FROM personas WHERE user_id = ?`, uid).Scan(&personaID); err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	rows, err := db.Query(`SELECT pc.id, pc.visitor_id, u.username,
		(SELECT m.content FROM persona_chat_messages m WHERE m.chat_id = pc.id ORDER BY m.id DESC LIMIT 1),
		pc.created_at
		FROM persona_chats pc JOIN users u ON u.id = pc.visitor_id
		WHERE pc.persona_id = ? AND pc.allow_owner_view = 1
		ORDER BY pc.id DESC`, personaID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, visitorID int64
		var username string
		var lastMsg string
		var createdAt time.Time
		if rows.Scan(&id, &visitorID, &username, &lastMsg, &createdAt) == nil {
			out = append(out, gin.H{
				"chat_id":    id,
				"visitor_id": visitorID,
				"visitor":    username,
				"last_msg":   lastMsg,
				"created_at": createdAt,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
