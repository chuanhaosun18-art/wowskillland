// SKILL.md 评测锚点解析：把 AI 引导生成的结构化 SKILL.md 区块解析进草稿字段与关键判断表。
// 这样 AI 引导生成的 Skill 一上传就带齐蒸馏度六维与四问素材，可直接通过门禁。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// findSkillMD 在 skill 目录下递归查找 SKILL.md（容忍 zip 内 kebab 子目录嵌套），未找到返回空串
func findSkillMD(skillID int64) string {
	root := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), "SKILL.md") {
			found = path
		}
		return nil
	})
	return found
}

// applySkillMDToDraft 上传 zip 解压登记后调用：读取磁盘上的 SKILL.md，
// 把「核心步骤 / 完成标准 / 关键判断 / 失败案例 / 适用边界」区块解析进草稿。
// 只更新解析出的非空内容，已有的手动内容用合并而非覆盖；SKILL.md 不存在或没有锚点区块时静默跳过。
func applySkillMDToDraft(skillID, verID int64) error {
	mdPath := findSkillMD(skillID)
	if mdPath == "" {
		return nil // 没有 SKILL.md（非引导生成的上传）不算错误，保持原样
	}
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return nil
	}
	md := string(raw)

	// 1) 核心步骤 → workflow（[{step,how,check}]，≥1 步才写入）
	steps := parseWorkflowSteps(md)
	if len(steps) > 0 {
		db.Exec(`UPDATE skill_versions SET workflow = ? WHERE id = ?`, jsonOrEmpty(steps), verID)
	}

	// 2) 完成标准 → done_criteria（[]string，≥1 条才写入）
	criteria := parseCriteria(md)
	if len(criteria) > 0 {
		db.Exec(`UPDATE skill_versions SET done_criteria = ? WHERE id = ?`, jsonOrEmpty(criteria), verID)
	}

	// 3) 关键判断 → decisions（四槽位，逐条校验 + 去重后插入）
	applyDecisionsFromMD(skillID, verID, md)

	// 4) 失败案例 → gotchas（[{trigger,symptom,consequence}]）
	gotchas := parseGotchas(md)
	if len(gotchas) > 0 {
		db.Exec(`UPDATE skill_versions SET gotchas = ? WHERE id = ?`, jsonOrEmpty(gotchas), verID)
	}

	// 5) 适用边界 → boundary（合并去重，不覆盖已有的人工内容）
	applyBoundaryFromMD(verID, md)

	log.Printf("applied SKILL.md eval anchors to skill=%d version=%d (workflow=%d criteria=%d gotchas=%d)",
		skillID, verID, len(steps), len(criteria), len(gotchas))
	return nil
}

// section 截取 markdown 中某个 ## 标题下的内容，直到下一个 ## 标题或结尾
func section(md, title string) string {
	re := regexp.MustCompile(`(?m)^##\s*` + regexp.QuoteMeta(title) + `\s*$`)
	loc := re.FindStringIndex(md)
	if loc == nil {
		return ""
	}
	start := loc[1]
	rest := md[start:]
	if nxt := regexp.MustCompile(`(?m)^##\s+`).FindStringIndex(rest); nxt != nil {
		rest = rest[:nxt[0]]
	}
	return strings.TrimSpace(rest)
}

// splitKV 按「键：值」切一行，兼容全角竖线 / 半角竖线分隔的多个键值段
func splitKV(line, key string) string {
	key = key + "："
	line = strings.ReplaceAll(line, "｜", "|")
	parts := strings.Split(line, "|")
	for _, p := range parts {
		if strings.Contains(p, key) {
			p = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p), "- "))
			return cleanKV(strings.TrimPrefix(p, key))
		}
	}
	// 兜底：整行里找「key：」到行尾/下一个中文键前
	if i := strings.Index(line, key); i >= 0 {
		rest := line[i+len(key):]
		if j := strings.Index(rest, "｜"); j >= 0 {
			rest = rest[:j]
		}
		return cleanKV(strings.TrimSpace(rest))
	}
	return ""
}

// cleanKV 去掉取值残留的 markdown 列表前缀（如 "- 步骤：xxx" 取出的 "- xxx"）
func cleanKV(v string) string {
	v = strings.TrimPrefix(v, "- ")
	v = strings.TrimPrefix(v, "-")
	return strings.TrimSpace(v)
}

// workflowStep 草稿 workflow 数组元素
type workflowStep struct {
	Step string `json:"step"`
	How  string `json:"how"`
	Check string `json:"check"`
}

// parseWorkflowSteps 解析「## 核心步骤」下的行：- 步骤：X ｜ 做法：Y ｜ 验收：Z
func parseWorkflowSteps(md string) []workflowStep {
	out := []workflowStep{}
	for _, line := range strings.Split(section(md, "核心步骤"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") || !strings.Contains(line, "步骤") {
			continue
		}
		step := splitKV(line, "步骤")
		how := splitKV(line, "做法")
		check := splitKV(line, "验收")
		if step == "" {
			continue
		}
		if how == "" && check == "" {
			step = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		}
		out = append(out, workflowStep{Step: step, How: how, Check: check})
	}
	return out
}

