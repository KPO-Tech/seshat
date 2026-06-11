// Package workspace implements the Workspace interface backed by pkg/sdk.
//
// NexusWorkspace is the active implementation (Option A — SDK-only path).
// All LLM traffic flows through pkg/sdk.Client; the Fantasy-based agent
// package (internal/nexustui/agent/) is NOT used here.
package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	tuiTools "github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools"
	mcptools "github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools/mcp"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/config"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/csync"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/history"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/lsp"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/oauth"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/permission"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/session"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/skills"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	"github.com/google/uuid"
)

// Verify compile-time that NexusWorkspace implements Workspace.
var _ Workspace = (*NexusWorkspace)(nil)

// NexusWorkspace adapts the nexus-engine SDK to the Workspace interface.
// It keeps an in-memory message store and publishes pubsub events that the
// nexustui UI consumes to render the conversation.
type NexusWorkspace struct {
	// SDK layer
	client  *sdk.Client
	workDir string
	model   string // "provider:model" string

	// Active session
	sessMu  sync.Mutex
	session *sdk.Session

	// In-memory stores (sessionID → records)
	msgMu    sync.RWMutex
	msgStore map[string][]message.Message // complete message list per session

	sessBroker  *pubsub.Broker[session.Session]
	msgBroker   *pubsub.Broker[message.Message]
	permBroker  *pubsub.Broker[permission.PermissionRequest]
	sessStore   map[string]session.Session
	sessionsMu  sync.RWMutex

	// tea.Program for Subscribe
	programMu sync.Mutex
	program   *tea.Program

	// streaming state (single active assistant message)
	streamMu   sync.Mutex
	streamMsg  *message.Message // the in-progress assistant message
	streamSess string           // session ID of the streaming message

	// debounce for streaming updates (33 ms)
	debounce *msgDebounce

	// busy flag
	busy atomic.Bool

	// cancel for the active submit goroutine
	submitMu     sync.Mutex
	submitCancel context.CancelFunc

	// Config (lazily built, mutex-guarded)
	cfgMu  sync.Mutex
	cfg    *config.Config

	// Permission: allow-all skip flag
	permSkip atomic.Bool
}

// ─── Constructor ──────────────────────────────────────────────────────────────

// NewNexusWorkspace creates a workspace backed by the given SDK client.
// modelStr is the "provider:model" string shown in the UI header.
func NewNexusWorkspace(client *sdk.Client, workDir, modelStr string) *NexusWorkspace {
	w := &NexusWorkspace{
		client:    client,
		workDir:   workDir,
		model:     modelStr,
		msgStore:  make(map[string][]message.Message),
		sessStore: make(map[string]session.Session),
		sessBroker: pubsub.NewBroker[session.Session](),
		msgBroker:  pubsub.NewBroker[message.Message](),
		permBroker: pubsub.NewBroker[permission.PermissionRequest](),
	}
	w.debounce = newMsgDebounce(33*time.Millisecond, func(msg message.Message, sessID string) {
		w.publishMsg(pubsub.UpdatedEvent, sessID, msg)
	})
	return w
}

// ─── Subscribe / event fan-out ────────────────────────────────────────────────

func (w *NexusWorkspace) Subscribe(p *tea.Program) {
	w.programMu.Lock()
	w.program = p
	w.programMu.Unlock()

	ctx := context.Background()

	// Fan out session events.
	go func() {
		ch := w.sessBroker.Subscribe(ctx)
		for ev := range ch {
			p.Send(ev)
		}
	}()

	// Fan out message events.
	go func() {
		ch := w.msgBroker.Subscribe(ctx)
		for ev := range ch {
			p.Send(ev)
		}
	}()

	// Fan out permission events.
	go func() {
		ch := w.permBroker.Subscribe(ctx)
		for ev := range ch {
			p.Send(ev)
		}
	}()
}

