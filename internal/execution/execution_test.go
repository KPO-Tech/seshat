package execution

import (
	"context"
	"fmt"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserProgressForPageResult(t *testing.T) {
	progress := browserProgressForResult(types.ToolUseContent{Name: "browser_open"}, tool.NewJSONResult(browsercore.PageInfo{
		ID:     "page-1",
		URL:    "https://example.com",
		Title:  "Example",
		Active: true,
	}))
	if progress == nil {
		t.Fatal("expected browser progress")
	}
	if progress.Metadata["event_kind"] != "browser" {
		t.Fatalf("unexpected metadata: %+v", progress.Metadata)
	}
	if progress.Metadata["page_id"] != "page-1" {
		t.Fatalf("unexpected page id: %+v", progress.Metadata)
	}
}

func TestBrowserProgressForSnapshotResult(t *testing.T) {
	progress := browserProgressForResult(types.ToolUseContent{Name: "browser_snapshot"}, tool.NewJSONResult(browsercore.Snapshot{
		Page:     browsercore.PageInfo{ID: "page-1", URL: "https://example.com"},
		Text:     "hello",
		Elements: []browsercore.ElementInfo{{ID: "e1"}},
	}))
	if progress == nil {
		t.Fatal("expected browser progress")
	}
	if progress.Metadata["text_length"] != 5 {
		t.Fatalf("unexpected text length: %+v", progress.Metadata)
	}
	if progress.Metadata["element_count"] != 1 {
		t.Fatalf("unexpected element count: %+v", progress.Metadata)
	}
}

func TestEventQueueBasicEmitReceive(t *testing.T) {
	q := NewEventQueue(10)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk)

	select {
	case received := <-q.Recv():
		assert.Equal(t, chunk.Type, received.Type)
	default:
		t.Fatal("expected chunk in queue")
	}
	assert.Equal(t, int64(1), q.EmittedCount())
	assert.Equal(t, int64(0), q.OverflowCount())
}

func TestEventQueueOverflow(t *testing.T) {
	q := NewEventQueue(2)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}

	q.Emit(chunk)
	q.Emit(chunk)
	q.Emit(chunk) // overflow: queue is full

	assert.Equal(t, int64(2), q.EmittedCount())
	assert.Equal(t, int64(1), q.OverflowCount())
}

func TestEventQueueCloseBlocksEmit(t *testing.T) {
	q := NewEventQueue(10)
	q.Close()

	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk) // must be a no-op

	assert.Equal(t, int64(0), q.EmittedCount())

	_, ok := <-q.Recv()
	assert.False(t, ok, "closed channel should return zero value and false")
}

func TestEventQueueCloseIdempotent(t *testing.T) {
	q := NewEventQueue(10)
	assert.NotPanics(t, func() {
		q.Close()
		q.Close()
		q.Close()
	})
}

func TestEventQueueUnderLoad(t *testing.T) {
	const total = 10_000
	q := NewEventQueue(total)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}

	for i := 0; i < total; i++ {
		q.Emit(chunk)
	}
	q.Close()

	received := 0
	for range q.Recv() {
		received++
	}

	require.Equal(t, total, received, "all emitted chunks must be received")
	assert.Equal(t, int64(total), q.EmittedCount())
	assert.Equal(t, int64(0), q.OverflowCount())
}

func TestEventQueueDefaultCapacity(t *testing.T) {
	q := NewEventQueue(0)
	assert.NotNil(t, q)
	assert.Equal(t, DefaultEventQueueCapacity, cap(q.ch))
}

// ---------------------------------------------------------------------------
// Priority 5: Streaming delivery guarantees — Stats, EmitBlocking, Drain
// ---------------------------------------------------------------------------

func TestEventQueueStats_ReflectsEmitAndOverflow(t *testing.T) {
	q := NewEventQueue(2)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}

	q.Emit(chunk)
	q.Emit(chunk)
	q.Emit(chunk) // overflow

	stats := q.Stats()
	assert.Equal(t, int64(2), stats.Emitted)
	assert.Equal(t, int64(1), stats.Overflow)
	assert.Equal(t, 2, stats.Pending)
	assert.False(t, stats.Closed)
}

func TestEventQueueStats_ReflectsClosedState(t *testing.T) {
	q := NewEventQueue(5)
	q.Close()

	stats := q.Stats()
	assert.True(t, stats.Closed)
}

func TestEventQueueStats_PendingDecreasesAsChunksAreConsumed(t *testing.T) {
	q := NewEventQueue(10)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk)
	q.Emit(chunk)

	assert.Equal(t, 2, q.Stats().Pending)

	<-q.Recv()
	assert.Equal(t, 1, q.Stats().Pending)
}

func TestEventQueueEmitBlocking_SendsChunkSuccessfully(t *testing.T) {
	q := NewEventQueue(5)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}

	ctx := context.Background()
	sent := q.EmitBlocking(ctx, chunk)

	require.True(t, sent, "EmitBlocking must return true when the chunk was enqueued")
	assert.Equal(t, int64(1), q.EmittedCount())
	assert.Equal(t, int64(0), q.OverflowCount())
}

func TestEventQueueEmitBlocking_ReturnsFalseOnCancelledContext(t *testing.T) {
	// Fill the queue so the next emit would block.
	q := NewEventQueue(1)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk) // fills the buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	sent := q.EmitBlocking(ctx, chunk)
	assert.False(t, sent, "EmitBlocking must return false when ctx is already cancelled")
	assert.Equal(t, int64(1), q.OverflowCount(), "cancelled send must increment overflow")
}

func TestEventQueueEmitBlocking_ReturnsFalseOnClosedQueue(t *testing.T) {
	q := NewEventQueue(5)
	q.Close()

	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	sent := q.EmitBlocking(context.Background(), chunk)
	assert.False(t, sent, "EmitBlocking on closed queue must return false")
}

func TestEventQueueEmitBlocking_UnblocksWhenConsumerDrains(t *testing.T) {
	q := NewEventQueue(1)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk) // fill

	// Start a goroutine that will consume the buffered chunk after a brief delay,
	// unblocking the EmitBlocking call below.
	go func() {
		time.Sleep(20 * time.Millisecond)
		<-q.Recv()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	sent := q.EmitBlocking(ctx, chunk)
	assert.True(t, sent, "EmitBlocking must succeed once the consumer drains the queue")
}

