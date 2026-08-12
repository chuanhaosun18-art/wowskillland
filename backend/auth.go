package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ---------- JWT ----------

var jwtSecret = []byte("skillhub-dev-secret-change-me")

func init() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		jwtSecret = []byte(s)
	}
}

type claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func generateToken(userID int64, username string) (string, error) {
	c := claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "skillhub",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(jwtSecret)
}

func parseToken(tokenStr string) (*claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if cl, ok := token.Claims.(*claims); ok && token.Valid {
		return cl, nil
	}
	return nil, fmt.Errorf("invalid token")
}

// authMiddleware 校验 Authorization: Bearer <token>，通过后将 userID 注入上下文
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		cl, err := parseToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("userID", cl.UserID)
		c.Set("username", cl.Username)
		c.Next()
	}
}

// ---------- 注册 / 登录 ----------

type registerReq struct {
	Username string       `json:"username" binding:"required"`
	Email    string       `json:"email"`
	Password string       `json:"password" binding:"required"`
	School   string       `json:"school"`
	Grade    string       `json:"grade"`
	Major    string       `json:"major"`
	Bio      string       `json:"bio"`
	AILevel  string       `json:"ai_level"` // 显式指定（可选）；优先用 ai_quiz 自动推导
	AIQuiz   *aiQuizInput `json:"ai_quiz"`  // 5 题问卷，非空时自动推导 ai_level
}

// aiQuizInput 注册问卷 5 题（用户原话 5 个方向）
type aiQuizInput struct {
	HeardOfLLM       bool `json:"heard_of_llm"`        // 1 是否听说过 ChatGPT 等大语言模型
	UsedLLM          bool `json:"used_llm"`           // 2 是否使用过大语言模型
	UsedAgent        bool `json:"used_agent"`         // 3 是否使用过 Codex / Agent 等 AI 编码代理
	HasAgentInstalled bool `json:"has_agent_installed"` // 4 电脑中是否装有 Codex 等 Agent 工具
	RanFullProject   bool `json:"ran_full_project"`   // 5 是否用上述 Agent 跑过完整项目
}

// inferAILevel 由问卷推导 AI 熟练度（阶梯式）：
//
//	never        没用过大模型（或没听说过）
//	beginner     用过大模型，但没用过 Agent
//	intermediate 用过 Agent，但没跑过完整项目
//	advanced     用过 Agent，且跑过完整项目
func inferAILevel(q aiQuizInput) string {
	if !q.HeardOfLLM || !q.UsedLLM {
		return "never"
	}
	if !q.UsedAgent {
		return "beginner"
	}
	if !q.RanFullProject {
		return "intermediate"
	}
	return "advanced"
}

// quizToJSON 问卷答案序列化为 JSON 字符串存库
func quizToJSON(q aiQuizInput) string {
	b, err := json.Marshal(q)
	if err != nil {
		return ""
	}
	return string(b)
}

// emailOrNil 空邮箱写入 NULL：email 列有 UNIQUE 约束，空字符串会与既有空邮箱用户冲突，
// 而 SQLite 的 UNIQUE 允许多个 NULL。
func emailOrNil(email string) interface{} {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	return email
}

