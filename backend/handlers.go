package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---------- 查询 ----------

// listSkills GET /api/skills?keyword=&category=&sort=
func listSkills(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	category := strings.TrimSpace(c.Query("category"))
	sortBy := c.DefaultQuery("sort", "newest")

	where := []string{}
	args := []interface{}{}

	if keyword != "" {
		where = append(where, "(name LIKE ? OR description LIKE ? OR category LIKE ? OR tags LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like, like)
	}
	if category != "" && category != "全部" {
		where = append(where, "category = ?")
		args = append(args, category)
	}

	// 排序口径按 PRD 改造：热度可以反映注意力，任务证据才能说明能力。
	// 因此不再提供「按评分」「按下载量」排序；旧的 sort=rating / sort=downloads 一律映射到证据排序。
	order := "COALESCE(s.quality_score,0) DESC, s.created_at DESC"
	switch sortBy {
	case "newest":
		order = "s.created_at DESC"
	case "oldest":
		order = "s.created_at ASC"
	case "rating", "downloads", "evidence", "quality":
		order = "COALESCE(s.quality_score,0) DESC, s.created_at DESC"
	}

	// 只有通过门禁发布的才进市场；草稿、经验笔记、待门禁、已归档都不出现。
	// 老库里没有 status 的记录按 published 处理，避免历史数据凭空消失。
	where = append(where, "COALESCE(NULLIF(s.status,''),'published') = 'published'")

	query := `SELECT s.id, s.owner_id, COALESCE(u.username,''), s.name, s.description,
		s.category, s.tags, s.version, s.icon, s.file_count, s.total_size,
		s.download_count, s.view_count, s.rating, s.rating_count, s.created_at, s.updated_at,
		COALESCE(s.proof_images,'[]')
		FROM skills s LEFT JOIN users u ON s.owner_id = u.id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY " + order

	rows, err := db.Query(query, args...)
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
			&s.DownloadCount, &s.ViewCount, &s.Rating, &s.RatingCount, &s.CreatedAt, &s.UpdatedAt,
			&proofRaw); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		s.ProofImages = parseProofImages(proofRaw)
		skills = append(skills, s)
	}
	c.JSON(http.StatusOK, gin.H{"data": skills, "total": len(skills)})
}

// getSkill GET /api/skills/:id
func getSkill(c *gin.Context) {
	id := c.Param("id")

	var s Skill
	var proofRaw string
	err := db.QueryRow(`SELECT s.id, s.owner_id, COALESCE(u.username,''), s.name, s.description,
		s.category, s.tags, s.version, s.icon, s.file_count, s.total_size,
		s.download_count, s.view_count, s.rating, s.rating_count, s.created_at, s.updated_at,
		COALESCE(s.proof_images,'[]')
		FROM skills s LEFT JOIN users u ON s.owner_id = u.id WHERE s.id = ?`, id).
		Scan(&s.ID, &s.OwnerID, &s.OwnerName, &s.Name, &s.Description,
			&s.Category, &s.Tags, &s.Version, &s.Icon, &s.FileCount, &s.TotalSize,
			&s.DownloadCount, &s.ViewCount, &s.Rating, &s.RatingCount, &s.CreatedAt, &s.UpdatedAt,
			&proofRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	s.ProofImages = parseProofImages(proofRaw)

	// 文件列表
	rows, err := db.Query(`SELECT id, skill_id, file_path, size, sha256 FROM skill_files WHERE skill_id = ? ORDER BY file_path`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var f SkillFile
			if err := rows.Scan(&f.ID, &f.SkillID, &f.FilePath, &f.Size, &f.SHA256); err == nil {
				s.Files = append(s.Files, f)
			}
		}
	}

	// 浏览量 +1
	db.Exec(`UPDATE skills SET view_count = view_count + 1 WHERE id = ?`, id)

	c.JSON(http.StatusOK, gin.H{"data": s})
}

// ---------- 创建 ----------

// createSkill POST /api/skills (multipart/form-data, 需登录)
// 字段: name, description, category, tags(JSON数组), version
// 文件: archive (zip)
func createSkill(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	description := c.PostForm("description")
	category := c.PostForm("category")
	tags := c.PostForm("tags")
	if tags == "" {
		tags = "[]"
	} else if !json.Valid([]byte(tags)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tags must be a JSON array string"})
		return
	}
	version := c.PostForm("version")
	if version == "" {
		version = "1.0.0"
	}
	// 发布者：来自登录 token
	ownerID := c.GetInt64("userID")

	// 创建 skill 记录。
	// 「上传进门禁」：队友通过网页上传的技能先进「待测试」，四问门禁通过后才上架市场。
	result, err := db.Exec(`INSERT INTO skills (owner_id, name, description, category, tags, version,
		status, origin, maintainer_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerID, name, description, category, tags, version,
		SkillStatusGated, OriginRouteUpload, ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	skillID, _ := result.LastInsertId()

	// 同步建一个版本行，后续门禁、Trust Card、溯源都挂在版本上。
	// proof_type 用 artifact_upload：承认「有现成产物但无平台执行轨迹」，蒸馏度封顶 0.85，
	// 想拿满分就在工作台里做一次真实任务。
	var verID int64
	if verRes, err := db.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal,
		done_criteria, workflow, boundary, contract, gotchas, proof_type)
		VALUES (?, ?, ?, ?, '[]', '[]', '{"not_applicable":[],"handoff_trigger":[],"fallback_path":""}',
		'{"input":"","output":"","permissions":["read_upload"]}', '[]', ?)`,
		skillID, version, description, description, ProofArtifactUpload); err == nil {
		if vid, err := verRes.LastInsertId(); err == nil {
			verID = vid
			db.Exec(`UPDATE skills SET current_version_id = ? WHERE id = ?`, verID, skillID)
			// 预生成四类测试用例，门禁页打开就能跑
			if ver, lerr := loadSkillVersion(verID); lerr == nil {
				seedEvalCases(skillID, verID, ver, nil, nil)
			}
			// 评测平台：接收测试契约（contract）与环境需求（env），未提交则自动推导
			contract := contractFromForm(c, skillID, name, description)
			saveContract(contract)
			generateCasesFromContract(skillID, verID, contract)
		}
	}

	// 处理上传的 zip 包
	archive, err := c.FormFile("archive")
	if err == nil {
		if err := saveAndExtractArchive(c, skillID, archive); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save archive failed: " + err.Error()})
			return
		}
		// 解析 SKILL.md 评测锚点（核心步骤/完成标准/关键判断/失败案例/适用边界）写入草稿，
		// 让 AI 引导生成的 Skill 一上传就带齐蒸馏度六维与四问素材。
		if verID > 0 {
			if err := applySkillMDToDraft(skillID, verID); err != nil {
				log.Printf("applySkillMDToDraft skill=%d: %v", skillID, err)
			}
			// 草稿就绪后用真实内容重播测试用例（覆盖创建时用空草稿播种的那批）
			if ver, lerr := loadSkillVersion(verID); lerr == nil {
				seedEvalCases(skillID, verID, ver, loadDecisions(skillID), nil)
			}
		}
	}

	// 处理评估指标证明图片（多张，字段名 proof_images）
	if form, err := c.MultipartForm(); err == nil {
		if files, ok := form.File["proof_images"]; ok && len(files) > 0 {
			urls, perr := saveProofImages(skillID, files)
			if perr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "save proof images failed: " + perr.Error()})
				return
			}
			if len(urls) > 0 {
				proofJSON, _ := json.Marshal(urls)
				if _, uerr := db.Exec(`UPDATE skills SET proof_images = ? WHERE id = ?`, string(proofJSON), skillID); uerr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "save proof images failed: " + uerr.Error()})
					return
				}
			}
		}
	}

	skill, err := getSkillByID(skillID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	contract, _ := loadContract(skillID)
	c.JSON(http.StatusCreated, gin.H{
		"data":     skill,
		"status":   SkillStatusGated,
		"contract": contract,
		"gate": gin.H{
			"published": false,
			"message":   "已上传，进入四问测试门禁。评测管道通过后才会出现在市场里。",
		},
	})
}

// saveAndExtractArchive 保存 zip 并解压、登记文件清单
func saveAndExtractArchive(c *gin.Context, skillID int64, archive *multipart.FileHeader) error {
	skillDir := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	os.MkdirAll(skillDir, 0o755)

	// 保存原始 zip
	archivePath := filepath.Join(ArchiveDir, fmt.Sprintf("%d.zip", skillID))
	src, err := archive.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	dst.Close()

	// 解压
	if err := extractZip(archivePath, skillDir); err != nil {
		return err
	}

	// 登记文件清单
	files, err := indexFiles(skillID, skillDir)
	if err != nil {
		return err
	}

	var totalSize int64
	for _, f := range files {
		if _, err := db.Exec(`INSERT INTO skill_files (skill_id, file_path, size, sha256) VALUES (?, ?, ?, ?)`,
			skillID, f.FilePath, f.Size, f.SHA256); err != nil {
			return err
		}
		totalSize += f.Size
	}

	_, err = db.Exec(`UPDATE skills SET file_count = ?, total_size = ?, archive_path = ? WHERE id = ?`,
		len(files), totalSize, archivePath, skillID)
	return err
}

// indexFiles 遍历目录生成文件清单（相对路径、大小、sha256）
func indexFiles(skillID int64, root string) ([]SkillFile, error) {
	var files []SkillFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		hash, err := sha256File(path)
		if err != nil {
			return err
		}
		files = append(files, SkillFile{
			SkillID:  skillID,
			FilePath: rel,
			Size:     info.Size(),
			SHA256:   hash,
		})
		return nil
	})
	return files, err
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

// ---------- 下载 ----------

// downloadSkill GET /api/skills/:id/download
func downloadSkill(c *gin.Context) {
	id := c.Param("id")
	var s Skill
	err := db.QueryRow(`SELECT id, name, archive_path FROM skills WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.ArchivePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	// 有原始 zip 直接下载；否则现场打包
	zipPath := s.ArchivePath
	if zipPath == "" || !fileExists(zipPath) {
		skillDir := filepath.Join(FilesDir, id)
		zipPath = filepath.Join(ArchiveDir, fmt.Sprintf("%s_download.zip", id))
		if err := zipDirectory(skillDir, zipPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "package failed: " + err.Error()})
			return
		}
	}

	db.Exec(`UPDATE skills SET download_count = download_count + 1 WHERE id = ?`, id)

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, sanitizeFilename(s.Name)))
	c.File(zipPath)
}

