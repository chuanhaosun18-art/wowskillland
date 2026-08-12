// 沉淀双通道（v1.3）
// 通道 A：AI 访谈式多轮口述——沉淀教练逐题追问，聊完由 LLM 结构化提取后走 backfill 落库。
// 通道 B：上传 Skill 包——multipart 收 zip，落库后每次都用 LLM 做四维评测：
//        可检索性 / 文件完备性 / 格式完整性 / 边界控制（硬门槛），结果落 upload_evals 展示。
// 两条通道共用 backfillExecution 的落库主体（runBackfill），保证产物同构。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- 表结构 ----------

// initSedimentSchema 沉淀评测结果表：每次上传 skill 包都会跑一次 LLM 四维评测并留档
func initSedimentSchema() {
	db.Exec(`CREATE TABLE IF NOT EXISTS upload_evals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		skill_id INTEGER NOT NULL,
		version_id INTEGER NOT NULL,
		dimensions TEXT NOT NULL,
		overall_verdict TEXT NOT NULL,
		overall_score REAL DEFAULT 0,
		llm_raw TEXT DEFAULT '',
		degraded INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
}

// ---------- 评测维度常量 ----------

const (
	EvalDimSearchable = "searchable" // 可检索性：能否被成功搜索到
	EvalDimComplete   = "complete"   // 文件完备性：SKILL.md 与被引用文件齐全、无空文件
	EvalDimFormat     = "format"     // 格式完整性：五锚点区块齐全、JSON 合法、无截断
	EvalDimBoundary   = "boundary"   // 边界控制（硬门槛）：不适用条件 + 交回给人触发点
)

var evalDimLabels = map[string]string{
	EvalDimSearchable: "可检索性",
	EvalDimComplete:   "文件完备性",
	EvalDimFormat:     "格式完整性",
	EvalDimBoundary:   "边界控制",
}

// evalDimension LLM 输出的单维评测
type evalDimension struct {
	Key        string   `json:"key"`
	Score      float64  `json:"score"`
	Verdict    string   `json:"verdict"` // pass | fail
	Issues     []string `json:"issues"`
	Suggestion string   `json:"suggestion"`
}

// packageEvalResult 一次上传评测的完整结果
type packageEvalResult struct {
	Dimensions []evalDimension `json:"dimensions"`
	Summary    string          `json:"summary"`
}

func (e *packageEvalResult) overall() (string, float64) {
	total := 0.0
	pass := true
	for _, d := range e.Dimensions {
		total += d.Score
		if d.Verdict != "pass" {
			pass = false
		}
	}
	n := len(e.Dimensions)
	if n == 0 {
		return "fail", 0
	}
	return map[bool]string{true: "pass", false: "fail"}[pass], total / float64(n)
}

// ---------- 通道 B：上传 Skill 包 + LLM 四维评测 ----------

// sedimentUpload POST /api/growth/sediment/upload（需登录，multipart）
// 字段：archive（zip，必填）、name（必填）、description、tags（JSON 数组字符串）
// 流程：落库 gated skill → 存 zip 解压登记 → 解析 SKILL.md 锚点进草稿 → LLM 四维评测 → 落库并返回评测卡。
func sedimentUpload(c *gin.Context) {
	uid := c.GetInt64("userID")
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	archive, err := c.FormFile("archive")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "archive (zip) is required"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(archive.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只接受 .zip 格式的 Skill 包"})
		return
	}
	description := c.PostForm("description")
	tags := c.PostForm("tags")
	if tags == "" {
		tags = "[]"
	} else if !json.Valid([]byte(tags)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tags must be a JSON array string"})
		return
	}

	// 与 createSkill 同构：先进「待测试/门禁」，四问门禁通过后才上架
	res, err := db.Exec(`INSERT INTO skills (owner_id, name, description, category, tags, version,
		status, origin, maintainer_id) VALUES (?, ?, ?, '', ?, '1.0', ?, ?, ?)`,
		uid, name, description, tags, SkillStatusGated, OriginRouteUpload, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	skillID, _ := res.LastInsertId()

	var verID int64
	if verRes, err := db.Exec(`INSERT INTO skill_versions (skill_id, version, description, goal,
		done_criteria, workflow, boundary, contract, gotchas, proof_type)
		VALUES (?, '1.0', ?, ?, '[]', '[]', '{"not_applicable":[],"handoff_trigger":[],"fallback_path":""}',
		'{"input":"","output":"","permissions":["read_upload"]}', '[]', ?)`,
		skillID, description, name, ProofArtifactUpload); err == nil {
		if vid, lerr := verRes.LastInsertId(); lerr == nil {
			verID = vid
			db.Exec(`UPDATE skills SET current_version_id = ? WHERE id = ?`, verID, skillID)
		}
	}
	if verID == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create version failed"})
		return
	}

	// 存 zip + 解压登记文件清单 + 解析 SKILL.md 锚点进草稿
	if err := saveAndExtractArchive(c, skillID, archive); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save archive failed: " + err.Error()})
		return
	}
	if err := applySkillMDToDraft(skillID, verID); err != nil {
		log.Printf("sediment-upload applySkillMDToDraft skill=%d: %v", skillID, err)
	}

	// 每次上传都跑 LLM 四维评测，结果落库
	eval, degraded := runPackageEval(skillID, verID, name, description)
	verdict, score := eval.overall()
	dimsJSON, _ := json.Marshal(eval.Dimensions)
	db.Exec(`INSERT INTO upload_evals (skill_id, version_id, dimensions, overall_verdict, overall_score, llm_raw, degraded)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		skillID, verID, string(dimsJSON), verdict, score, truncate(eval.Summary, 500), boolInt(degraded))

	c.JSON(http.StatusCreated, gin.H{
		"skill_id":   skillID,
		"version_id": verID,
		"status":     SkillStatusGated,
		"eval": gin.H{
			"dimensions":      eval.Dimensions,
			"summary":         eval.Summary,
			"overall_verdict": verdict,
			"overall_score":   score,
			"degraded":        degraded,
			"labels":          evalDimLabels,
			"hard_gate":       EvalDimBoundary,
		},
	})
}

