// 收尾闭环：两问 + 一句 verdict + 改进建议
// 与硬编码问题不同：两问的措辞与选项按每个 Skill 的 .md 文档（真实 zip 包内容）由 LLM 生成，
// 因为每次使用的 Skill 不同，值得追问的时刻与场景就不同。verdict 问题与选项是产品固定语义。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type closingQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// 通用兜底问题：LLM 不可用 / 无 skill 包时使用，保证收尾流程永不卡死
func defaultClosingQuestions() (closingQuestion, closingQuestion, closingQuestion) {
	return closingQuestion{
			Question: "哪个时刻你忘了时间？",
			Options:  []string{"做这件事的某个瞬间", "和人交流的某个瞬间"},
		}, closingQuestion{
			Question: "哪个时刻你想逃？",
			Options:  []string{"开始前的犹豫", "没有想逃的时刻"},
		}, closingQuestion{
			Question: "这个 Skill 在你的情况下成立吗？",
			Options:  []string{"成立 ✓", "不成立（补一句反例）"},
		}
}

const closingLLMPrompt = `你是产品收尾环节的问题设计师。用户刚走完一次 Skill 陪跑，现在平台要问他「两个问题 + 一句 verdict」：前两个问题（q1/q2）由你设计措辞与选项，verdict（q3）的问题措辞也由你结合该 Skill 定制，但选项固定。

必须严格只输出 JSON，不要 markdown 代码块，不要任何解释文字。格式：
{"q1":{"question":"…","options":["…","…"]},"q2":{"question":"…","options":["…","…"]},"q3":{"question":"…","options":["成立 ✓","不成立（补一句反例）"]}}

要求：
1. q1 追问「哪个时刻你忘了时间（进入心流、忘了在赶任务）」；q2 追问「哪个时刻你想逃（想放弃、抵触、拖延）」。
2. 两个问题的措辞必须结合当前 Skill 的工作流程 / 判断点 / 边界 / 坑 / 步骤名称，从【Skill 包内容】里真实取材，落到这个 Skill 具体会发生的时刻，不要写「做这件事的某个瞬间」这类空泛选项。
3. 每个问题给 2-4 个选项，选项必须是该 Skill 真实场景下会发生的情形，选项文案要具体（如「写引言卡住的时候」「第一次让它生成代码结构时」）。
4. q2 的选项里必须包含「没有想逃的时刻」这一项。
5. q3 是 verdict 判断：核心语义是「这个 Skill 在你的情况下成立吗？」，但问题措辞必须结合该 Skill 的判断点 / 边界 / 适用人群，落到它具体的成立条件上（例如「按这个剧本走完两周，你觉得它在你的处境里成立吗？」），不要用空泛问法。
6. q3 的 options 必须严格只有这两项：["成立 ✓", "不成立（补一句反例）"]。
7. 只输出 JSON。`

// genClosingQuestions POST /api/growth/executions/:id/closing/questions
// 按该 Skill 的 .md 文档生成两问的措辞与选项；失败/无内容时回退通用问题。
// 内容来源优先级：① 前端透传的 skill_content（mock skill 无 zip 包时用前端文档摘要）
//               ② execution 关联的真实 skill 包（exploreSkillPackage 读 zip 里的 .md）
func genClosingQuestions(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	exec, err := loadExecution(id)
	if err != nil || exec.UserID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}

	// 可选入参：前端把该 skill 的文档内容（mock skill 无 zip 包）透传上来
	var body struct {
		SkillContent string `json:"skill_content"`
	}
	_ = c.ShouldBindJSON(&body)

	q1, q2, q3 := defaultClosingQuestions()
	content := ""
	if exec.SkillVersionID != nil {
		content = exploreSkillPackage(*exec.SkillVersionID)
	}
	if content == "" {
		content = strings.TrimSpace(body.SkillContent)
	}
	if content != "" {
		if g1, g2, g3, ok := askClosingQuestions(c, exec, content); ok {
			q1, q2, q3 = g1, g2, g3
		}
	}
	c.JSON(http.StatusOK, gin.H{"q1": q1, "q2": q2, "q3": q3})
}

// askClosingQuestions 调 LLM 生成两问 + verdict 问题；返回 (q1, q2, q3, 是否成功)。
// 两轮重试：第一轮解析失败或选项不合格则重试一次，仍失败回退通用问题。
func askClosingQuestions(c *gin.Context, exec *Execution, pkg string) (closingQuestion, closingQuestion, closingQuestion, bool) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	// 包内容较长时截断，控制 token 成本；只保留与「真实时刻」相关的内容
	content := pkg
	if runes := []rune(pkg); len(runes) > 12000 {
		content = string(runes[:12000])
	}

	system := closingLLMPrompt
	user := fmt.Sprintf("Skill 名称：%s\n任务类型：%s\n\n【Skill 包内容】\n%s",
		exec.TaskTitle, exec.TaskIntent, content)

	raw, err := callGuideDeepSeekOpts(ctx, []chatMsg{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, 2048)
	if g1, g2, g3, ok := parseClosingQuestions(raw, err); ok {
		return g1, g2, g3, true
	}

	// 重试一次
	raw, err = callGuideDeepSeekOpts(ctx, []chatMsg{
		{Role: "system", Content: system + "\n上一次输出不合格，请严格按格式只输出 JSON。"},
		{Role: "user", Content: user},
	}, 2048)
	if g1, g2, g3, ok := parseClosingQuestions(raw, err); ok {
		return g1, g2, g3, true
	}
	return closingQuestion{}, closingQuestion{}, closingQuestion{}, false
}

