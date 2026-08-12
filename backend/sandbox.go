// 动态沙箱执行模块：Skill 代码只在隔离环境里跑。
// 本机 MVP：Sandbox 接口 + LocalSandbox（受限子进程：超时强制杀、空环境变量、临时目录隔离）。
// 生产：DockerSandbox（同接口，容器资源限制 + 只读挂载 + 网络隔离），代码结构已就位，按部署环境启用。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExecRequest 一次沙箱执行请求
type ExecRequest struct {
	// Command 要执行的命令（argv[0]），如 python / docker 内入口脚本
	Command string
	// Args 命令参数
	Args []string
	// Stdin 标准输入（多轮对话时传用户消息）
	Stdin string
	// Env 额外环境变量（KEY=VALUE）。实现层会用「白名单 + 最小环境」合并，绝不直接透传宿主环境。
	Env []string
	// WorkDir 工作目录（必须已由实现层创建在受控根目录内）
	WorkDir string
	// Timeout 超时，超时强制终止
	Timeout time.Duration
	// MemoryMB 内存上限（本机子进程尽力而为，生产由 Docker 强制）
	MemoryMB int
	// NetAccess 是否允许网络（默认拒绝；Mock API 场景由实现层注入本地 stub 而非真实外网）
	NetAccess bool
	// Checks 产出物确定性断言（F2P/P2P 强验证），执行完成后、清理目录前执行
	Checks []VerifyCheck
}

// ExecResult 一次沙箱执行结果
type ExecResult struct {
	Stdout       string
	Stderr       string
	ExitCode     int
	TimedOut     bool
	Duration     time.Duration
	Artifacts    []string      // 产出物（容器/工作目录内生成的文件，如图片、文档），相对 WorkDir 的路径
	CheckResults []CheckResult // 断言执行结果（请求带 Checks 时非空）
}

// Sandbox 统一执行抽象：本机子进程 / Docker 容器 / 未来云函数都实现它。
type Sandbox interface {
	// Exec 在隔离环境中执行一次命令并返回结果
	Exec(ctx context.Context, req ExecRequest) (*ExecResult, error)
	// Close 释放沙箱资源（停止并销毁容器等）
	Close() error
}

// ---------- 本机受限子进程实现（MVP，Windows/无 Docker 环境） ----------

// LocalSandbox 通过受限子进程模拟隔离：
//   - 环境变量只保留白名单（PATH 定位运行时），杜绝继承宿主敏感变量
//   - 工作目录在临时根目录下，跑完清理
//   - context 超时强制终止（Windows 上终止直接子进程；生产换 Docker 杀整个进程组）
type LocalSandbox struct {
	RootDir string
}

// NewLocalSandbox 创建本机沙箱，临时目录在 SKILLHUB_DATA/sandbox_tmp 下
func NewLocalSandbox() *LocalSandbox {
	root := filepath.Join(DataDir, "sandbox_tmp")
	os.MkdirAll(root, 0o755)
	// 启动时清理超过 1 小时的旧工作目录：产物目录保留给本管道合规/视觉 Agent 读取
	// （exec 结束后不再立刻删除，避免 runSkillScript 返回的绝对路径指向已删除目录）
	cutoff := time.Now().Add(-time.Hour)
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				os.RemoveAll(filepath.Join(root, e.Name()))
			}
		}
	}
	return &LocalSandbox{RootDir: root}
}