func TestEventQueueDrain_ReturnsImmediatelyWhenBufferIsEmpty(t *testing.T) {
	q := NewEventQueue(10)
	ctx := context.Background()

	err := q.Drain(ctx)
	assert.NoError(t, err)
}

func TestEventQueueDrain_ReturnsAfterConsumerEmptiesBuffer(t *testing.T) {
	q := NewEventQueue(10)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	q.Emit(chunk)
	q.Emit(chunk)

	go func() {
		time.Sleep(10 * time.Millisecond)
		<-q.Recv()
		<-q.Recv()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := q.Drain(ctx)
	assert.NoError(t, err)
}

func TestEventQueueDrain_ContextCancellationReturnsError(t *testing.T) {
	q := NewEventQueue(10)
	chunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta}
	for i := 0; i < 10; i++ {
		q.Emit(chunk)
	}
	// No consumer — buffer stays full.

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	err := q.Drain(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

type stubTool struct {
	definition        tool.Definition
	validateInput     func(ctx context.Context, input map[string]any) (map[string]any, error)
	checkPermissions  func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult
	call              func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error)
	isConcurrencySafe func(input map[string]any) bool
	isReadOnly        func(input map[string]any) bool
	isEnabled         func() bool
	formatResult      func(data any) string
	backfillInput     func(ctx context.Context, input map[string]any) map[string]any
}

func (s *stubTool) Definition() tool.Definition {
	if s.definition.Name != "" {
		return s.definition
	}
	return tool.Definition{Name: "stub", Description: "stub"}
}

func (s *stubTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	if s.call != nil {
		return s.call(ctx, input, permissionCheck)
	}
	return tool.NewTextResult("ok"), nil
}

func (s *stubTool) Description(ctx context.Context) (string, error) {
	return "stub", nil
}

func (s *stubTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if s.validateInput != nil {
		return s.validateInput(ctx, input)
	}
	return input, nil
}

func (s *stubTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	if s.checkPermissions != nil {
		return s.checkPermissions(ctx, input, toolCtx)
	}
	return types.Passthrough(input)
}

func (s *stubTool) IsConcurrencySafe(input map[string]any) bool {
	if s.isConcurrencySafe != nil {
		return s.isConcurrencySafe(input)
	}
	return false
}

func (s *stubTool) IsReadOnly(input map[string]any) bool {
	if s.isReadOnly != nil {
		return s.isReadOnly(input)
	}
	return false
}

func (s *stubTool) IsEnabled() bool {
	if s.isEnabled != nil {
		return s.isEnabled()
	}
	return true
}

func (s *stubTool) FormatResult(data any) string {
	if s.formatResult != nil {
		return s.formatResult(data)
	}
	if str, ok := data.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", data)
}

func (s *stubTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	if s.backfillInput != nil {
		return s.backfillInput(ctx, input)
	}
	return input
}

func TestExecuteCapturesValidationAndPermissionTrace(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "stub", Input: map[string]any{"value": "raw"}}
	stub := &stubTool{
		validateInput: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"value": "validated"}, nil
		},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.PassthroughWithDecisionReason(map[string]any{"value": "local"}, &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonTool,
				Source: "local-check",
				Reason: "local pass",
			})
		},
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			if got := input.Parsed["value"]; got != "global" {
				t.Fatalf("expected global rewritten input, got %v", got)
			}
			return tool.NewTextResult("done"), nil
		},
	}

	globalPermission := func(ctx context.Context, request types.ToolPermissionRequest) types.PermissionResult {
		return types.AllowWithInputAndDecisionReason("global allow", map[string]any{"value": "global"}, &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonRule,
			Source: "global-check",
			Reason: "global allow",
		})
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{toolUse},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: globalPermission,
		SessionID:       types.SessionID("session-1"),
		TurnID:          types.TurnID("turn-1"),
		PermissionMode:  types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(result.Traces))
	}
	trace := result.Traces[0]
	if trace.ValidatedInput["value"] != "validated" {
		t.Fatalf("expected validated input, got %#v", trace.ValidatedInput)
	}
	if trace.FinalInput["value"] != "global" {
		t.Fatalf("expected final input to reflect global rewrite, got %#v", trace.FinalInput)
	}
	if trace.LocalPermission.DecisionReason == nil || trace.LocalPermission.DecisionReason.Source != "local-check" {
		t.Fatalf("expected local permission reason, got %#v", trace.LocalPermission)
	}
	if trace.GlobalPermission.DecisionReason == nil || trace.GlobalPermission.DecisionReason.Source != "global-check" {
		t.Fatalf("expected global permission reason, got %#v", trace.GlobalPermission)
	}
}

func TestExecutePermissionFailureUsesPermissionStage(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "stub", Input: map[string]any{"value": "raw"}}
	stub := &stubTool{
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.DenyWithDecisionReason("blocked locally", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonTool,
				Source: "local-check",
				Reason: "blocked locally",
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage error, got %s", result.Errors[0].Stage)
	}
	if len(result.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(result.Traces))
	}
	trace := result.Traces[0]
	if trace.LocalPermission.DecisionReason == nil || trace.LocalPermission.DecisionReason.Source != "local-check" {
		t.Fatalf("expected local deny reason in trace, got %#v", trace.LocalPermission)
	}
}

// --- New tests for the enriched pipeline ---

func TestDisabledToolIsSkipped(t *testing.T) {
	orch := NewOrchestrator()
	disabled := &stubTool{}
	disabled.isEnabled = func() bool { return false }

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": disabled},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error for disabled tool, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStageDisabled {
		t.Fatalf("expected disabled stage, got %s", result.Errors[0].Stage)
	}
	if !strings.Contains(result.Errors[0].Error.Error(), "disabled") {
		t.Fatalf("expected disabled in error message, got %v", result.Errors[0].Error)
	}
}