// parseCriteria 解析「## 完成标准」下「1. xxx」或「- xxx」的行
func parseCriteria(md string) []string {
	out := []string{}
	for _, line := range strings.Split(section(md, "完成标准"), "\n") {
		line = strings.TrimSpace(line)
		line = regexp.MustCompile(`^\d+[.、]\s*`).ReplaceAllString(line, "")
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// gotchaItem 草稿 gotchas 数组元素
type gotchaItem struct {
	Trigger     string `json:"trigger"`
	Symptom     string `json:"symptom"`
	Consequence string `json:"consequence"`
}

// parseGotchas 解析「## 失败案例」下的行：- 触发：X ｜ 表现：Y ｜ 后果：Z
func parseGotchas(md string) []gotchaItem {
	out := []gotchaItem{}
	for _, line := range strings.Split(section(md, "失败案例"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		trigger := splitKV(line, "触发")
		if trigger == "" {
			continue
		}
		out = append(out, gotchaItem{
			Trigger:     trigger,
			Symptom:     splitKV(line, "表现"),
			Consequence: splitKV(line, "后果"),
		})
	}
	return out
}

// slotAliases 槽位中文别名 → 合法槽位（容忍 AI 或用户写中文而非英文枚举）
var slotAliases = map[string]string{
	"when_to_check":   SlotWhenToCheck,
	"回头验证":          SlotWhenToCheck,
	"停下来验证":         SlotWhenToCheck,
	"when_to_probe":   SlotWhenToProbe,
	"补充信息":          SlotWhenToProbe,
	"when_to_use_tool": SlotWhenToUseTool,
	"必须查":            SlotWhenToUseTool,
	"必须跑":            SlotWhenToUseTool,
	"when_to_switch":  SlotWhenToSwitch,
	"走不通":            SlotWhenToSwitch,
}

// applyDecisionsFromMD 解析「## 关键判断」下的行：
// - 槽位：when_to_xxx ｜ 触发：t ｜ 判断：j ｜ 场景：s
func applyDecisionsFromMD(skillID, verID int64, md string) {
	var execID int64
	db.QueryRow(`SELECT COALESCE(source_execution_id, 0) FROM skill_versions WHERE id = ?`, verID).Scan(&execID)
	existing := loadDecisions(skillID)
	hasDup := func(slot, trigger string) bool {
		for _, d := range existing {
			if d.InvalidatedAt == nil && d.Slot == slot && d.TriggerSignal == trigger {
				return true
			}
		}
		return false
	}
	for _, line := range strings.Split(section(md, "关键判断"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		slotKey := strings.TrimSpace(splitKV(line, "槽位"))
		slot := slotAliases[slotKey]
		if slot == "" {
			for k, v := range slotAliases {
				if strings.Contains(slotKey, k) {
					slot = v
					break
				}
			}
		}
		trigger := strings.TrimSpace(splitKV(line, "触发"))
		judgment := strings.TrimSpace(splitKV(line, "判断"))
		scope := strings.TrimSpace(splitKV(line, "场景"))
		if !isValidSlot(slot) || trigger == "" || judgment == "" || scope == "" {
			continue
		}
		if hasDup(slot, trigger) {
			continue
		}
		db.Exec(`INSERT INTO decisions (experience_exec_id, skill_id, slot, trigger_signal, judgment, scope,
			counter_example, source_step_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			execID, skillID, slot, trigger, judgment, scope, "", 0)
	}
}

// applyBoundaryFromMD 解析「## 适用边界」：- 不适用：X 与 - 交回给人：Y，合并去重写入草稿
func applyBoundaryFromMD(verID int64, md string) {
	sec := section(md, "适用边界")
	if sec == "" {
		return
	}
	var notApplicable, handoff []string
	for _, line := range strings.Split(sec, "\n") {
		line = strings.TrimSpace(line)
		if v := splitKV(line, "不适用"); v != "" {
			notApplicable = append(notApplicable, v)
		}
		if v := splitKV(line, "交回给人"); v != "" {
			handoff = append(handoff, v)
		}
	}
	if len(notApplicable) == 0 && len(handoff) == 0 {
		return
	}
	var b struct {
		NotApplicable  []string `json:"not_applicable"`
		HandoffTrigger []string `json:"handoff_trigger"`
		FallbackPath   string   `json:"fallback_path"`
	}
	_ = json.Unmarshal([]byte(mustBoundaryString(verID)), &b)
	b.NotApplicable = mergeUnique(b.NotApplicable, notApplicable)
	b.HandoffTrigger = mergeUnique(b.HandoffTrigger, handoff)
	db.Exec(`UPDATE skill_versions SET boundary = ? WHERE id = ?`, jsonOrEmpty(b), verID)
}

// mustBoundaryString 读当前 boundary（空则给空 JSON）
func mustBoundaryString(verID int64) string {
	var s string
	if err := db.QueryRow(`SELECT COALESCE(boundary,'') FROM skill_versions WHERE id = ?`, verID).Scan(&s); err != nil {
		return ""
	}
	if strings.TrimSpace(s) == "" {
		return `{"not_applicable":[],"handoff_trigger":[],"fallback_path":""}`
	}
	return s
}
