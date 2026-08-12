// DeepSeek LLM 集成：按用户 AI 熟练度生成 skill 的个性化介绍
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	deepseekURL  = "https://api.deepseek.com/chat/completions"
	deepseekModel = "deepseek-chat"
)

// 解释缓存：按 (userID, skillID) 缓存，避免重复调用 LLM 产生费用
type explainEntry struct {
	Content   string
	CreatedAt time.Time
}

var (
	explainMu   sync.Mutex
	explainCache = map[string]explainEntry{}
)

// 对所有水平都生效的总要求：任何水平都必须先简要介绍 skill 是什么、能解决什么问题；
// 操作讲解的详细程度与用户 AI 水平成反比（水平越低讲得越细）。
const commonGuide = `【总要求】
- 所有用户（无论什么水平）都必须先用 1-3 句话简要介绍：这个 skill 是什么、能解决什么问题、适合谁用。
- 介绍内容必须严格基于【技能信息】与【包内文件真实内容】中的真实信息（技能名称、分类、版本、标签、作者描述、包内文件清单、SKILL.md 等文件原文）展开，不得编造该技能不具备的功能、不存在的文件、脚本名称、依赖项或网址。
- 用户水平越低，「怎么使用」的讲解越详细（分步骤说明每步在哪里操作、输入什么、会得到什么）；水平越高，使用引导越简练（点到为止即可）。`

// 零基础（never/beginner）专属：必须讲清「在哪用、装什么、用什么 prompt 驱动、怎么下载」
const runwayGuide = `【零基础用户专属：必须讲清「在哪用、装什么、用什么 prompt、怎么下载」】
请基于【技能信息】与【包内文件真实内容】中真实出现的信息，用 markdown 小标题分四小节完整输出（放在「怎么使用」之前）：
1. 在什么软件上运行：使用这个 skill 需要打开哪个软件 / 平台（如 Trae、Cursor、ChatGPT、本地命令行、浏览器），没装的话先装哪个；
2. 需要安装什么依赖：必须安装的软件、插件、库或注册的账号（严格引用包内真实出现的名称；包内没写的，明说「请以官方 README 为准」，禁止编造）；
3. 用什么 prompt 驱动：给出一段可直接复制粘贴的启动 prompt，用【占位符】标出他需要替换成自己情况的地方；
4. 从哪里下载 / 获取：引用包内出现的仓库地址、安装命令或官方渠道；包内没有就说明去哪查（如 GitHub 搜索仓库名），禁止编造网址。
四小节必须完整、可信手可查，不允许用「去官网看看」一句带过。`

// 熟练度 -> 面向该水平的介绍要求
func levelGuide(level string) string {
	switch level {
	case "never":
		return "用户从未用过任何 AI 工具（如 ChatGPT、Trae、Codex 等），甚至可能没有相关概念。请用最通俗易懂的日常语言，避免专业术语；把「怎么使用」讲到最细：分成 3-5 个基础步骤，每一步都说明在哪里操作、输入什么、会看到什么结果，让他照着做就能跑起来。"
	case "beginner":
		return "用户刚接触 AI 工具，用过简单的对话式 AI。先简要介绍这个 skill 的用途，再把「怎么使用」讲得细致一些：分步骤说明如何准备材料、如何向 AI 描述需求、如何检查生成结果，术语要有简短解释。"
	case "intermediate":
		return "用户熟悉常见 AI 工具，会用但想更深入。介绍这个 skill 的核心功能亮点、适用的典型场景、与普通方式的区别；「怎么使用」讲清关键流程即可，再补充上手时值得注意的要点。"
	case "advanced":
		return "用户是资深 AI 玩家，熟悉自动化、Agent、工作流等概念。先用一两句话介绍这个 skill 是什么，「怎么使用」只需一句简单引导（不展开步骤）；重点用专业精炼的语言讲：技术构成、目录结构与设计思路、可配置项与扩展点、最佳实践以及可能的改进方向。"
	default: // 未设置默认为 beginner 档
		return levelGuide("beginner")
	}
}

func levelLabel(level string) string {
	switch level {
	case "never":
		return "从未用过"
	case "beginner":
		return "初级"
	case "intermediate":
		return "中级"
	case "advanced":
		return "高级"
	default:
		return "初级"
	}
}

// officialAgentInfo 主流 AI 编码助手的官方渠道（注入 prompt，避免 LLM 编造下载链接）
const officialAgentInfo = `- Trae（字节跳动出品，中文友好、免费）：官网 https://www.trae.ai
- Cursor（AI 代码编辑器）：官网 https://www.cursor.com
- Codex（OpenAI）：在 ChatGPT 应用内开启 Codex 功能`

