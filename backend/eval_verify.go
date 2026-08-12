// 强验证（F2P/P2P）模块：契约 verification 字段定义的确定性断言，替代「LLM 主观判完成」。
//   F2P（fail_to_pass）：产出物必须满足的验收断言（文件存在、包含关键内容等）
//   P2P（pass_to_pass）：输入变体下断言仍成立，防退化
// 断言在沙箱内执行（文件尚未清理时），结果随 sandbox_runs 落库，供判定 Agent 确定性打分。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// VerifyCheck 一条确定性断言
type VerifyCheck struct {
	Name    string `json:"name"`    // 断言名称（报告展示）
	Type    string `json:"type"`    // file_exists | file_non_empty | file_contains | file_regex | exit_zero | stdout_contains
	Target  string `json:"target"`  // 相对工作目录的文件路径（file_* 类必填）
	Pattern string `json:"pattern"` // file_contains / file_regex / stdout_contains 的模式
	Expect  *bool  `json:"expect"`  // nil/缺省 = 正向期望；显式 false = 反向断言（期望不存在 / 不包含）
	Group   string `json:"-"`       // 执行侧标记：f2p / p2p（不参与契约 JSON）
}

// CheckResult 一条断言的执行结果
type CheckResult struct {
	Group  string `json:"group"` // f2p / p2p
	Name   string `json:"name"`
	Type   string `json:"type"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// VerificationSpec 契约 verification 字段的解析结果
type VerificationSpec struct {
	FailToPass []VerifyCheck `json:"fail_to_pass"`
	PassToPass []VerifyCheck `json:"pass_to_pass"`
}

// parseVerification 解析契约 verification（JSON 对象字符串），失败返回空规格
func parseVerification(raw string) VerificationSpec {
	var v VerificationSpec
	if strings.TrimSpace(raw) == "" {
		return v
	}
	json.Unmarshal([]byte(raw), &v)
	return v
}

// allChecks 合并两组断言（沙箱执行时统一跑，判定时按 Group 区分 F2P/P2P）
func allChecks(v VerificationSpec) []VerifyCheck {
	var out []VerifyCheck
	for _, ch := range v.FailToPass {
		ch.Group = "f2p"
		out = append(out, ch)
	}
	for _, ch := range v.PassToPass {
		ch.Group = "p2p"
		out = append(out, ch)
	}
	return out
}

// runChecks 在沙箱工作目录内执行断言（必须在目录清理前调用）
func runChecks(workDir string, res *ExecResult, checks []VerifyCheck) []CheckResult {
	var out []CheckResult
	for _, ch := range checks {
		group := ch.Group
		if group != "f2p" && group != "p2p" {
			group = "f2p"
		}
		r := CheckResult{Group: group, Name: ch.Name, Type: ch.Type}
		ok, detail := runOneCheck(workDir, res, ch)
		if ch.Expect != nil && !*ch.Expect { // 显式 expect:false = 反向断言
			ok = !ok
			detail += "（反向断言取反）"
		}
		r.Passed = ok
		r.Detail = detail
		out = append(out, r)
	}
	return out
}

func runOneCheck(workDir string, res *ExecResult, ch VerifyCheck) (bool, string) {
	switch ch.Type {
	case "file_exists":
		p := filepath.Join(workDir, filepath.FromSlash(ch.Target))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return true, fmt.Sprintf("存在 %s", ch.Target)
		}
		return false, fmt.Sprintf("缺失 %s", ch.Target)
	case "file_non_empty":
		p := filepath.Join(workDir, filepath.FromSlash(ch.Target))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if fi.Size() > 0 {
				return true, fmt.Sprintf("%s 非空（%d B）", ch.Target, fi.Size())
			}
			return false, fmt.Sprintf("%s 为空文件", ch.Target)
		}
		return false, fmt.Sprintf("缺失 %s", ch.Target)
	case "file_contains", "file_regex":
		p := filepath.Join(workDir, filepath.FromSlash(ch.Target))
		b, err := os.ReadFile(p)
		if err != nil {
			return false, fmt.Sprintf("读取失败 %s", ch.Target)
		}
		if ch.Type == "file_contains" {
			ok := strings.Contains(string(b), ch.Pattern)
			return ok, fmt.Sprintf("%s 含模式 %q：%v", ch.Target, ch.Pattern, ok)
		}
		re, err := regexp.Compile(ch.Pattern)
		if err != nil {
			return false, "正则非法: " + ch.Pattern
		}
		ok := re.Match(b)
		return ok, fmt.Sprintf("%s 匹配正则 %q：%v", ch.Target, ch.Pattern, ok)
	case "exit_zero":
		ok := res.ExitCode == 0 && !res.TimedOut
		return ok, fmt.Sprintf("退出码 %d%s", res.ExitCode, map[bool]string{true: "", false: "（非 0）"}[ok])
	case "stdout_contains":
		ok := strings.Contains(res.Stdout, ch.Pattern)
		return ok, fmt.Sprintf("stdout 含 %q：%v", ch.Pattern, ok)
	default:
		return false, "未知断言类型 " + ch.Type
	}
}

// strongVerifySummary 聚合全部沙箱记录的断言结果：
// F2P 断言只在 completion 用例上计（验收标准），P2P 断言只在 robustness 用例上计（防退化）。
type strongVerifySummary struct {
	HasF2P, HasP2P      bool
	F2PTotal, F2PPassed int
	P2PTotal, P2PPassed int
	F2PFailed           []string // 失败的 F2P 断言（名称：详情）
	P2PFailed           []string
}

// summarizeStrongVerify 统计断言通过情况（用例级：该用例断言全部通过才算过，执行失败/超时视为不过）
func summarizeStrongVerify(transcripts []sandboxTranscript) strongVerifySummary {
	var s strongVerifySummary
	for _, t := range transcripts {
		switch t.EvalType {
		case EvalCompletion:
			var g []CheckResult
			for _, c := range t.Checks {
				if c.Group == "f2p" {
					g = append(g, c)
				}
			}
			if len(g) > 0 {
				s.HasF2P = true
			}
			s.F2PTotal++
			ok := len(g) > 0
			for _, c := range g {
				if !c.Passed {
					ok = false
					s.F2PFailed = append(s.F2PFailed, c.Name+"："+c.Detail)
				}
			}
			if ok {
				s.F2PPassed++
			}
		case EvalRobustness:
			var g []CheckResult
			for _, c := range t.Checks {
				if c.Group == "p2p" {
					g = append(g, c)
				}
			}
			if len(g) > 0 {
				s.HasP2P = true
			}
			s.P2PTotal++
			ok := len(g) > 0
			for _, c := range g {
				if !c.Passed {
					ok = false
					s.P2PFailed = append(s.P2PFailed, c.Name+"："+c.Detail)
				}
			}
			if ok {
				s.P2PPassed++
			}
		}
	}
	return s
}

// agentStrongVerification 强验证 Agent（确定性，非 LLM）：契约配了 verification 断言时，
// 用断言执行结果聚合打分；未配断言时返回空（不写入结果，保留 LLM 兜底）。
func agentStrongVerification(in evalAgentInput, transcripts []sandboxTranscript) []agentResult {
	spec := parseVerification(in.Contract.Verification)
	if len(spec.FailToPass) == 0 && len(spec.PassToPass) == 0 {
		return nil
	}
	s := summarizeStrongVerify(transcripts)
	var parts []string
	if s.HasF2P {
		parts = append(parts, fmt.Sprintf("F2P 通过 %d/%d", s.F2PPassed, s.F2PTotal))
		for _, f := range s.F2PFailed {
			parts = append(parts, "F2P 失败："+f)
		}
	}
	if s.HasP2P {
		parts = append(parts, fmt.Sprintf("P2P 通过 %d/%d", s.P2PPassed, s.P2PTotal))
		for _, f := range s.P2PFailed {
			parts = append(parts, "P2P 失败："+f)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "断言未在对应用例类型上执行（无 completion/robustness 记录）")
	}

	total, passed := 0, 0
	if s.HasF2P {
		total += s.F2PTotal
		passed += s.F2PPassed
	}
	if s.HasP2P {
		total += s.P2PTotal
		passed += s.P2PPassed
	}
	if total == 0 {
		total = 1
	}
	score := float64(passed) / float64(total)
	// F2P 对齐完成度阈值（0.8），P2P 对齐鲁棒性阈值（0.7），保证四问语义一致
	f2pOK := !s.HasF2P || float64(s.F2PPassed) >= float64(s.F2PTotal)*0.8
	p2pOK := !s.HasP2P || float64(s.P2PPassed) >= float64(s.P2PTotal)*0.7
	ok := f2pOK && p2pOK

	// 证据：全量断言明细（供报告展示）
	var all []CheckResult
	for _, t := range transcripts {
		all = append(all, t.Checks...)
	}
	evidence, _ := json.Marshal(all)

	return []agentResult{{
		Agent:            AgentStrongVerify,
		Item:             ItemStrongVerify,
		Score:            score,
		Threshold:        1.0,
		Passed:           ok,
		Reason:           strings.Join(parts, "；"),
		Evidence:         string(evidence),
		Confidence:       1.0,
		NeedsHumanReview: !ok,
	}}
}
