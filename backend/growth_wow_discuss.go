// 首页「Wow 多轮对话」意图识别 Agent
// 用户说「写论文」「画架构图」这类要动手的话时，识别意图并调出平台里最相关的 skill 卡片，
// 前端把卡片渲染成可点击的入口，点击跳转到 Skill 详情页。
// 与 crossroad 迷茫期对话的分工：迷茫期接「不知道」，这里优先接「要动手做」。
// 三条纪律（产品口径）：不测评、不建议、不承诺、不选边。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// wowDiscussReq 首页多轮对话入参
type wowDiscussReq struct {
	SessionID int64         `json:"session_id"`
	Utterance string        `json:"utterance"`
	History   []chatMsg     `json:"history"`
	Forks     interface{}   `json:"forks"`
}

// wowSkillBrief 意图识别上下文里携带的 skill 摘要（id 用数字，前端映射成 'be-'+id）
type wowSkillBrief struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Tags        string `json:"tags"`
	Icon        string `json:"icon"`
}

// wowIntentResult LLM 输出结构（严格 JSON）
type wowIntentResult struct {
	Intent          string  `json:"intent"`            // explore | decide | action | emotion
	MatchedSkillIDs []int64 `json:"matched_skill_ids"` // 命中的 skill id 数组（可空）
	Reply           string  `json:"reply"`             // 对用户说的话
}

// wowDiscussSystemPrompt 意图识别 agent 的 system prompt：身份 + 完整 skill 清单 + 输出要求
func wowDiscussSystemPrompt(skills []wowSkillBrief) string {
	var sb strings.Builder
	sb.WriteString(`你是 WowSkillLand 平台的意图识别 agent。你清楚平台的全部 Skill 库，知道下面列出的每个 Skill 能做什么。

你的任务：判断用户是在「继续聊困惑」，还是在「要动手做某件事」（写论文、画图、准备保研材料等）。如果用户明确表示要动手做某件事，就从下面的 Skill 清单里挑出最相关的 1-3 个 id。

产品口径三条纪律：不测评（不给用户打分评级）、不建议（不替用户做人生选择）、不承诺（不保证结果）、不选边（抉择类不替用户站队）。

必须严格只输出 JSON，不要 markdown 代码块，不要任何多余文字。格式：
{"intent":"explore|decide|action|emotion","matched_skill_ids":[数字数组],"reply":"对用户说的话"}

规则：
1. intent 取值：explore（继续聊困惑/探索）、decide（抉择困惑，如该不该保研）、action（明确要动手做某件事）、emotion（情绪崩溃/撑不住，先接住情绪）。
2. 用户明确表示要动手做某件事（写论文、画图、准备保研材料等）→ intent=action，并从下面 Skill 清单挑最相关的 1-3 个 id 填入 matched_skill_ids。
3. 纯倾诉 / 抉择困惑 → explore 或 decide，matched_skill_ids 填空数组 []。
4. 情绪崩溃 / 撑不住 → emotion，matched_skill_ids 填空数组 []。
5. reply 用第一人称、简洁、贴合用户原话，不把整张 Skill 表念一遍，也不要罗列一堆卡片。

【当前平台 Skill 清单】
`)
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("- id=%d 名称：%s 分类：%s 标签：%s 描述：%s\n",
			s.ID, s.Name, s.Category, s.Tags, s.Description))
	}
	return sb.String()
}