// 根据用户是否已安装 Agent 追加介绍要求
// parsed=false 表示问卷未知（老用户），保持原逻辑不额外引导
func agentInstallGuide(parsed, hasAgent bool) string {
	if !parsed {
		return ""
	}
	if hasAgent {
		return "用户电脑上已安装 AI 编码助手，介绍中不需要再讲如何安装工具。"
	}
	return fmt.Sprintf(`用户电脑上尚未安装任何 AI 编码助手（如 Trae、Codex、Cursor），不装的话无法运行本技能。
请在介绍最前面额外生成一个小节「开始之前：安装 AI 助手」，用最简洁的步骤（3 步以内）引导用户下载安装，并说明安装完成后回到本页再生成一次即可得到完整上手步骤。
下载渠道只能引用以下官方信息，禁止编造其他网址：
%s`, officialAgentInfo)
}

// parseAgentState 解析问卷，返回 (是否解析成功, 是否已安装 Agent)
func parseAgentState(user *User) (bool, bool) {
	if user == nil || user.AIQuiz == "" {
		return false, false
	}
	var q aiQuizInput
	if err := json.Unmarshal([]byte(user.AIQuiz), &q); err != nil {
		return false, false
	}
	return true, q.HasAgentInstalled
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Text    string `json:"text"` // 兼容前端沉淀模块发送的 {role,text} 结构
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callDeepSeek 调用 DeepSeek 对话补全接口
func callDeepSeek(ctx context.Context, messages []chatMsg) (string, error) {
	return callDeepSeekWithKey(ctx, messages, "DEEPSEEK_API_KEY")
}

// callGuideDeepSeek 引导对话专用：优先使用引导 key，未配置则回退到解读 key
func callGuideDeepSeek(ctx context.Context, messages []chatMsg) (string, error) {
	return callGuideDeepSeekOpts(ctx, messages, 0)
}

// callGuideDeepSeekOpts 同上；maxTokens > 0 时显式带上 max_tokens
func callGuideDeepSeekOpts(ctx context.Context, messages []chatMsg, maxTokens int) (string, error) {
	env := "DEEPSEEK_GUIDE_API_KEY"
	if os.Getenv(env) == "" {
		env = "DEEPSEEK_API_KEY"
	}
	return callDeepSeekWithKeyOpts(ctx, messages, env, maxTokens)
}

// callDeepSeekWithKey 调用 DeepSeek 对话补全接口（key 来源由 env 指定）
func callDeepSeekWithKey(ctx context.Context, messages []chatMsg, env string) (string, error) {
	return callDeepSeekWithKeyOpts(ctx, messages, env, 0)
}

// callDeepSeekWithKeyOpts 同上；maxTokens > 0 时显式带上 max_tokens，防止默认输出上限截断长 JSON
func callDeepSeekWithKeyOpts(ctx context.Context, messages []chatMsg, env string, maxTokens int) (string, error) {
	apiKey := os.Getenv(env)
	if apiKey == "" {
		return "", fmt.Errorf("%s 未配置", env)
	}
	reqBody := chatReq{
		Model:       deepseekModel,
		Messages:    messages,
		Temperature: 0.7,
	}
	if maxTokens > 0 {
		reqBody.MaxTokens = maxTokens
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepseekURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out chatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode llm response: %v", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("deepseek error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("deepseek returned no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// ---------- 视觉模型（图片理解，OpenAI 兼容） ----------

// visionMsgContent OpenAI 兼容的多模态消息内容
type visionPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type visionMsg struct {
	Role    string       `json:"role"`
	Content []visionPart `json:"content"`
}

type visionReq struct {
	Model    string      `json:"model"`
	Messages []visionMsg `json:"messages"`
}

// callVisionLLM 调用视觉模型理解图片，返回文字描述
func callVisionLLM(ctx context.Context, mime, imageBase64, prompt string) (string, error) {
	apiKey := os.Getenv("VISION_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("VISION_API_KEY 未配置")
	}
	baseURL := os.Getenv("VISION_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.siliconflow.cn/v1"
	}
	model := os.Getenv("VISION_MODEL")
	if model == "" {
		model = "Qwen/Qwen2.5-VL-72B-Instruct"
	}

	var part visionPart
	part.Type = "image_url"
	part.ImageURL.URL = fmt.Sprintf("data:%s;base64,%s", mime, imageBase64)
	body, _ := json.Marshal(visionReq{
		Model: model,
		Messages: []visionMsg{{
			Role: "user",
			Content: []visionPart{
				part,
				{Type: "text", Text: prompt},
			},
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out chatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode vision response: %v", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("vision error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("vision returned no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// skillPackageTextExt 探索 skill 包时视为「可读源码/文档」的扩展名
var skillPackageTextExt = map[string]bool{
	".md": true, ".txt": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".yaml": true, ".yml": true, ".json": true, ".sh": true, ".toml": true,
	".html": true, ".css": true, ".env": true, ".cfg": true, ".ini": true,
	".conf": true, ".sql": true, ".go": true, ".rb": true, ".java": true, ".r": true,
}

// exploreSkillPackage 探索整个 skill zip 包：列出全部文件清单，并读取各组关键文本内容
// （SKILL.md / references / scripts / gotchas / evals），生成结构化的「包内容快照」。
// 与 readSkillPackageText 的区别：不限 md/txt，覆盖 scripts/references 等源码文件，
// 供陪跑 Agent 准确回答「这个 skill 怎么用」，而不是凭 material 断章取义。
func exploreSkillPackage(skillID int64) string {
	zipPath := filepath.Join(ArchiveDir, fmt.Sprintf("%d.zip", skillID))
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return ""
	}
	defer zr.Close()

	// 顶层目录分组（含根目录的 SKILL.md）
	groupOrder := []string{"SKILL.md", "references", "scripts", "gotchas", "evals"}
	isGroup := map[string]bool{}
	for _, g := range groupOrder {
		isGroup[g] = true
	}
	groupFiles := map[string][]*zip.File{}
	var otherFiles []*zip.File

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(f.Name)
		grp := ""
		switch {
		case lower == "skill.md" || strings.HasSuffix(lower, "/skill.md"):
			grp = "SKILL.md"
		default:
			top := strings.SplitN(lower, "/", 2)[0]
			if isGroup[top] {
				grp = top
			} else {
				grp = "其他"
			}
		}
		if grp == "其他" {
			otherFiles = append(otherFiles, f)
		} else {
			groupFiles[grp] = append(groupFiles[grp], f)
		}
	}

	var sb strings.Builder
	total := len(zr.File)
	sb.WriteString(fmt.Sprintf("【Skill 包文件清单】共 %d 个文件：\n", total))
	// 文件清单按 SKILL.md → 各组 → 其他 顺序列出
	for _, g := range groupOrder {
		for _, f := range groupFiles[g] {
			sb.WriteString(fmt.Sprintf("- %s (%d B)\n", f.Name, f.UncompressedSize64))
		}
	}
	for _, f := range otherFiles {
		sb.WriteString(fmt.Sprintf("- %s (%d B)\n", f.Name, f.UncompressedSize64))
	}

	// 内容读取预算：SKILL.md 全文优先；每组最多读 3 个文本文件，单文件限 5KB
	const groupReadCap = 3
	const fileReadCap = 5 * 1024
	const totalBudget = 30 * 1024
	readText := func(f *zip.File) string {
		if f.UncompressedSize64 > 200*1024 {
			return "" // 太大跳过（如打包的 node_modules 等）
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		defer rc.Close()
		buf, err := io.ReadAll(io.LimitReader(rc, fileReadCap))
		if err != nil {
			return ""
		}
		text := strings.TrimSpace(string(buf))
		if text == "" {
			return ""
		}
		// 单行过长（如压缩 JS）截断，避免输出无意义的长行
		lines := strings.Split(text, "\n")
		if len(lines) > 60 {
			lines = lines[:60]
		}
		for i, ln := range lines {
			if len([]rune(ln)) > 200 {
				lines[i] = string([]rune(ln)[:200]) + "…"
			}
		}
		return strings.Join(lines, "\n")
	}

	budget := totalBudget
	for _, g := range groupOrder {
		picked := 0
		for _, f := range groupFiles[g] {
			if picked >= groupReadCap || budget <= 0 {
				break
			}
			if g == "SKILL.md" {
				// SKILL.md 恒读（包的核心入口）
			} else {
				ext := strings.ToLower(filepath.Ext(f.Name))
				if !skillPackageTextExt[ext] {
					continue // 组内二进制/图片只进清单，不读内容
				}
			}
			body := readText(f)
			if body == "" {
				continue
			}
			if len([]rune(body)) > budget {
				body = string([]rune(body)[:budget]) + "…"
			}
			sb.WriteString(fmt.Sprintf("\n===== %s 内容 =====\n%s\n", f.Name, body))
			budget -= len([]rune(body))
			picked++
		}
	}
	return sb.String()
}

// explainSkill GET /api/skills/:id/explain（需登录）
// 根据当前用户的 AI 熟练度 + 用户背景 + skill 内容（含 zip 包内文件原文），用 LLM 生成个性化介绍
func explainSkill(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid skill id"})
		return
	}

	skill, err := getSkillByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	// 从 DB 读取文件清单，让 LLM 了解 skill 包构成
	fileNames := []string{}
	rows, err := db.Query(`SELECT file_path FROM skill_files WHERE skill_id = ? ORDER BY file_path`, id)
	if err == nil {
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				fileNames = append(fileNames, p)
			}
		}
		rows.Close()
	}

	user, err := getUserByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 是否已安装 Agent（影响介绍中是否包含安装引导）
	agentParsed, hasAgent := parseAgentState(user)
	agentFlag := "u" // u=未知 0=未装 1=已装
	if agentParsed {
		if hasAgent {
			agentFlag = "1"
		} else {
			agentFlag = "0"
		}
	}

	// 命中缓存直接返回（key 含 ai_level + agent 状态：同水平但环境不同视为不同内容）
	key := fmt.Sprintf("%d:%s:%s:%d", uid, user.AILevel, agentFlag, skill.ID)
	explainMu.Lock()
	if e, ok := explainCache[key]; ok {
		explainMu.Unlock()
		c.JSON(http.StatusOK, gin.H{
			"data":        e.Content,
			"ai_level":    user.AILevel,
			"level_label": levelLabel(user.AILevel),
			"cached":      true,
		})
		return
	}
	explainMu.Unlock()

	// 构建 LLM 输入
	userProfile := fmt.Sprintf("学校：%s；专业：%s；年级：%s", user.School, user.Major, user.Grade)
	if userProfile == "学校：；专业：；年级：" {
		userProfile = "未填写"
	}
	agentEnv := agentInstallGuide(agentParsed, hasAgent)
	// 探索 skill 包全部文件真实内容（SKILL.md / references / scripts / gotchas / evals 含源码），
	// 让 LLM 能讲清真实的环境、依赖、prompt、脚本与下载方式
	packageText := exploreSkillPackage(skill.ID)
	if packageText == "" {
		packageText = "（包内未读取到可读的文本文件，仅有文件清单，请勿编造具体依赖或网址）"
	}
	skillInfo := fmt.Sprintf(
		"技能名称：%s\n分类：%s\n版本：%s\n标签：%s\n作者描述：%s\n包内主要文件：%s\n\n【包内文件真实内容】\n%s",
		skill.Name, skill.Category, skill.Version, skill.Tags, skill.Description,
		strings.Join(fileNames, "、"),
		packageText,
	)
	if len(fileNames) == 0 {
		skillInfo += "（该 skill 暂无文件包）"
	}

	// 零基础用户额外要求「四要素」小节；长度约束随水平放宽
	runway := ""
	lengthRule := "介绍控制在 150-300 字"
	if user.AILevel == "never" || user.AILevel == "beginner" {
		runway = runwayGuide
		lengthRule = "介绍控制在 500-900 字（四要素小节必须讲全，但不要灌水）"
	}

	prompt := fmt.Sprintf(`你是 SkillHub 平台的技能导览专家。请根据用户的 AI 使用水平，为一个技能生成个性化介绍。

【用户的 AI 水平】%s
【用户背景】%s
【用户 Agent 环境】%s
【技能信息】
%s

【要求】
%s

%s

%s

%s
介绍使用 markdown 格式输出（可用小标题、列表）。直接输出介绍内容，不要任何开场白。`,
		levelLabel(user.AILevel), userProfile, agentEnv, skillInfo,
		levelGuide(user.AILevel), commonGuide, runway, lengthRule)

	content, err := callDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: "你是一个专业的技能介绍助手，擅长根据读者水平调整讲解深度。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		log.Printf("explain skill %d for user %d: %v", skill.ID, uid, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI 生成失败：" + err.Error()})
		return
	}

	explainMu.Lock()
	explainCache[key] = explainEntry{Content: content, CreatedAt: time.Now()}
	explainMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"data":        content,
		"ai_level":    user.AILevel,
		"level_label": levelLabel(user.AILevel),
		"cached":      false,
	})
}
