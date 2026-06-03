package engine

import "github.com/EngineerProjects/nexus-engine/internal/types"

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
		s.responseChunkCallback(chunk)
	}
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
