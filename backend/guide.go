// AI 引导创建 Skill：多模态对话引导（打字/语音转文字/文件/图片）+ 生成符合 Claude 规范的 skill 包 zip
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---------- 环境变量 ----------
const (
	// DEEPSEEK_GUIDE_API_KEY 引导对话专用 key（与解读功能解耦）
	envGuideKey  = "DEEPSEEK_GUIDE_API_KEY"
	envVisionKey = "VISION_API_KEY" // 硅基流动等视觉模型 key，未配置时图片降级为附件
	envVisionURL = "VISION_BASE_URL"
	envVisionMod = "VISION_MODEL"
)

const (
	defaultVisionURL   = "https://api.siliconflow.cn/v1"
	defaultVisionModel = "Qwen/Qwen2.5-VL-72B-Instruct"
)

// ---------- 引导对话 ----------

// guideAttachment 一条消息中携带的附件
type guideAttachment struct {
	Type string `json:"type"` // "image" | "file"
	Name string `json:"name"`
	Mime string `json:"mime,omitempty"`
	Data string `json:"data,omitempty"` // base64 编码（图片字节 / 文本文件字节）
}

type guideChatRequest struct {
	Messages       []chatMsg        `json:"messages"`
	Attachment     *guideAttachment `json:"attachment,omitempty"`
	ConversationID int64            `json:"conversation_id"` // 引导对话保留（虚拟自己蒸馏素材），首次可为 0
}

// 引导教练 system prompt：开放式引导，skill 不限于固定 SOP（可以是经验知识型）
const guideSystemPrompt = `你是 SkillHub 平台的「Skill 复盘教练」。你的任务是像朋友一样，用通俗的中文对话，一步步引导用户把他自己亲身做成过的事、沉淀出的经验复盘清楚，最终产出一个可以发布到平台上的完整 Skill 包。

【用户是谁（最重要，务必记牢）】
用户是「过来人」：TA 已经亲手做成过某件事（比如考研上岸、保研成功、拿过竞赛奖、写过爆款论文、带过项目），现在来平台是复盘自己的成功之路，把它整理成可复用的 Skill 来帮助别人。
- 你要肯定用户的成就，态度是"你做到了，很了不起，把你的做法分享出来"。
- 你要引导的是"TA 当时是怎么做的、踩过什么坑、沉淀了什么方法论"，而不是教 TA 怎么做这件事。
- 严禁把用户当成正在做这件事的求助者：不要说"祝你考研顺利""建议你好好复习""你接下来要努力"这类话；用户不是来求建议的，用户是来复盘经验的。
- 引导方向是让用户把经验讲给"后来人"听：这个 Skill 能帮什么样的人、帮他们解决什么。

【Skill 是什么】
Skill 不只是「写论文、画图」这类有固定 SOP 的自动化流程，也可以是知识/经验型的，比如「保研经验复盘」「考研上岸方法论」「面试经验」。只要用户觉得「这套东西值得整理成可复用的指南」，就可以做成 Skill。

【引导目标：帮用户说清以下信息】
1. 用途：这个 Skill 帮用户解决什么问题、适合谁用（即"后来人"是谁）。
2. 输入：别人使用时会提供什么（材料、资料、描述、模板……）。
3. 输出：别人最终希望得到什么（文档、代码、分析结果、清单……）。
4. 核心内容：具体怎么做——分步骤的流程，或分主题的经验/知识框架，或必须遵守的规则与注意事项。
5. 细节与坑：关键细节、常见错误、失败案例——最容易踩的坑、出现什么信号就知道这条路走不通。
6. 关键判断：做这件事的过程里，在岔路口是怎么决策的——「出现什么信号 → 你要做什么判断 → 在什么场景成立」。这是方法论里最值钱的部分，要帮用户挖出来。
7. 适用边界：什么情况下这个 Skill 不适合用（不适用条件）；出现什么信号必须交回给人处理（人工接管触发点）。
8. 完成标准：怎么判断结果做完了、做对了——可验证的验收点。

【发布门禁（务必引导用户补全，否则 Skill 无法发布）】
平台发布前有四问测试和蒸馏度检查：
- 四问：可发现性（能不能被找到）、完成度（能不能做完）、稳定性（换输入还灵不灵）、边界停机（越界会不会停）。
- 蒸馏度六维：真实任务、明确结果、核心流程、关键判断、失败案例、适用边界；其中关键判断至少要填满两个槽位，适用边界是硬性要求，不接受折中。
引导时把上面 1-8 问全（尤其是 5/6/7/8，经验型用户最容易漏），生成的 Skill 就能直接通过门禁，用户不用再返工。

【引导策略】
- 每次只问 1-2 个问题，不要一次性抛出所有问题让用户回答。
- 用户说的内容模糊时，追问具体细节；用户不知道从何说起时，给出一个贴近他描述的示例帮他展开。
- 如果用户上传了图片或文件，先询问/确认这些材料的作用，并引导用户用文字补充关键信息。
- 用户提到做法的岔路口时（"当时我纠结过要不要X""差点踩坑"），主动追问当时的判断依据和触发信号，帮他把隐性经验显性化。
- 当用户已经提供了足够的信息（用途、输入、输出、核心步骤/框架、细节与坑、关键判断、适用边界、完成标准基本齐全）时，主动告诉他：可以点击「生成 Skill 包」。
- 全程使用简体中文，语气亲切、耐心、鼓励；回复保持简洁。
- 重要：每轮回复的末尾，用单独一行输出信息完整度标签，格式为【进度】N%（N 为 0-100 的整数）：信息很少时给 10-30，逐步补充后 40-80，信息基本齐全时 90，完全齐全时 100。该行之后不要再输出任何内容。`

