package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	tuimodel "github.com/EngineerProjects/nexus-engine/internal/tui/model"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

// chunkDebounce batches streaming text chunks at 33ms intervals (crush pattern).
// High-frequency token callbacks don't cause a TUI re-render on every token;
// instead they are accumulated and flushed as a single ChunkMsg tick.
type chunkDebounce struct {
	mu      sync.Mutex
	buf     string
	timer   *time.Timer
	flush   func(string)
	delay   time.Duration
}

func newChunkDebounce(delay time.Duration, flush func(string)) *chunkDebounce {
	return &chunkDebounce{delay: delay, flush: flush}
}

func (d *chunkDebounce) add(text string, immediate bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf += text
	if immediate {
		// Terminal events (tool calls, turn end) flush immediately.
		if d.timer != nil {
			d.timer.Stop()
			d.timer = nil
		}
		if d.buf != "" {
			d.flush(d.buf)
			d.buf = ""
		}
		return
	}
	if d.timer == nil {
		d.timer = time.AfterFunc(d.delay, func() {
			d.mu.Lock()
			text := d.buf
			d.buf = ""
			d.timer = nil
			d.mu.Unlock()
			if text != "" {
				d.flush(text)
			}
		})
	}
}

func (d *chunkDebounce) forceFlush() {
	d.add("", true)
}

// nexusWorkspace implements tui.Workspace by wrapping an sdk.Client.
// It bridges the engine's callback-based event model with the BubbleTea
// tea.Program.Send() pattern, with 33ms streaming debounce (crush pattern).
type nexusWorkspace struct {
	client     *sdk.Client
	model      string
	workDir    string
	permMode   string
	sqlitePath string // path to credentials DB, used by LoadProviderConfig

	mu      sync.RWMutex
	program *tea.Program

	sessionMu sync.Mutex
	session   *sdk.Session

	busy    atomic.Bool
	debounce *chunkDebounce
}

// newNexusWorkspace creates a nexusWorkspace. The workspace registers its own
// callbacks on the ClientConfig so that events are forwarded to the TUI.
func newNexusWorkspace(options runtimeOptions) (*nexusWorkspace, error) {
	w := &nexusWorkspace{
		model:      options.Model.String(),
		workDir:    options.WorkingDir,
		permMode:   string(options.PermissionMode),
		sqlitePath: options.SQLitePath,
	}
	w.debounce = newChunkDebounce(33*time.Millisecond, func(text string) {
		w.send(tui.ChunkMsg{Text: text})
	})

	client, err := newClient(
		options,
		w.promptFn,
		w.onProgress,
		w.onChunk,
	)
	if err != nil {
		return nil, err
	}
	w.client = client
	return w, nil
}

// ─── tui.Workspace implementation ─────────────────────────────────────────────

func (w *nexusWorkspace) Subscribe(p *tea.Program) {
	w.mu.Lock()
	w.program = p
	w.mu.Unlock()
}

func (w *nexusWorkspace) send(msg tea.Msg) {
	w.mu.RLock()
	p := w.program
	w.mu.RUnlock()
	if p != nil {
		p.Send(msg)
	}
}

func (w *nexusWorkspace) ListSessions(ctx context.Context) {
	go func() {
		infos, err := w.client.ListSessions()
		if err != nil {
			w.send(tui.SessionListMsg{Err: err})
			return
		}
		sessions := make([]tui.SessionInfo, 0, len(infos))
		for _, info := range infos {
			if info == nil {
				continue
			}
			id := string(info.ID)
			sessions = append(sessions, tui.SessionInfo{
				ID:      id,
				ShortID: shortIDStr(id),
				Turns:   info.TotalTurns,
				Tokens:  info.TotalTokens,
			})
		}
		w.send(tui.SessionListMsg{Sessions: sessions})
	}()
}

func (w *nexusWorkspace) CreateSession(ctx context.Context) {
	go func() {
		sess, err := w.client.CreateSession(ctx)
		if err != nil {
			w.send(tui.SessionCreatedMsg{Err: err})
			return
		}
		w.sessionMu.Lock()
		w.session = sess
		w.sessionMu.Unlock()
		w.send(tui.SessionCreatedMsg{ID: string(sess.GetID())})
	}()
}

func (w *nexusWorkspace) LoadSession(ctx context.Context, id string) {
	go func() {
		sess, err := w.client.LoadSession(ctx, sdk.SessionID(id))
		if err != nil {
			w.send(tui.SessionLoadedMsg{Err: err})
			return
		}
		w.sessionMu.Lock()
		w.session = sess
		w.sessionMu.Unlock()
		w.send(tui.SessionLoadedMsg{ID: string(sess.GetID())})
	}()
}

