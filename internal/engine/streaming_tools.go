package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	registry "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type streamingToolCoordinator struct {
	executor    *execution.StreamingExecutor
	tools       map[string]registry.Tool
	toolIndex   int
	currentTool *types.ToolUseContent
	inputJSON   strings.Builder
	submitted   map[int]types.ToolUseContent
}

func newStreamingToolCoordinator(
	ctx context.Context,
	loop *Loop,
	req RunRequest,
	state *MutableState,
	transcript []types.Message,
) *streamingToolCoordinator {
	if loop == nil || loop.orchestrator == nil || loop.permissionIntegrator == nil || len(req.Tools) == 0 {
		return nil
	}

	execReq := loop.buildExecuteRequest(nil, req, state, transcript)
	return &streamingToolCoordinator{
		executor:  execution.NewStreamingExecutor(ctx, loop.orchestrator, execReq, req.Tools, execReq.PermissionCheck, tool.ToolUseContext{}),
		tools:     req.Tools,
		submitted: make(map[int]types.ToolUseContent),
	}
}

func (c *streamingToolCoordinator) observe(chunk types.APIResponseChunk) {
	switch chunk.Type {
	case types.APIChunkTypeContentBlockStart:
		c.startBlock(chunk.ContentBlock)
	case types.APIChunkTypeContentBlockDelta:
		c.applyDelta(chunk)
	case types.APIChunkTypeContentBlockStop:
		c.finishCurrentTool()
	case types.APIChunkTypeMessageStop:
		c.finishCurrentTool()
	}
}

func (c *streamingToolCoordinator) startBlock(block types.ContentBlock) {
	c.currentTool = nil
	c.inputJSON.Reset()
	toolUse, ok := block.(types.ToolUseContent)
	if !ok {
		return
	}
	cloned := toolUse
	c.currentTool = &cloned
}

func (c *streamingToolCoordinator) applyDelta(chunk types.APIResponseChunk) {
	if c.currentTool == nil || chunk.DeltaType != "input_json_delta" {
		return
	}
	c.inputJSON.WriteString(chunk.PartialJSON)
	if input, ok := parseStreamingToolInput(c.inputJSON.String()); ok {
		c.currentTool.Input = input
	}
}

func (c *streamingToolCoordinator) finishCurrentTool() {
	if c.currentTool == nil {
		return
	}

	index := c.toolIndex
	c.toolIndex++

	toolUse := *c.currentTool
	c.currentTool = nil

	if c.inputJSON.Len() > 0 {
		if input, ok := parseStreamingToolInput(c.inputJSON.String()); ok {
			toolUse.Input = input
		}
	}
	c.inputJSON.Reset()

	if !c.canStreamExecute(toolUse) {
		return
	}
	c.submitted[index] = toolUse
	c.executor.SubmitToolUse(toolUse, index)
}

func (c *streamingToolCoordinator) canStreamExecute(toolUse types.ToolUseContent) bool {
	if c == nil || c.executor == nil || toolUse.Name == "" || toolUse.ID == "" {
		return false
	}
	t, ok := c.tools[toolUse.Name]
	if !ok || t == nil {
		return false
	}
	input := toolUse.Input
	return t.IsConcurrencySafe(input) && t.IsReadOnly(input)
}

// discard cancels any in-flight pre-streamed tool executions and waits for
// goroutines to finish. Safe to call on a nil coordinator.
func (c *streamingToolCoordinator) discard() {
	if c == nil || c.executor == nil {
		return
	}
	c.executor.Discard()
}

func (c *streamingToolCoordinator) complete(ctx context.Context) (map[int]execution.StreamingExecutionResult, error) {
	if c == nil || c.executor == nil || len(c.submitted) == 0 {
		return nil, nil
	}

	// Wait replaces the old 5 ms polling loop: it blocks on the WaitGroup and
	// calls Discard (writing canceled outcomes) if ctx is canceled.
	if err := c.executor.Wait(ctx); err != nil {
		return nil, err
	}

	results := make(map[int]execution.StreamingExecutionResult, len(c.submitted))
	for _, result := range c.executor.GetAllCompletedResults() {
		if _, ok := c.submitted[result.Index]; ok {
			results[result.Index] = result
		}
	}
	return results, nil
}

func (l *Loop) buildExecuteRequest(toolUses []types.ToolUseContent, req RunRequest, state *MutableState, transcript []types.Message) execution.ExecuteRequest {
	var permCtx *types.PermissionContext
	var permMode types.PermissionMode
	if state != nil {
		permCtx = state.PermissionContext
		permMode = state.PermissionMode
	} else {
		permCtx = req.PermissionContext
		permMode = req.PermissionMode
	}
	var permissionResolver types.PermissionResolver
	if l.permissionIntegrator != nil {
		permissionResolver = l.permissionIntegrator.ResolverWithContext(req.SessionID, req.TurnID, permCtx, transcript)
	}
	permissionCheck := req.PermissionCheck
	if permissionCheck == nil && permissionResolver != nil {
		permissionCheck = types.CanUseToolFunc(permissionResolver)
	}
	return execution.ExecuteRequest{
		ToolUses:           toolUses,
		Tools:              req.Tools,
		PermissionCheck:    permissionCheck,
		PermissionResolver: permissionResolver,
		PermissionContext:  clonePermissionContext(permCtx),
		SessionID:          req.SessionID,
		TurnID:             req.TurnID,
		WorkingDirectory:   req.WorkingDirectory,
		PermissionMode:     permMode,
		ProgressCallback:   req.ProgressCallback,
		DenialTracking:     req.DenialTracking,
		Transcript:         transcript,
	}
}