// guideChat POST /api/skills/guide/chat（需登录）
// 前端每次携带完整对话历史 + 可选附件；后端处理附件（图片→视觉模型描述或降级提示；文本文件→嵌入内容）后调用引导 LLM
func guideChat(c *gin.Context) {
	var req guideChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages is required"})
		return
	}

	// 引导对话保留：每轮自动保存到 persona_conversations，作为"虚拟自己"蒸馏素材
	uid := c.GetInt64("userID")
	convID := saveGuideConversation(uid, req.ConversationID, req.Messages)

	messages := make([]chatMsg, 0, len(req.Messages)+2)
	messages = append(messages, chatMsg{Role: "system", Content: guideSystemPrompt})
	messages = append(messages, req.Messages...)

	// 附件处理：注入一条"系统观察"说明
	if req.Attachment != nil && req.Attachment.Name != "" {
		note := processGuideAttachment(req.Attachment)
		if note != "" {
			messages = append(messages, chatMsg{Role: "system", Content: note})
		}
	}

	content, err := callGuideDeepSeek(context.Background(), messages)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI 生成失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": content, "conversation_id": convID})
}

// processGuideAttachment 处理附件，返回注入上下文的说明文本
func processGuideAttachment(att *guideAttachment) string {
	if att.Type == "image" {
		return processImageAttachment(att)
	}
	return processFileAttachment(att)
}

// processImageAttachment 图片：配置了视觉模型则调用理解；否则降级为参考附件
func processImageAttachment(att *guideAttachment) string {
	key := os.Getenv(envVisionKey)
	if key == "" || att.Data == "" {
		return fmt.Sprintf("用户上传了一张图片【%s】作为参考。当前未接入视觉模型，无法直接识别图片内容。请引导用户用文字描述这张图片里重要的信息（如果图片只是装饰/示意，可忽略）。", att.Name)
	}
	mime := att.Mime
	if mime == "" {
		mime = "image/png"
	}
	desc, err := callVisionLLM(context.Background(), mime, att.Data, "请用中文详细描述这张图片的内容，包括其中所有文字、结构、步骤、关键信息。如果图片模糊或内容不明，请如实说明。")
	if err != nil {
		return fmt.Sprintf("用户上传了一张图片【%s】作为参考，但图片理解失败（%v）。请引导用户用文字描述图片内容。", att.Name, err)
	}
	return fmt.Sprintf("用户上传了一张图片【%s】作为参考，以下是 AI 对图片内容的识别结果：\n%s\n（如果识别有误，请结合用户后续的文字说明修正）", att.Name, desc)
}

// processFileAttachment 文本文件直接嵌入内容；二进制仅附文件名
func processFileAttachment(att *guideAttachment) string {
	name := att.Name
	dot := strings.LastIndex(name, ".")
	ext := ""
	if dot >= 0 {
		ext = strings.ToLower(name[dot+1:])
	}
	if isTextExt(ext) && att.Data != "" {
		raw, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			return fmt.Sprintf("用户上传了文件【%s】，但文件内容解析失败。请引导用户用文字说明该文件的作用和关键内容。", name)
		}
		if len(raw) > 200*1024 {
			return fmt.Sprintf("用户上传了文件【%s】，内容较大（%.0f KB），仅截取前 200KB 供参考：\n%s", name, float64(len(raw))/1024, string(raw[:200*1024]))
		}
		return fmt.Sprintf("用户上传了文本文件【%s】，内容如下：\n---文件内容开始---\n%s\n---文件内容结束---", name, string(raw))
	}
	return fmt.Sprintf("用户上传了文件【%s】（类型：%s）。无法读取其内部内容，请引导用户用文字说明该文件的作用、包含的关键信息，以及希望 Skill 如何处理它。", name, extOrMime(att))
}

