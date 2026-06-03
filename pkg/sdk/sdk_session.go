package sdk

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/engine"
)

// Session represents a multi-turn conversation session.
type Session struct {
	client  *Client
	session *engine.Session
}

func (s *Session) SubmitMessage(ctx context.Context, content string) (*SessionResponse, error) {
	response, err := s.session.SubmitMessage(ctx, content)
	if err != nil {
		return nil, err
	}
	return &SessionResponse{
		Messages:    response.Messages,
		StopReason:  response.StopReason,
		ToolUses:    response.ToolUses,
		ToolResults: response.ToolResults,
		Usage:       response.Usage,
		TotalTokens: response.TotalTokens,
		TurnNumber:  response.TurnNumber,
		IsComplete:  response.IsComplete(),
		Compacted:   response.Compacted,
	}, nil
}

func (s *Session) SubmitMessageWithContent(ctx context.Context, text string, images []ImageContent) (*SessionResponse, error) {
	response, err := s.session.SubmitMessageWithContent(ctx, text, images)
	if err != nil {
		return nil, err
	}
	return &SessionResponse{
		Messages:    response.Messages,
		StopReason:  response.StopReason,
		ToolUses:    response.ToolUses,
		ToolResults: response.ToolResults,
		Usage:       response.Usage,
		TotalTokens: response.TotalTokens,
		TurnNumber:  response.TurnNumber,
		IsComplete:  response.IsComplete(),
		Compacted:   response.Compacted,
	}, nil
}

func (s *Session) RegisterTool(tool Tool) error {
	return s.session.RegisterTool(tool)
}

func (s *Session) UnregisterTool(name string) error {
	if s == nil || s.session == nil {
		return nil
	}
	return s.session.UnregisterTool(name)
}

func (s *Session) GetID() SessionID {
	return s.session.GetMetadata().ID
}

func (s *Session) GetMessages() []Message {
	return s.session.GetMessages()
}

func (s *Session) Close() error {
	return s.session.Close()
}

func (s *Session) Interrupt() error {
	return s.session.Interrupt()
}

func (s *Session) GetStatus() SessionStatus {
	metadata := s.session.GetMetadata()
	if metadata == nil {
		return ""
	}
	return metadata.Status
}

func (s *Session) GetMetadata() *SessionMetadata {
	return s.session.GetMetadata()
}

func (s *Session) GetPermissionMode() PermissionMode {
	return s.session.GetPermissionMode()
}

func (s *Session) GetExecutionMode() ExecutionMode {
	return ExecutionMode(s.session.GetExecutionMode())
}

func (s *Session) GetPermissionContext() *PermissionContext {
	return s.session.GetPermissionContext()
}

func (s *Session) SetPermissionMode(mode PermissionMode) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetPermissionMode(mode)
}

func (s *Session) SetSystemPromptTemplate(text string) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetSystemPromptTemplate(text)
}

func (s *Session) SetAppendSystemPrompt(text string) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetAppendSystemPrompt(text)
}

func (s *Session) SetWorkingDirectory(path string) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetWorkingDirectory(path)
}

func (s *Session) GetTurnNumber() int {
	if s == nil || s.session == nil {
		return 0
	}
	return s.session.GetTurnNumber()
}

func (s *Session) GetTotalTokens() int {
	if s == nil || s.session == nil {
		return 0
	}
	return s.session.GetTotalTokens()
}

func (s *Session) GetToolNames() []string {
	if s == nil || s.session == nil {
		return nil
	}
	return s.session.GetToolNames()
}

func (s *Session) SetProgressFn(progressFn func(ToolProgress)) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetProgressCallback(progressFn)
}

func (s *Session) SetResponseChunkFn(chunkFn func(ResponseChunk)) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetResponseChunkCallback(chunkFn)
}

func (s *Session) SetRuntimeEventFn(runtimeEventFn func(RuntimeEvent)) {
	if s == nil || s.session == nil {
		return
	}
	s.session.SetRuntimeEventCallback(runtimeEventFn)
}

func (s *Session) GetEventQueue() *EventQueue {
	return s.session.GetEventQueue()
}

func (s *Session) GetRuntimeEventQueue() *RuntimeEventQueue {
	return s.session.GetRuntimeEventQueue()
}

// newSDKSession wraps an engine session with the client's default callbacks.
func newSDKSession(c *Client, querySession *engine.Session) *Session {
	s := &Session{client: c, session: querySession}
	s.SetProgressFn(c.config.ProgressFn)
	s.SetResponseChunkFn(c.config.ResponseChunkFn)
	s.SetRuntimeEventFn(c.config.RuntimeEventFn)
	return s
}