func (s *LocalSandbox) Exec(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	start := time.Now()
	res := &ExecResult{}

	// 工作目录：必须落在受控根目录内（防路径穿越）
	workDir := req.WorkDir
	if workDir == "" {
		dir, err := os.MkdirTemp(s.RootDir, "run-")
		if err != nil {
			return nil, fmt.Errorf("create sandbox workdir: %w", err)
		}
		workDir = dir
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}
	rootAbs, _ := filepath.Abs(s.RootDir)
	if !strings.HasPrefix(abs+string(os.PathSeparator), rootAbs+string(os.PathSeparator)) {
		return nil, fmt.Errorf("workdir 越界：%s 不在沙箱根 %s 内", abs, rootAbs)
	}

	// 超时上下文
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if req.MemoryMB > 0 && req.MemoryMB < 64 {
		req.MemoryMB = 64 // 至少 64MB，避免误杀
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, req.Command, req.Args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(req.Stdin)

	// 最小环境：只保留定位运行时所需的 PATH；用户声明的 env 白名单追加
	cmd.Env = buildSandboxEnv(req.Env)
	if !req.NetAccess {
		// 本机子进程无法完全禁网，这里显式降级：记录日志并在文档中声明
		// 生产 Docker 用 --network=none + 本地 mock DNS。
		log.Printf("sandbox: 本机子进程无法强制断网，NetAccess=%v 仅记录；生产请用 DockerSandbox", req.NetAccess)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	res.Duration = time.Since(start)
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.ExitCode = 0
	if err != nil {
		if runCtx.Err() != nil {
			res.TimedOut = true
			res.ExitCode = -1 // 超时终止
		} else if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			// 命令本身找不到等启动错误：不算超时，调用方按失败处理
			return nil, fmt.Errorf("sandbox exec: %w", err)
		}
	}

	// 收集产出物：工作目录内、排除隐藏临时文件的普通文件
	filepath.Walk(workDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(workDir, p)
		if strings.HasPrefix(rel, ".") {
			return nil
		}
		res.Artifacts = append(res.Artifacts, filepath.ToSlash(rel))
		return nil
	})

	// 强验证（F2P/P2P）：产物断言必须在目录清理前执行
	if len(req.Checks) > 0 {
		res.CheckResults = runChecks(workDir, res, req.Checks)
	}

	// 工作目录不在此删除：产物（图片/文档）还需供管道后续合规/视觉 Agent 读取，
	// 目录由 NewLocalSandbox 启动时统一清理超过 1 小时的过期项。
	return res, nil
}

func (s *LocalSandbox) Close() error {
	return nil
}

// buildSandboxEnv 构建最小环境：白名单 + 用户声明项。绝不继承宿主环境变量。
func buildSandboxEnv(extra []string) []string {
	env := []string{}
	if runtime.GOOS == "windows" {
		env = append(env, "PATH="+os.Getenv("PATH"), "SystemRoot="+os.Getenv("SystemRoot"), "TEMP="+os.Getenv("TEMP"), "TMP="+os.Getenv("TMP"))
	} else {
		env = append(env, "PATH="+os.Getenv("PATH"), "HOME=/tmp")
	}
	return append(env, extra...)
}

// ---------- Docker 沙箱实现（生产环境） ----------

// DockerSandbox 用 Docker 容器隔离执行：
//   - 镜像：按 EnvRequirements.runtime 选择基础镜像（python:3.10-slim / pytorch+sd 等）
//   - 资源：--memory / --cpus / --pids-limit / --network=none（除非 NetAccess）
//   - 生命周期：run 结束即 rm，超时 docker kill
// 依赖 github.com/docker/docker 客户端；当前编译期可缺省（stub 提示安装）。
type DockerSandbox struct {
	BaseImage string
}

func NewDockerSandbox(baseImage string) *DockerSandbox {
	return &DockerSandbox{BaseImage: baseImage}
}