// getSedimentEval GET /api/growth/sediment/evals/:skillID
// 取该 skill 最近一次上传评测结果（评测后编辑再查 / 分享用）
func getSedimentEval(c *gin.Context) {
	skillID, err := strconv.ParseInt(c.Param("skillID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill id"})
		return
	}
	var dimsJSON, verdict string
	var score float64
	var degraded int
	err = db.QueryRow(`SELECT dimensions, overall_verdict, overall_score, degraded
		FROM upload_evals WHERE skill_id = ? ORDER BY id DESC LIMIT 1`, skillID).
		Scan(&dimsJSON, &verdict, &score, &degraded)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no eval found"})
		return
	}
	var dims []evalDimension
	json.Unmarshal([]byte(dimsJSON), &dims)
	c.JSON(http.StatusOK, gin.H{
		"skill_id":        skillID,
		"dimensions":      dims,
		"overall_verdict": verdict,
		"overall_score":   score,
		"degraded":        degraded == 1,
		"labels":          evalDimLabels,
		"hard_gate":       EvalDimBoundary,
	})
}

// evalPackageSystemPrompt 四维评测员 system prompt（对齐 Anthropic 官方 Agent Skill 规范）
const evalPackageSystemPrompt = `你是 Skill 包质量评测员。平台遵循 Anthropic 官方 Agent Skill 规范，每个上传的 Skill 包必须通过四维评测才能进入市场。
评测依据只有两样：用户的 name/description 声明 + 包内真实文件内容快照。要实事求是，有就是有，没有就是没有，禁止脑补。

Anthropic 官方规范要点（评测必须对照执行）：
- SKILL.md 顶部必须有 YAML frontmatter（--- 包裹），必填 name 与 description。
- name：kebab-case（小写字母、数字、连字符），≤64 字符，与包内文件夹名一致，禁止空格/大写/下划线，禁止泛化名（如 skill、helper）。
- description：第三人称（禁止「I can…/You can use…」），≤1024 字符，说明「做什么 + 何时用（Use this skill when…，可列具体触发场景）」，禁止包含保留词 anthropic/claude。description 是触发检索的唯一依据。
- body 用命令式（Create/Use/Prefer），≤500 行，说明每一步的 why；长文档拆到 references/（渐进披露，引用只允许一层）。
- 包内目录：SKILL.md 必在；references/（按需加载的文档）、scripts/（可执行代码）、assets/（模板素材）为可选标准目录。

严格只输出 JSON，不要 markdown 代码块，不要任何多余文字。必须且仅输出一个顶层对象，且顶层只能有 dimensions 与 summary 两个键；dimensions 必须是数组，数组内恰好 4 个对象（key 依次为 searchable、complete、format、boundary），严禁把任一维度对象平铺到顶层。完整格式示例：
{"dimensions":[{"key":"searchable","score":0.9,"verdict":"pass","issues":["name 含大写，应改 kebab-case"],"suggestion":"将 name 改为 lab-visit-mail"},{"key":"complete","score":1.0,"verdict":"pass","issues":["SKILL.md 与 references/template.md 均存在且非空"],"suggestion":""},{"key":"format","score":0.7,"verdict":"pass","issues":["description 被截断"],"suggestion":"补全 description 至完整句子"},{"key":"boundary","score":1.0,"verdict":"pass","issues":["已明确不适用条件与交回给人触发点"],"suggestion":""}],"summary":"一句话总评"}

四维判分标准：
1. searchable 可检索性：name 为合法 kebab-case 且不泛化；description 为第三人称、说明「帮谁解决什么问题、产出什么、何时触发」、覆盖用户会搜的关键词；tags 覆盖检索词（≥3 个）。frontmatter 缺失或 description 触发不清判 fail。
2. complete 文件完备性：SKILL.md 必须存在；SKILL.md 里提到的文件（references/scripts/gotchas/evals 等目录或具体文件名）必须在包内真实存在；不允许空文件、0 字节占位、坏链接。
3. format 格式完整性：SKILL.md 必须有合法 YAML frontmatter（name+description）；body 含五锚点区块「核心步骤 / 完成标准 / 关键判断 / 失败案例 / 适用边界」（以 ## 标题出现）；包内 JSON 文件必须可解析；内容不允许截断、乱码、残缺。
4. boundary 边界控制（硬门槛）：必须明确写出「不适用条件」（什么情况这套方法不能用）与「交回给人」触发点（出现什么信号必须人工介入或停机）。这两条缺失或含糊一律 fail，无论其他维度多好。
overall 规则：边界 fail 则整体 fail，其余维度各自独立判分。`