func (w *NexusWorkspace) Shutdown() {
	w.sessBroker.Shutdown()
	w.msgBroker.Shutdown()
	w.permBroker.Shutdown()
	if w.client != nil {
		_ = w.client.Close()
	}
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

func (w *NexusWorkspace) CreateSession(ctx context.Context, title string) (session.Session, error) {
	sess, err := w.client.CreateSession(ctx)
	if err != nil {
		return session.Session{}, fmt.Errorf("create session: %w", err)
	}
	id := string(sess.GetID())
	now := time.Now().UnixMilli()
	s := session.Session{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	w.sessionsMu.Lock()
	w.sessStore[id] = s
	w.sessionsMu.Unlock()

	w.sessMu.Lock()
	w.session = sess
	w.sessMu.Unlock()

	w.sessBroker.Publish(pubsub.CreatedEvent, s)
	return s, nil
}

func (w *NexusWorkspace) GetSession(_ context.Context, sessionID string) (session.Session, error) {
	w.sessionsMu.RLock()
	s, ok := w.sessStore[sessionID]
	w.sessionsMu.RUnlock()
	if ok {
		return s, nil
	}
	return session.Session{}, fmt.Errorf("session %q not found", sessionID)
}

func (w *NexusWorkspace) ListSessions(_ context.Context) ([]session.Session, error) {
	infos, err := w.client.ListSessions()
	if err != nil {
		return nil, err
	}
	result := make([]session.Session, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		now := time.Now().UnixMilli()
		updAt := info.UpdatedAt * 1000
		if updAt == 0 {
			updAt = now
		}
		creAt := info.CreatedAt * 1000
		if creAt == 0 {
			creAt = now
		}
		s := session.Session{
			ID:        string(info.ID),
			Title:     info.Title,
			CreatedAt: creAt,
			UpdatedAt: updAt,
		}
		w.sessionsMu.Lock()
		w.sessStore[s.ID] = s
		w.sessionsMu.Unlock()
		result = append(result, s)
	}
	return result, nil
}

func (w *NexusWorkspace) SaveSession(_ context.Context, sess session.Session) (session.Session, error) {
	w.sessionsMu.Lock()
	w.sessStore[sess.ID] = sess
	w.sessionsMu.Unlock()
	w.sessBroker.Publish(pubsub.UpdatedEvent, sess)
	return sess, nil
}

func (w *NexusWorkspace) DeleteSession(_ context.Context, sessionID string) error {
	w.sessMu.Lock()
	active := w.session != nil && string(w.session.GetID()) == sessionID
	var sdkSess *sdk.Session
	if active {
		sdkSess = w.session
		w.session = nil
	}
	w.sessMu.Unlock()

	if sdkSess != nil {
		_ = sdkSess.Interrupt()
		_ = sdkSess.Close()
	}
	if err := w.client.DeleteSession(sdk.SessionID(sessionID)); err != nil {
		return err
	}

	w.sessionsMu.Lock()
	s := w.sessStore[sessionID]
	delete(w.sessStore, sessionID)
	w.sessionsMu.Unlock()

	w.msgMu.Lock()
	delete(w.msgStore, sessionID)
	w.msgMu.Unlock()

	w.sessBroker.Publish(pubsub.DeletedEvent, s)
	return nil
}

func (w *NexusWorkspace) SetCurrentSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	// Fast path: session is already active.
	w.sessMu.Lock()
	if w.session != nil && string(w.session.GetID()) == sessionID {
		w.sessMu.Unlock()
		return nil
	}
	w.sessMu.Unlock()

	sess, err := w.client.LoadSession(ctx, sdk.SessionID(sessionID))
	if err != nil {
		return fmt.Errorf("activate session %q: %w", sessionID, err)
	}
	w.LoadSessionMessages(sessionID, sess.GetMessages())
	w.sessMu.Lock()
	w.session = sess
	w.sessMu.Unlock()
	return nil
}

// Agent tool session IDs use a simple "msgID:toolID" encoding.
func (w *NexusWorkspace) CreateAgentToolSessionID(messageID, toolCallID string) string {
	return messageID + ":" + toolCallID
}

func (w *NexusWorkspace) ParseAgentToolSessionID(sessionID string) (string, string, bool) {
	parts := strings.SplitN(sessionID, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func (w *NexusWorkspace) ListMessages(_ context.Context, sessionID string) ([]message.Message, error) {
	w.msgMu.RLock()
	msgs := make([]message.Message, len(w.msgStore[sessionID]))
	copy(msgs, w.msgStore[sessionID])
	w.msgMu.RUnlock()
	return msgs, nil
}

func (w *NexusWorkspace) ListUserMessages(_ context.Context, sessionID string) ([]message.Message, error) {
	w.msgMu.RLock()
	all := w.msgStore[sessionID]
	var result []message.Message
	for _, m := range all {
		if m.Role == message.User {
			result = append(result, m)
		}
	}
	w.msgMu.RUnlock()
	return result, nil
}

func (w *NexusWorkspace) ListAllUserMessages(_ context.Context) ([]message.Message, error) {
	w.msgMu.RLock()
	var result []message.Message
	for _, msgs := range w.msgStore {
		for _, m := range msgs {
			if m.Role == message.User {
				result = append(result, m)
			}
		}
	}
	w.msgMu.RUnlock()
	return result, nil
}

// ─── Agent ────────────────────────────────────────────────────────────────────

func (w *NexusWorkspace) AgentRun(ctx context.Context, sessionID, prompt string, _ ...message.Attachment) error {
	w.sessMu.Lock()
	sess := w.session
	w.sessMu.Unlock()

	if sess == nil || string(sess.GetID()) != sessionID {
		return fmt.Errorf("session %q not active", sessionID)
	}
	if w.busy.Load() {
		return fmt.Errorf("agent busy")
	}
	w.busy.Store(true)

	// Record the user message.
	now := time.Now().UnixMilli()
	userMsg := message.Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      message.User,
		Parts:     []message.ContentPart{message.TextContent{Text: prompt}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	w.appendMsg(sessionID, userMsg)
	w.msgBroker.Publish(pubsub.CreatedEvent, userMsg)

	// Create the assistant message placeholder.
	asstID := uuid.New().String()
	asstMsg := message.Message{
		ID:        asstID,
		SessionID: sessionID,
		Role:      message.Assistant,
		Parts:     []message.ContentPart{},
		CreatedAt: time.Now().UnixMilli(),
		UpdatedAt: time.Now().UnixMilli(),
	}
	w.appendMsg(sessionID, asstMsg)
	w.msgBroker.Publish(pubsub.CreatedEvent, asstMsg)

	w.streamMu.Lock()
	w.streamMsg = &asstMsg
	w.streamSess = sessionID
	w.streamMu.Unlock()

	submitCtx, cancel := context.WithCancel(ctx)
	w.submitMu.Lock()
	w.submitCancel = cancel
	w.submitMu.Unlock()

	go func() {
		defer func() {
			w.submitMu.Lock()
			w.submitCancel = nil
			w.submitMu.Unlock()
			cancel()
			w.busy.Store(false)
			w.debounce.forceFlush()
		}()

		resp, err := sess.SubmitMessage(submitCtx, prompt)

		w.streamMu.Lock()
		cur := w.streamMsg
		w.streamMsg = nil
		w.streamSess = ""
		w.streamMu.Unlock()

		if cur == nil {
			return
		}

		finishReason := message.FinishReasonEndTurn
		if err != nil {
			if submitCtx.Err() != nil {
				finishReason = message.FinishReasonCanceled
			} else {
				finishReason = message.FinishReasonError
			}
		} else if resp != nil {
			switch resp.StopReason {
			case "max_tokens":
				finishReason = message.FinishReasonMaxTokens
			case "tool_use":
				finishReason = message.FinishReasonToolUse
			}
		}

		cur.Parts = append(cur.Parts, message.Finish{
			Reason: finishReason,
			Time:   time.Now().UnixMilli(),
		})
		cur.UpdatedAt = time.Now().UnixMilli()
		w.updateMsg(sessionID, *cur)
		w.msgBroker.Publish(pubsub.UpdatedEvent, *cur)

		// Update session record.
		w.sessionsMu.Lock()
		if s, ok := w.sessStore[sessionID]; ok {
			s.UpdatedAt = time.Now().UnixMilli()
			if resp != nil && resp.Usage != nil {
				s.PromptTokens += int64(resp.Usage.InputTokens)
				s.CompletionTokens += int64(resp.Usage.OutputTokens)
			}
			w.sessStore[sessionID] = s
			w.sessBroker.Publish(pubsub.UpdatedEvent, s)
		}
		w.sessionsMu.Unlock()
	}()
	return nil
}

func (w *NexusWorkspace) AgentCancel(sessionID string) {
	w.submitMu.Lock()
	cancel := w.submitCancel
	w.submitMu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.sessMu.Lock()
	sess := w.session
	w.sessMu.Unlock()
	if sess != nil && string(sess.GetID()) == sessionID {
		_ = sess.Interrupt()
	}
	w.busy.Store(false)
}

func (w *NexusWorkspace) AgentIsBusy() bool                        { return w.busy.Load() }
func (w *NexusWorkspace) AgentIsSessionBusy(sessionID string) bool {
	if !w.busy.Load() {
		return false
	}
	w.sessMu.Lock()
	defer w.sessMu.Unlock()
	return w.session != nil && string(w.session.GetID()) == sessionID
}

func (w *NexusWorkspace) AgentIsReady() bool                              { return w.client != nil }
func (w *NexusWorkspace) AgentQueuedPrompts(_ string) int                 { return 0 }
func (w *NexusWorkspace) AgentQueuedPromptsList(_ string) []string        { return nil }
func (w *NexusWorkspace) AgentClearQueue(_ string)                        {}
func (w *NexusWorkspace) AgentSummarize(_ context.Context, _ string) error { return nil }
func (w *NexusWorkspace) UpdateAgentModel(_ context.Context) error        { return nil }
func (w *NexusWorkspace) InitCoderAgent(_ context.Context) error          { return nil }

func (w *NexusWorkspace) AgentModel() AgentModel {
	provider, modelID := w.splitModel()
	return AgentModel{
		CatwalkCfg: catwalk.Model{
			ID:            modelID,
			Name:          modelID,
			ContextWindow: 200000,
			CanReason:     strings.Contains(strings.ToLower(modelID), "think") || strings.Contains(strings.ToLower(modelID), "o3") || strings.Contains(strings.ToLower(modelID), "o1"),
		},
		ModelCfg: config.SelectedModel{
			Model:    modelID,
			Provider: provider,
		},
	}
}

func (w *NexusWorkspace) GetDefaultSmallModel(_ string) config.SelectedModel {
	provider, modelID := w.splitModel()
	return config.SelectedModel{Model: modelID, Provider: provider}
}

func (w *NexusWorkspace) splitModel() (provider, model string) {
	if parts := strings.SplitN(w.model, ":", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "anthropic", w.model
}

// ─── Permissions ──────────────────────────────────────────────────────────────

func (w *NexusWorkspace) PermissionGrant(perm permission.PermissionRequest) bool {
	w.permBroker.Publish(pubsub.UpdatedEvent, perm)
	return true
}

func (w *NexusWorkspace) PermissionGrantPersistent(perm permission.PermissionRequest) bool {
	return w.PermissionGrant(perm)
}

func (w *NexusWorkspace) PermissionDeny(perm permission.PermissionRequest) bool {
	w.permBroker.Publish(pubsub.UpdatedEvent, perm)
	return true
}

func (w *NexusWorkspace) PermissionSkipRequests() bool       { return w.permSkip.Load() }
func (w *NexusWorkspace) PermissionSetSkipRequests(skip bool) { w.permSkip.Store(skip) }

// ─── File Tracker (no-op) ─────────────────────────────────────────────────────

func (w *NexusWorkspace) FileTrackerRecordRead(_ context.Context, _, _ string) {}
func (w *NexusWorkspace) FileTrackerLastReadTime(_ context.Context, _, _ string) time.Time {
	return time.Time{}
}
func (w *NexusWorkspace) FileTrackerListReadFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

// ─── History (no-op) ─────────────────────────────────────────────────────────

func (w *NexusWorkspace) ListSessionHistory(_ context.Context, _ string) ([]history.File, error) {
	return nil, nil
}

// ─── LSP (no-op) ─────────────────────────────────────────────────────────────

func (w *NexusWorkspace) LSPStart(_ context.Context, _ string)          {}
func (w *NexusWorkspace) LSPStopAll(_ context.Context)                  {}
func (w *NexusWorkspace) LSPGetStates() map[string]LSPClientInfo        { return nil }
func (w *NexusWorkspace) LSPGetDiagnosticCounts(_ string) lsp.DiagnosticCounts {
	return lsp.DiagnosticCounts{}
}

// ─── Config ───────────────────────────────────────────────────────────────────

func (w *NexusWorkspace) Config() *config.Config {
	w.cfgMu.Lock()
	defer w.cfgMu.Unlock()
	if w.cfg != nil {
		return w.cfg
	}

	provider, modelID := w.splitModel()
	providerCfg := config.ProviderConfig{
		ID:   provider,
		Name: provider,
		Models: []catwalk.Model{
			{ID: modelID, Name: modelID, ContextWindow: 200000},
		},
	}
	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set(provider, providerCfg)

	cfg := &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Model: modelID, Provider: provider},
			config.SelectedModelTypeSmall: {Model: modelID, Provider: provider},
		},
		Providers: providers,
		Options:   &config.Options{TUI: &config.TUIOptions{}},
	}
	w.cfg = cfg
	return cfg
}

func (w *NexusWorkspace) WorkingDir() string { return w.workDir }

func (w *NexusWorkspace) Resolver() config.VariableResolver {
	return config.IdentityResolver()
}

// ─── Config mutations (stubs — UI writes, we ignore) ─────────────────────────

func (w *NexusWorkspace) UpdatePreferredModel(_ config.Scope, _ config.SelectedModelType, m config.SelectedModel) error {
	w.model = m.Provider + ":" + m.Model
	w.cfgMu.Lock()
	w.cfg = nil // invalidate cached config
	w.cfgMu.Unlock()
	return nil
}

func (w *NexusWorkspace) SetCompactMode(_ config.Scope, _ bool) error      { return nil }
func (w *NexusWorkspace) SetProviderAPIKey(_ config.Scope, _ string, _ any) error { return nil }
func (w *NexusWorkspace) SetConfigField(_ config.Scope, _ string, _ any) error    { return nil }
func (w *NexusWorkspace) RemoveConfigField(_ config.Scope, _ string) error         { return nil }
func (w *NexusWorkspace) ImportCopilot() (*oauth.Token, bool)                      { return nil, false }
func (w *NexusWorkspace) RefreshOAuthToken(_ context.Context, _ config.Scope, _ string) error {
	return nil
}

// ─── Project lifecycle (stubs) ────────────────────────────────────────────────

func (w *NexusWorkspace) ProjectNeedsInitialization() (bool, error)    { return false, nil }
func (w *NexusWorkspace) MarkProjectInitialized() error                { return nil }
func (w *NexusWorkspace) InitializePrompt() (string, error)            { return "", nil }
func (w *NexusWorkspace) ListSkills(_ context.Context) ([]skills.CatalogEntry, error) {
	cfg := w.Config()
	var skillsPaths []string
	var disabledSkills []string
	if cfg.Options != nil {
		skillsPaths = cfg.Options.SkillsPaths
		disabledSkills = cfg.Options.DisabledSkills
	}
	resolver := w.Resolver()
	discCfg := skills.DiscoveryConfig{
		SkillsPaths:    skillsPaths,
		DisabledSkills: disabledSkills,
		WorkingDir:     w.workDir,
		Resolver:       resolver.ResolveValue,
	}
	_, active, _ := skills.DiscoverFromConfig(discCfg)
	resolvedPaths := discCfg.ResolvePaths()
	return skills.Catalog(active, resolvedPaths, w.workDir), nil
}
func (w *NexusWorkspace) ReadSkill(_ context.Context, _ string) ([]byte, skills.SkillReadResult, error) {
	return nil, skills.SkillReadResult{}, nil
}

// ─── MCP (stubs) ──────────────────────────────────────────────────────────────

func (w *NexusWorkspace) MCPGetStates() map[string]mcptools.ClientInfo { return mcptools.GetStates() }
func (w *NexusWorkspace) MCPRefreshPrompts(_ context.Context, _ string)                         {}
func (w *NexusWorkspace) MCPRefreshResources(_ context.Context, _ string)                       {}
func (w *NexusWorkspace) RefreshMCPTools(_ context.Context, _ string)                           {}
func (w *NexusWorkspace) ReadMCPResource(_ context.Context, _, _ string) ([]MCPResourceContents, error) {
	return nil, nil
}
func (w *NexusWorkspace) GetMCPPrompt(_, _ string, _ map[string]string) (string, error) {
	return "", nil
}
func (w *NexusWorkspace) EnableDockerMCP(_ context.Context) error  { return nil }
func (w *NexusWorkspace) DisableDockerMCP() error                  { return nil }

// ─── SDK callback receivers (called from SDK event callbacks) ─────────────────

// HandleChunk processes a streaming text delta and updates the in-progress message.
func (w *NexusWorkspace) HandleChunk(delta string, isThinking bool) {
	w.streamMu.Lock()
	cur := w.streamMsg
	sessID := w.streamSess
	w.streamMu.Unlock()
	if cur == nil {
		return
	}

	if isThinking {
		cur.AppendReasoningContent(delta)
	} else {
		cur.AppendContent(delta)
	}
	cur.UpdatedAt = time.Now().UnixMilli()
	w.updateMsg(sessID, *cur)
	w.debounce.update(*cur, sessID)
}

// HandleToolProgress updates the in-progress tool call within the message.
func (w *NexusWorkspace) HandleToolProgress(toolUseID, toolName, status, msg string) {
	w.streamMu.Lock()
	cur := w.streamMsg
	sessID := w.streamSess
	w.streamMu.Unlock()
	if cur == nil {
		return
	}

	switch status {
	case "running", "pending":
		// Create or update a ToolCall part.
		found := false
		for i, p := range cur.Parts {
			if tc, ok := p.(message.ToolCall); ok && tc.ID == toolUseID {
				cur.Parts[i] = message.ToolCall{ID: toolUseID, Name: toolName, Input: tc.Input, Finished: false}
				found = true
				break
			}
		}
		if !found {
			cur.Parts = append(cur.Parts, message.ToolCall{ID: toolUseID, Name: toolName, Finished: false})
		}
	case "completed", "done":
		for i, p := range cur.Parts {
			if tc, ok := p.(message.ToolCall); ok && tc.ID == toolUseID {
				cur.Parts[i] = message.ToolCall{ID: toolUseID, Name: toolName, Input: tc.Input, Finished: true}
				break
			}
		}
		// Append a ToolResult.
		cur.Parts = append(cur.Parts, message.ToolResult{
			ToolCallID: toolUseID,
			Name:       toolName,
			Content:    msg,
		})
	case "failed", "error":
		for i, p := range cur.Parts {
			if tc, ok := p.(message.ToolCall); ok && tc.ID == toolUseID {
				cur.Parts[i] = message.ToolCall{ID: toolUseID, Name: toolName, Input: tc.Input, Finished: true}
				break
			}
		}
		cur.Parts = append(cur.Parts, message.ToolResult{
			ToolCallID: toolUseID,
			Name:       toolName,
			Content:    msg,
			IsError:    true,
		})
	}

	cur.UpdatedAt = time.Now().UnixMilli()
	w.updateMsg(sessID, *cur)
	w.debounce.update(*cur, sessID)
}

// HandlePermissionRequest emits a permission request to the nexustui UI.
func (w *NexusWorkspace) HandlePermissionRequest(req permission.PermissionRequest) {
	w.permBroker.Publish(pubsub.CreatedEvent, req)
}

// LoadSessionMessages populates the message store from the SDK session history
// (call after LoadSession completes).
func (w *NexusWorkspace) LoadSessionMessages(sessionID string, sdkMsgs []sdk.Message) {
	msgs := convertSDKMessages(sessionID, sdkMsgs)
	w.msgMu.Lock()
	w.msgStore[sessionID] = msgs
	w.msgMu.Unlock()
}

// ─── SDK glue — called from newClient() callbacks ────────────────────────────

// SetSDKClient wires the SDK client and registers unique TUI tools.
func (w *NexusWorkspace) SetSDKClient(client *sdk.Client) {
	w.client = client
	w.registerTUITools(nil)
}

// RegisterLSPTools registers the LSP-backed unique TUI tools with the live LSP manager.
// Call this once the LSP manager is available (after startup).
func (w *NexusWorkspace) RegisterLSPTools(lspManager *lsp.Manager) {
	w.registerTUITools(lspManager)
}

// registerTUITools registers unique nexustui tools (not covered by SDK builtins).
// lspManager may be nil; LSP tools will report "no LSP available" until one is set.
func (w *NexusWorkspace) registerTUITools(lspManager *lsp.Manager) {
	if w.client == nil {
		return
	}
	logFile := w.logFilePath()
	uniqueTools := []sdk.Tool{
		tuiTools.NewNexusLogsTool(logFile),
		tuiTools.NewDiagnosticsTool(lspManager),
		tuiTools.NewLSPRestartTool(lspManager),
		tuiTools.NewReferencesTool(lspManager),
	}
	for _, t := range uniqueTools {
		if err := w.client.RegisterTool(t); err != nil {
			slog.Debug("Failed to register TUI tool", "tool", t.Definition().Name, "error", err)
		}
	}
}

// logFilePath returns the path to the nexus log file, derived from config.
func (w *NexusWorkspace) logFilePath() string {
	cfg := w.Config()
	dataDir := ""
	if cfg.Options != nil {
		dataDir = cfg.Options.DataDirectory
	}
	if dataDir == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			dataDir = filepath.Join(cacheDir, "nexus-engine")
		} else {
			dataDir = os.TempDir()
		}
	}
	return filepath.Join(dataDir, "logs", "nexus.log")
}

