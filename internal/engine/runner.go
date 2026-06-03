package engine

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/hooks"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Runner executes a single turn through the canonical query loop.
type Runner struct {
	loop   *Loop
	config *RunnerConfig
}

// RunnerConfig represents the runner configuration.
type RunnerConfig struct {
	MaxIterations int  `json:"max_iterations"`
	AutoCompact   bool `json:"auto_compact"`
	MaxTurns      int  `json:"max_turns"`
}

// DefaultRunnerConfig returns default runner configuration.
func DefaultRunnerConfig() *RunnerConfig {
	return &RunnerConfig{
		MaxIterations: 10,
		AutoCompact:   true,
		MaxTurns:      100,
	}
}

// NewRunner creates a new turn runner.
func NewRunner(
	apiClient *providers.Client,
	orchestrator *execution.Orchestrator,
	compactor *compact.Engine,
	promptAssembler *prompt.Assembler,
	permissionIntegrator *permissions.Integrator,
	config *RunnerConfig,
) *Runner {
	if config == nil {
		config = DefaultRunnerConfig()
	}

	// Create a no-op hook executor for runner (hooks are managed at Engine level)
	hookExecutor := hooks.NewExecutor(hooks.NewRegistry())

	loop := NewLoop(
		apiClient,
		orchestrator,
		compactor,
		promptAssembler,
		permissionIntegrator,
		hookExecutor,
		&LoopConfig{
			MaxIterations:                config.MaxIterations,
			AutoCompact:                  config.AutoCompact,
			MaxTurns:                     config.MaxTurns,
			EnableStreaming:              true,
			MaxOutputTokensRecoveryLimit: DefaultLoopConfig().MaxOutputTokensRecoveryLimit,
			ContinuationNudgeLimit:       DefaultLoopConfig().ContinuationNudgeLimit,
		},
		nil, // monitoring
	)

	return &Runner{
		loop:   loop,
		config: config,
	}
}

// RunnerRequest represents a request to run a turn through the runner facade.
type RunnerRequest struct {
	Messages         []types.Message            `json:"messages"`
	SystemPrompt     string                     `json:"system_prompt"`
	Tools            map[string]tool.Tool       `json:"-"`
	SessionID        types.SessionID            `json:"session_id"`
	TurnID           types.TurnID               `json:"turn_id"`
	PermissionMode   types.PermissionMode       `json:"permission_mode"`
	PermissionCheck  types.CanUseToolFn         `json:"-"`
	DenialTracking   *types.DenialTrackingState `json:"-"`
	Model            types.ModelIdentifier      `json:"model"`
	MaxTokens        int                        `json:"max_tokens"`
	ProgressCallback func(types.ToolProgress)   `json:"-"`
	PromptVariables  map[string]string          `json:"prompt_variables,omitempty"`
}

// RunnerResult represents the result of running a turn through the runner facade.
type RunnerResult struct {
	Messages    []types.Message        `json:"messages"`
	StopReason  string                 `json:"stop_reason"`
	ToolUses    []types.ToolUseContent `json:"tool_uses"`
	ToolResults []tool.CallResult      `json:"tool_results"`
	Usage       *types.TokenUsage      `json:"usage"`
	Compacted   bool                   `json:"compacted"`
	Iterations  int                    `json:"iterations"`
}

// Run executes a single turn by delegating to the canonical query loop.
func (r *Runner) Run(ctx context.Context, req RunnerRequest) (RunnerResult, error) {
	result := r.loop.Run(ctx, RunRequest{
		Messages:         req.Messages,
		SystemPrompt:     req.SystemPrompt,
		Tools:            req.Tools,
		SessionID:        req.SessionID,
		TurnID:           req.TurnID,
		PermissionMode:   req.PermissionMode,
		PermissionCheck:  req.PermissionCheck,
		DenialTracking:   req.DenialTracking,
		Model:            req.Model,
		MaxTokens:        req.MaxTokens,
		ProgressCallback: req.ProgressCallback,
	})
	if result.Error != nil {
		return RunnerResult{}, result.Error
	}

	return RunnerResult{
		Messages:    result.Messages,
		StopReason:  result.StopReason,
		ToolUses:    result.ToolUses,
		ToolResults: result.ToolResults,
		Usage:       result.Usage,
		Compacted:   result.Compacted,
		Iterations:  result.Iterations,
	}, nil
}