// handleWowDiscuss POST /api/growth/wow/discuss（需登录）
// 任何异常都返回 200 + degraded=true + 空 skills，让前端走本地兜底，绝不 500。
func handleWowDiscuss(c *gin.Context) {
	var body wowDiscussReq
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusOK, wowDiscussFallback(body.SessionID, "我在听，你慢慢说。"))
		return
	}
	utterance := strings.TrimSpace(body.Utterance)
	if utterance == "" {
		c.JSON(http.StatusOK, wowDiscussFallback(body.SessionID, "我在听，你慢慢说。"))
		return
	}
	if len([]rune(utterance)) > 500 {
		utterance = string([]rune(utterance)[:500])
	}

	// 查全部 published skills（过滤写法与 handlers.go listSkills 保持一致）
	skills := listWowSkills()

	// session_id 为 0 时自增（毫秒时间戳，足够唯一）
	sessionID := body.SessionID
	if sessionID == 0 {
		sessionID = time.Now().UnixMilli()
	}

	// 记忆：situation 取当前原话，facts 简单拼接 history 内容（截断防爆）
	memory := gin.H{"situation": utterance, "facts": []string{}}
	if len(body.History) > 0 {
		facts := []string{}
		for _, h := range body.History {
			t := strings.TrimSpace(h.Content)
			if t == "" {
				continue
			}
			t = truncate(t, 40)
			if len(facts) >= 8 {
				break
			}
			facts = append(facts, t)
		}
		memory["facts"] = facts
	}

	// 调 LLM 做意图识别；失败/无 key 时降级为关键词匹配
	matched, reply, degraded := wowClassify(c, body, skills)

	// 组装返回
	next := "talk"
	routeExit := "talk"
	ctxSkills := []gin.H{}
	if len(matched) > 0 {
		next = "match"
		routeExit = "action"
		for _, s := range matched {
			ctxSkills = append(ctxSkills, gin.H{
				"id":          s.ID,
				"name":        s.Name,
				"description": s.Description,
				"category":    s.Category,
				"tags":        s.Tags,
				"icon":        s.Icon,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"reply":       reply,
		"next":        next,
		"route_exit":  routeExit,
		"session_id":  sessionID,
		"memory":      memory,
		"context":     gin.H{"skills": ctxSkills},
		"degraded":    degraded,
	})
}

// wowDiscussFallback 入参非法时的兜底返回（200 + degraded，前端自行降级）
func wowDiscussFallback(sessionID int64, reply string) gin.H {
	if sessionID == 0 {
		sessionID = time.Now().UnixMilli()
	}
	return gin.H{
		"reply":      reply,
		"next":       "talk",
		"route_exit": "talk",
		"session_id": sessionID,
		"memory":     gin.H{"situation": "", "facts": []string{}},
		"context":    gin.H{"skills": []gin.H{}},
		"degraded":   true,
	}
}

// listWowSkills 查询全部 published skill 的意图识别摘要（id/name/description/category/tags/icon）
func listWowSkills() []wowSkillBrief {
	rows, err := db.Query(`SELECT id, name, description, category, COALESCE(tags,''), COALESCE(icon,'')
		FROM skills
		WHERE COALESCE(NULLIF(status,''),'published') = 'published'
		ORDER BY id`)
	if err != nil {
		log.Printf("[wow-discuss] query skills: %v", err)
		return nil
	}
	defer rows.Close()

	skills := []wowSkillBrief{}
	for rows.Next() {
		var s wowSkillBrief
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Category, &s.Tags, &s.Icon); err != nil {
			log.Printf("[wow-discuss] scan skill: %v", err)
			continue
		}
		skills = append(skills, s)
	}
	return skills
}

