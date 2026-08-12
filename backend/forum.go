// 论坛（Forum）：不能成为 Skill 的经验、询问、寻找没有 Skill 的地方。
// 定位：前台搜索路由不到 Skill 时的出口。求经验（help）/ 找技能（looking_for）/
// 经验交流（experience）三类帖子，游客可读，登录可发帖与回复。
package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 论坛分类（前后端保持一致）
var ForumCategories = map[string]string{
	"help":        "求经验",
	"looking_for": "找技能",
	"experience":  "经验交流",
}

type ForumTopic struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Username   string    `json:"username"`
	Avatar     string    `json:"avatar,omitempty"`
	Title      string    `json:"title"`
	Content    string    `json:"content,omitempty"`
	Category   string    `json:"category"`
	CategoryLb string    `json:"category_label"`
	ReplyCount int       `json:"reply_count"`
	ViewCount  int       `json:"view_count"`
	LikeCount  int       `json:"like_count"`
	Liked      bool      `json:"liked"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ForumReply struct {
	ID        int64     `json:"id"`
	TopicID   int64     `json:"topic_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar,omitempty"`
	Content   string    `json:"content"`
	LikeCount int       `json:"like_count"`
	Liked     bool      `json:"liked"`
	CreatedAt time.Time `json:"created_at"`
}

func initForumSchema() {
	schema := `