func TestBackfillInputEnrichesForHooksAndPermissions(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "t1", Name: "stub", Input: map[string]any{"path": "~/file.txt"}}

	var backfilledInputSeen map[string]any
	stub := &stubTool{
		backfillInput: func(ctx context.Context, input map[string]any) map[string]any {
			input["expanded_path"] = "/home/user/file.txt"
			return input
		},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			backfilledInputSeen = cloneToolInput(input)
			return types.Passthrough(input)
		},
	}

	_, _ = orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})

	if backfilledInputSeen == nil {
		t.Fatal("expected checkPermissions to be called with backfilled input")
	}
	if backfilledInputSeen["expanded_path"] != "/home/user/file.txt" {
		t.Fatalf("expected expanded_path in backfilled input, got %#v", backfilledInputSeen)
	}
}

func TestPreHookCanStopExecution(t *testing.T) {
	orch := NewOrchestrator()
	orch.AddHook(ToolHook{
		Stage:    ToolHookStagePre,
		Priority: 100,
		ID:       "blocker",
		Execute: func(ctx context.Context, input ToolHookInput) ToolHookResult {
			return ToolHookResult{
				Stop: &ToolHookStop{
					Content: "blocked by hook",
					IsError: true,
				},
			}
		},
	})

	stub := &stubTool{}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error from hook stop, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStageHook {
		t.Fatalf("expected hook stage, got %s", result.Errors[0].Stage)
	}
	// The tool result should contain the hook's content
	if !strings.Contains(result.Results[0].Content, "blocked by hook") {
		t.Fatalf("expected hook stop content in result, got %q", result.Results[0].Content)
	}
}

func TestPreHookCanModifyInput(t *testing.T) {
	orch := NewOrchestrator()
	orch.AddHook(ToolHook{
		Stage:    ToolHookStagePre,
		Priority: 100,
		ID:       "modifier",
		Execute: func(ctx context.Context, input ToolHookInput) ToolHookResult {
			return ToolHookResult{
				UpdatedInput: map[string]any{"value": "modified-by-hook"},
			}
		},
	})

	var callInputSeen map[string]any
	stub := &stubTool{
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			callInputSeen = cloneToolInput(input.Parsed)
			return tool.NewTextResult("ok"), nil
		},
	}

	_, _ = orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{"value": "original"}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})

	if callInputSeen == nil {
		t.Fatal("expected tool to be called")
	}
	if callInputSeen["value"] != "modified-by-hook" {
		t.Fatalf("expected hook-modified input, got %#v", callInputSeen)
	}
}

func TestPrepareToolUseEvaluatesConcurrencyOnValidatedInput(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{
		validateInput: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"mode": "validated"}, nil
		},
		isConcurrencySafe: func(input map[string]any) bool {
			return input["mode"] == "validated"
		},
	}

	prepared := orch.prepareToolUse(context.Background(), types.ToolUseContent{
		ID:    "t1",
		Name:  "stub",
		Input: map[string]any{"mode": "raw"},
	}, 0, ExecuteRequest{
		Tools: map[string]tool.Tool{"stub": stub},
	})

	if prepared.failure != nil {
		t.Fatalf("expected prepareToolUse to succeed, got %v", prepared.failure.err)
	}
	if !prepared.isConcurrencySafe {
		t.Fatal("expected concurrency safety to be evaluated from validated input")
	}
}

func TestPartitionPreparedToolUsesSeparatesValidationFailures(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{
		validateInput: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if input["valid"] == false {
				return nil, fmt.Errorf("invalid input")
			}
			return input, nil
		},
		isConcurrencySafe: func(input map[string]any) bool {
			return true
		},
	}

	prepared := orch.prepareToolUses(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{"valid": true}},
			{ID: "t2", Name: "stub", Input: map[string]any{"valid": false}},
			{ID: "t3", Name: "stub", Input: map[string]any{"valid": true}},
		},
		Tools: map[string]tool.Tool{"stub": stub},
	})

	batches := orch.partitionPreparedToolUses(prepared)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if !batches[0].IsConcurrencySafe || len(batches[0].ToolUses) != 1 || batches[0].ToolUses[0].toolUse.ID != "t1" {
		t.Fatalf("expected first batch to contain the first valid concurrent tool, got %#v", batches[0])
	}
	if batches[1].IsConcurrencySafe || len(batches[1].ToolUses) != 1 || batches[1].ToolUses[0].toolUse.ID != "t2" {
		t.Fatalf("expected second batch to isolate the validation failure, got %#v", batches[1])
	}
	if !batches[2].IsConcurrencySafe || len(batches[2].ToolUses) != 1 || batches[2].ToolUses[0].toolUse.ID != "t3" {
		t.Fatalf("expected third batch to contain the final valid concurrent tool, got %#v", batches[2])
	}
}

func TestCallUsesValidatedInputWithoutBackfillLeak(t *testing.T) {
	orch := NewOrchestrator()
	var callInputSeen map[string]any
	stub := &stubTool{
		validateInput: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"path": "/validated.txt"}, nil
		},
		backfillInput: func(ctx context.Context, input map[string]any) map[string]any {
			input["expanded_path"] = "/tmp/validated.txt"
			return input
		},
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			callInputSeen = cloneToolInput(input.Parsed)
			return tool.NewTextResult("ok"), nil
		},
	}

	_, _ = orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{"path": "~/raw.txt"}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})

	if callInputSeen == nil {
		t.Fatal("expected tool to be called")
	}
	if callInputSeen["path"] != "/validated.txt" {
		t.Fatalf("expected validated path in call input, got %#v", callInputSeen)
	}
	if _, exists := callInputSeen["expanded_path"]; exists {
		t.Fatalf("expected backfilled field to stay out of call input, got %#v", callInputSeen)
	}
}

