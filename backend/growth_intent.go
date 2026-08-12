// F1 目标识别与四筛判定（PRD 第 6 章 F1、第 1.5 节伪需求拒绝策略）
// 核心约束：五类伪需求一律不进任务流、不落 Experience。
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// sieveResult 四把筛子
type sieveResult struct {
	Amortizable   bool   `json:"amortizable"`    // 可摊销：是否在大量人身上重复出现
	Testable      bool   `json:"testable"`       // 可测试：是否有可判断的完成标准
	Transferable  bool   `json:"transferable"`   // 可转移：关键判断能否脱离具体某个人
	ShortLoop     bool   `json:"short_loop"`     // 短链路：执行到看见结果是否够快
	ReasonIfFalse string `json:"reason_if_false"`
}

func (s sieveResult) allPassed() bool {
	return s.Amortizable && s.Testable && s.Transferable && s.ShortLoop
}

// intentResult P1 Prompt 的输出结构
type intentResult struct {
	TaskIntent      string      `json:"task_intent"`
	Confidence      float64     `json:"confidence"`
	Sieve           sieveResult `json:"sieve"`
	CurrentPosition string      `json:"current_position"`
	Gap             []string    `json:"gap"`
	NextStep        string      `json:"next_step"`
	ClarifyQuestion string      `json:"clarify_question"`
}

const intentSystemPrompt = `你是大学生成长平台的任务识别器。你的职责不是回答用户问题，而是判断这句话属于哪类任务、是否适合用「Skill（可复用的行动方法）」来解决。

允许进入任务流的 task_intent（只能取以下值）：
thesis_topic（论文选题打磨与收敛）、resume_rewrite（把科研经历改成产研岗位简历）、resume_jd_align（简历与具体 JD 对齐）、report_structure（组会汇报与答辩陈述结构）、mock_interview（模拟面试）、interview_review（面试复盘）、project_convergence（项目与竞赛方案收敛）、literature_review（文献综述入门与检索策略）、content_script（内容脚本与选题结构）

必须拒绝的 task_intent（同样只能取以下值）：
emotional_support（情绪与心理表达，如焦虑、室友矛盾）
life_decision（人生抉择，如要不要考研/出国/换专业）
zero_sum_competition（名额竞争，如保研名额、评奖）
realtime_fact（实时信息查询，如今年招聘政策、某公司在招什么）
resource_dependent（资源依赖，如怎么拿到内推、怎么让大牛导师带我）

四把筛子（逐项判断 true/false）：
- amortizable：这个任务是否在大量学生身上重复出现
- testable：是否存在可判断的完成标准
- transferable：关键判断能否被说清并脱离具体某个人
- short_loop：从执行到看见结果是否在一次会话到一周之内

判断原则：
1. 需求真实不等于适合 Skill 化。情绪类是最真实的需求，但最不该做成 Skill。
2. 如果用户话里既有情绪又有具体任务（例如「烦死了，选题还没定」），优先按任务处理。
3. 只有识别为允许类时，才填写 current_position / gap / next_step；被拒绝类一律留空。
4. confidence 低于 0.6 时必须给出一个 clarify_question，且只问一个问题。
5. next_step 必须是今天就能动手的一个具体动作，不要写「制定计划」这类空话。

严格只输出 JSON，不要 markdown 代码块，不要任何解释文字。格式：
{"task_intent":"","confidence":0.0,"sieve":{"amortizable":true,"testable":true,"transferable":true,"short_loop":true,"reason_if_false":""},"current_position":"","gap":[],"next_step":"","clarify_question":""}`

var growthJSONBlockRe = regexp.MustCompile("(?s)\\{.*\\}")

// extractJSON 从模型输出里抠出第一个 JSON 对象（容忍模型偶尔套 markdown 代码块）
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if m := growthJSONBlockRe.FindString(s); m != "" {
		return m
	}
	return s
}

