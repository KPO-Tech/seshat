package engine

import "github.com/KPO-Tech/seshat/internal/types"

func (s *Session) persistSessionState(previousMessages []types.Message) error {
	if s.store == nil || s.state == nil || s.state.Metadata == nil {
		return nil
	}
	return s.store.SaveSessionState(s.state.SessionID, s.state.MetadataSnapshot(), previousMessages, s.state.CloneMessages())
}

func (s *Session) handleProgressCallback(progress types.ToolProgress) {
	if s == nil {
		return
	}
	if s.progressCallback != nil {
		s.progressCallback(progress)
	}
	cloned := cloneToolProgress(progress)
	s.emitRuntimeEvent(types.RuntimeEvent{
		Type:         types.RuntimeEventTypeToolProgress,
		SessionID:    s.state.SessionID,
		TurnID:       s.state.TurnID,
		TurnNumber:   s.currentTurnNumber(),
		ToolProgress: &cloned,
	})
	if browserEvent := browserRuntimeEventFromProgress(cloned); browserEvent != nil {
		browserEvent.SessionID = s.state.SessionID
		browserEvent.TurnID = s.state.TurnID
		browserEvent.TurnNumber = s.currentTurnNumber()
		s.emitRuntimeEvent(*browserEvent)
	}
}

func (s *Session) handleResponseChunkCallback(chunk types.APIResponseChunk) {
	if s == nil {
		return
	}
	if s.responseChunkCallback != nil {
		// The top-level session (seshat-backend's /query/stream handler) wires
		// this callback to write each chunk as a raw, unnamed SSE line — see
		// query.go's onChunk. Wrapping the same chunk in a
		// response.chunk runtime event on top of that would just double the
		// bytes on the wire for a copy the frontend already ignores for the
		// main turn (no case for it in useChat.ts's top-level switch — see
		// handleSubagentEvent below for where it IS the primary channel).
		s.responseChunkCallback(chunk)
		return
	}
	// Sub-agent sessions never set responseChunkCallback (see
	// RunAgent/RunConfig.EventFn in internal/agent/runner.go) — for them,
	// this runtime event is the only channel streamed content reaches the
	// frontend through, tagged with AgentToolUseID by the spawning tool
	// (spawn_agent.go / agent_tool.go) so useChat.ts can route it to the
	// right sub-agent card.
	cloned := cloneAPIResponseChunk(chunk)
	s.emitRuntimeEvent(types.RuntimeEvent{
		Type:       types.RuntimeEventTypeResponseChunk,
		SessionID:  s.state.SessionID,
		TurnID:     s.state.TurnID,
		TurnNumber: s.currentTurnNumber(),
		Chunk:      &cloned,
	})
}

func (s *Session) emitRuntimeEvent(event types.RuntimeEvent) {
	if s == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = currentTime().UTC()
	}
	if s.runtimeEventCallback != nil {
		s.runtimeEventCallback(event)
	}
	if s.runtimeEventQueue != nil {
		s.runtimeEventQueue.Emit(event)
	}
}

// --- clone helpers ---

func cloneTokenUsage(usage *types.TokenUsage) *types.TokenUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func cloneToolProgress(progress types.ToolProgress) types.ToolProgress {
	cloned := progress
	if len(progress.Metadata) > 0 {
		cloned.Metadata = cloneRuntimeEventMetadata(progress.Metadata)
	}
	return cloned
}

func cloneAPIResponseChunk(chunk types.APIResponseChunk) types.APIResponseChunk {
	cloned := chunk
	if chunk.StopReason != nil {
		stopReason := *chunk.StopReason
		cloned.StopReason = &stopReason
	}
	if chunk.StopSequence != nil {
		stopSequence := *chunk.StopSequence
		cloned.StopSequence = &stopSequence
	}
	cloned.Usage = cloneTokenUsage(chunk.Usage)
	return cloned
}

func cloneRuntimeEventMetadata(metadata map[string]any) map[string]any {
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