func (w *nexusWorkspace) DeleteSession(_ context.Context, id string) error {
	return w.client.DeleteSession(sdk.SessionID(id))
}

func (w *nexusWorkspace) Submit(ctx context.Context, prompt string) {
	w.sessionMu.Lock()
	sess := w.session
	w.sessionMu.Unlock()

	if sess == nil || w.busy.Load() {
		return
	}
	w.busy.Store(true)

	go func() {
		w.send(tui.TurnStartMsg{
			SessionID: string(sess.GetID()),
		})
		resp, err := sess.SubmitMessage(ctx, prompt)
		w.busy.Store(false)

		done := tui.TurnDoneMsg{
			SessionID: string(sess.GetID()),
			Err:       err,
		}
		if resp != nil && resp.Usage != nil {
			done.InputTokens = resp.Usage.InputTokens
			done.OutputTokens = resp.Usage.OutputTokens
		}
		if resp != nil {
			done.StopReason = resp.StopReason
		}
		w.send(done)
	}()
}

func (w *nexusWorkspace) Cancel() {
	// The SDK doesn't expose a per-session cancel yet; cancel via context
	// when the workspace is closed or a parent context is cancelled.
	// For now, mark as not busy so the UI unblocks.
	w.busy.Store(false)
}

func (w *nexusWorkspace) ActiveSessionID() string {
	w.sessionMu.Lock()
	defer w.sessionMu.Unlock()
	if w.session == nil {
		return ""
	}
	return string(w.session.GetID())
}

func (w *nexusWorkspace) IsBusy() bool {
	return w.busy.Load()
}

func (w *nexusWorkspace) ModelString() string    { return w.model }
func (w *nexusWorkspace) WorkingDir() string     { return w.workDir }
func (w *nexusWorkspace) PermissionMode() string { return w.permMode }

func (w *nexusWorkspace) ListModels(ctx context.Context) {
	go func() {
		all := providers.AllProvidersInfo()
		var models []tui.ProviderModel
		for provider, info := range all {
			for _, m := range info.Models {
				models = append(models, tui.ProviderModel{
					Provider:    string(provider),
					Identifier:  m.Identifier,
					DisplayName: info.DisplayName + " / " + m.Identifier,
					Description: m.Description,
					Context:     m.ContextWindow,
				})
			}
		}
		w.send(tui.ModelListMsg{Models: models})
	}()
}

func (w *nexusWorkspace) SetModel(providerID, modelID string) {
	w.mu.Lock()
	w.model = providerID + ":" + modelID
	w.mu.Unlock()
	w.send(tui.ModelChangedMsg{Provider: providerID, Model: modelID})
}

// ─── Provider configuration ────────────────────────────────────────────────────

// providerCredKey returns the scoped credential key for a provider field.
// Format: "fieldKey:providerID" (e.g. "api_key:anthropic").
func providerCredKey(fieldKey, providerID string) string {
	return fieldKey + ":" + strings.ToLower(providerID)
}

func (w *nexusWorkspace) LoadProviderConfig(_ context.Context) []tui.ProviderStatus {
	// Load config so we can check current values.
	cfg, err := engineconfig.Load()
	if err != nil {
		cfg = engineconfig.Config{}
	}

	// Open DB (best-effort — if it fails we show all fields as unset).
	database, dbErr := openCredentialsDB(cfg)
	if dbErr == nil {
		defer database.Close()
	}

	getField := func(providerID, fieldKey string) (string, bool) {
		if database == nil {
			return "", false
		}
		// Per-provider key first, then global fallback.
		if v, ok, _ := database.GetCredential(context.Background(), providerCredKey(fieldKey, providerID)); ok && v != "" {
			return v, true
		}
		if v, ok, _ := database.GetCredential(context.Background(), fieldKey); ok && v != "" {
			return v, true
		}
		return "", false
	}

	providers := engineconfig.AvailableProviders()
	result := make([]tui.ProviderStatus, 0, len(providers))
	for _, p := range providers {
		fields := make([]tui.ProviderFieldStatus, 0, len(p.SetupFields))
		for _, f := range p.SetupFields {
			_, isSet := getField(string(p.Name), f.Key)
			fields = append(fields, tui.ProviderFieldStatus{
				Key:      f.Key,
				Label:    f.Label,
				EnvVar:   f.EnvVar,
				Secret:   f.Secret,
				Required: f.Required,
				IsSet:    isSet,
			})
		}
		result = append(result, tui.ProviderStatus{
			ID:          string(p.Name),
			DisplayName: p.DisplayName,
			Description: p.Description,
			NeedsKey:    len(p.SetupFields) > 0,
			Fields:      fields,
		})
	}
	return result
}