func parseClosingQuestions(raw string, err error) (closingQuestion, closingQuestion, closingQuestion, bool) {
	if err != nil {
		log.Printf("[closing] llm err: %v", err)
		return closingQuestion{}, closingQuestion{}, closingQuestion{}, false
	}
	var out struct {
		Q1 closingQuestion `json:"q1"`
		Q2 closingQuestion `json:"q2"`
		Q3 closingQuestion `json:"q3"`
	}
	jsonStr := extractJSON(raw)
	unmarshal := func(s string, o *struct {
		Q1 closingQuestion `json:"q1"`
		Q2 closingQuestion `json:"q2"`
		Q3 closingQuestion `json:"q3"`
	}) error {
		return json.Unmarshal([]byte(s), o)
	}
	if e := unmarshal(jsonStr, &out); e != nil {
		// 输出被截断时的兜底：按引号/括号自动闭合不完整 JSON
		if repaired := repairClosingJSON(jsonStr); repaired != jsonStr {
			if e2 := unmarshal(repaired, &out); e2 == nil && validClosingQuestion(out.Q1) && validClosingQuestion(out.Q2) {
				return out.Q1, out.Q2, verdictFallback(out.Q3), true
			}
		}
		log.Printf("[closing] unmarshal fail: %v raw=%q", e, truncate(raw, 500))
		return closingQuestion{}, closingQuestion{}, closingQuestion{}, false
	}
	if !validClosingQuestion(out.Q1) || !validClosingQuestion(out.Q2) {
		log.Printf("[closing] validation fail q1=%+v q2=%+v", out.Q1, out.Q2)
		return closingQuestion{}, closingQuestion{}, closingQuestion{}, false
	}
	// q3 单独兜底：LLM 没按固定选项输出时，退回默认 verdict 问题（选项固定不漂移）
	return out.Q1, out.Q2, verdictFallback(out.Q3), true
}

// verdictFallback 保证 verdict 问题的选项语义固定（成立/不成立），只保留 LLM 定制的问题措辞
func verdictFallback(q3 closingQuestion) closingQuestion {
	def := closingQuestion{
		Question: "这个 Skill 在你的情况下成立吗？",
		Options:  []string{"成立 ✓", "不成立（补一句反例）"},
	}
	if len([]rune(strings.TrimSpace(q3.Question))) < 4 {
		return def
	}
	q3.Question = strings.TrimSpace(q3.Question)
	// 选项固定，不采纳 LLM 自行发挥的选项
	q3.Options = def.Options
	return q3
}

func validClosingQuestion(q closingQuestion) bool {
	q.Question = strings.TrimSpace(q.Question)
	return len([]rune(q.Question)) >= 4 && len(q.Options) >= 2 && len(q.Options) <= 4
}

// submitClosing POST /api/growth/executions/:id/closing/submit
// 持久化两问的答案 + verdict + 用户手动输入的改进建议；并把执行置为 completed。
func submitClosing(c *gin.Context) {
	uid := c.GetInt64("userID")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Q1            string `json:"q1"`
		Q2            string `json:"q2"`
		Verdict       string `json:"verdict"`
		VerdictReason string `json:"verdict_reason"`
		Improvement   string `json:"improvement"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	exec, err := loadExecution(id)
	if err != nil || exec.UserID != uid {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}

	var skillID int64
	if exec.SkillVersionID != nil {
		skillID = *exec.SkillVersionID
	}
	verdict := strings.TrimSpace(body.Verdict)
	if verdict == "" {
		verdict = "成立"
	}
	// 归一化：选项文案可能带 ✓ / （补一句反例） 等装饰，只保留成立与否
	if strings.Contains(verdict, "成立") {
		verdict = "成立"
	} else if strings.Contains(verdict, "不成立") {
		verdict = "不成立"
	}

	db.Exec(`INSERT INTO exec_closings (execution_id, user_id, skill_id, q1, q2, verdict, verdict_reason, improvement)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, uid, skillID, strings.TrimSpace(body.Q1), strings.TrimSpace(body.Q2),
		verdict, strings.TrimSpace(body.VerdictReason), strings.TrimSpace(body.Improvement))

	// 收尾即完成：仍在 running 的执行在这里收口
	db.Exec(`UPDATE executions SET status = ?, ended_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`,
		ExecCompleted, id, ExecRunning)

	c.JSON(http.StatusOK, gin.H{"message": "收尾已记录", "verdict": verdict})
}

// repairClosingJSON 修复被截断的 JSON：从第一个 { 开始逐字符扫描，
// 自动闭合未闭合的字符串引号与括号栈，尽量还原 LLM 输出的完整 JSON。
func repairClosingJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	var stack []byte
	inStr, esc := false, false
	for _, r := range s[start:] {
		if esc {
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		if r == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch r {
		case '{', '[':
			stack = append(stack, byte(r))
		case '}':
			if n := len(stack); n > 0 && stack[n-1] == '{' {
				stack = stack[:n-1]
			}
		case ']':
			if n := len(stack); n > 0 && stack[n-1] == '[' {
				stack = stack[:n-1]
			}
		}
	}
	var sb strings.Builder
	sb.WriteString(s[start:])
	if inStr {
		sb.WriteByte('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			sb.WriteByte('}')
		} else {
			sb.WriteByte(']')
		}
	}
	return sb.String()
}
