package bash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/sandbox"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

const (
	// DefaultTimeout is the default command timeout.
	DefaultTimeout = 30 * time.Second

	// MaxTimeout is the maximum allowed timeout.
	MaxTimeout = 5 * time.Minute

	// MaxOutputSize is the default maximum captured output size (100 KB).
	MaxOutputSize = 100 * 1024

	// AbsoluteMaxOutputSize is the hard ceiling for output_bytes_cap (10 MB).
	AbsoluteMaxOutputSize = 10 * 1024 * 1024

	// headOutputFraction is the share of MaxOutputSize kept from the beginning
	// of the output when it exceeds the cap. The tail gets the remainder.
	// e.g. 0.4 → first 40 KB preserved, last 60 KB preserved.
	headOutputFraction = 0.4
)

// Tool represents the Bash tool.
type Tool struct {
	config            *ToolConfig
	securityValidator *SecurityValidator
	commandPolicy     *sandbox.CommandPolicy
	backgroundManager *BackgroundTaskManager
	shell             string // cached at construction time
	mu                sync.Mutex
}

// ToolConfig represents the Bash tool configuration.
type ToolConfig struct {
	Timeout          time.Duration
	MaxOutputSize    int64
	WorkingDirectory string
	EnableSandbox    bool
	// RequireSandbox makes the tool refuse to run when the workspace boundary is
	// set but Landlock is unavailable. Leave false for desktop use; set true for
	// multi-tenant server deployments.
	RequireSandbox bool
	// OutputChunkCallback, when set, receives each output chunk as it is produced
	// (streaming mode). If nil, output is buffered and returned at completion.
	OutputChunkCallback func(chunk string, stream string)
}

// DefaultToolConfig returns a sensible default configuration.
func DefaultToolConfig() *ToolConfig {
	cwd, _ := os.Getwd()
	return &ToolConfig{
		Timeout:          DefaultTimeout,
		MaxOutputSize:    MaxOutputSize,
		WorkingDirectory: cwd,
		EnableSandbox:    false,
	}
}

// NewTool creates a new Bash tool.
func NewTool(config *ToolConfig) *Tool {
	if config == nil {
		config = DefaultToolConfig()
	}

	bm := NewBackgroundTaskManager(runtimepath.BashTasksDir(""))
	_ = bm.Init()
	globalTaskManager = bm // expose to package-level callers (monitor, task-list tools)

	return &Tool{
		config:            config,
		securityValidator: NewSecurityValidator(),
		commandPolicy:     sandbox.NewDefaultCommandPolicy(),
		backgroundManager: bm,
		shell:             detectShell(),
	}
}

// Definition returns the tool definition.
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Bash",
		SearchHint:  SearchHint,
		Description: Description,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in seconds (default: 30, max: 300)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Short description of what the command does",
				},
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": "Run the command in the background and return immediately",
				},
				"stdin": map[string]any{
					"type":        "string",
					"description": "Optional data to pipe into the command's stdin",
				},
				"output_bytes_cap": map[string]any{
					"type":        "number",
					"description": "Maximum bytes to capture from output (default: 102400, max: 10485760)",
				},
			},
			"required": []string{"command"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run", "skill_agent"}},
	}
}

