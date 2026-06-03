package execution

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type ToolUseChunk struct {
	ToolUse   types.ToolUseContent
	Index     int
	IsPartial bool
}

type StreamingExecutor struct {
	orchestrator *Orchestrator
	req          ExecuteRequest
	tools        map[string]tool.Tool
	canUseTool   types.CanUseToolFn
	context      tool.ToolUseContext
	ctx          context.Context
	cancel       context.CancelFunc

	mu         sync.Mutex
	wg         sync.WaitGroup
	pending    []ToolUseChunk
	completed  []toolExecutionOutcome
	running    map[string]struct{}
	aborted    atomic.Bool
	maxWorkers int
}

func NewStreamingExecutor(
	ctx context.Context,
	orchestrator *Orchestrator,
	req ExecuteRequest,
	tools map[string]tool.Tool,
	canUseTool types.CanUseToolFn,
	toolUseContext tool.ToolUseContext,
) *StreamingExecutor {
	ctx, cancel := context.WithCancel(ctx)
	return &StreamingExecutor{
		orchestrator: orchestrator,
		req:          req,
		tools:        tools,
		canUseTool:   canUseTool,
		context:      toolUseContext,
		ctx:          ctx,
		cancel:       cancel,
		running:      make(map[string]struct{}),
		maxWorkers:   orchestrator.maxConcurrency,
	}
}

func (s *StreamingExecutor) SubmitToolUse(toolUse types.ToolUseContent, index int) {
	s.mu.Lock()
	s.pending = append(s.pending, ToolUseChunk{
		ToolUse:   toolUse,
		Index:     index,
		IsPartial: false,
	})
	s.mu.Unlock()
	s.dispatch()
}

func (s *StreamingExecutor) SubmitToolUseChunk(chunk ToolUseChunk) {
	s.mu.Lock()
	if chunk.IsPartial {
		for i := range s.pending {
			if s.pending[i].ToolUse.ID == chunk.ToolUse.ID {
				s.pending[i] = chunk
				s.mu.Unlock()
				// partial update — do not dispatch; tool input is not yet complete
				return
			}
		}
	}
	s.pending = append(s.pending, chunk)
	s.mu.Unlock()
	s.dispatch()
}

// dispatch drains the pending queue up to the worker cap.
// Safe to call from any goroutine; acquires its own lock.
func (s *StreamingExecutor) dispatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatchLocked()
}

// dispatchLocked must be called with s.mu held.
func (s *StreamingExecutor) dispatchLocked() {
	for !s.aborted.Load() && len(s.pending) > 0 && len(s.running) < s.maxWorkers {
		chunk := s.pending[0]
		s.pending = s.pending[1:]

		if _, exists := s.running[chunk.ToolUse.ID]; exists {
			continue
		}

		s.running[chunk.ToolUse.ID] = struct{}{}
		s.wg.Add(1)
		go s.executeToolAsync(chunk.ToolUse, chunk.Index)
	}
}

func (s *StreamingExecutor) executeToolAsync(toolUse types.ToolUseContent, index int) {
	// cleanup runs before wg.Done (LIFO) so that running is always up-to-date
	// before wg.Wait returns in Discard / Wait.
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.running, toolUse.ID)
		s.mu.Unlock()
		s.dispatch()
	}()

	if s.aborted.Load() {
		s.mu.Lock()
		s.completed = append(s.completed, toolExecutionOutcome{
			ToolUse:  toolUse,
			Index:    index,
			Error:    context.Canceled,
			Messages: []types.Message{},
		})
		s.mu.Unlock()
		return
	}

	execReq := s.req
	execReq.ToolUses = []types.ToolUseContent{toolUse}

	result, err := s.orchestrator.Execute(s.ctx, execReq)
	outcome := toolExecutionOutcome{
		ToolUse: toolUse,
		Index:   index,
		Result:  tool.CallResult{},
	}

	if err != nil {
		outcome.Result = tool.NewErrorResult(err)
		outcome.Error = err
	} else if len(result.Results) > 0 {
		outcome.Result = result.Results[0]
		if trim := 1 + len(outcome.Result.NewMessages); trim < len(result.Messages) {
			outcome.Messages = append([]types.Message(nil), result.Messages[trim:]...)
		}
		if len(result.Traces) > 0 {
			outcome.Trace = cloneTrace(result.Traces[0])
		}
		outcome.Progress = append([]types.ToolProgress(nil), result.ProgressUpdates...)
		if len(result.Errors) > 0 {
			outcome.Error = result.Errors[0].Error
			outcome.ErrorStage = result.Errors[0].Stage
		}
	} else {
		err = fmt.Errorf("streaming executor received no tool result for %s", toolUse.Name)
		outcome.Result = tool.NewErrorResult(err)
		outcome.Error = err
		outcome.ErrorStage = ErrorStageExecution
	}

	s.mu.Lock()
	s.completed = append(s.completed, outcome)
	s.mu.Unlock()
}