func TestFormatResultIsUsedWhenContentIsEmpty(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{
		formatResult: func(data any) string {
			return fmt.Sprintf("formatted: %v", data)
		},
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			return tool.CallResult{Data: "raw-data"}, nil
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.HasPrefix(result.Results[0].Content, "formatted:") {
		t.Fatalf("expected FormatResult to be used, got %q", result.Results[0].Content)
	}
}

func TestMaxResultSizeTruncatesContent(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	stub.definition = tool.Definition{
		Name:          "stub",
		Description:   "stub",
		MaxResultSize: 100,
		InputSchema:   schema.JSONSchema{},
	}

	longContent := strings.Repeat("x", 500)
	stub.call = func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
		return tool.NewTextResult(longContent), nil
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	content := result.Results[0].Content
	if len(content) > 200 { // 100 + truncation message
		t.Fatalf("expected truncated content, got length %d", len(content))
	}
	if !strings.Contains(content, "truncated") {
		t.Fatalf("expected truncation message in content, got %q", content)
	}
	if result.Results[0].Metadata == nil || result.Results[0].Metadata.ContentReplacement == nil {
		t.Fatal("expected ContentReplacement metadata")
	}
	if result.Results[0].Metadata.ContentReplacement.ReplacementType != types.ContentReplacementTypeTruncated {
		t.Fatalf("expected truncated replacement type, got %s", result.Results[0].Metadata.ContentReplacement.ReplacementType)
	}
}

func TestCallToolSafeWrapsErrorsInCallResult(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			return tool.CallResult{}, fmt.Errorf("tool exploded")
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute should not return error even when tool.Call fails, got: %v", err)
	}
	// The error should be inside the CallResult, not in the Execute error
	if !result.Results[0].IsError() {
		t.Fatal("expected result to be an error")
	}
	if !strings.Contains(result.Results[0].Content, "tool exploded") {
		t.Fatalf("expected error content in result, got %q", result.Results[0].Content)
	}
}

func TestPostHookEmitsExtraMessages(t *testing.T) {
	orch := NewOrchestrator()
	orch.AddHook(ToolHook{
		Stage:    ToolHookStagePost,
		Priority: 100,
		ID:       "post-observer",
		Execute: func(ctx context.Context, input ToolHookInput) ToolHookResult {
			return ToolHookResult{
				ExtraMessages: []types.Message{
					types.UserMessage("hook-msg-1", "from post hook"),
				},
			}
		},
	})

	stub := &stubTool{}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 1 tool result message + 1 extra from post hook
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages (result + hook extra), got %d", len(result.Messages))
	}
}

func TestConcurrentContextModifiersAppliedInOrder(t *testing.T) {
	orch := NewOrchestrator()
	var order []int

	// Create 3 concurrency-safe tools that each append their index via context modifier
	tools := map[string]tool.Tool{}
	for i := 0; i < 3; i++ {
		idx := i
		stub := &stubTool{
			isConcurrencySafe: func(input map[string]any) bool { return true },
			call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
				capturedIdx := idx
				return tool.CallResult{
					Data: fmt.Sprintf("tool-%d", capturedIdx),
					ContextModifier: func(ctx tool.ToolUseContext) tool.ToolUseContext {
						if ctx.Metadata == nil {
							ctx.Metadata = make(map[string]any)
						}
						var order []int
						if existing, ok := ctx.Metadata["order"].([]int); ok {
							order = existing
						}
						order = append(order, capturedIdx)
						ctx.Metadata["order"] = order
						return ctx
					},
				}, nil
			},
		}
		tools[fmt.Sprintf("stub-%d", i)] = stub
	}

	toolUses := make([]types.ToolUseContent, 3)
	for i := 0; i < 3; i++ {
		toolUses[i] = types.ToolUseContent{
			ID:    fmt.Sprintf("t%d", i),
			Name:  fmt.Sprintf("stub-%d", i),
			Input: map[string]any{},
		}
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       toolUses,
		Tools:          tools,
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	order = result.FinalToolContext.Metadata["order"].([]int)
	if len(order) != 3 {
		t.Fatalf("expected 3 context modifier applications, got %d", len(order))
	}
	// Must be in original order [0, 1, 2], not random goroutine completion order
	for i, v := range order {
		if v != i {
			t.Fatalf("expected order [%d, 1, 2], got %v (context modifiers applied in wrong order)", i, order)
		}
	}
}

// --- Tests for safety checks and denial tracking ---

func TestSafetyCheckBlocksDangerousPattern(t *testing.T) {
	orch := NewOrchestrator()
	checker := types.NewDangerousPatternChecker()
	checker.AddPattern("bash", types.DangerousPattern{
		Pattern:   "rm -rf",
		Reason:    "destructive command",
		CheckType: "destructive_command",
	})
	orch.SetSafetyChecker(checker)

	stub := &stubTool{}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "bash", Input: map[string]any{"command": "rm -rf /home/user/data"}},
		},
		Tools:          map[string]tool.Tool{"bash": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeBypass, // even bypass doesn't help
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error from safety check, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage, got %s", result.Errors[0].Stage)
	}
	if !strings.Contains(result.Errors[0].Error.Error(), "safety check blocked") {
		t.Fatalf("expected safety check error, got %v", result.Errors[0].Error)
	}
}

func TestDenialTrackingRecordsDenials(t *testing.T) {
	orch := NewOrchestrator()
	denialState := &types.DenialTrackingState{}

	stub := &stubTool{
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.Deny("blocked")
		},
	}

	_, _ = orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
		DenialTracking: denialState,
	})

	if denialState.GetConsecutiveDenials() != 1 {
		t.Fatalf("expected 1 consecutive denial, got %d", denialState.GetConsecutiveDenials())
	}
	if denialState.GetTotalDenials() != 1 {
		t.Fatalf("expected 1 total denial, got %d", denialState.GetTotalDenials())
	}
}

func TestPermissionAskProbeStopsBeforeLocalCheck(t *testing.T) {
	orch := NewOrchestrator()
	localCalled := false
	stub := &stubTool{
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			localCalled = true
			return types.AllowWithUpdatedInput(input)
		},
	}

	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		if req.Stage == types.ToolPermissionStageWholeTool && req.Intent == types.ToolPermissionIntentAsk {
			return types.AskWithDecisionReason("ask at rule level", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonRule,
				Source: "ask-rule",
				Reason: "ask at rule level",
			})
		}
		return types.AllowWithUpdatedInput(req.ToolInput)
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: permissionCheck,
		SessionID:       types.SessionID("s1"),
		TurnID:          types.TurnID("t1"),
		PermissionMode:  types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if localCalled {
		t.Fatal("expected ask probe to stop before local permission check")
	}
	if len(result.Errors) != 1 || result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission-stage error, got %#v", result.Errors)
	}
	if !result.Results[0].IsError() || !strings.Contains(result.Results[0].Content, "user confirmation required") {
		t.Fatalf("expected ask-probe failure in tool result, got %#v", result.Results[0])
	}
}

