// 在线聊天（Direct Chat）：用户之间的一对一实时聊天。
// 项目无 WebSocket/SSE，采用轮询实现：发消息为 POST，拉新消息为 GET ?after=<id>。
// 单连接死锁注意：SQLite SetMaxOpenConns(1) 下，任何 db.Query 返回的 rows 在 Close 前
// 都占用唯一连接，期间不得发起其他 db 调用。
package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func initChatSchema() {
	schema := `
CREATE TABLE IF NOT EXISTS direct_chats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_a_id INTEGER NOT NULL,
  user_b_id INTEGER NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_message_at DATETIME
);
CREATE TABLE IF NOT EXISTS direct_chat_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  sender_id INTEGER NOT NULL,
  content TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dchat_users ON direct_chats(user_a_id, user_b_id);
CREATE INDEX IF NOT EXISTS idx_dmsg_chat ON direct_chat_messages(chat_id);
`
	if _, err := db.Exec(schema); err != nil {
		panic("init chat schema failed: " + err.Error())
	}
}

// createDirectChat POST /api/chat/direct（需登录） body: {"user_id": 对方id}
// 已有会话则复用返回，否则新建
func createDirectChat(c *gin.Context) {
	uid := c.GetInt64("userID")
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	if req.UserID == uid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能与自己聊天"})
		return
	}
	var chatID int64
	err := db.QueryRow(`SELECT id FROM direct_chats
		WHERE (user_a_id = ? AND user_b_id = ?) OR (user_a_id = ? AND user_b_id = ?)`,
		uid, req.UserID, req.UserID, uid).Scan(&chatID)
	if err == nil {
		peer, _ := getUserByID(req.UserID)
		name := ""
		if peer != nil {
			name = peer.Username
		}
		c.JSON(http.StatusOK, gin.H{"chat_id": chatID, "peer": gin.H{"id": req.UserID, "username": name}})
		return
	}
	res, err := db.Exec(`INSERT INTO direct_chats (user_a_id, user_b_id) VALUES (?, ?)`, uid, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	peer, _ := getUserByID(req.UserID)
	name := ""
	if peer != nil {
		name = peer.Username
	}
	c.JSON(http.StatusOK, gin.H{"chat_id": id, "peer": gin.H{"id": req.UserID, "username": name}})
}

// listDirectChats GET /api/chat/direct（需登录）我的会话列表（含对方、最近消息）
func listDirectChats(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT dc.id,
			CASE WHEN dc.user_a_id = ? THEN dc.user_b_id ELSE dc.user_a_id END,
			u.username,
			(SELECT m.content FROM direct_chat_messages m WHERE m.chat_id = dc.id ORDER BY m.id DESC LIMIT 1),
			strftime('%Y-%m-%d %H:%M:%S', COALESCE(dc.last_message_at, dc.created_at))
		FROM direct_chats dc
		JOIN users u ON u.id = (CASE WHEN dc.user_a_id = ? THEN dc.user_b_id ELSE dc.user_a_id END)
		WHERE dc.user_a_id = ? OR dc.user_b_id = ?
		ORDER BY COALESCE(dc.last_message_at, dc.created_at) DESC`,
		uid, uid, uid, uid)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, peerID int64
		var name, lastMsg, tStr string
		if rows.Scan(&id, &peerID, &name, &lastMsg, &tStr) == nil {
			// COALESCE 表达式被 SQLite 推断为 TEXT，解析成时间再输出，与其他接口格式一致
			t, _ := time.Parse("2006-01-02 15:04:05", tStr)
			out = append(out, gin.H{
				"chat_id":    id,
				"peer_id":    peerID,
				"peer":       name,
				"last_msg":   lastMsg,
				"last_at":    t,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// directChatParticipants 返回会话双方 id；非参与者返回 false
func directChatParticipants(chatID int64) (a, b int64, ok bool) {
	err := db.QueryRow(`SELECT user_a_id, user_b_id FROM direct_chats WHERE id = ?`, chatID).Scan(&a, &b)
	return a, b, err == nil
}

// sendDirectMessage POST /api/chat/direct/:id/messages（需登录，参与者） body: {"content": "..."}
func sendDirectMessage(c *gin.Context) {
	uid := c.GetInt64("userID")
	chatID := parseID(c.Param("id"))
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	a, b, ok := directChatParticipants(chatID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	if uid != a && uid != b {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限操作该聊天"})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}
	res, err := db.Exec(`INSERT INTO direct_chat_messages (chat_id, sender_id, content) VALUES (?, ?, ?)`,
		chatID, uid, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	db.Exec(`UPDATE direct_chats SET last_message_at = CURRENT_TIMESTAMP WHERE id = ?`, chatID)

	// 通知接收方：有新私信
	peerID := a
	if peerID == uid {
		peerID = b
	}
	if me, _ := getUserByID(uid); me != nil {
		pushNotification(peerID, uid, "message", req.Content, me.Username, "", chatID)
	}

	c.JSON(http.StatusOK, gin.H{"message_id": id, "created_at": time.Now()})
}

// getDirectMessages GET /api/chat/direct/:id/messages?after=<id>（需登录，参与者）
// 轮询拉取 after 之后的新消息
func getDirectMessages(c *gin.Context) {
	uid := c.GetInt64("userID")
	chatID := parseID(c.Param("id"))
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	a, b, ok := directChatParticipants(chatID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	if uid != a && uid != b {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限查看该聊天"})
		return
	}
	after := parseID(c.Query("after"))
	rows, err := db.Query(`SELECT id, sender_id, content, created_at FROM direct_chat_messages
		WHERE chat_id = ? AND id > ? ORDER BY id`, chatID, after)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"messages": []gin.H{}})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, senderID int64
		var content string
		var t time.Time
		if rows.Scan(&id, &senderID, &content, &t) == nil {
			out = append(out, gin.H{"id": id, "sender_id": senderID, "content": content, "created_at": t})
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": out})
}