// register POST /api/auth/register
func register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 2 || len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be >= 2 chars and password >= 6 chars"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 有问卷则自动推导 ai_level（覆盖显式值），并把答案落库
	aiLevel := req.AILevel
	aiQuiz := ""
	if req.AIQuiz != nil {
		aiLevel = inferAILevel(*req.AIQuiz)
		aiQuiz = quizToJSON(*req.AIQuiz)
	}

	res, err := db.Exec(`INSERT INTO users (username, email, password_hash, school, grade, major, bio, ai_level, ai_quiz) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Username, emailOrNil(req.Email), string(hash), req.School, req.Grade, req.Major, req.Bio, aiLevel, aiQuiz)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
		return
	}
	uid, _ := res.LastInsertId()

	token, err := generateToken(uid, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := &User{
		ID:       uid,
		Username: req.Username,
		Email:    req.Email,
		School:   req.School,
		Grade:    req.Grade,
		Major:    req.Major,
		Bio:      req.Bio,
		AILevel:  aiLevel,
		AIQuiz:   aiQuiz,
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "user": user})
}

type loginReq struct {
	Account  string `json:"account" binding:"required"` // 用户名或邮箱
	Password string `json:"password" binding:"required"`
}

// login POST /api/auth/login
func login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account and password are required"})
		return
	}

	var u User
	var pwdHash string
	err := db.QueryRow(`SELECT id, username, COALESCE(email, ''), avatar, school, grade, major, bio, ai_level, ai_quiz, password_hash, created_at
		FROM users WHERE username = ? OR email = ?`, req.Account, req.Account).
		Scan(&u.ID, &u.Username, &u.Email, &u.Avatar, &u.School, &u.Grade, &u.Major, &u.Bio, &u.AILevel, &u.AIQuiz, &pwdHash, &u.CreatedAt)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码错误"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(pwdHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码错误"})
		return
	}

	token, err := generateToken(u.ID, u.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// me GET /api/auth/me
func me(c *gin.Context) {
	u, err := getUserByID(c.GetInt64("userID"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": u})
}

type updateUserReq struct {
	Email   string       `json:"email"`
	School  string       `json:"school"`
	Grade   string       `json:"grade"`
	Major   string       `json:"major"`
	Bio     string       `json:"bio"`
	Avatar  string       `json:"avatar"`
	AILevel string       `json:"ai_level"`
	AIQuiz  *aiQuizInput `json:"ai_quiz"` // 重答问卷时自动重新推导 ai_level
}

// updateUser PUT /api/users/:id
func updateUser(c *gin.Context) {
	uid := c.GetInt64("userID")
	if fmt.Sprint(uid) != c.Param("id") {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能修改其他用户资料"})
		return
	}
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
		return
	}
	// 重答问卷 → 重新推导水平；否则保留原值，避免误清空
	aiLevel := req.AILevel
	aiQuiz := ""
	if req.AIQuiz != nil {
		aiLevel = inferAILevel(*req.AIQuiz)
		aiQuiz = quizToJSON(*req.AIQuiz)
	} else {
		var curLevel, curQuiz string
		_ = db.QueryRow(`SELECT ai_level, ai_quiz FROM users WHERE id = ?`, uid).Scan(&curLevel, &curQuiz)
		aiQuiz = curQuiz
		if aiLevel == "" {
			aiLevel = curLevel
		}
	}
	if _, err := db.Exec(`UPDATE users SET email = ?, school = ?, grade = ?, major = ?, bio = ?, avatar = ?, ai_level = ?, ai_quiz = ? WHERE id = ?`,
		emailOrNil(req.Email), req.School, req.Grade, req.Major, req.Bio, req.Avatar, aiLevel, aiQuiz, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, err := getUserByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": u})
}

// mySkills GET /api/users/me/skills
func mySkills(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT s.id, s.owner_id, COALESCE(u.username,''), s.name, s.description,
		s.category, s.tags, s.version, s.icon, s.file_count, s.total_size,
		s.download_count, s.view_count, s.rating, s.created_at, s.updated_at,
		COALESCE(s.proof_images,'[]')
		FROM skills s LEFT JOIN users u ON s.owner_id = u.id
		WHERE s.owner_id = ? ORDER BY s.created_at DESC`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	skills := []Skill{}
	for rows.Next() {
		var s Skill
		var proofRaw string
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.OwnerName, &s.Name, &s.Description,
			&s.Category, &s.Tags, &s.Version, &s.Icon, &s.FileCount, &s.TotalSize,
			&s.DownloadCount, &s.ViewCount, &s.Rating, &s.CreatedAt, &s.UpdatedAt,
			&proofRaw); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		s.ProofImages = parseProofImages(proofRaw)
		skills = append(skills, s)
	}
	c.JSON(http.StatusOK, gin.H{"data": skills, "total": len(skills)})
}

// ---------- 工具 ----------

func getUserByID(id int64) (*User, error) {
	var u User
	err := db.QueryRow(`SELECT id, username, COALESCE(email, ''), avatar, school, grade, major, bio, ai_level, ai_quiz, created_at
		FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.Avatar, &u.School, &u.Grade, &u.Major, &u.Bio, &u.AILevel, &u.AIQuiz, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