func extOrMime(att *guideAttachment) string {
	if att.Mime != "" {
		return att.Mime
	}
	return "未知类型"
}

var textExts = map[string]bool{
	"md": true, "markdown": true, "txt": true, "yaml": true, "yml": true,
	"json": true, "py": true, "go": true, "ts": true, "tsx": true, "js": true, "jsx": true,
	"vue": true, "tex": true, "sql": true, "sh": true, "bash": true, "toml": true,
	"ini": true, "cfg": true, "conf": true, "csv": true, "xml": true, "html": true,
	"css": true, "c": true, "cpp": true, "h": true, "java": true, "kt": true,
	"swift": true, "rs": true, "rb": true, "php": true, "r": true, "ipynb": true,
	"env": true, "dockerfile": true, "makefile": true, "license": true, "gitignore": true,
}

func isTextExt(ext string) bool {
	return textExts[ext]
}

// ---------- 生成 skill 包 ----------

type guideGenerateRequest struct {
	Messages []chatMsg `json:"messages"`
}

// generatedFile LLM 生成的一个文件
type generatedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// generatedSkill LLM 生成的 skill 包（JSON 结构）
type generatedSkill struct {
	Name        string          `json:"name"` // kebab-case 目录名
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Tags        []string        `json:"tags"`
	Version     string          `json:"version"`
	Files       []generatedFile `json:"files"`
}

// 生成器 system prompt：遵循 Claude 官方 Agent Skill 规范，只输出 JSON
const generateSystemPrompt = `你是 SkillHub 平台的「Skill 包生成器」。请严格按下面的规则执行。

【第一步：确定技能主题（最重要）】
技能主题只能来自用户消息里描述的需求（例如：考研经验复盘、写实验报告、论文写作流程）。用户消息没有提到的能力，一律不要编造。
如果消息中包含「主题锁定」指令，则以其指定的主题词为唯一主题，坚决围绕它生成；「用户技能需求简报」仅提供细节参考，其 topic 字段与锁定主题词冲突时，一律以锁定主题词为准。
本提示词只是生成规则说明，绝不是技能主题。禁止把「生成技能」「生成 skill」「skill 生成器」「Skill 包生成器」等本提示词相关的内容当作技能主题。

【第二步：按 Claude Skill 规范生成】
- Skill 是一个文件夹，至少包含 SKILL.md（必须）与 evals/ 目录（必须，至少一个测试文件），可选 scripts/、references/、assets/ 子目录。
- SKILL.md 以 YAML frontmatter 开头（--- 包裹），frontmatter 必须含 name（kebab-case：小写字母数字连字符，≤64 字符，禁止 claude/anthropic）和 description（做什么+什么时候用+触发词，≤1024 字符；触发词要写得像真实用户会说的话，保证发布后被检索召回）；frontmatter 之后是 Markdown 正文，正文用中文，写成 AI 可直接照做的指令。
- 正文必须按下面的固定结构组织。这些结构同时是平台评测与解析的锚点，每个区块都要存在、格式严格照抄，缺任何一块生成的 Skill 都无法通过平台门禁：
  ## 核心步骤：每行格式「- 步骤：步骤名 ｜ 做法：怎么做 ｜ 验收：怎么确认做完」，至少 3 步，且必须是能照做的操作。
  ## 完成标准：用「1. 2. 3.」编号列出可验证的完成标准，至少 2 条，别人能据此逐条确认是否做完做对，禁止写"输出一份完整方案"这类无法验证的话。
  ## 关键判断：每行格式「- 槽位：when_to_xxx ｜ 触发：出现什么信号 ｜ 判断：怎么做判断 ｜ 场景：什么场景下成立」，至少 2 条；槽位只能取 when_to_check / when_to_probe / when_to_use_tool / when_to_switch 之一（分别对应：停下来回头验证、要求补充信息再动手、必须查/必须跑不能靠判断、发现这条路走不通）。
  ## 失败案例：每行格式「- 触发：什么情况 ｜ 表现：会怎样 ｜ 后果：导致什么结果」，至少 1 条，写真实容易犯的错。
  ## 适用边界：必须同时包含「- 不适用：什么情况下这个 Skill 不该用」（至少 1 条）和「- 交回给人：出现什么信号必须交给用户或人工处理」（至少 1 条）。
  ## 注意事项：其他补充规则（可省略）。
- 在 evals/ 下生成至少一个测试文件（如 evals/test.yaml）：YAML frontmatter 含 name、description，然后是 examples 列表，每条 example 含 input（用户说话的原话）与 expected（期望的完成标准/行为），至少 3 条覆盖正常、边界、越界三类。
- 不要放 README.md。
- 依据用户提供的全部信息生成；信息不足处用合理内容补全并注明。

【输出要求】
只输出一个 JSON 对象，不要输出任何其他文字、不要用 markdown 代码块包裹。JSON 结构：
{"name":"kebab-case 目录名","title":"中文展示标题","description":"一句话介绍(10-60字)","category":"论文写作或编程开发或数据科学或设计创作或效率工具或语言学习或其他","tags":["标签1"],"version":"1.0.0","files":[{"path":"SKILL.md","content":"完整内容"},{"path":"references/xx.md","content":"参考内容"}]}`

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// extractJSONObject 从任意文本中提取第一个 `{` 到最后一个 `}` 之间的内容（容忍 LLM 开场白/结尾语）
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// 阶段一 prompt：从对话中提炼技能需求简报（简单任务，弱模型也能保持主题）
const briefSystemPrompt = `你是 SkillHub 的需求分析师。请仔细阅读用户与 AI 的对话内容，提炼出用户想创建的 Skill 的完整需求，只输出一个 JSON 对象（不要输出任何其他文字、不要用 markdown 代码块包裹）：
{"topic":"技能主题，一句话说清（如：帮本科生整理保研经验与行动指南）","who":"适合谁用","input":"用户使用时会输入什么","output":"用户希望得到什么","core":"核心内容：分步骤流程或分主题经验框架（要点式）","details":"注意事项、常见错误、关键细节"}
【重要】topic 中的主题词必须逐字引用用户原话里的词（如「考研」「保研」「实习」「竞赛」「论文」），禁止替换成近义词（例如用户说「考研」，不得写成「保研」）。`

