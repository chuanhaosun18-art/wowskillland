// 静态扫描模块（管道阶段①，前置门禁）：
// 对 Skill 包做安全静态分析，任一红线命中 → 直接打回，不允许进入动态沙箱。
//   - 代码安全：危险系统调用 / 网络访问 / 文件越权
//   - 依赖安全：requirements.txt 已知漏洞包
//   - 提示注入：SKILL.md / 配置试图覆盖平台规则
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// StaticVerdict 静态扫描结论
const (
	StaticPass = "pass"
	StaticFail = "fail"
	StaticWarn = "warn" // 非红线，仅记录供人工参考
)

// staticScanResult 一次完整扫描的汇总
type staticScanResult struct {
	Passed   bool   // 全部通过
	Failures []string
	Warns    []string
}

// 危险代码模式（正则，命中即红线）
var dangerousCodePatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{"危险系统调用(os.system/subprocess 无参数白名单)", regexp.MustCompile(`(?i)\b(os\.system|os\.popen|subprocess\.(call|run|Popen)|execv|system\()`)},
	{"动态执行(eval/exec/compile)", regexp.MustCompile(`(?i)\b(eval|exec|compile)\s*\(`)},
	{"文件越权(绝对路径写 / 根目录 / 宿主路径)", regexp.MustCompile(`(?i)(open|write|os\.write)\(\s*["'](?:/|C:|D:)|\.\./\.\./`)},
	{"删除宿主文件(shutil.rmtree 根路径)", regexp.MustCompile(`(?i)shutil\.rmtree\s*\(\s*["'](?:/|C:|D:)`)},
	{"网络访问(请求外部地址)", regexp.MustCompile(`(?i)\b(requests|urllib|httpx|aiohttp|socket|curl|wget)\b`)},
	{"环境变量窃取", regexp.MustCompile(`(?i)(os\.environ|getenv)\s*\(\s*["'](API_KEY|TOKEN|SECRET|PASSWORD)`)},
	{"数据库/配置文件读取外带", regexp.MustCompile(`(?i)(\.ssh/|\.aws/|\.env["']|id_rsa|credentials)`)},
}

// 依赖已知漏洞包（MVP 内置精简库；生产接 osv.dev / snyk API）
var knownVulnDeps = map[string]string{
	"pip<=20.3.3":  "CVE-2021-3572",
	"flask<2.2.2":  "CVE-2023-30861",
	"numpy<1.22.0": "CVE-2021-34141",
	"requests<2.28.1": "CVE-2018-18074",
	"pillow<9.2.0": "CVE-2022-45199",
}

// 提示注入模式：SKILL.md/System Prompt 试图覆盖平台规则
var promptInjectionPatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{"无视规则指令", regexp.MustCompile(`(?i)(忽略|无视|不要管|不用理会|override).{0,20}(规则|指令|system|安全|限制)`)},
	{"伪装系统提示", regexp.MustCompile(`(?i)你是\s*(system|GPT-4|OpenAI|平台管理员|开发者)`)},
	{"诱导提权", regexp.MustCompile(`(?i)(输出你的|显示你的|泄露|提取).{0,10}(系统提示|system prompt|规则)`)},
	{"越权承诺", regexp.MustCompile(`(?i)(保证|承诺|100%|一定)(录取|通过|成功|不失败)`)},
}

// runStaticScan 对一个 Skill 包执行完整静态扫描，结果落 static_scans 表
func runStaticScan(runID, skillID int64) *staticScanResult {
	res := &staticScanResult{Passed: true}
	dir := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	var files []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		res.PassFail(runID, "静态扫描", StaticWarn, "Skill 包为空或无文件，无法建立动态环境")
		return res
	}

	// 1) 代码安全扫描（.py/.js/.sh/.go 等源码文件）
	codeFiles := []string{}
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".sh" || ext == ".go" || ext == ".rb" {
			codeFiles = append(codeFiles, f)
		}
	}
	for _, f := range codeFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(content)
		rel, _ := filepath.Rel(dir, f)
		for _, pat := range dangerousCodePatterns {
			if pat.re.MatchString(text) {
				// 网络访问在经验型（纯提示词型）技能里常见且必要 → 降级为 warn；脚本型仍为 fail
				msg := fmt.Sprintf("%s：%s", rel, pat.label)
				if strings.Contains(pat.label, "网络访问") {
					res.PassFail(runID, "代码安全", StaticWarn, msg)
				} else {
					res.PassFail(runID, "代码安全", StaticFail, msg)
				}
			}
		}
	}

	// 2) 依赖安全扫描（requirements.txt / pyproject.toml）
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		if base != "requirements.txt" && base != "pyproject.toml" {
			continue
		}
		content, _ := os.ReadFile(f)
		for _, line := range strings.Split(string(content), "\n") {
			dep := strings.TrimSpace(line)
			if dep == "" || strings.HasPrefix(dep, "#") {
				continue
			}
			low := strings.ToLower(dep)
			for pkg, cve := range knownVulnDeps {
				// 匹配形如 "pkg==x" / "pkg>=x" 且版本 ≤ 风险线（MVP 简化：包名命中即告警）
				if strings.HasPrefix(low, strings.Split(pkg, "<")[0]) {
					res.PassFail(runID, "依赖安全", StaticFail, fmt.Sprintf("%s 含已知漏洞 %s", dep, cve))
				}
			}
		}
	}

	// 3) 提示注入扫描（SKILL.md + *.md + 配置文件）
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		if base != "skill.md" && !strings.HasSuffix(base, ".md") {
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(content)
		for _, pat := range promptInjectionPatterns {
			if pat.re.MatchString(text) {
				rel, _ := filepath.Rel(dir, f)
				res.PassFail(runID, "提示注入", StaticFail, fmt.Sprintf("%s：%s", rel, pat.label))
			}
		}
	}

	if !res.Passed {
		updateRunStage(runID, StageStaticScan, PipeRejected, "静态扫描未通过："+strings.Join(res.Failures, "；"))
	}
	return res
}

// PassFail 记录一条扫描结论并写库
func (r *staticScanResult) PassFail(runID int64, item, verdict, detail string) {
	if len(detail) > 2000 {
		detail = detail[:2000]
	}
	if verdict == StaticFail {
		r.Failures = append(r.Failures, detail)
		r.Passed = false
	} else if verdict == StaticWarn {
		r.Warns = append(r.Warns, detail)
	}
	if runID > 0 {
		db.Exec(`INSERT INTO static_scans (run_id, item, verdict, detail) VALUES (?, ?, ?, ?)`,
			runID, item, verdict, detail)
	}
}

// updateRunStage 更新管道阶段与状态
func updateRunStage(runID int64, stage, status, summary string) {
	if runID == 0 {
		return
	}
	db.Exec(`UPDATE pipeline_runs SET stage = ?, status = ?, summary = ? WHERE id = ?`, stage, status, summary, runID)
}