// Call executes the tool.
func (t *Tool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	directCall := input.ToolContext == nil
	toolCtx := input.ToolContextValue()
	workingDirectory := t.effectiveWorkingDirectory(toolCtx)

	command, ok := input.Parsed["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return tool.NewErrorResult(fmt.Errorf("command is required and must be a string")), nil
	}

	timeout := t.config.Timeout
	if timeoutSec, ok := input.Parsed["timeout"].(float64); ok && timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
		if timeout > MaxTimeout {
			timeout = MaxTimeout
		}
	}

	description, _ := input.Parsed["description"].(string)
	runInBackground, _ := input.Parsed["run_in_background"].(bool)
	stdinData, _ := input.Parsed["stdin"].(string)

	outputBytesCap := t.config.MaxOutputSize
	if capVal, ok := input.Parsed["output_bytes_cap"].(float64); ok && capVal > 0 {
		outputBytesCap = int64(capVal)
		if outputBytesCap > AbsoluteMaxOutputSize {
			outputBytesCap = AbsoluteMaxOutputSize
		}
	}

	// Intercept apply_patch — must use the dedicated tool.
	if isApplyPatchInvocation(command) {
		return tool.NewErrorResult(fmt.Errorf(
			"apply_patch must be invoked via the apply_patch tool, not via bash — " +
				"use the apply_patch tool to get structured permission checks and approval")), nil
	}

	// ── Layer 1: Structural security (always, before any permission logic) ──
	if viol := t.securityValidator.ValidateCommand(command); viol != nil {
		return tool.NewErrorResult(fmt.Errorf("security violation: %s", viol.Reason)), nil
	}
	if viol := t.securityValidator.ValidateGitCommand(command); viol != nil {
		return tool.NewErrorResult(fmt.Errorf("git security violation: %s", viol.Reason)), nil
	}
	if viol := t.securityValidator.ValidateWorkspace(command, workingDirectory); viol != nil {
		return tool.NewErrorResult(fmt.Errorf("workspace violation: %s", viol.Reason)), nil
	}

	// ── Layer 2: Policy hard-deny (catastrophic commands, deny fragments) ──
	policyResult := t.commandPolicy.Evaluate(command)
	if policyResult.Decision == sandbox.DecisionDeny {
		return tool.NewErrorResult(fmt.Errorf("permission denied: %s", policyResult.Reason)), nil
	}

	execCtx := &ExecutionContext{
		Command:             command,
		Description:         description,
		Timeout:             timeout,
		WorkingDirectory:    workingDirectory,
		MaxOutputSize:       outputBytesCap,
		EnableSandbox:       t.config.EnableSandbox,
		RunInBackground:     runInBackground,
		Stdin:               stdinData,
		OutputChunkCallback: t.config.OutputChunkCallback,
	}

	// ── Layer 3: Approval pipeline (direct calls only) ────────────────────
	if directCall {
		// Evaluate already bakes in IsKnownSafe: DecisionAllow means the command
		// is either on the explicit allowlist or is a pure read/state-change command.
		// Both cases bypass the global approval pipeline entirely.
		// Mirrors Codex's ExecApprovalRequirement::Skip.
		skipApproval := policyResult.Decision == sandbox.DecisionAllow

		// Mode-specific fast-path (e.g. AcceptEdits allows certain writes).
		if !skipApproval {
			modeResult := checkPermissionsMode(command, toolCtx.PermissionMode, workingDirectory)
			switch modeResult.Behavior {
			case types.PermissionBehaviorDeny:
				return tool.NewErrorResult(fmt.Errorf("permission denied: %s", modeResult.Reason)), nil
			case types.PermissionBehaviorAllow:
				skipApproval = true
			}
		}

		if !skipApproval {
			if permissionCheck == nil {
				reason := policyResult.Reason
				if reason == "" {
					reason = "command requires approval"
				}
				return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
			}
			req := sandbox.PermissionRequest{
				ToolName:      "bash",
				Description:   description,
				Environment:   sandbox.EnvironmentLocal,
				Access:        sandbox.AccessExecute,
				Command:       command,
				Justification: description,
				Scope:         sandbox.ApprovalScopeToolCall,
				Metadata: map[string]any{
					"timeout_seconds":   timeout.Seconds(),
					"run_in_background": runInBackground,
				},
			}
			permissionResult, err := sandbox.ResolveToolPermission(ctx, permissionCheck, req, sandbox.ToolPermissionOptions{
				ToolInput: map[string]any{
					"command":           command,
					"timeout":           timeout.Seconds(),
					"description":       description,
					"run_in_background": runInBackground,
				},
				ToolUseID:              toolCtx.ToolUseID,
				SessionID:              toolCtx.SessionID,
				TurnID:                 toolCtx.TurnID,
				PermissionMode:         toolCtx.PermissionMode,
				WorkingDirectory:       execCtx.WorkingDirectory,
				IsToolRunningInSandbox: execCtx.EnableSandbox,
			})
			if err != nil {
				return tool.NewErrorResult(err), nil
			}
			if err := sandbox.ErrorForPermissionResult(permissionResult, "command requires approval"); err != nil {
				return tool.NewErrorResult(err), nil
			}
		}
	}

	// ── Execution ──────────────────────────────────────────────────────────
	if execCtx.RunInBackground || IsBackgroundCommand(command) {
		return t.executeBackground(ctx, execCtx)
	}

	result, err := t.executeCommand(ctx, execCtx)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("command execution failed: %w", err)), nil
	}

	content := t.formatOutput(execCtx, result)
	callResult := tool.NewTextResult(content)
	callResult.Metadata = &tool.ResultMetadata{
		ExecutionDuration: result.Duration,
		Additional: map[string]any{
			"exit_code":         result.ExitCode,
			"stdout":            result.Stdout,
			"stderr":            result.Stderr,
			"timeout":           result.Timeout,
			"cwd":               result.CWD,
			"description":       execCtx.Description,
			"run_in_background": execCtx.RunInBackground,
			"sandboxed":         result.Sandboxed,
		},
	}
	return callResult, nil
}