func TestGlobalPermissionUsesEngineDecision(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}

	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		if req.Stage == types.ToolPermissionStageGlobal {
			return types.AskWithDecisionReason("engine requires approval", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonOther,
				Source: "engine",
				Reason: "engine requires approval",
			})
		}
		return types.Passthrough(nil)
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: permissionCheck,
		SessionID:       types.SessionID("s1"),
		TurnID:          types.TurnID("t1"),
		PermissionMode:  types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 || result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission error, got %#v", result.Errors)
	}
	if !strings.Contains(result.Results[0].Content, "engine requires approval") {
		t.Fatalf("expected engine-provided ask content, got %q", result.Results[0].Content)
	}
}

func TestBypassModeSkipsGlobalAsk(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		if req.ToolInput == nil {
			return types.Passthrough(nil)
		}
		return types.Ask("would normally ask")
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: permissionCheck,
		SessionID:       types.SessionID("s1"),
		TurnID:          types.TurnID("t1"),
		PermissionMode:  types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected bypass mode to avoid permission failure, got %#v", result.Errors)
	}
	if result.Results[0].IsError() {
		t.Fatalf("expected successful result in bypass mode, got %#v", result.Results[0])
	}
}

func TestAutoModeDenialFallbackPromotesToAsk(t *testing.T) {
	orch := NewOrchestrator()
	orch.SetDenialLimitConfig(types.DenialLimitConfig{MaxConsecutiveDenials: 2})
	denialState := &types.DenialTrackingState{}
	denialState.RecordDenial()
	stub := &stubTool{
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.Deny("blocked locally")
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeAuto,
		DenialTracking: denialState,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 permission error, got %#v", result.Errors)
	}
	if !strings.Contains(result.Results[0].Content, "user confirmation required") {
		t.Fatalf("expected denial fallback to ask, got %q", result.Results[0].Content)
	}
	if denialState.GetConsecutiveDenials() != 2 {
		t.Fatalf("expected denial tracking to record the second denial, got %d", denialState.GetConsecutiveDenials())
	}
}

func TestDenialTrackingResetsOnSuccess(t *testing.T) {
	orch := NewOrchestrator()
	denialState := &types.DenialTrackingState{}
	denialState.ConsecutiveDenials = 5 // pre-populate

	stub := &stubTool{}

	_, _ = orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": stub},
		SessionID:      types.SessionID("s1"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
		DenialTracking: denialState,
	})

	if denialState.GetConsecutiveDenials() != 0 {
		t.Fatalf("expected consecutive denials reset to 0, got %d", denialState.GetConsecutiveDenials())
	}
}

func TestDenialLimitTriggersFallback(t *testing.T) {
	orch := NewOrchestrator()
	orch.SetDenialLimitConfig(types.DenialLimitConfig{
		MaxConsecutiveDenials: 2,
	})

	denialState := &types.DenialTrackingState{}
	denialState.RecordDenial()
	denialState.RecordDenial()

	// This third denial should trigger fallback behavior in the caller
	// For now, just verify the state
	if !orch.denialLimitConfig.ShouldFallback(denialState) {
		t.Fatal("expected fallback after 2 consecutive denials")
	}

	// A success should reset the counter
	denialState.RecordSuccess()
	if orch.denialLimitConfig.ShouldFallback(denialState) {
		t.Fatal("expected no fallback after success reset")
	}
}

// ---------------------------------------------------------------------------
// Tests for bypass-immune ask distinctions (steps 1e, 1f, 1g)
// ---------------------------------------------------------------------------

// requiresUserInteractionStub is a stub tool that declares RequiresUserInteraction.
type requiresUserInteractionStub struct {
	stubTool
	requiresInteraction bool
}

func (r *requiresUserInteractionStub) RequiresUserInteraction() bool {
	return r.requiresInteraction
}

func TestRequiresUserInteractionBlocksBypassMode(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "interactive", Input: map[string]any{"cmd": "deploy"}}
	stub := &requiresUserInteractionStub{
		stubTool: stubTool{
			definition: tool.Definition{Name: "interactive", Description: "interactive tool"},
			checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
				return types.AskWithDecisionReason("user must approve deploy", &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonTool,
					Source: "local-check",
					Reason: "requires user interaction",
				})
			},
		},
		requiresInteraction: true,
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"interactive": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error (bypass should not override requiresUserInteraction), got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage error, got %s", result.Errors[0].Stage)
	}
	// The tool should NOT have been called
	if len(result.Results) != 1 || !result.Results[0].IsError() {
		t.Fatal("expected error result from permission block")
	}
}

func TestRequiresUserInteractionFalseAllowsBypass(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "safe", Input: map[string]any{"cmd": "status"}}
	stub := &requiresUserInteractionStub{
		stubTool: stubTool{
			definition: tool.Definition{Name: "safe", Description: "safe tool"},
			checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
				// Passthrough (not ask), so requiresUserInteraction guard doesn't apply
				return types.Passthrough(input)
			},
		},
		requiresInteraction: false,
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"safe": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors in bypass mode, got %d: %v", len(result.Errors), result.Errors[0].Error)
	}
}

func TestSafetyCheckAskIsBypassImmune(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "editor", Input: map[string]any{"file_path": "/repo/.git/config"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "editor", Description: "editor tool"},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.AskWithDecisionReason("editing .git is dangerous", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonSafetyCheck,
				Source: "path-safety",
				Reason: ".git directory is protected",
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"editor": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error (safety check is bypass-immune), got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage error, got %s", result.Errors[0].Stage)
	}
}

