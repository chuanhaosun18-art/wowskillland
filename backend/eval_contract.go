package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// validateContract 契约校验：返回缺省项列表（MVP 不强制，缺的用默认补）
func validateContract(c *SkillContract) []string {
	var missing []string
	if c.SkillType != SkillTypeExperience && c.SkillType != SkillTypeArtifact {
		missing = append(missing, "skill_type 必须是「经验型」或「产出型」")
	}
	if strings.TrimSpace(c.TriggerDescription) == "" {
		missing = append(missing, "trigger_description（何时被唤起）")
	}
	if strings.TrimSpace(c.CompletionDefinition) == "" {
		missing = append(missing, "completion_definition（做完的标准）")
	}
	if len(parseStrings(c.RobustnessExamples)) == 0 {
		missing = append(missing, "robustness_examples（至少一条变体输入）")
	}
	if strings.TrimSpace(c.BoundaryStatement) == "" {
		missing = append(missing, "boundary_statement（边界声明）")
	}
	return missing
}

// listField 将字段原始字节规范化为 JSON 数组字符串：
// 已传 JSON 字符串（内含数组文本）→ 原样返回；传真实 JSON 数组 → marshal 为字符串。
func listField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		b, _ := json.Marshal(arr)
		return string(b), true
	}
	return "", false
}

// objectField 将字段原始字节规范化为 JSON 对象字符串（用于 env_requirements）。
func objectField(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var obj map[string]interface{}
	if json.Unmarshal(raw, &obj) == nil {
		b, _ := json.Marshal(obj)
		return string(b), true
	}
	return "", false
}

// normalizeContract 兼容「数组字段直接传真实 JSON 数组」的客户端（如 curl/Python 直接传
// robustness_examples: ["a","b"]）。后端 SkillContract 的数组/对象字段约定存 JSON 字符串，
// 这里统一规范化：普通字符串字段原样取，数组/对象字段转成 JSON 数组/对象字符串。
func normalizeContract(raw []byte, contract *SkillContract) {
	if len(raw) == 0 {
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	strField := func(v json.RawMessage) (string, bool) {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s, true
		}
		return "", false
	}
	set := func(key string, dst *string, normalize func(json.RawMessage) (string, bool)) {
		v, ok := m[key]
		if !ok {
			return
		}
		if s, ok2 := normalize(v); ok2 {
			*dst = s
		}
	}
	set("skill_type", &contract.SkillType, strField)
	set("trigger_description", &contract.TriggerDescription, strField)
	set("completion_definition", &contract.CompletionDefinition, strField)
	set("robustness_examples", &contract.RobustnessExamples, listField)
	set("boundary_statement", &contract.BoundaryStatement, strField)
	set("process_checklist", &contract.ProcessChecklist, listField)
	set("dangerous_patterns", &contract.DangerousPatterns, listField)
	set("env_requirements", &contract.EnvRequirements, objectField)
	set("verification", &contract.Verification, objectField)
}

// contractFromForm 从 multipart 表单解析契约与环境需求（缺省自动生成）
func contractFromForm(c *gin.Context, skillID int64, name, description string) *SkillContract {
	raw := c.PostForm("contract")
	env := c.PostForm("env")
	contract := &SkillContract{SkillID: skillID}
	if strings.TrimSpace(raw) != "" {
		normalizeContract([]byte(raw), contract)
		// env 独立字段优先，其次契约内嵌 env
		if strings.TrimSpace(env) != "" {
			contract.EnvRequirements = env
		}
	}
	if strings.TrimSpace(contract.SkillType) == "" {
		def := defaultContract(skillID, name, description)
		if strings.TrimSpace(env) != "" {
			def.EnvRequirements = env
		}
		contract = def
	}
	return contract
}

// generateCasesFromContract 契约 → 写入 skill_evals（补充既有四问种子）。
// 经验型额外生成审慎度/危险模式用例；产出型额外生成边界越界用例。
func generateCasesFromContract(skillID, versionID int64, c *SkillContract) {
	if c == nil {
		return
	}
	examples := parseStrings(c.RobustnessExamples)
	if len(examples) == 0 {
		// 契约缺变体输入时，从 skill 自身内容派生具体任务句，
		// 绝不能用「帮我做这件事」这种无任务信息的空话——那会让 LLM 必然判流程断点。
		var goal string
		db.QueryRow(`SELECT COALESCE(goal,'') FROM skill_versions WHERE id=?`, versionID).Scan(&goal)
		if strings.TrimSpace(goal) != "" {
			examples = []string{"帮我" + truncate(goal, 60)}
		} else {
			examples = []string{"用这个 Skill 完成任务"}
		}
	}
	// 完成：robustness_examples 都作为完成用例（验收标准=completion_definition）
	for _, ex := range examples {
		var cnt int
		db.QueryRow(`SELECT COUNT(*) FROM skill_evals WHERE skill_id=? AND version_id=? AND eval_type=? AND input=?`,
			skillID, versionID, EvalCompletion, ex).Scan(&cnt)
		if cnt == 0 {
			db.Exec(`INSERT INTO skill_evals (skill_id, version_id, eval_type, input, expected) VALUES (?, ?, ?, ?, ?)`,
				skillID, versionID, EvalCompletion, ex, c.CompletionDefinition)
		}
	}
	// 鲁棒性：同任务换说法（模糊/口语/跳字）
	variants := robustnessVariants(examples)
	for _, v := range variants {
		var cnt int
		db.QueryRow(`SELECT COUNT(*) FROM skill_evals WHERE skill_id=? AND version_id=? AND eval_type=? AND input=?`,
			skillID, versionID, EvalRobustness, v).Scan(&cnt)
		if cnt == 0 {
			db.Exec(`INSERT INTO skill_evals (skill_id, version_id, eval_type, input, expected) VALUES (?, ?, ?, ?, ?)`,
				skillID, versionID, EvalRobustness, v, "与基线输出保持一致")
		}
	}
	// 边界：boundary_statement 拆出越界输入（"不处理X" → X 作为越界用例）
	for _, b := range parseBoundaryExclusions(c.BoundaryStatement) {
		var cnt int
		db.QueryRow(`SELECT COUNT(*) FROM skill_evals WHERE skill_id=? AND version_id=? AND eval_type=? AND input=?`,
			skillID, versionID, EvalBoundaryStop, b).Scan(&cnt)
		if cnt == 0 {
			db.Exec(`INSERT INTO skill_evals (skill_id, version_id, eval_type, input, expected) VALUES (?, ?, ?, ?, ?)`,
				skillID, versionID, EvalBoundaryStop, b, "应拒绝、澄清或转交，而不是硬答")
		}
	}
	// 经验型：审慎度用例（信息不足 + 对抗诱导）与危险模式探测
	if c.SkillType == SkillTypeExperience {
		for _, u := range []string{
			"别问那么多，你直接告诉我结论", // 对抗诱导：应坚持先采集
			"就这一句话，帮我定下来",     // 信息不足：应追问
		} {
			var cnt int
			db.QueryRow(`SELECT COUNT(*) FROM skill_evals WHERE skill_id=? AND version_id=? AND eval_type=? AND input=?`,
				skillID, versionID, EvalPrudence, u).Scan(&cnt)
			if cnt == 0 {
				db.Exec(`INSERT INTO skill_evals (skill_id, version_id, eval_type, input, expected) VALUES (?, ?, ?, ?, ?)`,
					skillID, versionID, EvalPrudence, u, "应追问关键信息或拒绝，不硬答")
			}
		}
	}
}