// wowClassify 调 DeepSeek 做意图识别，返回 (命中的 skill, reply, 是否降级)。
// LLM 失败 / 无 key / JSON 解析失败一律降级为关键词匹配。
func wowClassify(c *gin.Context, body wowDiscussReq, skills []wowSkillBrief) ([]wowSkillBrief, string, bool) {
	if len(skills) == 0 {
		return nil, "我这边暂时没有找到对应的 Skill 卡，你再多说两句。", true
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	// 多轮上下文：system + 历史消息 + 当前原话
	messages := []chatMsg{{Role: "system", Content: wowDiscussSystemPrompt(skills)}}
	for _, h := range body.History {
		role := strings.TrimSpace(h.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(h.Content)
		if content == "" {
			continue
		}
		messages = append(messages, chatMsg{Role: role, Content: content})
	}
	messages = append(messages, chatMsg{Role: "user", Content: strings.TrimSpace(body.Utterance)})

	raw, err := callDeepSeekWithKeyOpts(ctx, messages, "DEEPSEEK_API_KEY", 2000)
	if err != nil {
		log.Printf("[wow-discuss] llm err: %v", err)
		return wowDegradeMatch(strings.TrimSpace(body.Utterance), skills)
	}

	var out wowIntentResult
	jsonStr := extractJSON(raw)
	if e := json.Unmarshal([]byte(jsonStr), &out); e != nil {
		// 输出被截断时的兜底：尝试自动闭合再解一次
		if repaired := repairClosingJSON(jsonStr); repaired != jsonStr {
			var out2 wowIntentResult
			if e2 := json.Unmarshal([]byte(repaired), &out2); e2 == nil && (out2.Reply != "" || len(out2.MatchedSkillIDs) > 0) {
				return wowPickSkills(out2, skills), strings.TrimSpace(out2.Reply), false
			}
		}
		log.Printf("[wow-discuss] unmarshal fail: %v raw=%q", e, truncate(raw, 500))
		return wowDegradeMatch(strings.TrimSpace(body.Utterance), skills)
	}
	return wowPickSkills(out, skills), strings.TrimSpace(out.Reply), false
}

// wowPickSkills 按 LLM 给出的 id 列表从清单里挑出 skill，保持清单顺序，最多 3 个
func wowPickSkills(out wowIntentResult, skills []wowSkillBrief) []wowSkillBrief {
	byID := map[int64]wowSkillBrief{}
	for _, s := range skills {
		byID[s.ID] = s
	}
	picked := []wowSkillBrief{}
	seen := map[int64]bool{}
	for _, id := range out.MatchedSkillIDs {
		if seen[id] {
			continue
		}
		if s, ok := byID[id]; ok {
			seen[id] = true
			picked = append(picked, s)
		}
		if len(picked) >= 3 {
			break
		}
	}
	return picked
}

// wowDegradeMatch 无 key / LLM 失败时的降级：把用户原话分词（补常见近义词）后，
// 与每个 skill 的 name+category+tags 做子串包含匹配（如「论文」命中 id=5，「图/画」命中 id=39）。
// 返回 (命中的 skill 列表, 降级文案, degraded=true)。
func wowDegradeMatch(utterance string, skills []wowSkillBrief) ([]wowSkillBrief, string, bool) {
	// 按标点/空白切词，过滤过短的碎片
	cleaned := strings.NewReplacer("，", " ", "。", " ", "？", " ", "！", " ", "、", " ",
		",", " ", ".", " ", "？", " ", "：", " ", "；", " ").Replace(utterance)
	tokens := []string{}
	for _, w := range strings.Fields(cleaned) {
		if len([]rune(w)) >= 2 {
			tokens = append(tokens, w)
		}
	}
	// 常见意图词兜底：整句是一个词时（如「最近要画架构图」）靠近义扩展命中
	ext := []string{}
	for _, t := range tokens {
		switch {
		case strings.Contains(t, "论文") || strings.Contains(t, "写作") || strings.Contains(t, "latex"):
			ext = append(ext, "论文", "LaTeX", "学术")
		case strings.Contains(t, "画") || strings.Contains(t, "图") || strings.Contains(t, "架构"):
			ext = append(ext, "图", "架构图", "Draw.io", "图表")
		case strings.Contains(t, "保研") || strings.Contains(t, "推免") || strings.Contains(t, "夏令营"):
			ext = append(ext, "保研", "推免", "夏令营", "材料")
		case strings.Contains(t, "简历") || strings.Contains(t, "求职") || strings.Contains(t, "面试"):
			ext = append(ext, "简历", "面试")
		}
	}
	tokens = append(tokens, ext...)

	type scored struct {
		s wowSkillBrief
		n int
	}
	matched := []scored{}
	for _, s := range skills {
		blob := s.Name + " " + s.Category + " " + s.Tags
		n := 0
		for _, t := range tokens {
			if t != "" && strings.Contains(blob, t) {
				n++
			}
		}
		if n > 0 {
			matched = append(matched, scored{s: s, n: n})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].n > matched[j].n })

	picked := []wowSkillBrief{}
	for _, m := range matched {
		picked = append(picked, m.s)
		if len(picked) >= 3 {
			break
		}
	}
	if len(picked) == 0 {
		return nil, "没搜到和这件事对得上的卡。你再说细一点，比如具体要做什么、卡在哪一步。", true
	}
	names := []string{}
	for _, p := range picked {
		names = append(names, p.Name)
	}
	return picked, "按关键词找到了这些卡：" + strings.Join(names, "、") + "。点开看看合不合用。", true
}
