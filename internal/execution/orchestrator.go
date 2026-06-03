package execution

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	runtimehooks "github.com/EngineerProjects/nexus-engine/internal/runtime/hooks"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Orchestrator
// ---------------------------------------------------------------------------

// Orchestrator manages parallel tool execution.
// Pipeline per tool use (aligned with OpenClaude's runToolUse):
//
//  1. resolve tool + IsEnabled check
//  2. ValidateInput
//  3. BackfillInput (observable enrichment for hooks/permissions)
//  4. Pre-tool hooks
//  5. Safety checks (bypass-immune)
//  6. Permission pipeline (deny rules → local check → always-allow → global check)
//  7. Denial tracking (auto mode)
//  8. tool.Call()
//  9. Post-tool hooks
//  10. FormatResult (tool-controlled serialisation)
//  11. Content replacement (max result size)
//  12. Context modifier (serial only, or batch-ordered after concurrent)
type Orchestrator struct {
	maxConcurrency int
	hooks          *runtimehooks.Registry

	safetyChecker     types.SafetyChecker
	denialLimitConfig types.DenialLimitConfig
	monitoring        *monitoring.System
}

// NewOrchestrator creates a new tool execution orchestrator.
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		maxConcurrency:    10,
		hooks:             runtimehooks.NewRegistry(),
		safetyChecker:     types.NewDangerousPatternChecker(),
		denialLimitConfig: types.DefaultDenialLimitConfig(),
	}
}

// SetMaxConcurrency sets the maximum concurrency.
func (o *Orchestrator) SetMaxConcurrency(max int) { o.maxConcurrency = max }

// SetMonitoring sets the monitoring system
func (o *Orchestrator) SetMonitoring(monitoringSys *monitoring.System) {
	o.monitoring = monitoringSys
}

// SetSafetyChecker sets the bypass-immune safety checker.
func (o *Orchestrator) SetSafetyChecker(checker types.SafetyChecker) {
	o.safetyChecker = checker
}

// SetDenialLimitConfig configures denial-based fallback behavior.
func (o *Orchestrator) SetDenialLimitConfig(config types.DenialLimitConfig) {
	o.denialLimitConfig = config
}

// AddHook registers a pre- or post-tool-use hook.
func (o *Orchestrator) AddHook(hook ToolHook) {
	o.hooks.Add(hook)
}

// ---------------------------------------------------------------------------
// Execute — top-level entry point
// ---------------------------------------------------------------------------

// Execute executes tools with proper parallelization.
func (o *Orchestrator) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	startTime := time.Now()
	req = normalizeExecuteRequest(req)

	// Record tool calls in monitoring
	if o.monitoring != nil {
		for _, toolUse := range req.ToolUses {
			o.monitoring.RecordToolCall(toolUse.Name)
		}
	}

	results := make([]tool.CallResult, len(req.ToolUses))
	messagesByIndex := make([][]types.Message, len(req.ToolUses))
	tracesByIndex := make([]ToolExecutionTrace, len(req.ToolUses))
	errors := make([]ExecutionError, 0)
	allProgress := make([]types.ToolProgress, 0)
	currentContext := o.newToolContext(req)

	preparedToolUses := o.prepareToolUses(ctx, req)
	batches := o.partitionPreparedToolUses(preparedToolUses)
	for _, batch := range batches {
		var outcomes []toolExecutionOutcome
		if batch.IsConcurrencySafe {
			outcomes = o.executeConcurrentBatch(ctx, batch, req, currentContext)
			sort.SliceStable(outcomes, func(i, j int) bool {
				return outcomes[i].Index < outcomes[j].Index
			})
			for _, outcome := range outcomes {
				currentContext = o.applyContextModifier(currentContext, outcome.Result)
			}
		} else {
			outcomes = o.executeSequentialBatch(ctx, batch, req, &currentContext)
		}

		for _, outcome := range outcomes {
			results[outcome.Index] = outcome.Result
			messagesByIndex[outcome.Index] = o.buildToolResultMessages(outcome.ToolUse, outcome.Result, req.TurnID)
			messagesByIndex[outcome.Index] = append(messagesByIndex[outcome.Index], outcome.Messages...)
			tracesByIndex[outcome.Index] = cloneTrace(outcome.Trace)
			allProgress = append(allProgress, outcome.Progress...)
			if req.ProgressCallback != nil {
				for _, progress := range outcome.Progress {
					req.ProgressCallback(progress)
				}
			}

			// Record tool execution results in monitoring
			if o.monitoring != nil {
				if outcome.Error != nil {
					o.monitoring.RecordToolFailure(outcome.ToolUse.Name, outcome.Error, 0) // Duration not available here
				} else {
					o.monitoring.RecordToolSuccess(outcome.ToolUse.Name, 0) // Duration not available here
				}
			}

			if outcome.Error != nil {
				errors = append(errors, ExecutionError{
					ToolUseID: outcome.ToolUse.ID,
					ToolName:  outcome.ToolUse.Name,
					Error:     outcome.Error,
					Stage:     outcome.ErrorStage,
				})
			}
		}

		if ctx.Err() != nil {
			return ExecuteResult{}, fmt.Errorf("execution cancelled: %w", ctx.Err())
		}
	}

	messages := make([]types.Message, 0, len(req.ToolUses))
	for _, batchMessages := range messagesByIndex {
		messages = append(messages, batchMessages...)
	}

	traces := make([]ToolExecutionTrace, 0, len(tracesByIndex))
	for _, trace := range tracesByIndex {
		traces = append(traces, cloneTrace(trace))
	}

	return ExecuteResult{
		Results:          results,
		Messages:         messages,
		Traces:           traces,
		Errors:           errors,
		TotalDuration:    time.Since(startTime),
		ProgressUpdates:  allProgress,
		FinalToolContext: currentContext,
	}, nil
}
