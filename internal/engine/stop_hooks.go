package engine

import (
	"context"
	"sort"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// StopHook is the post-turn extension point for runtime policy checks.
// Hooks run after the model/tool phase has produced a candidate terminal state,
// and may either append messages or request one more loop iteration.
type StopHook interface {
	Name() string
	Priority() int
	Execute(ctx context.Context, input StopHookInput) (StopHookResult, error)
}

// StopHookInput is a snapshot of the just-finished turn. We pass copies of the
// message/tool slices to hooks so they can inspect turn state without mutating
// the loop's canonical in-memory history.

// StopHookResult is intentionally small: hooks may add follow-up messages and/or
// force one more continuation cycle, but they do not directly mutate loop state.

type StopHookInput struct {
	SessionID  types.SessionID
	TurnID     types.TurnID
	StopReason string
	Messages   []types.Message
	ToolUses   []types.ToolUseContent
	Usage      *types.TokenUsage
	Iterations int
	Compacted  bool
	// Additional context for richer hook decisions
	RecoveryContext *RecoveryContext
	TotalTurnTokens int
	Model           types.ModelIdentifier
}

type StopHookResult struct {
	Continue bool
	Messages []types.Message
	// Error indicates a hard failure (stops the loop)
	Error error
}

// StopHookConfig controls stop hook execution
type StopHookConfig struct {
	// Timeout in milliseconds, 0 = no timeout
	Timeout int
	// Mode: "first" stops at first Continue, "all" runs all hooks
	Mode string
	// ContinueOnError continues loop if hook errors
	ContinueOnError bool
}

func (l *Loop) runStopHooks(ctx context.Context, state *MutableState, req RunRequest, stopReason string) (StopHookResult, error) {
	if len(l.config.StopHooks) == 0 {
		return StopHookResult{}, nil
	}

	// Priority order is part of the contract: higher-priority hooks see the rawer
	// end-of-turn state first and their messages are appended before lower-priority
	// hook output.

	hooks := append([]StopHook(nil), l.config.StopHooks...)
	sort.SliceStable(hooks, func(i, j int) bool {
		return hooks[i].Priority() > hooks[j].Priority()
	})

	// Determine mode: "first" (stop at first continue) or "all" (run all)
	mode := l.config.StopHookMode
	if mode == "" {
		mode = "first"
	}

	combined := StopHookResult{}
	for _, hook := range hooks {
		result, err := hook.Execute(ctx, StopHookInput{
			SessionID:       req.SessionID,
			TurnID:          req.TurnID,
			StopReason:      stopReason,
			Messages:        append([]types.Message(nil), state.Messages...),
			ToolUses:        append([]types.ToolUseContent(nil), state.ToolUses...),
			Usage:           state.Usage,
			Iterations:      state.Iterations,
			Compacted:       state.Compacted,
			RecoveryContext: state.RecoveryContext,
			TotalTurnTokens: state.TotalTurnTokens,
			Model:           req.Model,
		})

		// Check for hard error
		if err != nil && !l.config.StopHookContinueOnError {
			return StopHookResult{}, err
		}

		if len(result.Messages) > 0 {
			combined.Messages = append(combined.Messages, result.Messages...)
		}

		// In "first" mode, stop at first Continue
		if result.Continue && mode == "first" {
			combined.Continue = true
			break
		}

		// In "all" mode, collect all Continue flags
		if result.Continue {
			combined.Continue = true
		}
	}

	return combined, nil
}