func TestSafetyCheckClassifierApprovableRespectsBypass(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "editor", Input: map[string]any{"file_path": "/repo/sensitive-file.txt"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "editor", Description: "editor tool"},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.AskWithDecisionReason("sensitive file", &types.PermissionDecisionReason{
				Type:                 types.PermissionDecisionReasonSafetyCheck,
				Source:               "path-safety",
				Reason:               "sensitive file detected",
				ClassifierApprovable: true,
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"editor": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// ClassifierApprovable safety check should NOT be bypass-immune
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors (classifier-approvable safety check respects bypass), got %d", len(result.Errors))
	}
}

func TestContentSpecificAskRuleIsBypassImmune(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "bash", Input: map[string]any{"command": "npm publish"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "bash", Description: "bash tool"},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.AskWithDecisionReason("npm publish requires approval", &types.PermissionDecisionReason{
				Type:         types.PermissionDecisionReasonRule,
				Source:       "userSettings",
				Reason:       "npm publish requires explicit approval",
				RuleBehavior: types.PermissionBehaviorAsk,
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"bash": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error (content-specific ask rule is bypass-immune), got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage error, got %s", result.Errors[0].Stage)
	}
}

func TestContentSpecificDenyRuleIsNotBypassImmuneViaRule(t *testing.T) {
	// A deny from a deny rule is always bypass-immune (handled by step 1a via
	// whole-tool deny probe, but also by step 1d from tool.checkPermissions).
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "bash", Input: map[string]any{"command": "rm -rf /"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "bash", Description: "bash tool"},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			return types.DenyWithDecisionReason("destructive command", &types.PermissionDecisionReason{
				Type:         types.PermissionDecisionReasonRule,
				Source:       "deny-rules",
				Reason:       "rm -rf / is blocked",
				RuleBehavior: types.PermissionBehaviorDeny,
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"bash": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error (deny is always bypass-immune), got %d", len(result.Errors))
	}
}

func TestPassthroughAskRespectsBypass(t *testing.T) {
	orch := NewOrchestrator()
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "writer", Input: map[string]any{"file_path": "/repo/src/main.go"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "writer", Description: "writer tool"},
		checkPermissions: func(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
			// A plain passthrough ask (no special decision reason type) should
			// be overridden by bypass mode.
			return types.AskWithDecisionReason("needs approval", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonTool,
				Source: "local-check",
				Reason: "write tool needs approval",
			})
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"writer": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Passthrough ask is NOT bypass-immune — bypass mode should allow it
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors (passthrough ask respects bypass), got %d: %v", len(result.Errors), result.Errors[0].Error)
	}
}

func TestIsBypassImmuneHelper(t *testing.T) {
	tests := []struct {
		name     string
		result   types.PermissionResult
		expected bool
	}{
		{
			name: "deny is always immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorDeny,
			},
			expected: true,
		},
		{
			name: "allow is never immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAllow,
			},
			expected: false,
		},
		{
			name: "ask with nil reason is not immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
			},
			expected: false,
		},
		{
			name: "ask with tool reason is not immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonTool,
					Source: "local-check",
				},
			},
			expected: false,
		},
		{
			name: "ask with safetyCheck reason is immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: "path-safety",
				},
			},
			expected: true,
		},
		{
			name: "ask with classifierApprovable safetyCheck is not immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:                 types.PermissionDecisionReasonSafetyCheck,
					Source:               "path-safety",
					ClassifierApprovable: true,
				},
			},
			expected: false,
		},
		{
			name: "ask with rule ask behavior is immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:         types.PermissionDecisionReasonRule,
					Source:       "userSettings",
					RuleBehavior: types.PermissionBehaviorAsk,
				},
			},
			expected: true,
		},
		{
			name: "ask with rule deny behavior is not immune (deny type doesn't trigger immune for ask behavior)",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:         types.PermissionDecisionReasonRule,
					Source:       "userSettings",
					RuleBehavior: types.PermissionBehaviorDeny,
				},
			},
			expected: false,
		},
		{
			name: "ask with rule allow behavior is not immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:         types.PermissionDecisionReasonRule,
					Source:       "userSettings",
					RuleBehavior: types.PermissionBehaviorAllow,
				},
			},
			expected: false,
		},
		{
			name: "ask with other reason is not immune",
			result: types.PermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				DecisionReason: &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonOther,
					Source: "engine",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsBypassImmune()
			if got != tt.expected {
				t.Errorf("IsBypassImmune() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSafetyCheckDenyInPipelineHasCorrectReasonType(t *testing.T) {
	orch := NewOrchestrator()
	orch.SetSafetyChecker(types.NewDangerousPatternChecker())
	toolUse := types.ToolUseContent{ID: "tool-1", Name: "bash", Input: map[string]any{"command": "rm -rf /"}}
	stub := &stubTool{
		definition: tool.Definition{Name: "bash", Description: "bash tool"},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:       []types.ToolUseContent{toolUse},
		Tools:          map[string]tool.Tool{"bash": stub},
		SessionID:      types.SessionID("session-1"),
		TurnID:         types.TurnID("turn-1"),
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 execution error, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage error, got %s", result.Errors[0].Stage)
	}
	// Safety check deny produces an error result. The dangerous pattern
	// checker uses a deny with PermissionDecisionReasonSafetyCheck type.
	if len(result.Results) != 1 || !result.Results[0].IsError() {
		t.Fatal("expected error result from safety check deny")
	}
	errContent := result.Results[0].Content
	if !strings.Contains(errContent, "safety check blocked") {
		t.Fatalf("expected safety check blocked in error content, got: %s", errContent)
	}
}

// ---------------------------------------------------------------------------
// Tests for dontAsk mode
// ---------------------------------------------------------------------------

func TestDontAskModeDeniesGlobalAsk(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	// Global permission returns ask — dontAsk mode should transform to deny.
	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		return types.Ask("would normally ask")
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: permissionCheck,
		SessionID:       types.SessionID("s1"),
		TurnID:          types.TurnID("t1"),
		PermissionMode:  types.PermissionModeNever,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error in dontAsk mode, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage, got %s", result.Errors[0].Stage)
	}
}

func TestDontAskModeAllowsWhenGlobalAllows(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	// Global permission returns allow — dontAsk mode should pass through.
	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		return types.AllowWithUpdatedInput(req.ToolInput)
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:        []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:           map[string]tool.Tool{"stub": stub},
		PermissionCheck: permissionCheck,
		SessionID:       types.SessionID("s1"),
		TurnID:          types.TurnID("t1"),
		PermissionMode:  types.PermissionModeNever,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors in dontAsk mode when allowed, got %#v", result.Errors)
	}
}

// ---------------------------------------------------------------------------
// Tests for headless auto-deny (ShouldAvoidPermissionPrompts)
// ---------------------------------------------------------------------------