// Description returns a human-readable description.
func (t *Tool) Description(_ context.Context) (string, error) {
	return "Execute shell commands with security validation, permission checks, foreground and background execution", nil
}

// ValidateInput validates and normalises bash input.
func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	command, ok := input["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is required and must be a string")
	}

	normalized := make(map[string]any, len(input))
	for k, v := range input {
		normalized[k] = v
	}
	if timeoutSec, ok := normalized["timeout"].(float64); ok && timeoutSec > 0 {
		if timeout := time.Duration(timeoutSec) * time.Second; timeout > MaxTimeout {
			normalized["timeout"] = MaxTimeout.Seconds()
		}
	}
	return normalized, nil
}

// CheckPermissions performs bash-specific permission checks before the global pipeline.
func (t *Tool) CheckPermissions(_ context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	command, _ := input["command"].(string)
	if strings.TrimSpace(command) == "" {
		return types.Deny("command is required and must be a string")
	}

	// Intercept apply_patch invocations: the model must use the dedicated
	// apply_patch tool. Running it through bash bypasses the structured
	// permission and approval pipeline.
	if isApplyPatchInvocation(command) {
		return types.Deny("Use the apply_patch tool directly — do not call apply_patch via bash. " +
			"The dedicated tool provides structured permission checks, approval previews, and progress events.")
	}

	workingDir := t.effectiveWorkingDirectory(toolCtx)

	// Structural security checks.
	if viol := t.securityValidator.ValidateCommand(command); viol != nil {
		return types.Deny(fmt.Sprintf("security violation: %s", viol.Reason))
	}
	if viol := t.securityValidator.ValidateGitCommand(command); viol != nil {
		return types.Deny(fmt.Sprintf("git security violation: %s", viol.Reason))
	}
	if viol := t.securityValidator.ValidateWorkspace(command, workingDir); viol != nil {
		return types.Deny(fmt.Sprintf("workspace violation: %s", viol.Reason))
	}

	// Policy hard-deny.
	policyResult := t.commandPolicy.Evaluate(command)
	if policyResult.Decision == sandbox.DecisionDeny {
		return types.Deny(policyResult.Reason)
	}

	// Mode-specific fast-paths (AcceptEdits, etc.).
	modeResult := checkPermissionsMode(command, toolCtx.PermissionMode, workingDir)
	if modeResult.Behavior == types.PermissionBehaviorDeny || modeResult.Behavior == types.PermissionBehaviorAllow {
		return modeResult
	}

	// DecisionAllow: known-safe or pure read/state-change — no approval needed.
	// DecisionAsk: needs human decision from the global pipeline.
	// (Evaluate already bakes in IsKnownSafe, so no redundant call needed here.)
	switch policyResult.Decision {
	case sandbox.DecisionAllow:
		return types.Passthrough(input)
	case sandbox.DecisionAsk:
		return types.Ask(policyResult.Reason)
	}
	return types.Passthrough(input)
}

// IsConcurrencySafe returns whether this tool use can run concurrently.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	command, _ := input["command"].(string)
	cl := t.securityValidator.GetCommandClassification(command)
	return cl == ClassificationReadOnly || cl == ClassificationSearch
}

// IsReadOnly returns whether this tool use is read-only.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	return t.IsConcurrencySafe(input)
}

// IsEnabled returns whether this tool is currently active.
func (t *Tool) IsEnabled() bool { return true }

