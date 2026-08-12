// 批量导入技能工具（运营侧）。
// 用途：把队友按规范整理的技能目录一次性灌入本地 SQLite 库，并直接标为 published 进市场。
// 背景：队友无法访问 8080 服务，资料打包发过来，由本机跑这个工具入库。
// 格式：见 docs/IMPORT_GUIDE.md —— 每个技能一个目录，内含 skill.json（必填）、skill.zip（可选）、proofs/（可选）。
//
// 用法：
//
//	import_skills.exe -dir D:\skills_import
//	import_skills.exe -dir D:\skills_import -force     # 同名技能已存在时覆盖而不是跳过
//
// 编译：
//
//	cd d:\dy黑客松\backend && go build -o import_skills.exe ./tools/import_skills
package main

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// 与 growth_db.go 保持一致（独立程序不 import main 包）
const (
	StatusPublished = "published"
	OriginOpsImport = "ops_import" // 运营批量导入
	OpsUsername     = "ops_import"
	OpsEmail        = "ops_import@skillhub.local"
)

// allowedIntents 与后端 AllowedIntents 白名单一致；未知 intent 导入时置空。
var allowedIntents = map[string]bool{
	"thesis_topic": true, "resume_rewrite": true, "resume_jd_align": true,
	"report_structure": true, "mock_interview": true, "interview_review": true,
	"project_convergence": true, "literature_review": true, "content_script": true,
}

// skillMeta skill.json 的字段结构
type skillMeta struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	Version      string   `json:"version"`
	TaskIntent   string   `json:"task_intent"`
	Goal         string   `json:"goal"`
	DoneCriteria []string `json:"done_criteria"`
	Workflow     []string `json:"workflow"`
	Boundary     boundary `json:"boundary"`
	Contract     contract `json:"contract"`
	Gotchas      []string `json:"gotchas"`
}

type boundary struct {
	NotApplicable  []string `json:"not_applicable"`
	HandoffTrigger []string `json:"handoff_trigger"`
	FallbackPath   string   `json:"fallback_path"`
}

type contract struct {
	Input       string   `json:"input"`
	Output      string   `json:"output"`
	Permissions []string `json:"permissions"`
}

type importResult struct {
	Dir    string `json:"dir"`
	Name   string `json:"name"`
	Status string `json:"status"` // imported / skipped_duplicate / error
	Detail string `json:"detail,omitempty"`
}

func main() {
	log.SetFlags(0)
	dir := flag.String("dir", "", "导入根目录：每个子目录是一个技能（含 skill.json）")
	force := flag.Bool("force", false, "同名技能已存在时覆盖（先删旧再插新）")
	flag.Parse()

	if *dir == "" {
		log.Fatal("用法: import_skills.exe -dir <导入根目录> [-force]")
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("解析目录失败: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		log.Fatalf("目录不存在: %s", root)
	}

	// 数据目录与后端保持一致：环境变量 SKILLHUB_DATA 可覆盖，默认 D:\skillhub-data
	dataDir := os.Getenv("SKILLHUB_DATA")
	if dataDir == "" {
		dataDir = `D:\skillhub-data`
	}
	archiveDir := filepath.Join(dataDir, "archives")
	filesDir := filepath.Join(dataDir, "files")
	proofsDir := filepath.Join(dataDir, "proofs")
	dbPath := filepath.Join(dataDir, "skillhub.db")
	for _, d := range []string{archiveDir, filesDir, proofsDir} {
		os.MkdirAll(d, 0o755)
	}

	// busy_timeout：服务可能同时在跑，避免立刻报 database is locked
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(10000)")
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ownerID, err := ensureOpsUser(db)
	if err != nil {
		log.Fatalf("准备运营账号失败: %v", err)
	}

	// 收集所有含 skill.json 的技能目录
	var skillDirs []string
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Fatalf("读取目录失败: %v", err)
	}
	for _, e := range entries {
		// 跳过隐藏目录和以下划线开头的目录（模板/说明目录，如 skills/_template）
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "skill.json")); err == nil {
			skillDirs = append(skillDirs, filepath.Join(root, e.Name()))
		}
	}
	sort.Strings(skillDirs)
	if len(skillDirs) == 0 {
		log.Fatalf("未在 %s 下找到任何含 skill.json 的技能目录", root)
	}

	log.Printf("数据目录: %s", dataDir)
	log.Printf("发现 %d 个技能目录，开始导入（同名已存在: %v）", len(skillDirs), *force)

	results := []importResult{}
	for _, sd := range skillDirs {
		r := importOne(db, sd, ownerID, archiveDir, filesDir, proofsDir, *force)
		results = append(results, r)
		log.Printf("[%s] %s %s", r.Status, r.Name, r.Detail)
	}

	// 汇总报告
	ok, dup, fail := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "imported":
			ok++
		case "skipped_duplicate":
			dup++
		case "error":
			fail++
		}
	}
	log.Printf("导入完成：成功 %d，跳过重复 %d，失败 %d", ok, dup, fail)
	for _, r := range results {
		if r.Status == "error" {
			log.Printf("  失败项: %s (%s)", r.Dir, r.Detail)
		}
	}
}