// interpretGoal POST /api/growth/goals/interpret
// 输入用户原话，返回任务卡；伪需求走拒绝策略且不创建任何记录。
func interpretGoal(c *gin.Context) {
	var body struct {
		Utterance string `json:"utterance"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	utterance := strings.TrimSpace(body.Utterance)
	if len([]rune(utterance)) < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "再多说两句，我需要知道你现在卡在哪"})
		return
	}
	if len([]rune(utterance)) > 500 {
		utterance = string([]rune(utterance)[:500])
	}

	// 原话一律入语料库：它既是召回索引来源，也是需求信号来源（PRD 8.4 / 13.7）
	db.Exec(`INSERT INTO description_corpus (utterance, source) VALUES (?, 'goal_input')`, utterance)

	res, err := classifyIntent(utterance)
	if err != nil {
		// 兜底：不报错页，降级为手选任务列表（PRD F1 验收第 4 条）
		log.Printf("intent classify failed: %v", err)
		options := []gin.H{}
		for k, v := range AllowedIntents {
			options = append(options, gin.H{"task_intent": k, "label": v})
		}
		c.JSON(http.StatusOK, gin.H{
			"mode":     "manual_fallback",
			"message":  "我暂时没法自动判断你的情况，直接选一个更接近的任务吧",
			"options":  options,
		})
		return
	}

	// ---------- 三态路由（PRD v1.1 §1.5，判定必须按此顺序）----------

	// 第一顺位：情绪类立即返回，不再做任何后续判断
	if res.TaskIntent == IntentEmotionalSupport {
		c.JSON(http.StatusOK, gin.H{
			"mode":        "rejected",
			"task_intent": res.TaskIntent,
			"reason":      RejectedIntents[IntentEmotionalSupport],
			"response":    rejectionResponse(IntentEmotionalSupport),
			"resources":   rejectionResources(IntentEmotionalSupport),
		})
		return
	}

	// 第二顺位：「该不该」型抉择——不给建议，只展示别人的分支与代价
	if res.TaskIntent == IntentLifeDecision && looksUndecided(utterance) {
		c.JSON(http.StatusOK, gin.H{
			"mode":        "rejected",
			"task_intent": res.TaskIntent,
			"reason":      RejectedIntents[IntentLifeDecision],
			"response":    rejectionResponse(IntentLifeDecision),
			"resources":   rejectionResources(IntentLifeDecision),
			"branches":    lifeDecisionBranches(),
		})
		return
	}

	// 第三顺位：编排态。已经决定了方向、或名额竞争、或资源依赖 → 给编排不给承诺。
	// 注意 life_decision 但不含「该不该」措辞的（例如「我决定考研了，接下来怎么排」）也走这里。
	if orchIntent, ok := OrchestrationRouteIntents[res.TaskIntent]; ok ||
		res.TaskIntent == IntentLifeDecision {
		if orchIntent == "" {
			orchIntent = guessOrchestrationIntent(utterance)
		}
		c.JSON(http.StatusOK, gin.H{
			"mode":                 "orchestration",
			"task_intent":          res.TaskIntent,
			"orchestration_intent": orchIntent,
			"label":                OrchestrationIntents[orchIntent],
			"message":              "这件事的结果我不敢承诺，但接下来几周该做什么可以排清楚——用别人真走过的路来排。",
			"next":                 "probe",
		})
		return
	}

	// 其余仍在拒绝集合里的（如 realtime_fact）
	if reason, rejected := RejectedIntents[res.TaskIntent]; rejected {
		c.JSON(http.StatusOK, gin.H{
			"mode":        "rejected",
			"task_intent": res.TaskIntent,
			"reason":      reason,
			"response":    rejectionResponse(res.TaskIntent),
			"resources":   rejectionResources(res.TaskIntent),
		})
		return
	}

	// 不在允许集合内（模型给了没见过的值）→ 当作识别失败走兜底
	if _, ok := AllowedIntents[res.TaskIntent]; !ok {
		options := []gin.H{}
		for k, v := range AllowedIntents {
			options = append(options, gin.H{"task_intent": k, "label": v})
		}
		c.JSON(http.StatusOK, gin.H{
			"mode":    "manual_fallback",
			"message": "这件事我还没有把握，选一个更接近的任务吧",
			"options": options,
		})
		return
	}

	// 置信度不足 → 只问一轮澄清
	if res.Confidence < 0.6 && strings.TrimSpace(res.ClarifyQuestion) != "" {
		c.JSON(http.StatusOK, gin.H{
			"mode":             "clarify",
			"task_intent":      res.TaskIntent,
			"clarify_question": res.ClarifyQuestion,
		})
		return
	}

	// 四筛未全过 → 说明为什么不用 Skill 解决
	if !res.Sieve.allPassed() {
		c.JSON(http.StatusOK, gin.H{
			"mode":        "not_skillable",
			"task_intent": res.TaskIntent,
			"sieve":       res.Sieve,
			"message":     "这类问题我们不做成 Skill：" + res.Sieve.ReasonIfFalse,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"mode": "task",
		"task_card": gin.H{
			"task_intent":      res.TaskIntent,
			"task_label":       AllowedIntents[res.TaskIntent],
			"current_position": res.CurrentPosition,
			"gap":              res.Gap,
			"next_step":        res.NextStep,
			"sieve":            res.Sieve,
		},
	})
}

// classifyIntent 调用 LLM 做识别，并对输出做 schema 校验（失败重试一次）
func classifyIntent(utterance string) (*intentResult, error) {
	msgs := []chatMsg{
		{Role: "system", Content: intentSystemPrompt},
		{Role: "user", Content: utterance},
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := callDeepSeek(context.Background(), msgs)
		if err != nil {
			lastErr = err
			continue
		}
		var res intentResult
		if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(res.TaskIntent) == "" {
			lastErr = errEmptyIntent
			continue
		}
		return &res, nil
	}
	return nil, lastErr
}

// errEmptyIntent 模型返回了空 intent
var errEmptyIntent = &intentError{"模型未给出 task_intent"}

type intentError struct{ msg string }

func (e *intentError) Error() string { return e.msg }

// rejectionResponse 五类伪需求各自的回应口径。
// 注意：情绪类不做评判、不展开、不追问，只承接并给资源。
func rejectionResponse(intent string) string {
	switch intent {
	case IntentEmotionalSupport:
		return "听起来这段时间挺不好过的。这件事我帮不上——它需要的是能坐下来听你说话的人，不是一套流程。如果你愿意，学校的心理支持是免费的，也不会留在任何记录里。"
	case IntentLifeDecision:
		return "这个我不给建议。不是回避，是这类选择没有「做对了」的标准，别人的答案套到你身上大概率是错的。我可以给你看几个真实走过不同路的人，以及他们各自付出的代价，你自己判断。"
	case IntentZeroSum:
		return "这件事的结果由分配规则和其他人的表现决定，任何声称能提高成功率的方法都在骗你。我能做的是把规则讲清楚，以及帮你做好其中你能控制的那部分。"
	case IntentRealtimeFact:
		return "这是一条需要查最新信息的事实，不是一套可复用的方法。我给你查，但不会把它做成 Skill——那样明年就是错的。"
	case IntentResourceDep:
		return "这件事的关键变量不在方法上，而在你拿不到的资源上。我可以帮你做其中能转移的那部分，比如怎么写第一封联系邮件；剩下的部分我不会假装能解决。"
	}
	return ""
}

// looksUndecided 判断是不是「该不该」型措辞。
// 这是三态里最细的一条线：还在犹豫 → 不给建议；已经决定 → 给编排。
func looksUndecided(s string) bool {
	markers := []string{"该不该", "要不要", "值不值", "值得吗", "好还是", "还是考", "选哪个", "怎么选", "纠结"}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	// 「是否」单独出现也算，但「是否已经」这类陈述不算
	if strings.Contains(s, "是否") && !strings.Contains(s, "是否已") {
		return true
	}
	return false
}

// guessOrchestrationIntent 从原话粗判编排方向。判不出来时给保研（当前唯一有 Path 的方向）。
func guessOrchestrationIntent(s string) string {
	switch {
	case strings.Contains(s, "保研") || strings.Contains(s, "推免") || strings.Contains(s, "夏令营"):
		return "postgrad_recommend"
	case strings.Contains(s, "考研"):
		return "postgrad_exam"
	case strings.Contains(s, "出国") || strings.Contains(s, "留学") || strings.Contains(s, "申请"):
		return "study_abroad"
	case strings.Contains(s, "秋招") || strings.Contains(s, "春招") || strings.Contains(s, "找工作") || strings.Contains(s, "求职"):
		return "job_season"
	case strings.Contains(s, "进组") || strings.Contains(s, "科研") || strings.Contains(s, "导师"):
		return "research_entry"
	case strings.Contains(s, "竞赛") || strings.Contains(s, "比赛"):
		return "competition_season"
	}
	return "postgrad_recommend"
}

// lifeDecisionBranches 「该不该」型问题的回应：只给别人走过的分支与代价，不给建议。
func lifeDecisionBranches() []gin.H {
	rows, err := db.Query(`SELECT goal_label, walked_count, COALESCE(branch_summary,'{}'), provenance
		FROM paths ORDER BY walked_count DESC LIMIT 5`)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var goal, branch, prov string
		var walked int
		if rows.Scan(&goal, &walked, &branch, &prov) == nil {
			out = append(out, gin.H{
				"goal_label":      goal,
				"walked_count":    walked,
				"branch_summary":  rawOrDefault(branch, "{}"),
				"provenance":      prov,
				"provenance_note": provenanceNote(prov),
			})
		}
	}
	return out
}

// rejectionResources 拒绝时给出的替代动作
func rejectionResources(intent string) []gin.H {
	switch intent {
	case IntentEmotionalSupport:
		return []gin.H{
			{"label": "学校心理健康中心", "hint": "多数高校提供免费预约咨询"},
			{"label": "先不聊这个，看看我的下一步", "action": "goto_home"},
		}
	case IntentLifeDecision:
		return []gin.H{
			{"label": "看看走过不同路的人", "action": "goto_graph"},
		}
	case IntentZeroSum:
		return []gin.H{
			{"label": "只做我能控制的那部分", "action": "goto_home"},
		}
	case IntentResourceDep:
		return []gin.H{
			{"label": "先写好联系邮件", "action": "goto_home"},
		}
	}
	return []gin.H{}
}