// FormatResult serialises the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches the input — no-op for bash.
func (t *Tool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// PreparePermissionMatcher compiles content-specific permission matching for bash rules.
func (t *Tool) PreparePermissionMatcher(_ context.Context, input map[string]any) (func(ruleContent string) bool, error) {
	command := strings.TrimSpace(fmt.Sprintf("%v", input["command"]))
	if command == "" {
		return nil, nil
	}
	return func(ruleContent string) bool {
		return permissionRuleContentMatches(ruleContent, command)
	}, nil
}

// ── Internal types ─────────────────────────────────────────────────────────────

// ExecutionContext holds the parameters for a single command execution.
type ExecutionContext struct {
	Command          string
	Description      string
	Timeout          time.Duration
	WorkingDirectory string
	MaxOutputSize    int64
	EnableSandbox    bool
	RunInBackground  bool
	// Stdin, when non-empty, is piped into the command's stdin.
	Stdin string
	// OutputChunkCallback, when set, is called for each output chunk (streaming).
	// stream is "stdout" or "stderr".
	OutputChunkCallback func(chunk string, stream string)
}

// CommandResult holds the output of a completed command.
type CommandResult struct {
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Timeout   bool   `json:"timeout"`
	Duration  int64  `json:"duration_ms"`
	CWD       string `json:"cwd,omitempty"`
	Sandboxed bool   `json:"sandboxed"`
}

// ── Execution ──────────────────────────────────────────────────────────────────

func (t *Tool) executeBackground(ctx context.Context, execCtx *ExecutionContext) (tool.CallResult, error) {
	// Strip the trailing & if present — it's a shell-level signal, not part of the command.
	command := strings.TrimSpace(execCtx.Command)
	if strings.HasSuffix(command, "&") {
		command = strings.TrimSpace(command[:len(command)-1])
	}

	_, actualCommand := parseEnvVars(command)
	task, err := t.backgroundManager.StartBackgroundTask(
		ctx,
		actualCommand,
		execCtx.WorkingDirectory,
		t.buildEnvironment(nil),
		t.shell,
	)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to start background task: %w", err)), nil
	}

	label := execCtx.Description
	if label == "" {
		label = actualCommand
	}
	content := fmt.Sprintf(
		"Background task started\nDescription: %s\nCommand: %s\nTask ID: %s\nWorking directory: %s",
		label, task.Command, task.ID, execCtx.WorkingDirectory,
	)

	result := tool.NewTextResult(content)
	result.Metadata = &tool.ResultMetadata{
		Additional: map[string]any{
			"task_id":     task.ID,
			"command":     task.Command,
			"description": execCtx.Description,
			"background":  true,
			"output_path": task.Output.Path,
			"working_dir": execCtx.WorkingDirectory,
		},
	}
	return result, nil
}

func (t *Tool) executeCommand(ctx context.Context, execCtx *ExecutionContext) (*CommandResult, error) {
	startTime := time.Now()
	cmdCtx, cancel := context.WithTimeout(ctx, execCtx.Timeout)
	defer cancel()

	_, actualCommand := parseEnvVars(execCtx.Command)
	cmdPath, cmdArgs, sandboxEnv, sandboxed := commandWithLandlock(t.shell, []string{"-c", actualCommand}, execCtx.WorkingDirectory)

	if execCtx.WorkingDirectory != "" && !sandboxed && t.config != nil && t.config.RequireSandbox {
		return nil, fmt.Errorf("sandbox required but unavailable on this host: refusing to run the command unconfined")
	}

	cmd := exec.CommandContext(cmdCtx, cmdPath, cmdArgs...)
	if execCtx.WorkingDirectory != "" {
		cmd.Dir = execCtx.WorkingDirectory
	}
	cmd.Env = append(t.buildEnvironment(nil), sandboxEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Wire up stdin if provided; otherwise leave it nil (closed).
	if execCtx.Stdin != "" {
		cmd.Stdin = strings.NewReader(execCtx.Stdin)
	}

	// Kill the whole process group on timeout/cancel to prevent leaked children.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second

	maxOut := execCtx.MaxOutputSize
	if maxOut <= 0 {
		maxOut = MaxOutputSize
	}

	stdout, stderr, exitCode, timedOut := t.runCommand(cmdCtx, cmd, maxOut, execCtx.OutputChunkCallback)
	return &CommandResult{
		ExitCode:  exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
		Timeout:   timedOut,
		Duration:  time.Since(startTime).Milliseconds(),
		CWD:       execCtx.WorkingDirectory,
		Sandboxed: sandboxed,
	}, nil
}

func (t *Tool) runCommand(ctx context.Context, cmd *exec.Cmd, maxOutputSize int64, chunkCb func(string, string)) (stdout, stderr string, exitCode int, timeout bool) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Sprintf("failed to capture stdout: %v", err), 1, false
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Sprintf("failed to capture stderr: %v", err), 1, false
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Sprintf("failed to start command: %v", err), 1, false
	}

	stdoutBuf := newCappedBuffer(maxOutputSize)
	stderrBuf := newCappedBuffer(maxOutputSize)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyWithCallback(stdoutBuf, stdoutPipe, "stdout", chunkCb)
	}()
	go func() {
		defer wg.Done()
		copyWithCallback(stderrBuf, stderrPipe, "stderr", chunkCb)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return stdoutBuf.String(), stderrBuf.String(), 124, true
	case errors.Is(ctx.Err(), context.Canceled):
		return stdoutBuf.String(), stderrBuf.String(), 130, false
	case waitErr != nil:
		if exitError, ok := waitErr.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return stdoutBuf.String(), stderrBuf.String(), status.ExitStatus(), false
			}
		}
		return stdoutBuf.String(), stderrBuf.String(), 1, false
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, false
}