// ---------- 删除 ----------

// deleteSkill DELETE /api/skills/:id（仅属主可删）
func deleteSkill(c *gin.Context) {
	id := c.Param("id")
	uid := c.GetInt64("userID")

	var ownerID *int64
	err := db.QueryRow(`SELECT owner_id FROM skills WHERE id = ?`, id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	if ownerID == nil || *ownerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅技能属主可删除"})
		return
	}

	db.Exec(`DELETE FROM skill_files WHERE skill_id = ?`, id)
	result, err := db.Exec(`DELETE FROM skills WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	// 清理磁盘文件
	os.RemoveAll(filepath.Join(FilesDir, id))
	os.Remove(filepath.Join(ArchiveDir, id+".zip"))
	os.RemoveAll(filepath.Join(ProofsDir, id))
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ---------- 工具 ----------

func getSkillByID(id int64) (*Skill, error) {
	var s Skill
	var proofRaw string
	err := db.QueryRow(`SELECT s.id, s.owner_id, COALESCE(u.username,''), s.name, s.description,
		s.category, s.tags, s.version, s.icon, s.file_count, s.total_size,
		s.download_count, s.view_count, s.rating, s.rating_count, s.created_at, s.updated_at,
		COALESCE(s.proof_images,'[]')
		FROM skills s LEFT JOIN users u ON s.owner_id = u.id WHERE s.id = ?`, id).
		Scan(&s.ID, &s.OwnerID, &s.OwnerName, &s.Name, &s.Description,
			&s.Category, &s.Tags, &s.Version, &s.Icon, &s.FileCount, &s.TotalSize,
			&s.DownloadCount, &s.ViewCount, &s.Rating, &s.RatingCount, &s.CreatedAt, &s.UpdatedAt,
			&proofRaw)
	if err != nil {
		return nil, err
	}
	s.ProofImages = parseProofImages(proofRaw)
	return &s, nil
}

// parseProofImages 把数据库中的 JSON 数组字符串解析为图片 URL 列表
func parseProofImages(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(s), &urls); err != nil {
		return nil
	}
	return urls
}

// isImageExt 是否允许作为证明图片的扩展名
func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// saveProofImages 保存多张证明图片到 ProofsDir/<skillID>/，返回可访问的 URL 列表
func saveProofImages(skillID int64, files []*multipart.FileHeader) ([]string, error) {
	dir := filepath.Join(ProofsDir, fmt.Sprintf("%d", skillID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	urls := []string{}
	for i, fh := range files {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if !isImageExt(ext) {
			continue // 只接受图片，其余忽略
		}
		if fh.Size > 10*1024*1024 {
			continue // 单张超过 10MB 忽略
		}
		name := fmt.Sprintf("%d%s", i+1, ext)
		dst := filepath.Join(dir, name)
		src, err := fh.Open()
		if err != nil {
			return urls, err
		}
		d, err := os.Create(dst)
		if err != nil {
			src.Close()
			return urls, err
		}
		if _, err := io.Copy(d, src); err != nil {
			src.Close()
			d.Close()
			return urls, err
		}
		src.Close()
		d.Close()
		urls = append(urls, fmt.Sprintf("/uploads/proofs/%d/%s", skillID, name))
	}
	return urls, nil
}

func nullableInt64(s string) interface{} {
	if s == "" {
		return nil
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_")
	return replacer.Replace(name)
}