CREATE TABLE IF NOT EXISTS forum_topics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  content TEXT DEFAULT '',
  category TEXT DEFAULT 'help',
  reply_count INTEGER DEFAULT 0,
  view_count INTEGER DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS forum_replies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  topic_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS forum_likes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  target_type TEXT NOT NULL,
  target_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(target_type, target_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_forum_topics_updated ON forum_topics(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_forum_replies_topic ON forum_replies(topic_id);
CREATE INDEX IF NOT EXISTS idx_forum_likes_target ON forum_likes(target_type, target_id);
`
	if _, err := db.Exec(schema); err != nil {
		panic("init forum schema failed: " + err.Error())
	}
}

// currentUserID 游客接口中尝试解析登录用户；未登录返回 0
func currentUserID(c *gin.Context) int64 {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		if cl, err := parseToken(strings.TrimPrefix(h, "Bearer ")); err == nil {
			return cl.UserID
		}
	}
	return 0
}

// scanTopicRows 统一扫描帖子行（含作者信息与点赞数）
func scanTopicRows(rows interface{ Next() bool; Scan(...interface{}) error }, withContent bool) []ForumTopic {
	out := []ForumTopic{}
	for rows.Next() {
		var t ForumTopic
		var category string
		if withContent {
			if rows.Scan(&t.ID, &t.UserID, &t.Username, &t.Avatar, &t.Title, &t.Content, &category,
				&t.ReplyCount, &t.ViewCount, &t.LikeCount, &t.Liked, &t.CreatedAt, &t.UpdatedAt) != nil {
				continue
			}
		} else {
			if rows.Scan(&t.ID, &t.UserID, &t.Username, &t.Avatar, &t.Title, &category,
				&t.ReplyCount, &t.ViewCount, &t.LikeCount, &t.Liked, &t.CreatedAt, &t.UpdatedAt) != nil {
				continue
			}
		}
		t.Category = category
		t.CategoryLb = ForumCategories[category]
		if t.CategoryLb == "" {
			t.CategoryLb = category
		}
		out = append(out, t)
	}
	return out
}

// listForumTopics GET /api/forum/topics
// 支持 keyword（标题/内容模糊匹配）与 category 筛选；游客可用
func listForumTopics(c *gin.Context) {
	kw := strings.TrimSpace(c.Query("keyword"))
	cat := c.Query("category")

	where := []string{"1=1"}
	args := []interface{}{}
	if kw != "" {
		where = append(where, "(title LIKE ? OR content LIKE ?)")
		like := "%" + kw + "%"
		args = append(args, like, like)
	}
	if cat != "" && cat != "全部" {
		if _, ok := ForumCategories[cat]; ok {
			where = append(where, "category = ?")
			args = append(args, cat)
		}
	}

	// SELECT 里的 EXISTS 占位符在前，uid 需放在参数最前面
	uid := currentUserID(c)
	queryArgs := append([]interface{}{uid}, args...)
	rows, err := db.Query(`SELECT t.id, t.user_id, COALESCE(u.username,'匿名'), COALESCE(u.avatar,''),
		t.title, t.category, t.reply_count, t.view_count,
		(SELECT COUNT(*) FROM forum_likes fl WHERE fl.target_type='topic' AND fl.target_id=t.id) AS like_count,
		EXISTS(SELECT 1 FROM forum_likes fl WHERE fl.target_type='topic' AND fl.target_id=t.id AND fl.user_id=?) AS liked,
		t.created_at, t.updated_at
		FROM forum_topics t LEFT JOIN users u ON u.id = t.user_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY t.updated_at DESC LIMIT 100`, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	c.JSON(http.StatusOK, gin.H{"data": scanTopicRows(rows, false), "total": 0})
}

// createForumTopic POST /api/forum/topics（需登录）
func createForumTopic(c *gin.Context) {
	var req struct {
		Title    string `json:"title" binding:"required"`
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题不能为空"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if len([]rune(req.Title)) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题太短了，多说几个字让大家知道你想聊什么"})
		return
	}
	if len([]rune(req.Title)) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题最长 80 字"})
		return
	}
	if len([]rune(req.Content)) > 5000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容最长 5000 字"})
		return
	}
	if _, ok := ForumCategories[req.Category]; !ok {
		req.Category = "help"
	}

	uid := c.GetInt64("userID")
	res, err := db.Exec(`INSERT INTO forum_topics (user_id, title, content, category) VALUES (?, ?, ?, ?)`,
		uid, req.Title, req.Content, req.Category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发帖失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": id}})
}

// getForumTopic GET /api/forum/topics/:id（游客可用），返回详情 + 回复列表，浏览量 +1
func getForumTopic(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var t ForumTopic
	var category string
	uid := currentUserID(c)
	qerr := db.QueryRow(`SELECT t.id, t.user_id, COALESCE(u.username,'匿名'), COALESCE(u.avatar,''),
		t.title, t.content, t.category, t.reply_count, t.view_count,
		(SELECT COUNT(*) FROM forum_likes fl WHERE fl.target_type='topic' AND fl.target_id=t.id) AS like_count,
		EXISTS(SELECT 1 FROM forum_likes fl WHERE fl.target_type='topic' AND fl.target_id=t.id AND fl.user_id=?) AS liked,
		t.created_at, t.updated_at
		FROM forum_topics t LEFT JOIN users u ON u.id = t.user_id
		WHERE t.id = ?`, uid, id).
		Scan(&t.ID, &t.UserID, &t.Username, &t.Avatar, &t.Title, &t.Content, &category,
			&t.ReplyCount, &t.ViewCount, &t.LikeCount, &t.Liked, &t.CreatedAt, &t.UpdatedAt)
	if qerr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "帖子不存在"})
		return
	}
	t.Category = category
	t.CategoryLb = ForumCategories[category]
	if t.CategoryLb == "" {
		t.CategoryLb = category
	}

	db.Exec(`UPDATE forum_topics SET view_count = view_count + 1 WHERE id = ?`, id)
	t.ViewCount++

	rows, err := db.Query(`SELECT r.id, r.topic_id, r.user_id, COALESCE(u.username,'匿名'), COALESCE(u.avatar,''),
		r.content,
		(SELECT COUNT(*) FROM forum_likes fl WHERE fl.target_type='reply' AND fl.target_id=r.id) AS like_count,
		EXISTS(SELECT 1 FROM forum_likes fl WHERE fl.target_type='reply' AND fl.target_id=r.id AND fl.user_id=?) AS liked,
		r.created_at
		FROM forum_replies r LEFT JOIN users u ON u.id = r.user_id
		WHERE r.topic_id = ? ORDER BY r.created_at ASC`, uid, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	replies := []ForumReply{}
	for rows.Next() {
		var r ForumReply
		if rows.Scan(&r.ID, &r.TopicID, &r.UserID, &r.Username, &r.Avatar, &r.Content,
			&r.LikeCount, &r.Liked, &r.CreatedAt) == nil {
			replies = append(replies, r)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": t, "replies": replies})
}

// createForumReply POST /api/forum/topics/:id/replies（需登录）
func createForumReply(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "回复内容不能为空"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if len([]rune(req.Content)) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "回复最长 2000 字"})
		return
	}

	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forum_topics WHERE id = ?`, id).Scan(&exists); err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "帖子不存在"})
		return
	}

	uid := c.GetInt64("userID")
	res, err := db.Exec(`INSERT INTO forum_replies (topic_id, user_id, content) VALUES (?, ?, ?)`, id, uid, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "回复失败"})
		return
	}
	rid, _ := res.LastInsertId()
	db.Exec(`UPDATE forum_topics SET reply_count = reply_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": rid}})
}

// toggleLike 点赞/取消点赞的公共逻辑：targetType ∈ {topic, reply}
func toggleLike(c *gin.Context, targetType, targetTable string) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// 目标必须存在
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+targetTable+` WHERE id = ?`, id).Scan(&exists); err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "对象不存在"})
		return
	}

	uid := c.GetInt64("userID")
	var cnt int
	db.QueryRow(`SELECT COUNT(*) FROM forum_likes WHERE target_type = ? AND target_id = ? AND user_id = ?`,
		targetType, id, uid).Scan(&cnt)

	liked := false
	if cnt > 0 {
		db.Exec(`DELETE FROM forum_likes WHERE target_type = ? AND target_id = ? AND user_id = ?`, targetType, id, uid)
	} else {
		if _, err := db.Exec(`INSERT INTO forum_likes (target_type, target_id, user_id) VALUES (?, ?, ?)`, targetType, id, uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "点赞失败"})
			return
		}
		liked = true
	}

	var likeCount int
	db.QueryRow(`SELECT COUNT(*) FROM forum_likes WHERE target_type = ? AND target_id = ?`, targetType, id).Scan(&likeCount)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"liked": liked, "like_count": likeCount}})
}

// likeTopic POST /api/forum/topics/:id/like（需登录）
func likeTopic(c *gin.Context) {
	toggleLike(c, "topic", "forum_topics")
}

// likeReply POST /api/forum/replies/:id/like（需登录）
func likeReply(c *gin.Context) {
	toggleLike(c, "reply", "forum_replies")
}