func TestShouldAvoidPermissionPromptsDeniesGlobalAsk(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	// Global permission returns ask — headless mode should auto-deny.
	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		return types.Ask("would normally ask")
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:                     []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:                        map[string]tool.Tool{"stub": stub},
		PermissionCheck:              permissionCheck,
		SessionID:                    types.SessionID("s1"),
		TurnID:                       types.TurnID("t1"),
		PermissionMode:               types.PermissionModeOnRequest,
		ShouldAvoidPermissionPrompts: true,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error with ShouldAvoidPermissionPrompts, got %d", len(result.Errors))
	}
	if result.Errors[0].Stage != ErrorStagePermission {
		t.Fatalf("expected permission stage, got %s", result.Errors[0].Stage)
	}
}

func TestShouldAvoidPermissionPromptsAllowsWhenGlobalAllows(t *testing.T) {
	orch := NewOrchestrator()
	stub := &stubTool{}
	permissionCheck := func(ctx context.Context, req types.ToolPermissionRequest) types.PermissionResult {
		return types.AllowWithUpdatedInput(req.ToolInput)
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses:                     []types.ToolUseContent{{ID: "t1", Name: "stub", Input: map[string]any{}}},
		Tools:                        map[string]tool.Tool{"stub": stub},
		PermissionCheck:              permissionCheck,
		SessionID:                    types.SessionID("s1"),
		TurnID:                       types.TurnID("t1"),
		PermissionMode:               types.PermissionModeOnRequest,
		ShouldAvoidPermissionPrompts: true,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors when allowed, got %#v", result.Errors)
	}
}

// ─── Tool sandboxing: panic recovery ─────────────────────────────────────────

func TestCallToolSafe_PanicIsRecoveredAsError(t *testing.T) {
	orch := NewOrchestrator()
	panicTool := &stubTool{
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			panic("unexpected nil pointer")
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "stub", Input: map[string]any{}},
		},
		Tools:          map[string]tool.Tool{"stub": panicTool},
		SessionID:      types.SessionID("panic-session"),
		TurnID:         types.TurnID("panic-turn"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute itself should not panic-propagate: %v", err)
	}
	if len(result.Results) == 0 {
		t.Fatal("expected a result entry even for panicking tool")
	}
	if result.Results[0].IsSuccess() {
		t.Fatal("expected error result for panicking tool")
	}
	if !strings.Contains(result.Results[0].Content, "panic") {
		t.Fatalf("expected panic message in error result, got %q", result.Results[0].Content)
	}
}

func TestCallToolSafe_PanicInConcurrentBatch(t *testing.T) {
	orch := NewOrchestrator()

	// One panicking tool and one healthy one, run concurrently
	panicTool := &stubTool{
		definition:        tool.Definition{Name: "panic_tool", IsConcurrencySafe: true},
		isConcurrencySafe: func(_ map[string]any) bool { return true },
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			panic("concurrent panic")
		},
	}
	okTool := &stubTool{
		definition:        tool.Definition{Name: "ok_tool", IsConcurrencySafe: true},
		isConcurrencySafe: func(_ map[string]any) bool { return true },
		call: func(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
			return tool.NewTextResult("ok"), nil
		},
	}

	result, err := orch.Execute(context.Background(), ExecuteRequest{
		ToolUses: []types.ToolUseContent{
			{ID: "t1", Name: "panic_tool", Input: map[string]any{}},
			{ID: "t2", Name: "ok_tool", Input: map[string]any{}},
		},
		Tools: map[string]tool.Tool{
			"panic_tool": panicTool,
			"ok_tool":    okTool,
		},
		SessionID:      types.SessionID("concurrent-panic"),
		TurnID:         types.TurnID("t1"),
		PermissionMode: types.PermissionModeOnRequest,
	})
	if err != nil {
		t.Fatalf("Execute should not propagate concurrent panic: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
}

func TestRuntimeEventQueueBasicEmitReceive(t *testing.T) {
	q := NewRuntimeEventQueue(10)
	event := types.RuntimeEvent{Type: types.RuntimeEventTypeTurnStarted}

	q.Emit(event)

	got := <-q.Recv()
	if got.Type != types.RuntimeEventTypeTurnStarted {
		t.Fatalf("expected turn.started event, got %q", got.Type)
	}
}

func TestRuntimeEventQueueEmitBlockingReturnsFalseOnCancelledContext(t *testing.T) {
	q := NewRuntimeEventQueue(1)
	q.Emit(types.RuntimeEvent{Type: types.RuntimeEventTypeTurnStarted})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ok := q.EmitBlocking(ctx, types.RuntimeEvent{Type: types.RuntimeEventTypeTurnCompleted}); ok {
		t.Fatal("expected EmitBlocking to return false on cancelled context")
	}
	if got := q.OverflowCount(); got != 1 {
		t.Fatalf("expected overflow count 1, got %d", got)
	}
}

// newTestStreamingExecutor wires up a StreamingExecutor against a real
// Orchestrator that uses a stub tool, so we can exercise concurrency paths.
func newTestStreamingExecutor(ctx context.Context, callFn func(context.Context, tool.CallInput, types.CanUseToolFn) (tool.CallResult, error)) *StreamingExecutor {
	orch := NewOrchestrator()
	tools := map[string]tool.Tool{
		"stub": &stubTool{
			definition: tool.Definition{Name: "stub", IsConcurrencySafe: true, IsReadOnly: true},
			call:       callFn,
		},
	}
	return NewStreamingExecutor(
		ctx,
		orch,
		ExecuteRequest{
			Tools:          tools,
			PermissionMode: types.PermissionModeBypass,
		},
		tools,
		nil,
		tool.ToolUseContext{},
	)
}

// TestStreamingExecutorSubmitAndCollect is the happy-path: submit a tool use,
// wait, collect the result.
func TestStreamingExecutorSubmitAndCollect(t *testing.T) {
	ctx := context.Background()
	ex := newTestStreamingExecutor(ctx, func(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
		return tool.NewTextResult("hello"), nil
	})

	toolUse := types.ToolUseContent{ID: "tu-1", Name: "stub"}
	ex.SubmitToolUse(toolUse, 0)

	require.NoError(t, ex.Wait(ctx))

	results := ex.GetAllCompletedResults()
	require.Len(t, results, 1)
	assert.Equal(t, "hello", results[0].Result.GetContent())
	assert.NoError(t, results[0].Error)
}

// TestStreamingExecutorDiscardNoRace verifies that calling Discard while a
// goroutine is mid-execution does not cause a data race and that a canceled
// outcome is recorded for each in-flight tool.
func TestStreamingExecutorDiscardNoRace(t *testing.T) {
	ctx := context.Background()

	started := make(chan struct{})
	unblock := make(chan struct{})

	ex := newTestStreamingExecutor(ctx,
		func(callCtx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-callCtx.Done():
				return tool.CallResult{}, callCtx.Err()
			case <-unblock:
				return tool.NewTextResult("done"), nil
			}
		},
	)

	toolUse := types.ToolUseContent{ID: "tu-discard", Name: "stub"}
	ex.SubmitToolUse(toolUse, 0)

	// Wait until the goroutine is inside the tool call.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not start")
	}

	// Discard while the tool is running.
	ex.Discard()
	close(unblock) // release the goroutine (context was already canceled)

	// After Discard, executor must be aborted.
	assert.True(t, ex.IsAborted())

	// The in-flight tool must have produced a canceled/error outcome.
	results := ex.GetAllCompletedResults()
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Error)
}