// copyWithCallback copies src to dst while calling chunkCb for each read chunk.
// When chunkCb is nil it degrades to a plain io.Copy.
func copyWithCallback(dst io.Writer, src io.Reader, stream string, chunkCb func(string, string)) {
	const chunkSize = 8192
	buf := make([]byte, chunkSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			dst.Write([]byte(chunk)) //nolint:errcheck
			if chunkCb != nil {
				chunkCb(chunk, stream)
			}
		}
		if err != nil {
			break
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func (t *Tool) effectiveWorkingDirectory(toolCtx tool.ToolUseContext) string {
	if d := strings.TrimSpace(toolCtx.WorkingDirectory); d != "" {
		return d
	}
	if t != nil && t.config != nil {
		if d := strings.TrimSpace(t.config.WorkingDirectory); d != "" {
			return d
		}
	}
	return "."
}

func (t *Tool) buildEnvironment(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func (t *Tool) formatOutput(execCtx *ExecutionContext, result *CommandResult) string {
	var b strings.Builder

	if execCtx.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", execCtx.Description)
	}
	fmt.Fprintf(&b, "Command: %s\n", execCtx.Command)
	fmt.Fprintf(&b, "Exit code: %d | Duration: %dms", result.ExitCode, result.Duration)
	if result.Timeout {
		b.WriteString(" | TIMEOUT")
	}
	if result.Sandboxed {
		b.WriteString(" | sandboxed")
	}
	b.WriteString("\n")

	if result.CWD != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", result.CWD)
	}
	if result.Stdout != "" {
		b.WriteString("\nOutput:\n")
		b.WriteString(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			b.WriteString("\n")
		}
	}
	if result.Stderr != "" {
		b.WriteString("\nErrors:\n")
		b.WriteString(result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// IsBackgroundCommand checks if command ends with & (background operator).
func IsBackgroundCommand(command string) bool {
	return strings.HasSuffix(strings.TrimSpace(command), "&")
}

// isApplyPatchInvocation returns true when the command invokes apply_patch or
// applypatch as a bare shell command or as a segment inside a composed command.
// Codex intercepts these at the exec/sandbox level; Nexus denies them at the
// tool layer and requires the model to use the dedicated apply_patch tool.
func isApplyPatchInvocation(command string) bool {
	if applyPatchCommandName(command) {
		return true
	}
	p := NewParser()
	parsed := p.ParseCommand(command)
	for _, seg := range parsed.Segments {
		if applyPatchCommandName(seg.Text) {
			return true
		}
	}
	return false
}

// applyPatchCommandName checks whether the first token of s is apply_patch / applypatch.
func applyPatchCommandName(s string) bool {
	_, actual := parseEnvVars(strings.TrimSpace(s))
	fields := strings.Fields(actual)
	if len(fields) == 0 {
		return false
	}
	name := fields[0]
	// strip any path prefix: /usr/local/bin/apply_patch → apply_patch
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = name[idx+1:]
	}
	return name == "apply_patch" || name == "applypatch"
}

// detectShell picks the best available shell, cached at startup.
func detectShell() string {
	for _, sh := range []string{"bash", "sh", "zsh"} {
		if _, err := exec.LookPath(sh); err == nil {
			return sh
		}
	}
	return "sh"
}

// ── cappedBuffer — head + tail biased truncation ───────────────────────────────

// cappedBuffer captures up to `max` bytes of output from a command, keeping
// the first headOutputFraction*max bytes and the last remaining bytes, with a
// clear marker between them. This preserves both the startup output (context)
// and the final output (result) of a long-running command.
type cappedBuffer struct {
	max  int64
	head bytes.Buffer // first headSize bytes
	tail bytes.Buffer // ring buffer for the last tailSize bytes
	// tailRing is a pre-allocated ring buffer of size tailSize.
	tailRing []byte
	tailPos  int  // write position in ring
	tailFull bool // ring has been filled at least once
	headFull bool // head is full
	headSize int64
	tailSize int64
	total    int64 // total bytes received
}

func newCappedBuffer(max int64) *cappedBuffer {
	if max <= 0 {
		max = MaxOutputSize
	}
	headSize := int64(float64(max) * headOutputFraction)
	tailSize := max - headSize
	return &cappedBuffer{
		max:      max,
		headSize: headSize,
		tailSize: tailSize,
		tailRing: make([]byte, tailSize),
	}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	b.total += int64(n)

	for _, c := range p {
		if !b.headFull {
			b.head.WriteByte(c)
			if int64(b.head.Len()) >= b.headSize {
				b.headFull = true
			}
			continue
		}
		// Write into the ring buffer for the tail.
		b.tailRing[b.tailPos] = c
		b.tailPos++
		if b.tailPos >= len(b.tailRing) {
			b.tailPos = 0
			b.tailFull = true
		}
	}
	return n, nil
}

func (b *cappedBuffer) String() string {
	if !b.headFull {
		// Everything fits in head — no truncation.
		return b.head.String()
	}

	var out strings.Builder
	out.Write(b.head.Bytes())

	// Reconstruct tail from ring buffer.
	var tail []byte
	if b.tailFull {
		tail = append(b.tailRing[b.tailPos:], b.tailRing[:b.tailPos]...)
	} else {
		tail = b.tailRing[:b.tailPos]
	}

	omitted := b.total - b.headSize - int64(len(tail))
	if omitted > 0 {
		fmt.Fprintf(&out, "\n... (%d bytes omitted) ...\n", omitted)
	}
	out.Write(tail)
	return out.String()
}

// ── Tool metadata ──────────────────────────────────────────────────────────────

// ToolName is the tool name.
const ToolName = "bash"

// Description is the tool description.
const Description = `Execute shell commands in the current directory. Supports explicit background execution and basic execution metadata.

Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile (bash or zsh).

Important: avoid using this tool to run find, grep, cat, head, tail, sed, awk, or echo commands unless explicitly instructed or after you have verified that a dedicated tool cannot accomplish your task. Prefer the dedicated tools instead:
- File search: use Glob (not find or ls)
- Content search: use Grep (not grep or rg)
- Read files: use FileRead (not cat/head/tail)
- Edit files: use FileEdit (not sed/awk)
- Write files: use FileWrite (not echo >/cat <<EOF)
- Communication: output text directly (not echo/printf)

Instructions:
- If your command will create new directories or files, first use this tool to run ls to verify the parent directory exists and is the correct location.
- Always quote file paths that contain spaces with double quotes in your command.
- Try to maintain your current working directory throughout the session by using absolute paths and avoiding cd. You may use cd if the user explicitly requests it.
- You may specify an optional timeout in milliseconds (up to 300000ms / 5 minutes). By default, your command times out after 30000ms.
- You can use run_in_background to run the command in the background and return immediately.

When issuing multiple commands:
- If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message.
- If the commands depend on each other and must run sequentially, use a single Bash call with && to chain them together.
- Use ; only when you need to run commands sequentially but do not care if earlier commands fail.
- Do not use newlines to separate commands.

Git safety:
- Prefer to create a new commit rather than amending an existing commit.
- Before running destructive operations, consider whether there is a safer alternative.
- Never skip hooks or bypass signing unless the user explicitly asks.
- Only create commits when requested by the user.

Avoid unnecessary sleep commands:
- Do not sleep between commands that can run immediately.
- If your command is long running and you want notification when it finishes, use run_in_background.
- Do not retry failing commands in a sleep loop - diagnose the root cause.
- If you must sleep, keep the duration short (1-5 seconds).

Use the gh command via Bash for GitHub-related tasks such as issues, pull requests, checks, and releases.`

// SearchHint is a hint for tool search functionality.
const SearchHint = "run shell commands in the terminal"