// runPackageEval 跑一次 LLM 四维评测；LLM 失败时用确定性事实降级出结果（degraded=true）。
func runPackageEval(skillID, verID int64, name, description string) (*packageEvalResult, bool) {
	snapshot := exploreSkillPackage(skillID)
	facts := packageFacts(skillID, verID)

	var sb strings.Builder
	sb.WriteString("【用户声明】\n")
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("description: " + description + "\n\n")
	sb.WriteString("【确定性事实（来自磁盘扫描，可信）】\n" + facts + "\n")
	sb.WriteString("【包内容快照】\n" + snapshot)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	raw, err := callGuideDeepSeekOpts(ctx, []chatMsg{
		{Role: "system", Content: evalPackageSystemPrompt},
		{Role: "user", Content: sb.String()},
	}, 4000)
	if err != nil {
		log.Printf("sediment-eval llm err: %v", err)
		return degradedEval(facts), true
	}

	var out packageEvalResult
	jsonStr := extractJSON(raw)
	if e := json.Unmarshal([]byte(jsonStr), &out); e != nil {
		// 兜底1：LLM 偶发把部分维度对象平铺到顶层（如 {"dimensions":[{searchable}],"complete":{...}}），归一化后重试
		if norm := coalesceEvalJSON(jsonStr); norm != jsonStr {
			if e2 := json.Unmarshal([]byte(norm), &out); e2 == nil && len(out.Dimensions) > 0 {
				return &out, false
			}
		}
		if repaired := repairClosingJSON(jsonStr); repaired != jsonStr {
			if e2 := json.Unmarshal([]byte(repaired), &out); e2 == nil && len(out.Dimensions) > 0 {
				return &out, false
			}
		}
		log.Printf("sediment-eval unmarshal fail: %v raw=%q", e, truncate(raw, 500))
		return degradedEval(facts), true
	}
	return &out, false
}