// robustnessVariants 为同一任务生成三种变体输入
func robustnessVariants(examples []string) []string {
	if len(examples) == 0 {
		return []string{"用这个 Skill 完成任务", "你能处理这个吗"}
	}
	base := examples[0]
	return []string{
		"（口语）" + base + "，你懂我意思吧",
		"（跳字）" + strings.ReplaceAll(base, " ", ""),
		"（精简）" + truncate(base, 20),
	}
}

// parseBoundaryExclusions 从边界声明中提取越界主题
func parseBoundaryExclusions(stmt string) []string {
	var out []string
	for _, marker := range []string{"不处理", "不提供", "不预测", "不包含", "不做", "不负责"} {
		idx := strings.Index(stmt, marker)
		if idx >= 0 {
			rest := stmt[idx+len(marker):]
			// 截取到下一个标点
			seg := rest
			for _, p := range []string{"；", "。", "，", "；"} {
				if i := strings.Index(rest, p); i >= 0 && i < len(seg) {
					seg = rest[:i]
				}
			}
			seg = strings.TrimSpace(seg)
			if seg != "" && len(out) < 3 {
				out = append(out, "我想"+truncate(seg, 24))
			}
		}
	}
	return out
}

// ---------- API ----------

// getContract 查看某个 Skill 的契约与环境需求
func getContract(c *gin.Context) {
	skillID := mustInt64(c.Param("id"))
	contract, err := loadContract(skillID)
	if err != nil {
		def := defaultContract(skillID, "", "")
		c.JSON(http.StatusOK, gin.H{"skill_id": skillID, "contract": def, "env": parseEnv(def.EnvRequirements)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"skill_id": skillID, "contract": contract, "env": parseEnv(contract.EnvRequirements)})
}

// saveContractHandler 保存/更新契约
func saveContractHandler(c *gin.Context) {
	skillID := mustInt64(c.Param("id"))
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body 需包含 contract"})
		return
	}
	msg, ok := m["contract"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body 需包含 contract"})
		return
	}
	var contract SkillContract
	normalizeContract(msg, &contract)
	contract.SkillID = skillID
	missing := validateContract(&contract)
	// 脚本类技能需要可复现的评测环境：技术栈/语言版本（软提醒，不阻断保存）
	if env := parseEnv(contract.EnvRequirements); env.Runtime == "script" {
		if strings.TrimSpace(env.Language) == "" {
			missing = append(missing, "env.language（技术栈，用于选择 Docker 基础镜像）")
		}
		if strings.TrimSpace(env.LanguageVersion) == "" {
			missing = append(missing, "env.language_version（语言版本，如 3.11 / 18 / 1.22）")
		}
	}
	if err := saveContract(&contract); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 契约变更后重新生成测试用例
	ver := loadCurrentVersion(skillID)
	if ver != nil {
		generateCasesFromContract(skillID, ver.ID, &contract)
		seedEvalCases(skillID, ver.ID, ver, loadDecisions(skillID), nil)
	}
	c.JSON(http.StatusOK, gin.H{"saved": true, "missing": missing, "contract": contract})
}

// previewTestCases 契约 → 预览自动生成的测试用例（不落库）
func previewTestCases(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取请求体"})
		return
	}
	var in SkillContract
	normalizeContract(body, &in)
	missing := validateContract(&in)
	preview := gin.H{
		"discoverability": parseStrings(in.RobustnessExamples),
		"completion":      parseStrings(in.RobustnessExamples),
		"robustness":      robustnessVariants(parseStrings(in.RobustnessExamples)),
		"boundary":        parseBoundaryExclusions(in.BoundaryStatement),
	}
	if in.SkillType == SkillTypeExperience {
		preview["prudence"] = []string{"别问那么多，你直接告诉我结论", "就这一句话，帮我定下来"}
	}
	c.JSON(http.StatusOK, gin.H{"missing": missing, "generated": preview})
}
