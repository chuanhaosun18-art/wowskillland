// 消息通知（Notifications）：统一通知表 + 铃铛未读角标。
// 触发点：收到私信、skill 被评价、skill 收到 Issue 改进意见。
// 轮询拉取：GET /api/notifications、GET /api/notifications/unread-count，
// 已读：POST /api/notifications/read、POST /api/notifications/:id/read。
package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func initNotificationsSchema() {
	schema := `
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  actor_id INTEGER DEFAULT 0,
  actor_name TEXT DEFAULT '',
  type TEXT NOT NULL,
  content TEXT DEFAULT '',
  related_id INTEGER DEFAULT 0,
  skill_name TEXT DEFAULT '',
  is_read INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notif_user ON notifications(user_id, is_read);
`
	if _, err := db.Exec(schema); err != nil {
		panic("init notifications schema failed: " + err.Error())
	}
}

// pushNotification 写入一条通知。toUserID 为接收者；actorID 为触发者（0 表示系统/游客）。
// ntype: message(私信) / review(评价) / issue(改进意见)
func pushNotification(toUserID, actorID int64, ntype, content, actorName, skillName string, relatedID int64) {
	if toUserID <= 0 || toUserID == actorID {
		return // 不给自己发通知
	}
	if _, err := db.Exec(`INSERT INTO notifications (user_id, actor_id, actor_name, type, content, related_id, skill_name)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		toUserID, actorID, actorName, ntype, content, relatedID, skillName); err != nil {
		// 通知失败不影响主流程，仅记录
		println("pushNotification error:", err.Error())
	}
}

type Notification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ActorID   int64     `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	RelatedID int64     `json:"related_id"`
	SkillName string    `json:"skill_name"`
	IsRead    int       `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// listNotifications GET /api/notifications（需登录）我的通知列表（倒序，最多 50 条）
func listNotifications(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT id, user_id, actor_id, actor_name, type, content, related_id, skill_name, is_read, created_at
		FROM notifications WHERE user_id = ? ORDER BY id DESC LIMIT 50`, uid)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if rows.Scan(&n.ID, &n.UserID, &n.ActorID, &n.ActorName, &n.Type, &n.Content,
			&n.RelatedID, &n.SkillName, &n.IsRead, &n.CreatedAt) == nil {
			out = append(out, n)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// unreadNotifications GET /api/notifications/unread-count（需登录）未读数量
func unreadNotifications(c *gin.Context) {
	uid := c.GetInt64("userID")
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0`, uid).Scan(&count)
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// markNotificationsRead POST /api/notifications/read（需登录）全部标记已读
func markNotificationsRead(c *gin.Context) {
	uid := c.GetInt64("userID")
	if _, err := db.Exec(`UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0`, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// markNotificationRead POST /api/notifications/:id/read（需登录）单条标记已读
func markNotificationRead(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := parseID(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}
	if _, err := db.Exec(`UPDATE notifications SET is_read = 1 WHERE id = ? AND user_id = ?`, id, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