// coalesceEvalJSON 解析兜底：LLM 偶尔把部分维度对象平铺到顶层（如 {"dimensions":[{searchable}],"complete":{...}}），
// 把它们归一化回 dimensions 数组（补上 key 字段），保证四个维度都在数组内、结构合法。
func coalesceEvalJSON(jsonStr string) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &top); err != nil {
		return jsonStr
	}
	dims := []json.RawMessage{}
	if raw, ok := top["dimensions"]; ok {
		if err := json.Unmarshal(raw, &dims); err != nil {
			dims = []json.RawMessage{}
		}
	}
	seen := map[string]bool{}
	for _, d := range dims {
		var tmp struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(d, &tmp); err == nil && tmp.Key != "" {
			seen[tmp.Key] = true
		}
	}
	changed := false
	for _, key := range []string{"searchable", "complete", "format", "boundary"} {
		if seen[key] {
			continue
		}
		raw, ok := top[key]
		if !ok {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		_, hasScore := obj["score"]
		_, hasVerdict := obj["verdict"]
		if !hasScore && !hasVerdict {
			continue
		}
		obj["key"], _ = json.Marshal(key)
		if merged, err := json.Marshal(obj); err == nil {
			dims = append(dims, merged)
			changed = true
		}
	}
	if !changed {
		return jsonStr
	}
	var b strings.Builder
	b.WriteString(`{"dimensions":`)
	dm, _ := json.Marshal(dims)
	b.Write(dm)
	b.WriteString(`,"summary":`)
	if s, ok := top["summary"]; ok {
		b.Write(s)
	} else {
		b.WriteString(`""`)
	}
	b.WriteString(`}`)
	return b.String()
}

// packageFacts 磁盘确定性事实：SKILL.md 是否存在、锚点区块是否齐全、文件数、空文件
func packageFacts(skillID, verID int64) string {
	var sb strings.Builder
	mdPath := findSkillMD(skillID)
	if mdPath == "" {
		sb.WriteString("- SKILL.md：缺失（这是必检项，直接判 fail）\n")
	} else {
		md, err := os.ReadFile(mdPath)
		if err != nil {
			sb.WriteString("- SKILL.md：存在但无法读取\n")
		} else {
			text := string(md)
			anchors := []string{"核心步骤", "完成标准", "关键判断", "失败案例", "适用边界"}
			hit := []string{}
			for _, a := range anchors {
				if strings.Contains(text, a) {
					hit = append(hit, a)
				}
			}
			miss := strings.Join(missingAnchors(anchors, hit), "、")
			if miss == "" {
				miss = "无"
			}
			sb.WriteString(fmt.Sprintf("- SKILL.md：存在（%d 字符），五锚点区块命中：%s（缺：%s）\n",
				len([]rune(text)), strings.Join(hit, "、"), miss))
			if strings.TrimSpace(text) == "" {
				sb.WriteString("- SKILL.md：内容为空\n")
			}
		}
	}

	root := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	files, empties := 0, 0
	var emptyNames []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		files++
		if info.Size() == 0 {
			empties++
			emptyNames = append(emptyNames, filepath.Base(path))
		}
		return nil
	})
	sb.WriteString(fmt.Sprintf("- 包内文件总数：%d；空文件：%d 个（%s）\n", files, empties, strings.Join(emptyNames, "、")))

	// 版本字段佐证：草稿里是否已有解析出的 workflow/边界
	var workflow, boundary string
	db.QueryRow(`SELECT COALESCE(workflow,'[]'), COALESCE(boundary,'{}') FROM skill_versions WHERE id = ?`, verID).
		Scan(&workflow, &boundary)
	var steps []json.RawMessage
	json.Unmarshal([]byte(workflow), &steps)
	var b map[string]interface{}
	json.Unmarshal([]byte(boundary), &b)
	sb.WriteString(fmt.Sprintf("- SKILL.md 已解析进草稿：核心步骤 %d 条；适用边界字段 %s\n",
		len(steps), jsonOrEmpty(b)))
	return sb.String()
}

