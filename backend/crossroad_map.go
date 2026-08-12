// 迷茫期路由器 · S5 路口分叉 + A6 地图叙事
//
// S5：抉择型 moment 进入路口页，只看「走过的人」——绝对人数与代价原话，不给百分比。
// A6：探索地图叙事只讲走过的路，禁止评价词（不测评、不建议、不承诺延伸到叙事）。 
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// forkLite forks 表一行（S5 路口页）
type forkLite struct {
	ID            int64    `json:"id"`
	JunctionLabel string   `json:"junction_label"`
	WalkedCount   int      `json:"walked_count"`
	Branches      []string `json:"branches"`
	SwitchNode    string   `json:"switch_node"`
}

// listForks GET /api/crossroad/forks?q=关键词  路口分叉（只显示绝对人数）
func listForks(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	query := `SELECT id, junction_label, walked_count, branches, switch_node FROM forks`
	args := []interface{}{}
	if q != "" {
		query += ` WHERE junction_label LIKE ?`
		args = append(args, "%"+q+"%")
	}
	query += ` ORDER BY walked_count DESC LIMIT 50`
	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer rows.Close()
	out := []forkLite{}
	for rows.Next() {
		var f forkLite
		var branchesJSON string
		if rows.Scan(&f.ID, &f.JunctionLabel, &f.WalkedCount, &branchesJSON, &f.SwitchNode) == nil {
			_ = json.Unmarshal([]byte(branchesJSON), &f.Branches)
			out = append(out, f)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// upsertFork POST /api/crossroad/forks  记录一个路口被走到（decide 态用户经过路口）
// body {junction_label, branch?}
func upsertFork(c *gin.Context) {
	var body struct {
		JunctionLabel string `json:"junction_label"`
		Branch        string `json:"branch"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	label := strings.TrimSpace(body.JunctionLabel)
	if label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路口不能为空"})
		return
	}

	var id int64
	var walked int
	var branchesJSON string
	err := db.QueryRow(`SELECT id, walked_count, branches FROM forks WHERE junction_label = ?`, label).
		Scan(&id, &walked, &branchesJSON)
	if err != nil {
		// 新建
		branches := []string{}
		if strings.TrimSpace(body.Branch) != "" {
			branches = append(branches, strings.TrimSpace(body.Branch))
		}
		bj, _ := json.Marshal(branches)
		res, ierr := db.Exec(`INSERT INTO forks (junction_label, walked_count, branches) VALUES (?, 1, ?)`, label, string(bj))
		if ierr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
			return
		}
		id, _ = res.LastInsertId()
		walked = 1
	} else {
		// 更新：人数 +1，分支去重追加
		var branches []string
		_ = json.Unmarshal([]byte(branchesJSON), &branches)
		added := false
		if strings.TrimSpace(body.Branch) != "" {
			b := strings.TrimSpace(body.Branch)
			dup := false
			for _, x := range branches {
				if x == b {
					dup = true
					break
				}
			}
			if !dup {
				branches = append(branches, b)
				added = true
			}
		}
		if added {
			bj, _ := json.Marshal(branches)
			_, _ = db.Exec(`UPDATE forks SET walked_count = walked_count + 1, branches = ? WHERE id = ?`, string(bj), id)
		} else {
			_, _ = db.Exec(`UPDATE forks SET walked_count = walked_count + 1 WHERE id = ?`, id)
		}
		walked++
	}
	c.JSON(http.StatusOK, gin.H{"fork_id": id, "junction_label": label, "walked_count": walked})
}

// ---------- A6 地图叙事（探索地图） ----------

// mapNarrativeSystemPrompt A6 叙事 prompt：禁评价词
const mapNarrativeSystemPrompt = `你在把一个人的迷茫旅程，讲成一张「探索地图」的叙事。

输入是他的迷茫记录（每次的「不知道」）、走过的一步卡、以及尝试结果。

铁律：
1. 只叙述走过的路，不评价：禁止出现「很棒」「加油」「优秀」「你一定行」等任何评价或鼓励性判断。也禁止预测结果。
2. 用第二人称「你」，像地图旁的文字，语气平实。
3. 结构：先一句话概括这段路的形状 → 按时间顺序讲每一步卡和结果 → 最后一句停在「现在走到了这里」，不催不劝。
4. 300 字以内，输出纯文本，不要 markdown、不要列表符号。`

// narrateMap POST /api/crossroad/map/narrate  用我的迷茫记录生成地图叙事
func narrateMap(c *gin.Context) {
	uid := c.GetInt64("userID")

	// 取我的迷茫记录（已路由的）
	mrows, err := db.Query(`SELECT id, raw_text, routed_state, created_at FROM moments WHERE user_id = ? ORDER BY id DESC LIMIT 20`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	type momentRow struct {
		ID     int64
		Raw    string
		State  string
		Date   string
	}
	var moments []momentRow
	for mrows.Next() {
		var m momentRow
		if mrows.Scan(&m.ID, &m.Raw, &m.State, &m.Date) == nil {
			moments = append(moments, m)
		}
	}
	mrows.Close()

	if len(moments) == 0 {
		c.JSON(http.StatusOK, gin.H{"narrative": "地图还是空的。去 S1 说一句「我不知道」开始。"})
		return
	}

	// 取我的尝试（走过的卡）
	arows, err := db.Query(`SELECT c.title, a.status, a.verdict, a.created_at
		FROM attempts a LEFT JOIN cards c ON c.id = a.card_id WHERE a.user_id = ? ORDER BY a.id LIMIT 20`, uid)
	if err == nil {
		defer arows.Close()
	}

	var sb strings.Builder
	sb.WriteString("【迷茫记录】\n")
	for _, m := range moments {
		sb.WriteString(fmt.Sprintf("- %s（%s态，%s）\n", m.Raw, momentStateLabel(m.State), m.Date))
	}
	sb.WriteString("【走过的卡】\n")
	if arows != nil {
		for arows.Next() {
			var title string
			var status, verdict, date string
			if arows.Scan(&title, &status, &verdict, &date) == nil {
				verdictCN := map[string]string{"done": "走完了", "partial": "走了一半", "not_done": "没走通"}[verdict]
				if verdictCN == "" {
					verdictCN = status
				}
				sb.WriteString(fmt.Sprintf("- 卡《%s》：%s（%s）\n", title, verdictCN, date))
			}
		}
	}

	content, err := callGuideDeepSeek(context.Background(), []chatMsg{
		{Role: "system", Content: mapNarrativeSystemPrompt},
		{Role: "user", Content: sb.String()},
	})
	if err != nil {
		// 降级：不生成叙事，返回朴素时间轴
		c.JSON(http.StatusOK, gin.H{"narrative": "", "degraded": true, "moments": moments})
		return
	}
	narrative := strings.TrimSpace(content)
	// 叙事禁评价词兜底（即使模型违反也拦一道）
	for _, w := range []string{"很棒", "加油", "优秀", "一定行", "厉害"} {
		if strings.Contains(narrative, w) {
			narrative = strings.ReplaceAll(narrative, w, "")
		}
	}
	c.JSON(http.StatusOK, gin.H{"narrative": narrative, "moments": moments})
}

// myCrossroadMap GET /api/crossroad/map  探索地图素材（S4 个人页）
func myCrossroadMap(c *gin.Context) {
	uid := c.GetInt64("userID")

	mrows, err := db.Query(`SELECT id, raw_text, routed_state, routed_reason, created_at FROM moments WHERE user_id = ? ORDER BY id DESC LIMIT 50`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	defer mrows.Close()
	moments := []gin.H{}
	for mrows.Next() {
		var id int64
		var raw, state, reason, date string
		if mrows.Scan(&id, &raw, &state, &reason, &date) == nil {
			moments = append(moments, gin.H{
				"id": id, "raw_text": raw, "routed_state": state,
				"routed_reason": reason, "created_at": date,
				"state_label": momentStateLabel(state),
			})
		}
	}
	// 已读完再查假设/尝试（SQLite 单连接：避免循环内查询）
	hypMap := map[int64][]gin.H{}
	hr, err := db.Query(`SELECT h.moment_id, h.id, h.label, h.evidence_quote, h.card_id FROM hypotheses h ORDER BY h.id`)
	if err == nil {
		for hr.Next() {
			var momentID, hid int64
			var label, quote string
			var cardID *int64
			if hr.Scan(&momentID, &hid, &label, &quote, &cardID) == nil {
				hypMap[momentID] = append(hypMap[momentID], gin.H{"id": hid, "label": label, "evidence_quote": quote, "card_id": cardID})
			}
		}
		hr.Close()
	}

	arows, err := db.Query(`SELECT a.id, a.card_id, c.title, a.moment_id, a.status, a.verdict, a.created_at
		FROM attempts a LEFT JOIN cards c ON c.id = a.card_id WHERE a.user_id = ? ORDER BY a.id DESC LIMIT 50`, uid)
	attByMoment := map[int64][]gin.H{}
	if err == nil {
		for arows.Next() {
			var aid, cardID int64
			var title, status, verdict, date string
			var momentID *int64
			if arows.Scan(&aid, &cardID, &title, &momentID, &status, &verdict, &date) == nil {
				mk := int64(0)
				if momentID != nil {
					mk = *momentID
				}
				attByMoment[mk] = append(attByMoment[mk], gin.H{"id": aid, "card_id": cardID, "card_title": title, "status": status, "verdict": verdict, "created_at": date})
			}
		}
		arows.Close()
	}

	for i := range moments {
		mid := moments[i]["id"].(int64)
		if v, ok := hypMap[mid]; ok {
			moments[i]["hypotheses"] = v
		} else {
			moments[i]["hypotheses"] = []gin.H{}
		}
		if v, ok := attByMoment[mid]; ok {
			moments[i]["attempts"] = v
		} else {
			moments[i]["attempts"] = []gin.H{}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": moments})
}