// OnChunk is the sdk.ResponseChunk callback — translates to HandleChunk.
func (w *NexusWorkspace) OnChunk(chunk sdk.ResponseChunk) {
	switch chunk.Type {
	case sdk.ResponseChunkTypeContentBlockDelta:
		switch chunk.DeltaType {
		case "text_delta", "":
			w.HandleChunk(chunk.Delta, false)
		case "thinking_delta":
			w.HandleChunk(chunk.Delta, true)
		}
	case sdk.ResponseChunkTypeMessageStop:
		w.debounce.forceFlush()
	}
}

// OnProgress is the sdk.ToolProgress callback.
func (w *NexusWorkspace) OnProgress(p sdk.ToolProgress) {
	msg := p.Message
	if msg == "" {
		msg = string(p.Stage)
	}
	w.HandleToolProgress(p.ToolUseID, p.ToolName, string(p.Stage), msg)
}

// OnRuntimeEvent handles subagent events (no-op for now).
func (w *NexusWorkspace) OnRuntimeEvent(_ sdk.RuntimeEvent) {}

// OnSessionTitled updates the session title in our local store.
func (w *NexusWorkspace) OnSessionTitled(id sdk.SessionID, title string) {
	sessID := string(id)
	w.sessionsMu.Lock()
	if s, ok := w.sessStore[sessID]; ok {
		s.Title = title
		s.UpdatedAt = time.Now().UnixMilli()
		w.sessStore[sessID] = s
		w.sessBroker.Publish(pubsub.UpdatedEvent, s)
	}
	w.sessionsMu.Unlock()
}