func missingAnchors(all, hit []string) []string {
	has := map[string]bool{}
	for _, h := range hit {
		has[h] = true
	}
	miss := []string{}
	for _, a := range all {
		if !has[a] {
			miss = append(miss, a)
		}
	}
	return miss
}

// degradedEval LLM 不可用时的确定性降级评测：只依据磁盘事实，四维各自判 pass/fail，score 给 0.5 基准分
func degradedEval(facts string) *packageEvalResult {
	mdOK := strings.Contains(facts, "SKILL.md：存在")
	anchorsOK := mdOK && strings.Contains(facts, "缺：无")
	boundaryOK := mdOK && !strings.Contains(facts, "缺：适用边界")
	emptyOK := strings.Contains(facts, "空文件：0 个")

	dims := []evalDimension{
		{Key: EvalDimSearchable, Score: 0.5, Verdict: "fail", Issues: []string{"LLM 不可用，无法评估描述与检索词覆盖，请人工确认 name/description/tags"}, Suggestion: "检查名称是否独特、描述是否说清帮谁解决什么问题、tags 是否覆盖检索词"},
	}
	if mdOK && anchorsOK && emptyOK {
		dims = append(dims, evalDimension{Key: EvalDimComplete, Score: 0.6, Verdict: "pass", Issues: nil, Suggestion: ""})
		dims = append(dims, evalDimension{Key: EvalDimFormat, Score: 0.6, Verdict: "pass", Issues: nil, Suggestion: ""})
	} else {
		issues := []string{"SKILL.md 缺失或锚点区块不全"}
		if !emptyOK {
			issues = append(issues, "包内存在空文件")
		}
		dims = append(dims, evalDimension{Key: EvalDimComplete, Score: 0.3, Verdict: "fail", Issues: issues, Suggestion: "补上 SKILL.md 与各引用文件，删掉空文件"})
		dims = append(dims, evalDimension{Key: EvalDimFormat, Score: 0.3, Verdict: "fail", Issues: issues, Suggestion: "补上核心步骤/完成标准/关键判断/失败案例/适用边界五锚点"})
	}
	if boundaryOK {
		dims = append(dims, evalDimension{Key: EvalDimBoundary, Score: 0.6, Verdict: "pass", Issues: nil, Suggestion: ""})
	} else {
		dims = append(dims, evalDimension{Key: EvalDimBoundary, Score: 0.2, Verdict: "fail", Issues: []string{"适用边界区块缺失或未明确不适用条件/交回给人触发点"}, Suggestion: "在 SKILL.md 里写清不适用条件与人工接管触发点"})
	}
	return &packageEvalResult{
		Dimensions: dims,
		Summary:    "评测服务暂不可用，本次为确定性降级结果：只检查了文件事实，描述与检索性需人工复核。",
	}
}

// ---------- 通道 A：AI 访谈式多轮口述沉淀 ----------