// ensureOpsUser 创建或复用运营导入账号，返回其 id
func ensureOpsUser(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, OpsUsername).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := db.Exec(`INSERT INTO users (username, email, password_hash, bio) VALUES (?, ?, ?, ?)`,
		OpsUsername, OpsEmail, "!", "运营批量导入的共享账号")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// importOne 导入单个技能目录
func importOne(db *sql.DB, dir string, ownerID int64, archiveDir, filesDir, proofsDir string, force bool) importResult {
	meta, err := loadMeta(filepath.Join(dir, "skill.json"))
	if err != nil {
		return importResult{Dir: dir, Status: "error", Detail: "skill.json 解析失败: " + err.Error()}
	}
	if strings.TrimSpace(meta.Name) == "" {
		return importResult{Dir: dir, Status: "error", Detail: "name 不能为空"}
	}
	if meta.Version == "" {
		meta.Version = "1.0.0"
	}
	if !allowedIntents[meta.TaskIntent] {
		meta.TaskIntent = ""
	}
	if len(meta.Tags) == 0 {
		meta.Tags = []string{}
	}
	tagsJSON, _ := json.Marshal(meta.Tags)

	// 同名检查
	var existsID int64
	err = db.QueryRow(`SELECT id FROM skills WHERE name = ?`, meta.Name).Scan(&existsID)
	if err == nil {
		if !force {
			return importResult{Dir: dir, Name: meta.Name, Status: "skipped_duplicate", Detail: "同名技能已存在，跳过（加 -force 覆盖）"}
		}
		// 覆盖：删除旧技能及附属数据
		if err := deleteSkill(db, existsID); err != nil {
			return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "覆盖旧技能失败: " + err.Error()}
		}
		log.Printf("  已删除旧技能 id=%d，重新导入 %s", existsID, meta.Name)
	} else if err != sql.ErrNoRows {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "查重失败: " + err.Error()}
	}

	// 质量初值：基于字段完整度，0.3~0.8，保证列表排序合理（新导入技能不会垫底也不会霸榜）
	score := scoreMeta(meta, dir, proofsDir)

	tx, err := db.Begin()
	if err != nil {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "开启事务失败: " + err.Error()}
	}
	defer tx.Rollback()

	// 1. 插入 skills（直接 published 进市场，origin 标记为运营导入）
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := tx.Exec(`INSERT INTO skills (owner_id, name, description, category, tags, version, status, origin, maintainer_id, quality_score, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerID, meta.Name, meta.Description, meta.Category, string(tagsJSON), meta.Version,
		StatusPublished, OriginOpsImport, ownerID, score, now, now)
	if err != nil {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "插入 skills 失败: " + err.Error()}
	}
	skillID, _ := res.LastInsertId()

	// 2. 同步创建版本行（与后端 createSkill 结构一致），并回填 current_version_id
	doneC, _ := json.Marshal(meta.DoneCriteria)
	workflow, _ := json.Marshal(meta.Workflow)
	boundaryJSON, _ := json.Marshal(meta.Boundary)
	contractJSON, _ := json.Marshal(meta.Contract)
	gotchas, _ := json.Marshal(meta.Gotchas)
	verRes, err := tx.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal, done_criteria, workflow, boundary, contract, gotchas, published_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		skillID, meta.Version, meta.Description, meta.Goal, string(doneC), string(workflow),
		string(boundaryJSON), string(contractJSON), string(gotchas), now)
	if err != nil {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "插入 skill_versions 失败: " + err.Error()}
	}
	verID, _ := verRes.LastInsertId()
	if _, err := tx.Exec(`UPDATE skills SET current_version_id = ? WHERE id = ?`, verID, skillID); err != nil {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "回填版本失败: " + err.Error()}
	}

	// 3. 处理技能包 zip：存档到 archives/<id>.zip，解压到 files/<id>/，登记 skill_files
	zipPath := filepath.Join(dir, "skill.zip")
	if fi, err := os.Stat(zipPath); err == nil && !fi.IsDir() {
		if err := importArchive(tx, skillID, zipPath, archiveDir, filesDir); err != nil {
			return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "处理 skill.zip 失败: " + err.Error()}
		}
	}

	// 4. 处理评估指标图片：复制到 proofs/<id>/，写入 proof_images
	proofDir := filepath.Join(dir, "proofs")
	if info, err := os.Stat(proofDir); err == nil && info.IsDir() {
		urls, err := importProofs(tx, skillID, proofDir, proofsDir)
		if err != nil {
			return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "处理 proofs 失败: " + err.Error()}
		}
		if len(urls) > 0 {
			pj, _ := json.Marshal(urls)
			if _, err := tx.Exec(`UPDATE skills SET proof_images = ? WHERE id = ?`, string(pj), skillID); err != nil {
				return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "写入 proof_images 失败: " + err.Error()}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return importResult{Dir: dir, Name: meta.Name, Status: "error", Detail: "提交事务失败: " + err.Error()}
	}
	return importResult{Dir: dir, Name: meta.Name, Status: "imported", Detail: fmt.Sprintf("id=%d quality=%.2f", skillID, score)}
}

// loadMeta 读取并解析 skill.json
func loadMeta(path string) (skillMeta, error) {
	var m skillMeta
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	return m, nil
}

// scoreMeta 按字段完整度打分（0.3~0.8）
func scoreMeta(m skillMeta, dir string, proofsDir string) float64 {
	s := 0.3
	if strings.TrimSpace(m.Description) != "" {
		s += 0.1
	}
	if len(m.Tags) >= 3 {
		s += 0.1
	}
	if strings.TrimSpace(m.Goal) != "" {
		s += 0.05
	}
	if len(m.DoneCriteria) >= 2 {
		s += 0.05
	}
	if len(m.Workflow) >= 3 {
		s += 0.1
	}
	if len(m.Boundary.NotApplicable) > 0 || len(m.Boundary.HandoffTrigger) > 0 || m.Boundary.FallbackPath != "" {
		s += 0.05
	}
	if len(m.Gotchas) >= 1 {
		s += 0.05
	}
	if fi, err := os.Stat(filepath.Join(dir, "skill.zip")); err == nil && !fi.IsDir() {
		s += 0.05
	}
	if fi, err := os.Stat(filepath.Join(dir, "proofs")); err == nil && fi.IsDir() {
		s += 0.05
	}
	_ = proofsDir
	if s > 0.8 {
		s = 0.8
	}
	return s
}

// importArchive 存档 zip + 解压 + 登记文件清单（与后端 saveAndExtractArchive 行为一致）
func importArchive(tx *sql.Tx, skillID int64, srcZip, archiveDir, filesDir string) error {
	skillDir := filepath.Join(filesDir, fmt.Sprintf("%d", skillID))
	os.MkdirAll(skillDir, 0o755)
	dstZip := filepath.Join(archiveDir, fmt.Sprintf("%d.zip", skillID))

	if err := copyFile(srcZip, dstZip); err != nil {
		return err
	}
	if err := extractZip(dstZip, skillDir); err != nil {
		return err
	}

	var totalSize int64
	count := 0
	err := filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		hash, err := sha256File(path)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO skill_files (skill_id, file_path, size, sha256) VALUES (?, ?, ?, ?)`,
			skillID, rel, info.Size(), hash); err != nil {
			return err
		}
		totalSize += info.Size()
		count++
		return nil
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE skills SET file_count = ?, total_size = ?, archive_path = ? WHERE id = ?`,
		count, totalSize, dstZip, skillID)
	return err
}

// importProofs 复制证明图片到 proofs/<id>/，返回 URL 列表（URL 格式与后端 saveProofImages 一致）
func importProofs(tx *sql.Tx, skillID int64, srcDir, proofsDir string) ([]string, error) {
	dstDir := filepath.Join(proofsDir, fmt.Sprintf("%d", skillID))
	os.MkdirAll(dstDir, 0o755)
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}
	var urls []string
	idx := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !isImageExt(ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > 10*1024*1024 {
			log.Printf("  跳过超大证明图片: %s (>10MB)", e.Name())
			continue
		}
		idx++
		name := fmt.Sprintf("%d%s", idx, ext)
		if err := copyFile(filepath.Join(srcDir, e.Name()), filepath.Join(dstDir, name)); err != nil {
			return urls, err
		}
		urls = append(urls, fmt.Sprintf("/uploads/proofs/%d/%s", skillID, name))
	}
	return urls, nil
}

// deleteSkill 删除技能及其附属数据（覆盖导入时用）
func deleteSkill(db *sql.DB, skillID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM skill_files WHERE skill_id = ?`,
		`DELETE FROM skill_versions WHERE skill_id = ?`,
		`DELETE FROM skill_evals WHERE skill_id = ?`,
		`DELETE FROM eval_runs WHERE skill_id = ?`,
		`DELETE FROM exec_feedbacks WHERE skill_id = ?`,
		`DELETE FROM version_candidates WHERE skill_id = ?`,
		`DELETE FROM skill_reviews WHERE skill_id = ?`,
		`DELETE FROM skill_issues WHERE skill_id = ?`,
		`DELETE FROM likes WHERE skill_id = ?`,
		`DELETE FROM skills WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, skillID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// extractZip 解压 zip 到目标目录（与后端 zip.go 同款实现，含路径穿越防护和
// Windows Compress-Archive 的 "\\" 目录条目兼容）。
func extractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(f.Name), "./"))
		if name == "." || strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)
		if strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, "\\") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