func (w *nexusWorkspace) SaveProviderField(ctx context.Context, providerID, fieldKey, value string) error {
	cfg, _ := engineconfig.Load()
	database, err := openCredentialsDB(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	return database.UpsertCredential(ctx, providerCredKey(fieldKey, providerID), value)
}

func (w *nexusWorkspace) DeleteProviderField(ctx context.Context, providerID, fieldKey string) error {
	cfg, _ := engineconfig.Load()
	database, err := openCredentialsDB(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	return database.DeleteCredential(ctx, providerCredKey(fieldKey, providerID))
}

func (w *nexusWorkspace) Close() {
	w.client.Close()
}

// ─── SDK callback bridges ──────────────────────────────────────────────────────

func (w *nexusWorkspace) onChunk(chunk sdk.ResponseChunk) {
	switch chunk.Type {
	case sdk.ResponseChunkTypeContentBlockDelta:
		switch chunk.DeltaType {
		case "text_delta", "":
			// Debounce text deltas at 33ms (crush pattern) — avoids
			// re-rendering the TUI on every token during streaming.
			w.debounce.add(chunk.Delta, false)
		case "thinking_delta":
			w.send(tui.ChunkMsg{Text: chunk.Delta, IsThinking: true})
		}
	case sdk.ResponseChunkTypeMessageStop:
		// Flush any buffered text immediately when the stream ends.
		w.debounce.forceFlush()
	}
}

func (w *nexusWorkspace) onProgress(progress sdk.ToolProgress) {
	label := progress.Message
	if label == "" {
		label = string(progress.Stage)
	}
	w.send(tui.ToolProgressMsg{
		ToolUseID: progress.ToolUseID,
		ToolName:  progress.ToolName,
		Status:    string(progress.Stage),
		Label:     label,
	})
}

// promptFn blocks the calling (agent) goroutine until the TUI resolves it.
func (w *nexusWorkspace) promptFn(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	respCh := make(chan tui.PromptResponse, 1)

	opts := make([]tui.PromptOption, len(req.Options))
	for i, o := range req.Options {
		opts[i] = tui.PromptOption{Label: o.Label, Value: o.Value}
	}

	w.send(tui.PromptRequestMsg{
		Type:     string(req.Type),
		Message:  req.Message,
		Options:  opts,
		Response: respCh,
	})

	select {
	case resp := <-respCh:
		if resp.Cancelled {
			return sdk.PromptResponse{Cancelled: true}, nil
		}
		return sdk.PromptResponse{Value: resp.Value}, nil
	case <-ctx.Done():
		return sdk.PromptResponse{Cancelled: true}, ctx.Err()
	}
}

// ─── TUI entry point ──────────────────────────────────────────────────────────

// runInteractive starts the BubbleTea TUI. Called by runChat when a TTY is detected.
func runInteractive(ctx context.Context, options runtimeOptions) error {
	if err := validateProviderSetup(options); err != nil {
		return err
	}

	// Redirect all log output to a file before entering alt-screen (crush pattern).
	// Without this, monitoring logs and stdlib log output bleed into the TUI.
	options.Monitoring = buildTUIMonitoring()
	log.SetOutput(io.Discard)

	ws, err := newNexusWorkspace(options)
	if err != nil {
		return err
	}
	defer ws.Close()

	return tuimodel.Run(ws, ctx)
}

// buildTUIMonitoring creates a monitoring system that writes to a log file
// instead of stdout, so logs don't interfere with the TUI alt-screen.
func buildTUIMonitoring() *sdk.MonitoringSystem {
	logDir := filepath.Join(nexusLogDir(), "logs")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "nexus.log")

	logger := monitoring.NewLoggerWithConfig(&monitoring.LoggerConfig{
		Level:    monitoring.LogLevelInfo,
		Output:   "file",
		FilePath: logPath,
		Format:   "text",
	})
	return monitoring.NewSystem(logger)
}

// nexusLogDir returns ~/.nexus or a temp fallback.
func nexusLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".nexus")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func shortIDStr(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