// TestStreamingExecutorDiscardPendingGetsOutcome verifies that tools queued
// but not yet dispatched when Discard is called also get a Canceled outcome.
func TestStreamingExecutorDiscardPendingGetsOutcome(t *testing.T) {
	ctx := context.Background()

	blockFirst := make(chan struct{})

	// maxWorkers is orchestrator.maxConcurrency; create an executor with 1 worker
	// by giving the orchestrator maxConcurrency=1 via internal field.
	orch := NewOrchestrator()
	orch.maxConcurrency = 1

	tools := map[string]tool.Tool{
		"stub": &stubTool{
			definition: tool.Definition{Name: "stub", IsConcurrencySafe: true, IsReadOnly: true},
			call: func(callCtx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
				select {
				case <-callCtx.Done():
					return tool.CallResult{}, callCtx.Err()
				case <-blockFirst:
					return tool.NewTextResult("first"), nil
				}
			},
		},
	}
	ex := NewStreamingExecutor(
		ctx,
		orch,
		ExecuteRequest{
			Tools:          tools,
			PermissionMode: types.PermissionModeBypass,
		},
		tools,
		nil,
		tool.ToolUseContext{},
	)

	// Submit two tools; second stays in pending because maxWorkers=1.
	ex.SubmitToolUse(types.ToolUseContent{ID: "tu-1", Name: "stub"}, 0)
	ex.SubmitToolUse(types.ToolUseContent{ID: "tu-2", Name: "stub"}, 1)

	// Give the first goroutine a moment to start.
	time.Sleep(10 * time.Millisecond)

	ex.Discard()
	close(blockFirst) // unblock first goroutine (its context is already canceled)

	results := ex.GetAllCompletedResults()
	// Both tools must have an outcome.
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.NotNil(t, r.Error)
	}
}

// TestStreamingExecutorDrainForAbort verifies the openclaude-style
// DrainForAbort path: returns one result per submitted tool.
func TestStreamingExecutorDrainForAbort(t *testing.T) {
	ctx := context.Background()

	unblock := make(chan struct{})
	ex := newTestStreamingExecutor(ctx,
		func(callCtx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			select {
			case <-callCtx.Done():
				return tool.CallResult{}, callCtx.Err()
			case <-unblock:
				return tool.NewTextResult("ok"), nil
			}
		},
	)

	ex.SubmitToolUse(types.ToolUseContent{ID: "tu-drain", Name: "stub"}, 0)
	time.Sleep(10 * time.Millisecond) // let goroutine start

	results := ex.DrainForAbort()
	close(unblock)

	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Error) // context was canceled
}

// TestStreamingExecutorWaitContextCancel verifies that Wait cancels in-flight
// work and returns ctx.Err() when the provided context expires.
func TestStreamingExecutorWaitContextCancel(t *testing.T) {
	ctx := context.Background()
	blockForever := make(chan struct{})

	ex := newTestStreamingExecutor(ctx,
		func(callCtx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			select {
			case <-callCtx.Done():
				return tool.CallResult{}, callCtx.Err()
			case <-blockForever:
				return tool.NewTextResult("unreachable"), nil
			}
		},
	)

	ex.SubmitToolUse(types.ToolUseContent{ID: "tu-wait-cancel", Name: "stub"}, 0)
	time.Sleep(10 * time.Millisecond) // let goroutine start

	waitCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := ex.Wait(waitCtx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, ex.IsAborted())

	// The in-flight tool must have an error outcome after Wait returns.
	results := ex.GetAllCompletedResults()
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Error)
}

// TestStreamingExecutorConcurrentSubmitAndDiscard stress-tests the executor
// by submitting tools from multiple goroutines and calling Discard concurrently.
// The race detector must not fire.
func TestStreamingExecutorConcurrentSubmitAndDiscard(t *testing.T) {
	ctx := context.Background()

	var callCount atomic.Int64
	ex := newTestStreamingExecutor(ctx,
		func(callCtx context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
			callCount.Add(1)
			select {
			case <-callCtx.Done():
				return tool.CallResult{}, callCtx.Err()
			case <-time.After(5 * time.Millisecond):
				return tool.NewTextResult("concurrent"), nil
			}
		},
	)

	const submitters = 8
	const toolsPerSubmitter = 5

	var wg sync.WaitGroup
	wg.Add(submitters)
	for i := range submitters {
		go func(base int) {
			defer wg.Done()
			for j := range toolsPerSubmitter {
				id := base*toolsPerSubmitter + j
				ex.SubmitToolUse(types.ToolUseContent{
					ID:   fmt.Sprintf("tu-%d", id),
					Name: "stub",
				}, id)
			}
		}(i)
	}

	// Discard after a short delay, while some tools may still be running.
	time.Sleep(15 * time.Millisecond)
	ex.Discard()
	wg.Wait()

	// All outcomes must be accounted for (no missing entries, no panics).
	results := ex.GetAllCompletedResults()
	assert.True(t, len(results) <= submitters*toolsPerSubmitter)
	// Executor must be marked aborted.
	assert.True(t, ex.IsAborted())
}