// PromptFn blocks the agent goroutine waiting for the UI to resolve a permission request.
// For now, we auto-allow all requests since the permission dialog works differently.
func (w *NexusWorkspace) PromptFn(_ context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	toolName, _ := req.Metadata["tool_name"].(string)
	permReq := permission.PermissionRequest{
		ID:          uuid.New().String(),
		ToolName:    toolName,
		Description: req.Message,
		Action:      string(req.Type),
	}
	w.permBroker.Publish(pubsub.CreatedEvent, permReq)

	// For now, auto-allow. A proper implementation would block here until the UI resolves.
	return sdk.PromptResponse{Value: true}, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (w *NexusWorkspace) appendMsg(sessionID string, msg message.Message) {
	w.msgMu.Lock()
	w.msgStore[sessionID] = append(w.msgStore[sessionID], msg)
	w.msgMu.Unlock()
}

func (w *NexusWorkspace) updateMsg(sessionID string, msg message.Message) {
	w.msgMu.Lock()
	msgs := w.msgStore[sessionID]
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].ID == msg.ID {
			msgs[i] = msg
			break
		}
	}
	w.msgMu.Unlock()
}

func (w *NexusWorkspace) publishMsg(evt pubsub.EventType, _ string, msg message.Message) {
	w.msgBroker.Publish(evt, msg)
}

