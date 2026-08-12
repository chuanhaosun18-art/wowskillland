// 评测报告与人工复核台 API：
//   GET  /api/growth/eval/skills/:id/report           评测进度与最终报告
//   GET  /api/growth/eval/skills/:id/human-review      待人工复核的评测项
//   POST /api/growth/eval/human-review/submit          人工复核提交（结果回写标注库）
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// evalReport 一次评测的完整报告
type evalReport struct {
	SkillID    int64             `json:"skill_id"`
	Run        *PipelineRun      `json:"run"`
	Contract   *SkillContract    `json:"contract"`
	Env        EnvRequirements   `json:"env"`
	StaticScan []map[string]string `json:"static_scans"`
	Sandbox    []map[string]interface{} `json:"sandbox_runs"`
	Results    []map[string]interface{} `json:"results"`
	NeedReview int               `json:"need_review_count"`
}

// getEvalReport GET /api/growth/eval/skills/:id/report
func getEvalReport(c *gin.Context) {
	skillID := mustInt64(c.Param("id"))
	if skillID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	run, err := newestPipelineRun(skillID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"skill_id": skillID, "run": nil, "message": "尚未运行评测管道"})
		return
	}

	report := evalReport{SkillID: skillID, Run: run}

	// 契约与环境
	if contract, err := loadContract(skillID); err == nil {
		report.Contract = contract
		report.Env = parseEnv(contract.EnvRequirements)
		report.Env.Image = dockerImageFor(report.Env) // 报告展示：实际用于复现的 Docker 镜像
	}

	// 静态扫描明细
	if rows, err := db.Query(`SELECT item, verdict, detail FROM static_scans WHERE run_id = ? ORDER BY id`, run.ID); err == nil {
		for rows.Next() {
			var item, verdict, detail string
			if rows.Scan(&item, &verdict, &detail) == nil {
				report.StaticScan = append(report.StaticScan, map[string]string{"item": item, "verdict": verdict, "detail": detail})
			}
		}
		rows.Close()
	}

	// 沙箱执行记录（交互日志 + 产出物 + 强验证断言）
	if rows, err := db.Query(`SELECT input, transcript, output, artifacts, checks, duration_ms, timeout FROM sandbox_runs WHERE run_id = ? ORDER BY id`, run.ID); err == nil {
		for rows.Next() {
			var input, transcript, output, artifacts, checks string
			var durationMS, timeout int
			if rows.Scan(&input, &transcript, &output, &artifacts, &checks, &durationMS, &timeout) == nil {
				report.Sandbox = append(report.Sandbox, map[string]interface{}{
					"input": input, "transcript": transcript, "output": output,
					"artifacts": artifacts, "checks": checks, "duration_ms": durationMS, "timeout": timeout == 1,
				})
			}
		}
		rows.Close()
	}

	// 评测结果 + 人工复核状态
	needReview := 0
	if rows, err := db.Query(`SELECT id, agent, item, score, threshold, passed, reason, evidence, confidence, needs_human_review
		FROM pipeline_results WHERE run_id = ? ORDER BY id`, run.ID); err == nil {
		for rows.Next() {
			var id int64
			var agent, item, reason, evidence string
			var score, threshold, confidence float64
			var passed, needsReview int
			if rows.Scan(&id, &agent, &item, &score, &threshold, &passed, &reason, &evidence, &confidence, &needsReview) == nil {
				if needsReview == 1 {
					needReview++
				}
				report.Results = append(report.Results, map[string]interface{}{
					"id": id, "agent": agent, "item": item, "score": score, "threshold": threshold,
					"passed": passed == 1, "reason": reason, "evidence": evidence,
					"confidence": confidence, "needs_human_review": needsReview == 1,
				})
			}
		}
		rows.Close()
	}
	report.NeedReview = needReview
	c.JSON(http.StatusOK, report)
}

// getHumanReviewCases GET /api/growth/eval/skills/:id/human-review
// 列出该 Skill 最近一次管道中所有标记为「需人工复核」的评测项
func getHumanReviewCases(c *gin.Context) {
	skillID := mustInt64(c.Param("id"))
	run, err := newestPipelineRun(skillID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"items": []interface{}{}})
		return
	}
	type reviewCase struct {
		ResultID int64             `json:"result_id"`
		Agent    string            `json:"agent"`
		Item     string            `json:"item"`
		Score    float64           `json:"score"`
		Reason   string            `json:"reason"`
		Evidence string            `json:"evidence"`
		Review   map[string]string `json:"review,omitempty"`
	}
	var items []reviewCase
	rows, err := db.Query(`SELECT pr.id, pr.agent, pr.item, pr.score, pr.reason, pr.evidence,
		COALESCE((SELECT decision FROM human_reviews hr WHERE hr.result_id = pr.id ORDER BY hr.id DESC LIMIT 1), '')
		FROM pipeline_results pr WHERE pr.run_id = ? AND pr.needs_human_review = 1 ORDER BY pr.id`, run.ID)
	if err == nil {
		for rows.Next() {
			var rc reviewCase
			var decision string
			if rows.Scan(&rc.ResultID, &rc.Agent, &rc.Item, &rc.Score, &rc.Reason, &rc.Evidence, &decision) == nil {
				if decision != "" {
					rc.Review = map[string]string{"decision": decision}
				}
				items = append(items, rc)
			}
		}
		rows.Close()
	}
	c.JSON(http.StatusOK, gin.H{"run_id": run.ID, "items": items})
}

// submitHumanReview POST /api/growth/eval/human-review/submit
// body: {"result_id":1,"decision":"approve|reject|revise","note":"理由"}
// 人工结果作为标注数据回写，未来用于微调质量评判 Agent
func submitHumanReview(c *gin.Context) {
	uid := c.GetInt64("userID")
	var in struct {
		ResultID int64  `json:"result_id"`
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.ResultID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body 需包含 result_id 与 decision"})
		return
	}
	switch in.Decision {
	case "approve", "reject", "revise":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision 必须为 approve/reject/revise"})
		return
	}
	if _, err := db.Exec(`INSERT INTO human_reviews (result_id, reviewer_id, decision, note) VALUES (?, ?, ?, ?)`,
		in.ResultID, uid, in.Decision, in.Note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 复核后：approve 视为通过（清除待复核标记），reject 视为否决，revise 保留待复核
	var mark int
	var passed int
	switch in.Decision {
	case "approve":
		mark, passed = 0, 1
	case "reject":
		mark, passed = 0, 0
	default:
		mark, passed = 1, 0
	}
	db.Exec(`UPDATE pipeline_results SET needs_human_review = ?, passed = ? WHERE id = ?`, mark, passed, in.ResultID)
	// 回写标注库（MVP：落到 desc_case 用表等价物 human_reviews 已满足；生产接标注服务）
	c.JSON(http.StatusOK, gin.H{"submitted": true, "result_id": in.ResultID, "decision": in.Decision})
}
