package execution

import (
	"context"
	"fmt"
	"sync"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
)

func (o *Orchestrator) executeConcurrentBatch(
	ctx context.Context,
	batch executionBatch,
	req ExecuteRequest,
	toolCtx tool.ToolUseContext,
) []toolExecutionOutcome {
	outcomes := make([]toolExecutionOutcome, len(batch.ToolUses))
	var wg sync.WaitGroup
	for idx, prepared := range batch.ToolUses {
		wg.Add(1)
		go func(localIdx int, prepared preparedToolUse) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					outcomes[localIdx] = toolExecutionOutcome{
						ToolUse: prepared.toolUse,
						Index:   prepared.index,
						Result:  tool.NewErrorResult(fmt.Errorf("tool panic: %v", r)),
					}
				}
			}()
			outcomes[localIdx] = o.executePreparedTool(ctx, prepared, req, toolCtx)
		}(idx, prepared)
	}
	wg.Wait()
	return outcomes
}

func (o *Orchestrator) executeSequentialBatch(
	ctx context.Context,
	batch executionBatch,
	req ExecuteRequest,
	currentContext *tool.ToolUseContext,
) []toolExecutionOutcome {
	outcomes := make([]toolExecutionOutcome, 0, len(batch.ToolUses))
	for _, prepared := range batch.ToolUses {
		outcome := o.executePreparedTool(ctx, prepared, req, *currentContext)
		*currentContext = o.applyContextModifier(*currentContext, outcome.Result)
		outcomes = append(outcomes, outcome)
		if ctx.Err() != nil {
			return outcomes
		}
	}
	return outcomes
}