// sedimentChat POST /api/growth/sediment/chat（需登录）
// 无状态多轮：前端每次携带完整对话历史，教练逐题追问；每轮末尾输出【进度】N% 与【抽取】JSON。
func sedimentChat(c *gin.Context) {
	var body struct {
		Messages []chatMsg `json:"messages"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	history := cleanHistory(body.Messages)
	if len(history) == 0 {
		c.JSON(http.StatusOK, sedimentChatResp(
			"来，先跟我讲讲你亲手做成过的那件事吧——什么事、最后做成了什么样？", 0, nil, true))
		return
	}

	messages := append([]chatMsg{{Role: "system", Content: sedimentCoachSystemPrompt}}, history...)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	content, err := callGuideDeepSeekOpts(ctx, messages, 3000)
	if err != nil {
		log.Printf("sediment-chat llm err: %v", err)
		c.JSON(http.StatusOK, sedimentChatResp("AI 暂时没回应，你继续讲就行，我记着前面说的。", 0, nil, true))
		return
	}

	reply, progress, extracted := parseSedimentChat(content)
	c.JSON(http.StatusOK, gin.H{
		"reply":     reply,
		"progress":  progress,
		"extracted": extracted,
		"degraded":  false,
	})
}

// sedimentChatResp 降级/开场兜底
func sedimentChatResp(reply string, progress int, extracted interface{}, degraded bool) gin.H {
	if extracted == nil {
		extracted = gin.H{"task_title": "", "before": "", "after": "", "decisions": []backfillDecision{}}
	}
	return gin.H{"reply": reply, "progress": progress, "extracted": extracted, "degraded": degraded}
}

// sedimentCoachSystemPrompt 访谈教练：引导复盘「做成的一件事」，逐题追问，按 Anthropic 官方 Agent Skill 规范收集素材
const sedimentCoachSystemPrompt = `你是 WowSkillLand 平台的「沉淀访谈教练」。用户是过来人——TA 亲手做成过某件事（保研上岸、拿竞赛奖、论文写好、带过项目……），现在来平台复盘这段经历，把它变成一张符合 Anthropic 官方 Agent Skill 规范的可复用 Skill 卡，帮后来人少踩坑。

你的任务：像朋友一样，用简体中文逐题引导 TA 把这段经历讲清楚。不要教 TA 怎么做这件事，只让 TA 讲当时自己是怎么做的、踩过什么坑、沉淀了什么方法。

【要收集的信息（按顺序引导，一次只问 1-2 个问题，绝不一次性全抛）】
0. Skill 命名与描述（Anthropic 规范，全程边收边沉淀）：目标 Skill 名用 kebab-case（小写+连字符，如 follow-up-lab-email）；描述必须第三人称、说清「帮谁解决什么问题、产出什么、何时触发」（Use this skill when…）。听到 TA 讲清这两点后再进入下面收集。
1. 是什么事 + 最后做成什么样：复述 TA 的原话确认任务（一句概括），并问清最终的产物/结果是什么。
2. 关键判断（最值钱，至少 2 条）：做这件事的岔路口 TA 是怎么决策的——「出现什么信号 → 做了什么判断 → 在什么场景成立」。主动追问四种岔路口：在哪一步停下来回头验证 / 什么情况下要求补充信息而不是直接动手 / 哪一步必须查、必须跑 / 什么现象一出现就知道这条路走不通。
3. 失败案例：踩过的坑、做错的事、后果是什么。
4. 适用边界（硬门槛，必问）：什么情况下这套方法不适用？出现什么信号必须交回给人处理、不能照做？
5. 当时的感受：最难熬或最爽的一刻（故事层，简单带过即可）。

【纪律】
- 每次只问 1-2 题；用户说得模糊就追问具体细节；用户不知道从哪说起就给出贴近 TA 描述的小示例。
- 用户提到岔路口（"当时纠结过""差点踩坑"）时，立刻追问当时的判断依据和触发信号。
- 信息基本齐全（任务、产物、至少 2 条关键判断、失败案例、适用边界）时，告诉 TA：可以点「完成沉淀」了。
- 回复简洁、亲切、鼓励；全程简体中文。

【输出格式铁律】回复正文结束后，最后两行必须输出标签，之后不要再有任何内容：
【进度】N%（0-100 整数，按已收集信息估计）
【抽取】{"task_title":"","before":"","after":"","decisions":[{"slot":"when_to_check|when_to_probe|when_to_use_tool|when_to_switch","trigger_signal":"","judgment":"","scope":""}]}
【抽取】只放对话里已经明确确认、且能从 TA 原话找到依据的内容；没有的一律空字符串，decisions 空数组。`

// parseSedimentChat 解析教练回复尾部的【进度】与【抽取】标签，取不到就给出安全的默认值
func parseSedimentChat(content string) (string, int, gin.H) {
	lines := strings.Split(content, "\n")
	progress := 0
	extracted := gin.H{"task_title": "", "before": "", "after": "", "decisions": []backfillDecision{}}
	bodyLines := []string{}

	progressRe := regexp.MustCompile(`^【进度】\s*(\d{1,3})%?`)
	extractRe := regexp.MustCompile(`^【抽取】\s*(\{.*)`)
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if m := progressRe.FindStringSubmatch(trimmed); m != nil {
			if p, err := strconv.Atoi(m[1]); err == nil && p >= 0 && p <= 100 {
				progress = p
			}
			continue // 标签行不进正文
		}
		if m := extractRe.FindStringSubmatch(trimmed); m != nil {
			var out struct {
				TaskTitle string             `json:"task_title"`
				Before    string             `json:"before"`
				After     string             `json:"after"`
				Decisions []backfillDecision `json:"decisions"`
			}
			if json.Unmarshal([]byte(extractJSON(m[1])), &out) == nil {
				extracted = gin.H{
					"task_title": out.TaskTitle,
					"before":     out.Before,
					"after":      out.After,
					"decisions":  out.Decisions,
				}
			}
			continue
		}
		bodyLines = append(bodyLines, ln)
	}
	return strings.TrimSpace(strings.Join(bodyLines, "\n")), progress, extracted
}

// sedimentFinish POST /api/growth/sediment/finish（需登录）
// 访谈收尾：LLM 从完整对话提取 backfill 结构，校验原话依据（无来源即丢弃），复用 runBackfill 落库。
func sedimentFinish(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		Messages []chatMsg `json:"messages"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	history := cleanHistory(body.Messages)
	transcript := buildTranscript(history)
	if len([]rune(transcript)) < 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "聊得太短了，再多讲几句细节：是什么事、怎么做成的、踩过什么坑。"})
		return
	}

	extracted, err := extractBackfill(transcript)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "整理失败：" + err.Error()})
		return
	}

	// 无来源即丢弃：关键判断必须能在原话里找到依据
	kept := extracted.Decisions[:0]
	for _, d := range extracted.Decisions {
		if !isValidSlot(d.Slot) {
			continue
		}
		if !evidenceIn(transcript, d.TriggerSignal) || !evidenceIn(transcript, d.Judgment) || !evidenceIn(transcript, d.Scope) {
			continue
		}
		kept = append(kept, d)
	}
	extracted.Decisions = kept
	// 补录场景只有「做之前/做之后」两个自述阶段，判断统一挂到阶段 1
	for i := range extracted.Decisions {
		extracted.Decisions[i].StageIndex = 1
	}

	if strings.TrimSpace(extracted.After) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "还没聊清楚最终做成了什么，补一句再收尾吧。"})
		return
	}
	if extracted.TaskIntent == "" || !isProductive(extracted.TaskIntent) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没能判断出这段经验属于哪个任务类型（论文/简历/汇报/项目/文献），补充说明是做什么事再收尾。"})
		return
	}

	log.Printf("sediment-finish: uid=%d intent=%s title=%q decisions=%d transcript=%d 字",
		uid, extracted.TaskIntent, extracted.TaskTitle, len(extracted.Decisions), len([]rune(transcript)))
	runBackfill(c, uid, *extracted)
}