// convertSDKMessages converts the SDK message history to nexustui message types.
func convertSDKMessages(sessionID string, sdkMsgs []sdk.Message) []message.Message {
	// Pre-build tool result map keyed by tool_use_id.
	type resultEntry struct {
		content  string
		isError  bool
		metadata string
	}
	resultMap := make(map[string]resultEntry)
	for _, m := range sdkMsgs {
		if m.Role != sdk.RoleUser {
			continue
		}
		for _, block := range m.Content {
			if tr, ok := block.(sdk.ToolResultContent); ok {
				meta := ""
				if tr.Metadata != nil {
					if data, err := json.Marshal(tr.Metadata); err == nil {
						meta = string(data)
					}
				}
				resultMap[tr.ToolUseID] = resultEntry{
					content:  tr.Content,
					isError:  tr.IsError,
					metadata: meta,
				}
			}
		}
	}

	var out []message.Message
	for _, m := range sdkMsgs {
		ts := m.Timestamp.UnixMilli()
		if ts <= 0 {
			ts = time.Now().UnixMilli()
		}

		msg := message.Message{
			ID:        string(m.ID),
			SessionID: sessionID,
			CreatedAt: ts,
			UpdatedAt: ts,
		}

		switch m.Role {
		case sdk.RoleUser:
			msg.Role = message.User
			for _, block := range m.Content {
				if t, ok := block.(sdk.TextContent); ok {
					msg.Parts = append(msg.Parts, message.TextContent{Text: t.Text})
				}
			}
		case sdk.RoleAssistant:
			msg.Role = message.Assistant
			if m.Metadata != nil && m.Metadata.StopReason != "" {
				finishReason := sdkStopToFinish(m.Metadata.StopReason)
				msg.Parts = append(msg.Parts, message.Finish{
					Reason: finishReason,
					Time:   ts,
				})
			}
			for _, block := range m.Content {
				switch b := block.(type) {
				case sdk.TextContent:
					// Insert text before the Finish part if present.
					msg.Parts = prependPart(msg.Parts, message.TextContent{Text: b.Text})
				case sdk.ThinkingContent:
					msg.Parts = prependPart(msg.Parts, message.ReasoningContent{Thinking: b.Thinking})
				case sdk.ToolUseContent:
					inputJSON, _ := json.Marshal(b.Input)
					tc := message.ToolCall{
						ID:       b.ID,
						Name:     b.Name,
						Input:    string(inputJSON),
						Finished: true,
					}
					msg.Parts = prependPart(msg.Parts, tc)
					if r, ok := resultMap[b.ID]; ok {
						msg.Parts = prependPart(msg.Parts, message.ToolResult{
							ToolCallID: b.ID,
							Name:       b.Name,
							Content:    r.content,
							Metadata:   r.metadata,
							IsError:    r.isError,
						})
					}
				}
			}
		}

		if len(msg.Parts) > 0 {
			out = append(out, msg)
		}
	}
	return out
}