func (s *StreamingExecutor) GetCompletedResults() []toolExecutionOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	completed := make([]toolExecutionOutcome, len(s.completed))
	copy(completed, s.completed)
	s.completed = s.completed[:0]

	return completed
}

func (s *StreamingExecutor) GetAllCompleted() []toolExecutionOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	completed := make([]toolExecutionOutcome, len(s.completed))
	copy(completed, s.completed)

	return completed
}

func (s *StreamingExecutor) GetAllCompletedResults() []StreamingExecutionResult {
	outcomes := s.GetAllCompleted()
	results := make([]StreamingExecutionResult, 0, len(outcomes))
	for _, outcome := range outcomes {
		results = append(results, StreamingExecutionResult{
			ToolUse:    outcome.ToolUse,
			Index:      outcome.Index,
			Result:     outcome.Result,
			Messages:   append([]types.Message(nil), outcome.Messages...),
			Error:      outcome.Error,
			ErrorStage: outcome.ErrorStage,
			Progress:   append([]types.ToolProgress(nil), outcome.Progress...),
			Trace:      cloneTrace(outcome.Trace),
		})
	}
	return results
}

func (s *StreamingExecutor) GetPendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.pending) + len(s.running)
}

// Wait blocks until all submitted tools complete or ctx is canceled.
// On cancellation Discard is called and ctx.Err() is returned, giving callers
// the same "drain for abort" semantics as openclaude's getRemainingResults.
func (s *StreamingExecutor) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.Discard()
		return ctx.Err()
	}
}

// Discard cancels the derived context (so in-flight Execute calls return
// promptly), marks the executor aborted, waits for all goroutines to exit,
// and records a Canceled outcome for every tool that never got to run.
// After Discard returns, GetAllCompletedResults contains one entry per tool
// that was submitted — either a real result or a Canceled error.
func (s *StreamingExecutor) Discard() {
	// Cancel derived context first so in-flight Execute calls unblock quickly.
	s.cancel()

	s.mu.Lock()
	s.aborted.Store(true)
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()

	// Wait for every in-flight goroutine; each one writes a canceled outcome
	// before calling wg.Done, so s.running is empty when Wait returns.
	s.wg.Wait()

	// Record canceled outcomes for tools that were queued but never dispatched.
	if len(pending) > 0 {
		s.mu.Lock()
		for _, chunk := range pending {
			s.completed = append(s.completed, toolExecutionOutcome{
				ToolUse:  chunk.ToolUse,
				Index:    chunk.Index,
				Error:    context.Canceled,
				Messages: []types.Message{},
			})
		}
		s.mu.Unlock()
	}
}

// DrainForAbort mirrors openclaude's getRemainingResults: it aborts all
// in-flight and pending tools and returns one StreamingExecutionResult per
// submitted tool — real result if the tool finished before the abort, or a
// Canceled error for anything still in-flight. Callers use this to build
// synthetic tool_result messages that keep the transcript consistent.
func (s *StreamingExecutor) DrainForAbort() []StreamingExecutionResult {
	s.Discard()
	return s.GetAllCompletedResults()
}

func (s *StreamingExecutor) IsAborted() bool {
	return s.aborted.Load()
}