// dockerImageFor 按环境需求选择 Docker 基础镜像，用于复现技能运行环境：
//  1. 契约显式配置 base_image 时优先采用（作者最清楚镜像）；
//  2. 否则按 language + language_version 推断（python → python:<ver>-slim 等）；
//  3. 兜底按 runtime 选默认（python:3.10-slim；图像类走 pytorch 镜像）。
func dockerImageFor(env EnvRequirements) string {
	if strings.TrimSpace(env.BaseImage) != "" {
		return strings.TrimSpace(env.BaseImage)
	}
	lang := strings.ToLower(strings.TrimSpace(env.Language))
	ver := strings.TrimSpace(env.LanguageVersion)
	if lang != "" {
		switch lang {
		case "python", "python3":
			if ver == "" {
				ver = "3.10"
			}
			if env.GPU {
				return "pytorch/pytorch:" + ver + "-cuda12.1-cudnn8-runtime"
			}
			return "python:" + ver + "-slim"
		case "node", "nodejs":
			if ver == "" {
				ver = "18"
			}
			return "node:" + ver + "-slim"
		case "go", "golang":
			if ver == "" {
				ver = "1.22"
			}
			return "golang:" + ver + "-alpine"
		case "bash", "sh", "shell":
			return "alpine:3.19"
		}
	}
	if env.GPU {
		return "pytorch/pytorch:2.1.0-cuda12.1-cudnn8-runtime" // 图像生成类
	}
	return "python:3.10-slim" // 纯模型类不需要容器，走 LLM API
}

func (s *DockerSandbox) Exec(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	// 生产实现要点（本机无 Docker 时该分支不启用，返回错误即可）：
	// 1. docker run --rm --network=none --memory=Xm --cpus=2 --pids-limit=128
	//    -v <workdir>:/workspace -w /workspace <image> <command> <args...>
	// 2. 超时用 docker kill（而非 kill 宿主进程）
	// 3. 产出物收集：挂载卷目录内非隐藏文件
	return nil, fmt.Errorf("DockerSandbox 未启用：生产环境请安装 Docker 与 docker SDK 客户端")
}

func (s *DockerSandbox) Close() error {
	return nil
}

// ---------- Skill 执行封装：Agent 与管道统一从这里调用 Skill ----------

// skillRunRequest 单次与 Skill 交互的请求
type skillRunRequest struct {
	SkillID  int64
	Input    string   // 用户消息
	Contract *SkillContract
	// 会话上下文（多轮：前面 user/assistant 交替内容），JSON 数组字符串
	History string
}

// skillRunResult 单次与 Skill 交互的结果
type skillRunResult struct {
	Reply     string        // Skill 的回复
	Artifacts []string      // 产出物（如生成的论文/图片路径）
	Checks    []CheckResult // 强验证断言结果（script 类且契约配了 verification 时非空）
	TimedOut  bool
	Error     string
}

// runSkillOnce 执行一次 Skill 交互，按环境需求分派：
//   - runtime=model  → LLM API（Skill 的系统提示词 = SKILL.md 正文）
//   - runtime=script → 本机受限子进程跑 start_command，输入走 stdin
func runSkillOnce(ctx context.Context, req skillRunRequest) *skillRunResult {
	env := EnvRequirements{}
	if req.Contract != nil {
		env = parseEnv(req.Contract.EnvRequirements)
	}
	if env.Runtime == "" {
		env.Runtime = "model"
	}
	if env.TimeoutS == 0 {
		env.TimeoutS = 60
	}

	switch env.Runtime {
	case "script":
		return runSkillScript(ctx, req, env)
	default: // model
		return runSkillModel(ctx, req, env)
	}
}

// loadSkillPrompt 读取 Skill 包内的 SKILL.md 正文作为系统提示词
func loadSkillPrompt(skillID int64) string {
	dir := filepath.Join(FilesDir, fmt.Sprintf("%d", skillID))
	for _, cand := range []string{"SKILL.md", "skill.md"} {
		b, err := os.ReadFile(filepath.Join(dir, cand))
		if err == nil {
			return string(b)
		}
	}
	return ""
}

