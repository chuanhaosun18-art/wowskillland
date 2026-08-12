// 迷茫期路由器 · A1 路由器 + S1 入口对话
//
// S1：唯一入口，只有一个输入框「说说你现在的不知道」。
// 路由器判定迷茫形态四态：探索型 / 抉择型 / 行动型 / 情绪类（拦截，给校内心理支持入口）。
// 两条硬约束（答辩即卖点）：
//  1. 情绪类立即拦截，不落任何记录。
//  2. 判定结果必须明示给用户——路由透明本身是信任的一部分，判错了用户可以一键改道。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// momentLite moments 表的一行（S1/S2/S4 展示用）
type momentLite struct {
	ID           int64  `json:"id"`
	RawText      string `json:"raw_text"`
	RoutedState  string `json:"routed_state"`
	RoutedReason string `json:"routed_reason"`
	Context      string `json:"context"`
	CreatedAt    string `json:"created_at"`
}

// routerResult A1 路由器的判定结果
type routerResult struct {
	State       string `json:"state"`         // explore / decide / act / emotion
	Label       string `json:"label"`         // 中文名
	Message     string `json:"message"`       // 明示给用户的判定结果
	EmotionNote string `json:"emotion_note,omitempty"` // 情绪拦截时给的心理支持指引
}

// momentStateLabel 四态中文标签
func momentStateLabel(s string) string {
	switch s {
	case MomentExplore:
		return "探索型"
	case MomentDecide:
		return "抉择型"
	case MomentAct:
		return "行动型"
	case MomentEmotion:
		return "情绪类"
	default:
		return "未知"
	}
}

// routerSystemPrompt A1 路由器判定 prompt
const routerSystemPrompt = `你是一个「迷茫期路由器」。用户会说出此刻的「不知道」，请把它分类到四种迷茫形态之一。

四种形态的定义：
1. explore（探索型）：还不知道自己想要什么，在多个方向之间茫然，需要先找几件值得试的小事。典型信号：「不知道要什么」「没想清楚」「有点迷茫」「什么都想试试但都不确定」。
2. decide（抉择型）：已经有明确的几个选项，在它们之间做选择。典型信号：「A还是B」「该保研还是就业」「要不要转专业」「选哪个」。
3. act（行动型）：已经决定方向，需要的是接下来怎么排、怎么做。典型信号：「决定考研了怎么排」「开始准备了，接下来呢」「第一步做什么」。
4. emotion（情绪类）：主要是情绪宣泄而非任务，包括焦虑、孤独、崩溃、无意义感等。典型信号：「我好焦虑」「很孤独」「撑不下去了」「不知道活着有什么意义」。情绪类必须拦截，不进入任务流。

规则：
- 只输出 JSON：{"state": "explore|decide|act|emotion", "message": "用中文明示给用户的判定结果"}。
- message 必须明示判定结果（「听起来你还没想清楚要什么——我们先不聊选择，先找几件值得试的小事。」这种口吻），让用户能确认或改道。
- 情绪类 message 里给出关怀与校内心理支持入口的提示。
- 不允许输出 JSON 以外的任何内容。`

// routeMoment A1 路由器：一句话分类四态
// POST /api/crossroad/moments  输入 {raw_text}  输出 判定结果 + 落库（情绪类不落库）
func routeMoment(c *gin.Context) {
	uid := c.GetInt64("userID")
	var body struct {
		RawText string `json:"raw_text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	raw := strings.TrimSpace(body.RawText)
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "说点什么吧"})
		return
	}

	prompt := fmt.Sprintf("用户说：%q\n\n请判定他的迷茫形态。", raw)
	content, err := callGuideDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: routerSystemPrompt},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		log.Printf("crossroad route: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "路由判定失败：" + err.Error()})
		return
	}

	var res routerResult
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &res); err != nil || res.State == "" {
		log.Printf("crossroad route: bad llm json: %s", content)
		c.JSON(http.StatusBadGateway, gin.H{"error": "路由判定异常，请重试"})
		return
	}

	// 情绪类：拦截，不落任何记录（产品纪律）
	if res.State == MomentEmotion {
		c.JSON(http.StatusOK, gin.H{
			"routed": false,
			"result": res,
			"note":   "情绪类已被拦截，未保存任何记录。如需倾诉或支持，可前往学校心理中心或拨打心理支持热线。",
		})
		return
	}

	// 其余形态落库 moments
	ctxJSON, _ := json.Marshal(map[string]string{"grade": "", "weekly_hours": ""})
	resTx, err := db.Exec(`INSERT INTO moments (user_id, raw_text, routed_state, routed_reason, context)
		VALUES (?, ?, ?, ?, ?)`, uid, raw, res.State, res.Message, string(ctxJSON))
	if err != nil {
		log.Printf("crossroad route: insert moment: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	id, _ := resTx.LastInsertId()

	c.JSON(http.StatusOK, gin.H{
		"routed": true,
		"moment_id": id,
		"result": res,
		"state_label": momentStateLabel(res.State),
		"next": map[string]string{
			MomentExplore: "start_interview", // S2 探索流：访谈 → 假设 → 选卡
			MomentDecide:  "show_fork",       // S5 路口页：只看走过的人
			MomentAct:     "show_orchestration", // F17 编排态（存量能力）
		}[res.State],
	})
}

// listMyMoments 我的迷茫记录（探索地图的时间轴素材）
// GET /api/crossroad/moments
func listMyMoments(c *gin.Context) {
	uid := c.GetInt64("userID")
	rows, err := db.Query(`SELECT id, raw_text, routed_state, routed_reason, context, created_at
		FROM moments WHERE user_id = ? ORDER BY id DESC LIMIT 100`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	list := []momentLite{}
	for rows.Next() {
		var m momentLite
		if err := rows.Scan(&m.ID, &m.RawText, &m.RoutedState, &m.RoutedReason, &m.Context, &m.CreatedAt); err == nil {
			list = append(list, m)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// getMoment 单个迷茫记录详情
// GET /api/crossroad/moments/:momentId
func getMoment(c *gin.Context) {
	uid := c.GetInt64("userID")
	id := c.Param("momentId")
	var m momentLite
	err := db.QueryRow(`SELECT id, raw_text, routed_state, routed_reason, context, created_at
		FROM moments WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&m.ID, &m.RawText, &m.RoutedState, &m.RoutedReason, &m.Context, &m.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": m})
}