// cleanHistory 过滤非法角色与空内容，截断到最近 40 条
func cleanHistory(msgs []chatMsg) []chatMsg {
	out := []chatMsg{}
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if content == "" {
			content = strings.TrimSpace(m.Text) // 前端沉淀模块发 {role,text}，兼容回退
		}
		if role != "user" && role != "assistant" {
			continue
		}
		if content == "" {
			continue
		}
		out = append(out, chatMsg{Role: role, Content: truncate(content, 2000)})
	}
	if len(out) > 40 {
		out = out[len(out)-40:]
	}
	return out
}

// buildTranscript 把对话历史拼成纯文本，供 LLM 提取与溯源校验
func buildTranscript(history []chatMsg) string {
	var sb strings.Builder
	for _, m := range history {
		if m.Role == "user" {
			sb.WriteString(m.Content + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// evidenceIn 溯源校验：逐字命中 或 关键词重合度 ≥0.6（容忍 LLM 摘录时少量改写）
func evidenceIn(transcript, field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	if strings.Contains(transcript, field) {
		return true
	}
	return overlapScore(keyTerms(transcript), keyTerms(field)) >= 0.6
}

// extractBackfillSystemPrompt 访谈收尾的结构化提取器（对齐 Anthropic 官方 Agent Skill 规范）
const extractBackfillSystemPrompt = `你是经验沉淀的结构化抽取器。把用户复盘对话整理成补录请求，产出要符合 Anthropic 官方 Agent Skill 规范。只抽取对话里明确说到的内容，没说的留空，禁止脑补。

命名与描述规范（Anthropic 官方）：
- skill_name：kebab-case（小写字母、数字、连字符），≤64 字符，如 follow-up-lab-email；对话里没有合适名字就根据任务内容拟一个，禁止泛化名（skill、helper）。
- skill_description：第三人称（禁止 I can…/You can use…），一句话说清「帮谁解决什么问题、产出什么、何时触发」，≤1024 字符；没有依据就留空。

任务类型从下面选（输出键名，选最接近的一次性可交付任务）：
thesis_topic 论文选题打磨与收敛
resume_rewrite 把科研经历改成产研岗位简历
resume_jd_align 简历与具体 JD 对齐
report_structure 组会汇报与答辩陈述结构
project_convergence 项目与竞赛方案收敛
literature_review 文献综述入门与检索策略
（保研/考研/求职这类人生工程不是可交付任务，若对话是这类内容，选其中最接近的可交付子任务；仍对不上则 task_intent 输出空字符串）

关键判断 slot 只允许四个键之一：
when_to_check（在哪一步停下回头验证）/ when_to_probe（什么情况要求补充信息再动手）/ when_to_use_tool（哪一步必须查、必须跑）/ when_to_switch（什么现象出现就知道走不通）
trigger_signal 是触发信号（出现什么现象），judgment 是当时做的判断/做法，scope 是适用场景。只放对话里明确有依据的判断，最多 4 条。

严格只输出 JSON，不要 markdown 代码块：
{"task_intent":"","task_title":"","skill_name":"","skill_description":"","before":"","after":"","decisions":[{"slot":"","trigger_signal":"","judgment":"","scope":""}]}`

// extractBackfill 调 LLM 提取补录结构；解析失败走 repairClosingJSON 容错，仍失败返回 error
func extractBackfill(transcript string) (*backfillReq, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	raw, err := callGuideDeepSeekOpts(ctx, []chatMsg{
		{Role: "system", Content: extractBackfillSystemPrompt},
		{Role: "user", Content: "用户的复盘对话：\n" + transcript},
	}, 3000)
	if err != nil {
		return nil, err
	}
	var out backfillReq
	jsonStr := extractJSON(raw)
	if e := json.Unmarshal([]byte(jsonStr), &out); e != nil {
		if repaired := repairClosingJSON(jsonStr); repaired != jsonStr {
			if e2 := json.Unmarshal([]byte(repaired), &out); e2 == nil && strings.TrimSpace(out.After) != "" {
				return &out, nil
			}
		}
		return nil, fmt.Errorf("抽取结果无法解析，请再聊一两句后重试")
	}
	if strings.TrimSpace(out.TaskIntent) == "" {
		out.TaskIntent = pickIntent(transcript)
	}
	return &out, nil
}

// pickIntent LLM 没给出合法 task_intent 时，用关键词重合在允许列表里猜一个最接近的
func pickIntent(transcript string) string {
	best, bestScore := "", 0.0
	tt := keyTerms(truncate(transcript, 600))
	for k, label := range AllowedIntents {
		s := overlapScore(tt, keyTerms(k+" "+label))
		if s > bestScore {
			best, bestScore = k, s
		}
	}
	if bestScore >= 0.25 {
		return best
	}
	return ""
}

// ---------- 小工具 ----------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