// prependPart inserts a part before any existing Finish parts (so text/tools
// appear before the finish marker in the parts slice).
func prependPart(parts []message.ContentPart, p message.ContentPart) []message.ContentPart {
	// Find the first Finish part index.
	for i, part := range parts {
		if _, ok := part.(message.Finish); ok {
			// Insert before it.
			result := make([]message.ContentPart, 0, len(parts)+1)
			result = append(result, parts[:i]...)
			result = append(result, p)
			result = append(result, parts[i:]...)
			return result
		}
	}
	return append(parts, p)
}

func sdkStopToFinish(stopReason string) message.FinishReason {
	switch stopReason {
	case "end_turn", "stop":
		return message.FinishReasonEndTurn
	case "max_tokens":
		return message.FinishReasonMaxTokens
	case "tool_use":
		return message.FinishReasonToolUse
	default:
		return message.FinishReasonEndTurn
	}
}

// ─── msgDebounce — batches streaming pubsub updates at a fixed interval ───────

type msgDebounce struct {
	mu      sync.Mutex
	latestMsg  message.Message
	latestSess string
	dirty      bool
	timer      *time.Timer
	delay      time.Duration
	flush      func(message.Message, string)
}

func newMsgDebounce(delay time.Duration, flush func(message.Message, string)) *msgDebounce {
	return &msgDebounce{delay: delay, flush: flush}
}

func (d *msgDebounce) update(msg message.Message, sessID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.latestMsg = msg
	d.latestSess = sessID
	d.dirty = true
	if d.timer == nil {
		d.timer = time.AfterFunc(d.delay, d.tick)
	}
}

func (d *msgDebounce) tick() {
	d.mu.Lock()
	if !d.dirty {
		d.timer = nil
		d.mu.Unlock()
		return
	}
	msg := d.latestMsg
	sess := d.latestSess
	d.dirty = false
	d.timer = nil
	d.mu.Unlock()
	d.flush(msg, sess)
}

func (d *msgDebounce) forceFlush() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	if !d.dirty {
		d.mu.Unlock()
		return
	}
	msg := d.latestMsg
	sess := d.latestSess
	d.dirty = false
	d.mu.Unlock()
	d.flush(msg, sess)
}
