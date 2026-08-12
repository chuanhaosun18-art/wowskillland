package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 中间件 ----------

// optionalAuth 可选登录：有合法 token 则注入 userID/username，游客直接放行
func optionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if strings.HasPrefix(h, "Bearer ") {
			if cl, err := parseToken(strings.TrimPrefix(h, "Bearer ")); err == nil {
				c.Set("userID", cl.UserID)
				c.Set("username", cl.Username)
			}
		}
		c.Next()
	}
}

// ---------- 评分 / 评价 ----------

type SkillReview struct {
	ID        int64     `json:"id"`
	SkillID   int64     `json:"skill_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// submitReview POST /api/skills/:id/reviews (需登录)
// body: {"rating": 1-5, "comment": "..."}  一人一评，重复提交为更新
func submitReview(c *gin.Context) {
	id := c.Param("id")
	uid := c.GetInt64("userID")

	var s Skill
	err := db.QueryRow(`SELECT id, owner_id, name FROM skills WHERE id = ?`, id).Scan(&s.ID, &s.OwnerID, &s.Name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	var req struct {
		Rating  int    `json:"rating" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating is required"})
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
		return
	}
	req.Comment = strings.TrimSpace(req.Comment)

	// UPSERT：同一用户对同一 skill 只能有一条评价，重复提交覆盖
	if _, err := db.Exec(`INSERT INTO skill_reviews (skill_id, user_id, rating, comment)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(skill_id, user_id) DO UPDATE SET
			rating = excluded.rating,
			comment = excluded.comment,
			updated_at = CURRENT_TIMESTAMP`, id, uid, req.Rating, req.Comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重算该 skill 的平均分与评分人数
	db.Exec(`UPDATE skills SET
		rating = COALESCE((SELECT AVG(rating) FROM skill_reviews WHERE skill_id = ?), 0),
		rating_count = (SELECT COUNT(*) FROM skill_reviews WHERE skill_id = ?)
		WHERE id = ?`, id, id, id)

	// 通知 skill 属主：收到新评价
	if s.OwnerID != nil {
		if me, _ := getUserByID(uid); me != nil {
			content := req.Comment
			if content == "" {
				content = "给你的 Skill 打出了 " + strconv.Itoa(req.Rating) + " 星"
			}
			pushNotification(*s.OwnerID, uid, "review", content, me.Username, s.Name, s.ID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "review submitted"})
}

// listReviews GET /api/skills/:id/reviews (游客可用，带 token 时返回当前用户评价)
func listReviews(c *gin.Context) {
	id := c.Param("id")

	var s Skill
	if err := db.QueryRow(`SELECT id FROM skills WHERE id = ?`, id).Scan(&s.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	rows, err := db.Query(`SELECT r.id, r.skill_id, r.user_id, COALESCE(u.username,''), r.rating, r.comment, r.created_at, r.updated_at
		FROM skill_reviews r LEFT JOIN users u ON r.user_id = u.id
		WHERE r.skill_id = ? ORDER BY r.updated_at DESC, r.id DESC`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	reviews := []SkillReview{}
	for rows.Next() {
		var r SkillReview
		if err := rows.Scan(&r.ID, &r.SkillID, &r.UserID, &r.Username, &r.Rating, &r.Comment, &r.CreatedAt, &r.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		reviews = append(reviews, r)
	}

	var ratingAvg float64
	var ratingCount int
	db.QueryRow(`SELECT COALESCE(AVG(rating),0), COUNT(*) FROM skill_reviews WHERE skill_id = ?`, id).Scan(&ratingAvg, &ratingCount)

	// 当前登录用户是否已评价
	var myReview *SkillReview
	if uid, ok := c.Get("userID"); ok {
		var r SkillReview
		err := db.QueryRow(`SELECT r.id, r.skill_id, r.user_id, COALESCE(u.username,''), r.rating, r.comment, r.created_at, r.updated_at
			FROM skill_reviews r LEFT JOIN users u ON r.user_id = u.id
			WHERE r.skill_id = ? AND r.user_id = ?`, id, uid).Scan(
			&r.ID, &r.SkillID, &r.UserID, &r.Username, &r.Rating, &r.Comment, &r.CreatedAt, &r.UpdatedAt)
		if err == nil {
			myReview = &r
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":         reviews,
		"my_review":    myReview,
		"rating_avg":   ratingAvg,
		"rating_count": ratingCount,
	})
}

// ---------- Issue 反馈（类 GitHub issue） ----------

type SkillIssue struct {
	ID        int64      `json:"id"`
	SkillID   int64      `json:"skill_id"`
	UserID    int64      `json:"user_id"`
	Username  string     `json:"username"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Status    string     `json:"status"` // open / closed
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// createIssue POST /api/skills/:id/issues (需登录)
// body: {"title": "...", "body": "..."}
func createIssue(c *gin.Context) {
	id := c.Param("id")
	uid := c.GetInt64("userID")

	var s Skill
	if err := db.QueryRow(`SELECT id, owner_id, name FROM skills WHERE id = ?`, id).Scan(&s.ID, &s.OwnerID, &s.Name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	var req struct {
		Title string `json:"title" binding:"required"`
		Body  string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)

	result, err := db.Exec(`INSERT INTO skill_issues (skill_id, user_id, title, body, status)
		VALUES (?, ?, ?, ?, 'open')`, id, uid, req.Title, strings.TrimSpace(req.Body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	issueID, _ := result.LastInsertId()

	// 通知 skill 属主：收到新的改进意见（Issue）
	if s.OwnerID != nil {
		if me, _ := getUserByID(uid); me != nil {
			pushNotification(*s.OwnerID, uid, "issue", req.Title, me.Username, s.Name, s.ID)
		}
	}

	issue, err := getIssueByID(issueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issue})
}

// listIssues GET /api/skills/:id/issues (游客可用)
func listIssues(c *gin.Context) {
	id := c.Param("id")

	var s Skill
	if err := db.QueryRow(`SELECT id FROM skills WHERE id = ?`, id).Scan(&s.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	rows, err := db.Query(`SELECT i.id, i.skill_id, i.user_id, COALESCE(u.username,''), i.title, i.body, i.status, i.created_at, i.closed_at
		FROM skill_issues i LEFT JOIN users u ON i.user_id = u.id
		WHERE i.skill_id = ? ORDER BY i.id DESC`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	issues := []SkillIssue{}
	for rows.Next() {
		var i SkillIssue
		if err := rows.Scan(&i.ID, &i.SkillID, &i.UserID, &i.Username, &i.Title, &i.Body, &i.Status, &i.CreatedAt, &i.ClosedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		issues = append(issues, i)
	}
	c.JSON(http.StatusOK, gin.H{"data": issues})
}

// closeIssue PATCH /api/issues/:id (仅 issue 作者可关闭/重新打开)
// body: {"status": "closed" | "open"}
func closeIssue(c *gin.Context) {
	id := c.Param("id")
	uid := c.GetInt64("userID")

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}
	if req.Status != "open" && req.Status != "closed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be open or closed"})
		return
	}

	var ownerID int64
	if err := db.QueryRow(`SELECT user_id FROM skill_issues WHERE id = ?`, id).Scan(&ownerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
		return
	}
	if ownerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅 issue 作者可操作"})
		return
	}

	var closedAt interface{}
	if req.Status == "closed" {
		closedAt = time.Now()
	}
	if _, err := db.Exec(`UPDATE skill_issues SET status = ?, closed_at = ? WHERE id = ?`,
		req.Status, closedAt, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	issue, err := getIssueByID(parseID(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issue})
}

func getIssueByID(id int64) (*SkillIssue, error) {
	var i SkillIssue
	err := db.QueryRow(`SELECT i.id, i.skill_id, i.user_id, COALESCE(u.username,''), i.title, i.body, i.status, i.created_at, i.closed_at
		FROM skill_issues i LEFT JOIN users u ON i.user_id = u.id WHERE i.id = ?`, id).
		Scan(&i.ID, &i.SkillID, &i.UserID, &i.Username, &i.Title, &i.Body, &i.Status, &i.CreatedAt, &i.ClosedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// parseID 将路由参数转为 int64，失败返回 0
func parseID(s string) int64 {
	var id int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + int64(ch-'0')
	}
	return id
}
