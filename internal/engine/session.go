package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Session represents an active query session.
type Session struct {
	engine *Engine
	state  *SessionState
	store  SessionStore
	config *Config

	progressCallback      func(types.ToolProgress)
	responseChunkCallback func(types.APIResponseChunk)
	runtimeEventCallback  func(types.RuntimeEvent)

	eventQueue        *execution.EventQueue
	runtimeEventQueue *execution.RuntimeEventQueue

	// Per-session system prompt overrides.
	appendSystemPromptOverride   *string
	systemPromptTemplateOverride *string
	workingDirectoryOverride     *string

	// mu guards cancelFn.
	mu sync.Mutex
	// cancelFn cancels the context passed to the current Loop.Run.
	cancelFn context.CancelFunc
}

// NewSession creates a new session.
func (e *Engine) NewSession(ctx context.Context) (*Session, error) {
	sessionID := types.NewSessionID(generateID())
	return e.NewSessionFromState(ctx, sessionID, nil, nil)
}

// NewSessionFromState creates a session from existing state or initializes a new one.
func (e *Engine) NewSessionFromState(
	ctx context.Context,
	sessionID types.SessionID,
	metadata *types.SessionMetadata,
	messages []types.Message,
) (*Session, error) {
	// createdFresh is true when creating a brand-new session:
	// - metadata is nil (classic NewSession path), OR
	// - sessionID is empty (SDK CreateSession path: metadata with Title but no ID).
	// Both cases require an immediate store write so LoadSession works before
	// the first SubmitMessage.
	createdFresh := metadata == nil || sessionID == ""
	if sessionID == "" {
		sessionID = types.NewSessionID(generateID())
	}
	if metadata == nil {
		metadata = &types.SessionMetadata{
			ID:            sessionID,
			Status:        types.SessionStatusActive,
			CreatedAt:     currentTime(),
			UpdatedAt:     currentTime(),
			Model:         e.config.Model.String(),
			TotalTurns:    0,
			TotalTokens:   0,
			MaxTokens:     e.config.MaxTokens,
			SchemaVersion: types.SessionMetadataSchemaVersion,
		}
	} else {
		migrateSessionMetadata(metadata)
		metadata.ID = sessionID
		if metadata.Status == "" {
			metadata.Status = types.SessionStatusActive
		}
		if metadata.UpdatedAt.IsZero() {
			metadata.UpdatedAt = currentTime()
		}
		if metadata.Model == "" {
			metadata.Model = e.config.Model.String()
		}
		if metadata.MaxTokens == 0 {
			metadata.MaxTokens = e.config.MaxTokens
		}
	}
	if metadata.RootPath == "" {
		metadata.RootPath = e.workingDirectory()
	}

	if e.memoryService != nil {
		if err := e.memoryService.LoadProject(metadata.RootPath); err != nil {
			slog.Warn("failed to load project memory", "error", err)
		}
		if err := e.memoryService.LoadUser(); err != nil {
			slog.Warn("failed to load user memory", "error", err)
		}
		if err := e.memoryService.LoadCrossSession(); err != nil {
			slog.Warn("failed to load cross-session memory", "error", err)
		}
	}

	surfaceBuilder := tool.NewSurfaceBuilder(e.toolRegistry)
	tools, err := surfaceBuilder.BuildToolMap(ctx, tool.SurfaceBuildRequest{
		IncludeReadOnly:    true,
		IncludeDestructive: true,
		SurfaceProfile:     sessionToolSurfaceProfile(metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build tool surface: %w", err)
	}

	messageCopy := append([]types.Message(nil), messages...)
	turnNumber := metadata.TotalTurns
	if turnNumber < 0 {
		turnNumber = 0
	}
	turnID := nextTurnID(messageCopy)

	sessionState := NewSessionState(sessionID, turnID, messageCopy, tools, turnNumber, metadata.TotalTokens, metadata)
	sessionState.SetPermissionContext(normalizePermissionContext(sessionState.PermissionContextSnapshot(), e.defaultPermissionContext()))

	session := &Session{
		engine:            e,
		state:             sessionState,
		store:             e.sessionStore,
		config:            e.config,
		eventQueue:        execution.NewEventQueue(execution.DefaultEventQueueCapacity),
		runtimeEventQueue: execution.NewRuntimeEventQueue(execution.DefaultRuntimeEventQueueCapacity),
	}

	// createdFresh is true when the caller is creating a brand-new session:
	// - metadata was nil (classic NewSession path), OR
	// - sessionID was empty (SDK path that passes pre-built metadata but no ID).
	// In both cases the store entry must be written immediately so that
	// LoadSession can succeed before the first SubmitMessage.
	if createdFresh && e.sessionStore != nil {
		if err := e.sessionStore.SaveSessionState(sessionID, metadata, nil, messageCopy); err != nil {
			return nil, fmt.Errorf("failed to persist new session: %w", err)
		}
	}

	return session, nil
}

// SubmitMessage submits a plain-text user message to the session.
func (s *Session) SubmitMessage(ctx context.Context, content string) (*SessionResponse, error) {
	msg := types.UserMessage(fmt.Sprintf("msg-%d", len(s.state.Messages)+1), content)
	return s.submitWithMessage(ctx, msg, content)
}

// SubmitMessageWithContent submits a user message that may include image content blocks.
func (s *Session) SubmitMessageWithContent(ctx context.Context, text string, images []types.ImageContent) (*SessionResponse, error) {
	var msg types.Message
	if len(images) > 0 {
		msg = types.UserMessageWithImage(fmt.Sprintf("msg-%d", len(s.state.Messages)+1), text, images...)
	} else {
		msg = types.UserMessage(fmt.Sprintf("msg-%d", len(s.state.Messages)+1), text)
	}
	return s.submitWithMessage(ctx, msg, text)
}

// submitWithMessage is the shared implementation for SubmitMessage and SubmitMessageWithContent.
func (s *Session) submitWithMessage(ctx context.Context, userMsg types.Message, text string) (*SessionResponse, error) {
	if err := s.enforceMaxTurns(); err != nil {
		return nil, err
	}

	userMsg.Timestamp = currentTime().UTC()
	userMsg.Metadata = &types.MessageMetadata{
		TurnID: s.state.TurnID.String(),
	}

	previousMessages := s.state.CloneMessages()
	s.state.Messages = append(s.state.Messages, userMsg)
	if s.state.Metadata != nil {
		s.state.Metadata.UpdatedAt = currentTime()
	}
	persistedMessages := s.state.CloneMessages()
	if err := s.persistSessionState(previousMessages); err != nil {
		return nil, fmt.Errorf("failed to persist accepted user input: %w", err)
	}
	s.rememberUserDirectives(text)
	s.emitRuntimeEvent(types.RuntimeEvent{
		Type:          types.RuntimeEventTypeTurnStarted,
		SessionID:     s.state.SessionID,
		TurnID:        s.state.TurnID,
		TurnNumber:    s.currentTurnNumber(),
		ExecutionMode: s.state.CurrentExecutionMode(),
	})

	apiReq, err := s.buildAPIRequest(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build API request: %w", err)
	}
	runtimeTools := s.state.EffectiveToolSurface(s.engine.toolRegistry)

	permissionContext := s.state.PermissionContextSnapshot()
	loopReq := RunRequest{
		Messages:              s.state.CloneMessages(),
		SystemPrompt:          apiReq.SystemPrompt,
		SystemPromptBlocks:    apiReq.SystemPromptBlocks,
		ProviderTools:         apiReq.Tools,
		Tools:                 runtimeTools,
		ToolRegistry:          s.engine.toolRegistry,
		RefreshSystemPrompt:   s.refreshSystemPrompt,
		AutoDetectStage:       s.config.PromptStage == prompt.StageDefault,
		SessionID:             s.state.SessionID,
		TurnID:                s.state.TurnID,
		WorkingDirectory:      s.workingDirectory(),
		PermissionMode:        permissionContext.Mode,
		PermissionContext:     permissionContext,
		Model:                 s.config.Model,
		MaxTokens:             s.config.MaxTokens,
		ProgressCallback:      s.handleProgressCallback,
		ResponseChunkCallback: s.handleResponseChunkCallback,
		EventQueue:            s.eventQueue,
		DenialTracking:        s.state.DenialTrackingState(),
	}

	turnCtx, cancel := context.WithCancel(ctx)
	emitter := func(event types.RuntimeEvent) {
		s.emitRuntimeEvent(event)
	}
	turnCtx = context.WithValue(turnCtx, types.RuntimeEventEmitterKey, emitter)
	s.mu.Lock()
	s.cancelFn = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelFn = nil
		s.mu.Unlock()
		cancel()
	}()

	loopResult := s.engine.loop.Run(turnCtx, loopReq)
	if loopResult.Error != nil {
		s.emitRuntimeEvent(types.RuntimeEvent{
			Type:       types.RuntimeEventTypeTurnFailed,
			SessionID:  s.state.SessionID,
			TurnID:     s.state.TurnID,
			TurnNumber: s.currentTurnNumber(),
			Error:      loopResult.Error.Error(),
		})
		if loopResult.RecoveryContext != nil {
			s.state.StoreRecoveryContext(loopResult.RecoveryContext)
		}
		if len(loopResult.Messages) > len(s.state.Messages) {
			s.state.AdvanceTurn(loopResult.Usage, loopResult.Messages)
			_ = s.persistSessionState(persistedMessages)
		}
		return nil, fmt.Errorf("query loop failed: %w", loopResult.Error)
	}
	s.state.RegisterDiscoveredDeferredTools(loopResult.DiscoveredDeferred)
	if loopResult.PermissionContext != nil {
		s.state.SetPermissionContext(loopResult.PermissionContext)
	}
	if loopResult.RecoveryContext != nil {
		s.state.StoreRecoveryContext(loopResult.RecoveryContext)
	}

	s.state.AdvanceTurn(loopResult.Usage, loopResult.Messages)
	if err := s.persistSessionState(persistedMessages); err != nil {
		return nil, fmt.Errorf("failed to persist session state after turn: %w", err)
	}
	s.rememberToolUsage(loopResult.ToolUses, loopResult.ToolResults)

	// After the very first completed turn, kick off async title generation.
	// We capture the first user message from persistedMessages (which includes
	// it) so we don't have to scan the growing messages slice later.
	if s.state.Metadata != nil && s.state.Metadata.TotalTurns == 1 {
		if firstText := firstUserMessageText(persistedMessages); firstText != "" {
			sid := s.state.SessionID
			go s.engine.generateTitleAsync(sid, firstText)
		}
	}

	response := &SessionResponse{
		Messages:    s.state.CloneMessages(),
		StopReason:  loopResult.StopReason,
		ToolUses:    loopResult.ToolUses,
		ToolResults: loopResult.ToolResults,
		Usage:       loopResult.Usage,
		TotalTokens: s.state.TotalTokens,
		TurnNumber:  s.state.TurnNumber,
		Compacted:   loopResult.Compacted,
	}
	s.emitRuntimeEvent(types.RuntimeEvent{
		Type:          types.RuntimeEventTypeTurnCompleted,
		SessionID:     s.state.SessionID,
		TurnID:        s.state.TurnID,
		TurnNumber:    response.TurnNumber,
		StopReason:    response.StopReason,
		ExecutionMode: s.state.CurrentExecutionMode(),
		Usage:         cloneTokenUsage(response.Usage),
	})

	return response, nil
}

// Interrupt cancels the current turn and marks the session as interrupted.
func (s *Session) Interrupt() error {
	s.mu.Lock()
	fn := s.cancelFn
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
	s.state.MarkInterrupted()
	messages := s.state.CloneMessages()
	return s.persistSessionState(messages)
}

// Close closes the session, persists memory, and releases the browser session.
func (s *Session) Close() error {
	if s.eventQueue != nil {
		s.eventQueue.Close()
	}
	if s.runtimeEventQueue != nil {
		s.runtimeEventQueue.Close()
	}
	s.state.MarkClosed()
	messages := s.state.CloneMessages()

	if s.engine.memoryService != nil {
		s.rememberSessionSummary()
		if err := s.engine.memoryService.SaveProject(); err != nil {
			slog.Warn("failed to save project memory", "error", err)
		}
		if err := s.engine.memoryService.SaveUser(); err != nil {
			slog.Warn("failed to save user memory", "error", err)
		}
		if err := s.engine.memoryService.SaveCrossSession(); err != nil {
			slog.Warn("failed to save cross-session memory", "error", err)
		}
	}

	if s.engine != nil && s.engine.browserManager != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.engine.browserManager.CloseSession(closeCtx, s.state.SessionID)
		cancel()
	}

	return s.persistSessionState(messages)
}

// GetSessionID returns the stable identifier for this session.
func (s *Session) GetSessionID() types.SessionID {
	return s.state.SessionID
}

func (s *Session) enforceMaxTurns() error {
	if s == nil || s.config == nil {
		return nil
	}
	maxTurns := s.config.MaxTurns
	if maxTurns <= 0 {
		return nil
	}
	if s.state.TurnNumber >= maxTurns {
		return fmt.Errorf("session turn limit reached: completed %d turns (max %d)", s.state.TurnNumber, maxTurns)
	}
	return nil
}

// firstUserMessageText returns the text of the first user message in msgs,
// truncated to 500 runes. Returns "" if no user message is found.
func firstUserMessageText(msgs []types.Message) string {
	for _, msg := range msgs {
		if msg.Role != types.RoleUser {
			continue
		}
		for _, block := range msg.Content {
			if t, ok := block.(types.TextContent); ok && t.Text != "" {
				runes := []rune(t.Text)
				if len(runes) > 500 {
					return string(runes[:500])
				}
				return t.Text
			}
		}
	}
	return ""
}
