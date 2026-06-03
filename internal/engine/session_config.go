package engine

import (
	"fmt"
	"sort"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// --- callbacks ---

// SetProgressCallback wires a host-level observer for tool execution progress.
func (s *Session) SetProgressCallback(fn func(types.ToolProgress)) {
	if s == nil {
		return
	}
	s.progressCallback = fn
}

// SetResponseChunkCallback wires a host-level observer for live model chunks.
func (s *Session) SetResponseChunkCallback(fn func(types.APIResponseChunk)) {
	if s == nil {
		return
	}
	s.responseChunkCallback = fn
}

// SetRuntimeEventCallback wires a host-level observer for structured runtime events.
func (s *Session) SetRuntimeEventCallback(fn func(types.RuntimeEvent)) {
	if s == nil {
		return
	}
	s.runtimeEventCallback = fn
}

// --- queues ---

// GetEventQueue returns the session's streaming event queue.
func (s *Session) GetEventQueue() *execution.EventQueue {
	return s.eventQueue
}

// GetRuntimeEventQueue returns the session's structured runtime event queue.
func (s *Session) GetRuntimeEventQueue() *execution.RuntimeEventQueue {
	return s.runtimeEventQueue
}

// --- tool registration ---

func (s *Session) RegisterTool(t tool.Tool) error {
	def := t.Definition()
	if promptAware, ok := t.(promptAwareTool); ok {
		promptAware.SetPromptFn(s.engine.promptFn)
	}

	if _, exists := s.state.Tools[def.Name]; exists {
		return fmt.Errorf("tool '%s' already registered", def.Name)
	}
	s.state.Tools[def.Name] = t

	for _, alias := range def.Aliases {
		if _, exists := s.state.Tools[alias]; exists {
			return fmt.Errorf("tool alias '%s' conflicts with existing tool", alias)
		}
		s.state.Tools[alias] = t
	}
	return nil
}

func (s *Session) RegisterTools(tools []tool.Tool) error {
	for _, t := range tools {
		if err := s.RegisterTool(t); err != nil {
			return err
		}
	}
	return nil
}

// UnregisterTool removes a tool from the session by name.
func (s *Session) UnregisterTool(name string) error {
	if s == nil || s.state == nil {
		return fmt.Errorf("session is nil")
	}
	if _, ok := s.state.Tools[name]; !ok {
		return fmt.Errorf("tool %q not found", name)
	}
	delete(s.state.Tools, name)
	return nil
}

// --- metadata accessors ---

func (s *Session) GetMetadata() *types.SessionMetadata {
	return s.state.MetadataSnapshot()
}

func (s *Session) GetPermissionMode() types.PermissionMode {
	return s.state.CurrentPermissionMode()
}

func (s *Session) GetExecutionMode() string {
	return s.state.CurrentExecutionMode()
}

func (s *Session) GetPermissionContext() *types.PermissionContext {
	return s.state.PermissionContextSnapshot()
}

func (s *Session) GetMessages() []types.Message {
	return s.state.CloneMessages()
}

func (s *Session) GetTurnNumber() int {
	if s == nil || s.state == nil {
		return 0
	}
	return s.state.TurnNumber
}

func (s *Session) GetTotalTokens() int {
	if s == nil || s.state == nil {
		return 0
	}
	return s.state.TotalTokens
}

func (s *Session) GetToolNames() []string {
	if s == nil || s.state == nil || len(s.state.Tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.state.Tools))
	for name := range s.state.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// --- setters ---

func (s *Session) SetPermissionMode(mode types.PermissionMode) {
	if s == nil || s.state == nil {
		return
	}
	ctx := s.state.PermissionContextSnapshot()
	if ctx == nil {
		ctx = &types.PermissionContext{}
	}
	ctx.Mode = mode
	s.state.SetPermissionContext(ctx)
}

// SetSystemPromptTemplate sets a per-session override that fully replaces the
// default system prompt. Pass an empty string to clear the override.
func (s *Session) SetSystemPromptTemplate(text string) {
	if s == nil {
		return
	}
	if text == "" {
		s.systemPromptTemplateOverride = nil
	} else {
		s.systemPromptTemplateOverride = &text
	}
}

// SetAppendSystemPrompt sets a per-session override for the append system prompt.
func (s *Session) SetAppendSystemPrompt(text string) {
	if s == nil {
		return
	}
	if text == "" {
		s.appendSystemPromptOverride = nil
	} else {
		s.appendSystemPromptOverride = &text
	}
}

// SetWorkingDirectory overrides the working directory for this session's turns.
func (s *Session) SetWorkingDirectory(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if path == "" {
		s.workingDirectoryOverride = nil
	} else {
		s.workingDirectoryOverride = &path
	}
}

func (s *Session) currentTurnNumber() int {
	if s == nil || s.state == nil {
		return 0
	}
	return s.state.TurnNumber + 1
}
