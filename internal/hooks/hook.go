// Package hooks provides shell-command-based pre-tool hooks, adapted from
// Charm's crush project (MIT). Hooks fire before each tool execution and can:
//   - Allow (exit 0, no JSON output or {"decision":"allow"})
//   - Deny  (exit 2, or {"decision":"deny","reason":"..."})
//   - Halt turn (exit 49, or {"halt":true,"reason":"..."})
//   - Rewrite input (exit 0, {"updatedInput":"...","decision":"allow"})
//   - Inject context (exit 0, {"context":"..."})
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// HaltExitCode is the exit code that halts the entire turn (crush convention).
const HaltExitCode = 49

// HookMetadata is embedded in tool response metadata so the UI can
// display a hook indicator.
type HookMetadata struct {
	HookCount    int        `json:"hook_count"`
	Decision     string     `json:"decision"`
	Halt         bool       `json:"halt,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	InputRewrite bool       `json:"input_rewrite,omitempty"`
	Hooks        []HookInfo `json:"hooks,omitempty"`
}

// HookInfo identifies a single hook that ran and its individual result.
type HookInfo struct {
	Name         string `json:"name"`
	Matcher      string `json:"matcher,omitempty"`
	Decision     string `json:"decision"`
	Halt         bool   `json:"halt,omitempty"`
	Reason       string `json:"reason,omitempty"`
	InputRewrite bool   `json:"input_rewrite,omitempty"`
}

// Decision is the outcome of a hook.
type Decision int

const (
	DecisionNone  Decision = iota // Hook didn't express a preference.
	DecisionAllow                 // Explicitly allows; skips normal permission prompt.
	DecisionDeny                  // Blocks the tool call.
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "none"
	}
}

// HookConfig declares a shell hook. Stored in .nexus.yaml under hooks.pre_tool_use.
type HookConfig struct {
	// Matcher is a regex against the tool name. Empty = match all.
	Matcher string `yaml:"matcher" json:"matcher,omitempty"`
	// Command is the shell command to execute.
	Command string `yaml:"command" json:"command"`
	// Timeout in seconds (default 30).
	Timeout int `yaml:"timeout" json:"timeout,omitempty"`
}

func (h HookConfig) timeout() time.Duration {
	if h.Timeout <= 0 {
		return 30 * time.Second
	}
	return time.Duration(h.Timeout) * time.Second
}

// HookResult is the outcome of one hook run.
type HookResult struct {
	Decision     Decision
	Halt         bool
	Reason       string
	UpdatedInput string // non-empty = rewrite tool input JSON
	Context      string // non-empty = append to tool result
}

// AggregateResult is the merged outcome of all matching hooks.
type AggregateResult struct {
	Decision     Decision
	Halt         bool
	Reason       string
	UpdatedInput string
	Context      string
	HookCount    int
	Hooks        []HookInfo
}

// compiled pairs a HookConfig with its compiled matcher.
type compiled struct {
	cfg     HookConfig
	matcher *regexp.Regexp
}

// Runner executes PreToolUse hooks before each tool call.
type Runner struct {
	hooks      []compiled
	cwd        string
	projectDir string
}

// NewRunner builds a Runner from configs. Hooks with invalid matchers are skipped.
func NewRunner(cfgs []HookConfig, cwd, projectDir string) *Runner {
	hooks := make([]compiled, 0, len(cfgs))
	for _, c := range cfgs {
		h := compiled{cfg: c}
		if c.Matcher != "" {
			re, err := regexp.Compile(c.Matcher)
			if err != nil {
				slog.Warn("hooks: invalid matcher, skipping", "matcher", c.Matcher, "error", err)
				continue
			}
			h.matcher = re
		}
		hooks = append(hooks, h)
	}
	return &Runner{hooks: hooks, cwd: cwd, projectDir: projectDir}
}

// Run executes all matching hooks for the given tool call.
func (r *Runner) Run(ctx context.Context, sessionID, toolName, toolInputJSON string) (AggregateResult, error) {
	matching := r.matching(toolName)
	if len(matching) == 0 {
		return AggregateResult{Decision: DecisionNone}, nil
	}

	// Deduplicate by command.
	seen := make(map[string]bool)
	var unique []HookConfig
	for _, h := range matching {
		if !seen[h.Command] {
			seen[h.Command] = true
			unique = append(unique, h)
		}
	}

	env := buildEnv(sessionID, toolName, r.cwd, r.projectDir, toolInputJSON)
	results := make([]HookResult, len(unique))
	var wg sync.WaitGroup
	wg.Add(len(unique))
	for i, h := range unique {
		go func(idx int, cfg HookConfig) {
			defer wg.Done()
			results[idx] = r.runOne(ctx, cfg, env, toolInputJSON)
		}(i, h)
	}
	wg.Wait()

	agg := aggregate(unique, results, toolInputJSON)
	agg.HookCount = len(unique)
	slog.Info("hooks: pre_tool_use", "tool", toolName, "hooks", len(unique), "decision", agg.Decision.String(), "halt", agg.Halt)
	return agg, nil
}

func (r *Runner) matching(toolName string) []HookConfig {
	var out []HookConfig
	for _, h := range r.hooks {
		if h.matcher == nil || h.matcher.MatchString(toolName) {
			out = append(out, h.cfg)
		}
	}
	return out
}

func (r *Runner) runOne(ctx context.Context, cfg HookConfig, env []string, inputJSON string) HookResult {
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout())
	defer cancel()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}

	cmd := exec.CommandContext(ctx, shell, "-c", cfg.Command)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = r.cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(inputJSON + "\n")

	err := cmd.Run()
	if err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		switch exitCode {
		case 2:
			reason := strings.TrimSpace(stderr.String())
			if reason == "" {
				reason = "blocked by hook"
			}
			return HookResult{Decision: DecisionDeny, Reason: reason}
		case HaltExitCode:
			reason := strings.TrimSpace(stderr.String())
			if reason == "" {
				reason = "turn halted by hook"
			}
			return HookResult{Decision: DecisionDeny, Halt: true, Reason: reason}
		default:
			slog.Warn("hooks: non-blocking error", "command", cfg.Command, "exit", exitCode)
			return HookResult{Decision: DecisionNone}
		}
	}

	return parseStdout(stdout.String())
}

// stdout JSON schema (crush-compatible):
// {"decision":"allow"|"deny","reason":"...","updatedInput":"...","context":"...","halt":true}
type hookOutput struct {
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	UpdatedInput string `json:"updatedInput"`
	Context      string `json:"context"`
	Halt         bool   `json:"halt"`
}

func parseStdout(out string) HookResult {
	out = strings.TrimSpace(out)
	if out == "" {
		return HookResult{Decision: DecisionNone}
	}
	var h hookOutput
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		// Non-JSON stdout from a hook means context (pass-through).
		return HookResult{Decision: DecisionNone, Context: out}
	}
	result := HookResult{
		Halt:         h.Halt,
		Reason:       h.Reason,
		UpdatedInput: h.UpdatedInput,
		Context:      h.Context,
	}
	switch strings.ToLower(h.Decision) {
	case "allow":
		result.Decision = DecisionAllow
	case "deny":
		result.Decision = DecisionDeny
	default:
		result.Decision = DecisionNone
	}
	if h.Halt {
		result.Decision = DecisionDeny
	}
	return result
}

func aggregate(configs []HookConfig, results []HookResult, inputJSON string) AggregateResult {
	agg := AggregateResult{Decision: DecisionNone}
	for i, r := range results {
		cfg := configs[i]
		info := HookInfo{
			Name:         cfg.Command,
			Matcher:      cfg.Matcher,
			Decision:     r.Decision.String(),
			Halt:         r.Halt,
			Reason:       r.Reason,
			InputRewrite: r.UpdatedInput != "",
		}
		agg.Hooks = append(agg.Hooks, info)

		if r.Halt {
			agg.Halt = true
			agg.Decision = DecisionDeny
			if r.Reason != "" {
				agg.Reason = r.Reason
			}
		}
		if r.Decision == DecisionDeny && agg.Decision != DecisionDeny {
			agg.Decision = DecisionDeny
			if r.Reason != "" {
				agg.Reason = r.Reason
			}
		}
		if r.Decision == DecisionAllow && agg.Decision == DecisionNone {
			agg.Decision = DecisionAllow
		}
		if r.UpdatedInput != "" {
			agg.UpdatedInput = r.UpdatedInput
		}
		if r.Context != "" {
			if agg.Context != "" {
				agg.Context += "\n"
			}
			agg.Context += r.Context
		}
	}
	return agg
}

func buildEnv(sessionID, toolName, cwd, projectDir, inputJSON string) []string {
	return []string{
		"NEXUS_HOOK_EVENT=pre_tool_use",
		fmt.Sprintf("NEXUS_TOOL_NAME=%s", toolName),
		fmt.Sprintf("NEXUS_SESSION_ID=%s", sessionID),
		fmt.Sprintf("NEXUS_CWD=%s", cwd),
		fmt.Sprintf("NEXUS_PROJECT_DIR=%s", projectDir),
		fmt.Sprintf("NEXUS_TOOL_INPUT=%s", inputJSON),
	}
}