// extractSkillBrief 阶段一：调用 LLM 提炼需求简报
func extractSkillBrief(ctx context.Context, msgs []chatMsg) (string, error) {
	messages := make([]chatMsg, 0, len(msgs)+2)
	messages = append(messages, chatMsg{Role: "system", Content: briefSystemPrompt})
	messages = append(messages, msgs...)
	return callGuideDeepSeek(ctx, messages)
}

// 常见主题词表：优先匹配用户在对话中明确提到的技能主题（防止 LLM 把主题替换成近义词）
var topicKeywords = []string{"考研", "保研", "考公", "留学", "实习", "竞赛", "论文", "面试", "健身", "写作", "绘画", "编程", "数据分析", "项目管理"}

// extractTopicKeyword 从用户消息中精确抽取主题词（返回第一个命中词，未命中返回空串）
func extractTopicKeyword(msgs []chatMsg) string {
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, kw := range topicKeywords {
			if strings.Contains(m.Content, kw) {
				return kw
			}
		}
	}
	return ""
}

// guideGenerate POST /api/skills/guide/generate（需登录）
// 两阶段生成：先提炼需求简报（锚定主题），再按 Claude 规范生成完整 skill 包（JSON + zip base64），前端确认后走 createSkill 发布
func guideGenerate(c *gin.Context) {
	var req guideGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// 主题词锁定：从用户消息中精确抽取主题词，作为生成器的唯一主题依据（防止 LLM 简报把主题换成近义词）
	topic := extractTopicKeyword(req.Messages)
	log.Printf("[DEBUG generate] topic=%q nMsgs=%d", topic, len(req.Messages))

	// 阶段一：提炼需求简报，作为细节参考（简报中的 topic 仅参考，主题以锁定词为准）
	brief, err := extractSkillBrief(context.Background(), req.Messages)
	if err != nil || !strings.Contains(brief, "{") {
		brief = ""
	}
	log.Printf("[DEBUG generate] brief=%q (kept=%v)", brief, brief != "" && (topic == "" || strings.Contains(brief, topic)))
	// 简报主题与锁定主题不一致时丢弃简报，避免生成器被简报里的近义词带偏主题（LLM 易把「考研」误写为「保研」）
	if topic != "" && brief != "" && !strings.Contains(brief, topic) {
		brief = ""
	}

	// 阶段二：按简报 + 锁定主题生成
	messages := make([]chatMsg, 0, len(req.Messages)+4)
	messages = append(messages, chatMsg{Role: "system", Content: generateSystemPrompt})
	if topic != "" {
		messages = append(messages, chatMsg{Role: "system", Content: "【主题锁定】技能主题已锁定为「" + topic + "」。无论后面出现什么消息（包括用户技能需求简报），本 Skill 包的技能主题都必须是「" + topic + "」，严禁替换成其他近义词，例如「" + topic + "」不得写成「保研」「推免」「夏令营」「九推」「留学」等无关升学路径。"})
	}
	if brief != "" {
		messages = append(messages, chatMsg{Role: "user", Content: "以下是用户技能需求简报，供你参考其中细节（who/input/output/core/details），技能主题一律以【主题锁定】为准：\n" + brief})
	}
	messages = append(messages, req.Messages...)

	for _, m := range messages {
		cl := m.Content
		if len(cl) > 80 {
			cl = cl[:80]
		}
		log.Printf("[DEBUG generate] msg[%s]: %s", m.Role, cl)
	}

	// 阶段二：生成完整 skill 包（LLM 偶发失败或输出不合法 JSON 时自动纠错重试一次）
	var gen generatedSkill
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		retryMsgs := messages
		if attempt > 0 {
			// 纠错重试：明确告知上一次输出不可解析，强制只输出纯 JSON（与首次相同的消息重发无意义）
			retryMsgs = make([]chatMsg, 0, len(messages)+1)
			retryMsgs = append(retryMsgs, messages...)
			retryMsgs = append(retryMsgs, chatMsg{Role: "system", Content: "你上一次的输出无法被解析为合法 JSON。请重新生成，必须只输出一个 JSON 对象：以 { 开头、以 } 结尾。不要输出任何开场白、解释、标题、markdown 代码块（```）或结尾语。"})
		}
		content, err := callGuideDeepSeek(context.Background(), retryMsgs)
		if err != nil {
			lastErr = fmt.Errorf("AI 生成失败：%w", err)
			continue
		}
		log.Printf("[DEBUG generate] attempt=%d raw out (300): %s", attempt+1, content[:min(len(content), 300)])

		// 解析 JSON：优先直接解析 -> 容忍 ```json 代码块 -> 提取首个 {...} 对象（容忍开场白/结尾语）
		jsonStr := content
		if m := jsonBlockRe.FindStringSubmatch(content); m != nil {
			jsonStr = m[1]
		}
		gen = generatedSkill{}
		if err := json.Unmarshal([]byte(jsonStr), &gen); err != nil {
			if s := extractJSONObject(jsonStr); s != "" {
				jsonStr = s
			}
			if err := json.Unmarshal([]byte(jsonStr), &gen); err != nil {
				log.Printf("[DEBUG generate] attempt=%d parse fail: %v; raw(300)=%q", attempt+1, err, content[:min(len(content), 300)])
				lastErr = fmt.Errorf("AI 输出解析失败，请重试：%w", err)
				continue
			}
		}
		if gen.Name == "" || len(gen.Files) == 0 {
			lastErr = fmt.Errorf("AI 输出缺少必要字段（name / files），请重试")
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": lastErr.Error()})
		return
	}

	// 校验/修正 name 为 kebab-case
	gen.Name = sanitizeKebab(gen.Name)
	if gen.Version == "" {
		gen.Version = "1.0.0"
	}

	// 打包 zip（内存）
	zipBytes, err := buildSkillZip(gen.Files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打包 zip 失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"name":        gen.Name,
			"title":       gen.Title,
			"description": gen.Description,
			"category":    gen.Category,
			"tags":        gen.Tags,
			"version":     gen.Version,
			"files":       gen.Files,
			"zip_base64":  base64.StdEncoding.EncodeToString(zipBytes),
		},
	})
}

// buildSkillZip 将生成的文件列表打包为 zip（根目录为 skill 名称的 kebab-case 目录）
func buildSkillZip(files []generatedFile) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		p := strings.TrimPrefix(f.Path, "/")
		if p == "" {
			continue
		}
		w, err := zw.Create(p)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.Content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitizeKebab 修正 name 为 kebab-case（小写字母数字连字符）
func sanitizeKebab(name string) string {
	var sb strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			sb.WriteRune(r)
			lastDash = false
		} else if !lastDash && sb.Len() > 0 {
			sb.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		out = "my-skill"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