// runSkillModel 模型类 Skill：LLM 大脑 + SKILL.md 系统提示
func runSkillModel(ctx context.Context, req skillRunRequest, env EnvRequirements) *skillRunResult {
	prompt := loadSkillPrompt(req.SkillID)
	if strings.TrimSpace(prompt) == "" {
		prompt = "你是技能「" + skillName(req.SkillID) + "」。请根据用户输入完成对应任务，信息不足时先追问。"
	}
	msg := []chatMsg{{Role: "system", Content: prompt}}
	// 历史消息（若有）：History 是 JSON 数组字符串
	var hist []chatMsg
	if strings.TrimSpace(req.History) != "" {
		json.Unmarshal([]byte(req.History), &hist)
		msg = append(msg, hist...)
	}
	msg = append(msg, chatMsg{Role: "user", Content: req.Input})

	timeout := time.Duration(env.TimeoutS) * time.Second
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reply, err := callDeepSeek(callCtx, msg)
	if err != nil {
		if callCtx.Err() != nil {
			return &skillRunResult{TimedOut: true, Error: "skill 超时（" + timeout.String() + "）"}
		}
		return &skillRunResult{Error: "skill 执行失败: " + err.Error()}
	}
	return &skillRunResult{Reply: reply}
}

// runSkillScript 脚本类 Skill：在受限子进程/容器里跑 start_command，输入走 stdin
func runSkillScript(ctx context.Context, req skillRunRequest, env EnvRequirements) *skillRunResult {
	cmdStr := strings.TrimSpace(env.StartCommand)
	if cmdStr == "" {
		return &skillRunResult{Error: "env.start_command 未配置，无法在沙箱中启动"}
	}
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return &skillRunResult{Error: "start_command 为空"}
	}

	// 工作目录：skill 包目录复制到沙箱临时目录再执行，避免直接执行被清理、也防越界。
	// 旧实现直接传 FilesDir/<id>，会触发 Exec 的越界校验（不在 sandbox_tmp 内）导致 script 类全失败。
	sb := NewLocalSandbox()
	workDir, err := os.MkdirTemp(sb.RootDir, fmt.Sprintf("skill-%d-", req.SkillID))
	if err != nil {
		return &skillRunResult{Error: "创建沙箱工作目录失败: " + err.Error()}
	}
	src := filepath.Join(FilesDir, fmt.Sprintf("%d", req.SkillID))
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		copyDir(src, workDir)
	}

	// 契约强验证断言（F2P/P2P）随执行下发，产物文件尚在时校验
	var checks []VerifyCheck
	if req.Contract != nil {
		checks = allChecks(parseVerification(req.Contract.Verification))
	}

	execReq := ExecRequest{
		Command:  parts[0],
		Args:     parts[1:],
		Stdin:    req.Input,
		WorkDir:  workDir,
		Timeout:  time.Duration(env.TimeoutS) * time.Second,
		MemoryMB: env.MemoryMB,
		Checks:   checks,
	}
	res, err := sb.Exec(ctx, execReq)
	if err != nil {
		return &skillRunResult{Error: err.Error()}
	}
	if res.TimedOut {
		return &skillRunResult{TimedOut: true, Error: "skill 超时"}
	}
	// 组装产出物绝对路径（供合规/安全 Agent 读取）
	artifacts := []string{}
	for _, a := range res.Artifacts {
		artifacts = append(artifacts, filepath.Join(workDir, filepath.FromSlash(a)))
	}
	return &skillRunResult{
		Reply:     strings.TrimSpace(res.Stdout),
		Artifacts: artifacts,
		Checks:    res.CheckResults,
		Error:     res.Stderr,
	}
}

// copyDir 递归复制目录内容（脚本类 Skill 需要把包内脚本/素材带进沙箱工作目录）
func copyDir(src, dst string) {
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == src {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			os.MkdirAll(target, 0o755)
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		os.WriteFile(target, b, 0o644)
		return nil
	})
}

// skillName 取 Skill 名称（供缺省 prompt 使用）
func skillName(skillID int64) string {
	var nm string
	db.QueryRow(`SELECT name FROM skills WHERE id = ?`, skillID).Scan(&nm)
	return nm
}
