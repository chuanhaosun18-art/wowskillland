package main

import (
	"database/sql"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// 数据模型
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	Avatar    string    `json:"avatar,omitempty"`
	School    string    `json:"school,omitempty"`
	Grade     string    `json:"grade,omitempty"`
	Major     string    `json:"major,omitempty"`
	Bio       string    `json:"bio,omitempty"`
	AILevel   string    `json:"ai_level,omitempty"` // AI 熟练度：never/beginner/intermediate/advanced
	AIQuiz    string    `json:"ai_quiz,omitempty"`  // 注册问卷 5 题答案（JSON），用于推导 ai_level
	CreatedAt time.Time `json:"created_at"`
}

type SkillFile struct {
	ID       int64  `json:"id"`
	SkillID  int64  `json:"skill_id"`
	FilePath string `json:"file_path"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256,omitempty"`
}

type Skill struct {
	ID            int64        `json:"id"`
	OwnerID       *int64       `json:"owner_id"` // 可空：游客发布的 skill 无属主
	OwnerName     string       `json:"owner_name,omitempty"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Category      string       `json:"category"`
	Tags          string       `json:"tags"` // JSON 数组字符串
	Version       string       `json:"version"`
	Icon          string       `json:"icon,omitempty"`
	FileCount     int          `json:"file_count"`
	TotalSize     int64        `json:"total_size"`
	ArchivePath   string       `json:"-"`
	DownloadCount int          `json:"download_count"`
	ViewCount     int          `json:"view_count"`
	Rating        float64      `json:"rating"`
	RatingCount   int          `json:"rating_count"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ProofImages   []string     `json:"proof_images,omitempty"` // 评估指标证明图片 URL 列表
	Files         []SkillFile  `json:"files,omitempty"`
}

var db *sql.DB

// initDB 初始化数据库并建表
func initDB() {
	var err error
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写者，避免锁冲突

	schema := `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  email TEXT UNIQUE,
  password_hash TEXT NOT NULL,
  avatar TEXT DEFAULT '',
  school TEXT DEFAULT '',
  grade TEXT DEFAULT '',
  major TEXT DEFAULT '',
  bio TEXT DEFAULT '',
  ai_level TEXT DEFAULT '',
  ai_quiz TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skills (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id INTEGER,
  name TEXT NOT NULL,
  description TEXT DEFAULT '',
  category TEXT DEFAULT '',
  tags TEXT DEFAULT '[]',
  version TEXT DEFAULT '1.0.0',
  icon TEXT DEFAULT '',
  file_count INTEGER DEFAULT 0,
  total_size INTEGER DEFAULT 0,
  archive_path TEXT DEFAULT '',
  download_count INTEGER DEFAULT 0,
  view_count INTEGER DEFAULT 0,
  rating REAL DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skill_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  file_path TEXT NOT NULL,
  size INTEGER DEFAULT 0,
  sha256 TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS likes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  skill_id INTEGER NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, skill_id)
);

CREATE TABLE IF NOT EXISTS skill_reviews (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  rating INTEGER NOT NULL,
  comment TEXT DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(skill_id, user_id)
);

CREATE TABLE IF NOT EXISTS skill_issues (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT DEFAULT '',
  status TEXT DEFAULT 'open',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  closed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category);
CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name);
CREATE INDEX IF NOT EXISTS idx_files_skill ON skill_files(skill_id);
CREATE INDEX IF NOT EXISTS idx_reviews_skill ON skill_reviews(skill_id);
CREATE INDEX IF NOT EXISTS idx_issues_skill ON skill_issues(skill_id);
`
	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("init schema failed: %v", err)
	}

	// 老库迁移：为已存在的 users 表补充画像字段（列已存在时忽略错误）
	userMigrations := []string{
		"ALTER TABLE users ADD COLUMN school TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN grade TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN major TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN bio TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN ai_level TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN ai_quiz TEXT DEFAULT ''",
	}
	for _, m := range userMigrations {
		if _, err := db.Exec(m); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				log.Printf("migrate warning: %v", err)
			}
		}
	}

	// 老库迁移：skills 表补充评分人数、评估指标证明图片
	skillMigrations := []string{
		"ALTER TABLE skills ADD COLUMN rating_count INTEGER DEFAULT 0",
		"ALTER TABLE skills ADD COLUMN proof_images TEXT DEFAULT '[]'",
	}
	for _, m := range skillMigrations {
		if _, err := db.Exec(m); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				log.Printf("migrate warning: %v", err)
			}
		}
	}

	// 成长闭环相关表（增量迁移，见 growth_db.go）
	initGrowthSchema()

	// 论坛相关表（见 forum.go）
	initForumSchema()

	log.Println("database initialized:", DBPath)
}