func (l *Loop) executeToolsForResponse(
	ctx context.Context,
	toolUses []types.ToolUseContent,
	req RunRequest,
	state *MutableState,
	transcript []types.Message,
) (execution.ExecuteResult, error) {
	coordinator := l.activeStreamingTools
	l.activeStreamingTools = nil
	if coordinator == nil {
		return l.executeTools(ctx, toolUses, req, state, transcript)
	}

	streamed, err := coordinator.complete(ctx)
	if err != nil {
		return execution.ExecuteResult{}, err
	}
	if len(streamed) == 0 {
		return l.executeTools(ctx, toolUses, req, state, transcript)
	}

	remainingToolUses := make([]types.ToolUseContent, 0, len(toolUses))
	remainingIndexes := make([]int, 0, len(toolUses))
	for idx, toolUse := range toolUses {
		if _, ok := streamed[idx]; ok {
			continue
		}
		remainingToolUses = append(remainingToolUses, toolUse)
		remainingIndexes = append(remainingIndexes, idx)
	}

	remainingResult, err := l.executeTools(ctx, remainingToolUses, req, state, transcript)
	if err != nil {
		return execution.ExecuteResult{}, err
	}

	return mergeToolExecutionResults(toolUses, req.TurnID, streamed, remainingIndexes, remainingResult), nil
}

func mergeToolExecutionResults(
	toolUses []types.ToolUseContent,
	turnID types.TurnID,
	streamed map[int]execution.StreamingExecutionResult,
	remainingIndexes []int,
	remaining execution.ExecuteResult,
) execution.ExecuteResult {
	total := len(toolUses)
	results := make([]tool.CallResult, total)
	traces := make([]execution.ToolExecutionTrace, total)
	progress := append([]types.ToolProgress(nil), remaining.ProgressUpdates...)
	errors := append([]execution.ExecutionError(nil), remaining.Errors...)
	messageBatches := make([][]types.Message, total)

	for idx, result := range streamed {
		results[idx] = result.Result
		traces[idx] = result.Trace
		progress = append(progress, result.Progress...)
		messageBatches[idx] = buildToolResultMessages(result.ToolUse, result.Result, turnID, result.Messages)
		if result.Error != nil {
			errors = append(errors, execution.ExecutionError{
				ToolUseID: result.ToolUse.ID,
				ToolName:  result.ToolUse.Name,
				Error:     result.Error,
				Stage:     result.ErrorStage,
			})
		}
	}

	remainingMessageBatches := splitToolMessageBatches(remaining.Messages)
	for localIdx, globalIdx := range remainingIndexes {
		if localIdx < len(remaining.Results) {
			results[globalIdx] = remaining.Results[localIdx]
		}
		if localIdx < len(remaining.Traces) {
			traces[globalIdx] = remaining.Traces[localIdx]
		}
		if localIdx < len(remainingMessageBatches) {
			messageBatches[globalIdx] = remainingMessageBatches[localIdx]
		}
	}

	messages := make([]types.Message, 0, len(remaining.Messages))
	for _, batch := range messageBatches {
		messages = append(messages, batch...)
	}

	return execution.ExecuteResult{
		Results:          results,
		Messages:         messages,
		Traces:           traces,
		Errors:           errors,
		TotalDuration:    remaining.TotalDuration,
		ProgressUpdates:  progress,
		FinalToolContext: remaining.FinalToolContext,
	}
}

func splitToolMessageBatches(messages []types.Message) [][]types.Message {
	if len(messages) == 0 {
		return nil
	}

	batches := make([][]types.Message, 0)
	current := make([]types.Message, 0)
	for _, message := range messages {
		if isToolResultMessage(message) && len(current) > 0 {
			batches = append(batches, current)
			current = make([]types.Message, 0)
		}
		current = append(current, message)
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func isToolResultMessage(message types.Message) bool {
	for _, block := range message.Content {
		if _, ok := block.(types.ToolResultContent); ok {
			return true
		}
	}
	return false
}

func buildToolResultMessages(toolUse types.ToolUseContent, result tool.CallResult, turnID types.TurnID, extraMessages []types.Message) []types.Message {
	metadata := map[string]any{"tool_name": toolUse.Name}
	if content := result.GetContent(); content != "" {
		metadata["content"] = content
	}
	if result.Metadata != nil {
		if result.Metadata.ExecutionDuration > 0 {
			metadata["execution_duration_ms"] = result.Metadata.ExecutionDuration
		}
		if result.Metadata.ContentReplacement != nil {
			metadata["content_replacement"] = result.Metadata.ContentReplacement
		}
		for key, value := range result.Metadata.Additional {
			metadata[key] = value
		}
	}
	content := resultContent(result)
	if sanitized, detected, reason := sanitizeToolResult(content); detected {
		content = sanitized
		metadata["security_injection_detected"] = true
		metadata["security_injection_reason"] = reason
	}

	toolResultMessage := types.UserMessage(fmt.Sprintf("msg-%s-result", toolUse.ID), "")
	toolResultMessage.Content = []types.ContentBlock{types.ToolResultContent{
		ToolUseID: toolUse.ID,
		Content:   content,
		IsError:   result.IsError(),
		Metadata:  &metadata,
	}}
	toolResultMessage.Metadata = &types.MessageMetadata{TurnID: turnID.String()}

	messages := make([]types.Message, 0, 1+len(result.NewMessages)+len(extraMessages))
	messages = append(messages, toolResultMessage)
	messages = append(messages, result.NewMessages...)
	messages = append(messages, extraMessages...)
	return messages
}

func resultContent(result tool.CallResult) string {
	if result.GetContent() != "" {
		return result.GetContent()
	}
	return fmt.Sprintf("%v", result.GetData())
}

func parseStreamingToolInput(raw string) (map[string]any, bool) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, true
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, false
	}
	return input, true
}
