package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/cmd/cli/appdir"
	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	db "github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	tuiapp "github.com/EngineerProjects/nexus-engine/internal/tui/app"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	skillspkg "github.com/EngineerProjects/nexus-engine/pkg/skills"
)

// chunkDebounce batches streaming text chunks at 33ms intervals (crush pattern).
// High-frequency token callbacks don't cause a TUI re-render on every token;
// instead they are accumulated and flushed as a single ChunkMsg tick.
type chunkDebounce struct {
	mu    sync.Mutex
	buf   string
	timer *time.Timer
	flush func(string)
	delay time.Duration
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
	clientMu   sync.RWMutex
	client     *sdk.Client
	clientOpts runtimeOptions
	model      string
	workDir    string
	permMode   string
	sqlitePath string // path to credentials DB, used by LoadProviderConfig

	mu      sync.RWMutex
	program *tea.Program

	sessionMu sync.Mutex
	session   *sdk.Session

	// reloadMu serialises reloadClient calls and lets CreateSession/LoadSession
	// wait for any in-flight provider reload before they bind to a client.
	reloadMu sync.Mutex

	busy     atomic.Bool
	debounce *chunkDebounce

	// submitMu guards submitCancel, which is non-nil while a Submit goroutine runs.
	submitMu     sync.Mutex
	submitCancel context.CancelFunc

	subagentMu   sync.Mutex
	subagentLogs map[string]string // keyed by AgentToolUseID
}

// credKeyOllamaModels is the DB key for the cached Ollama model list.
const credKeyOllamaModels = "ollama:models"

type ollamaCachedModel struct {
	ID      string `json:"id"`
	Context int    `json:"ctx,omitempty"`
}

// probeOllamaInBackground discovers Ollama models, caches them in the DB,
// and sends a ModelListMsg so the picker refreshes if it happens to be open.
// Safe to call multiple times; each call spawns a goroutine that runs to completion.
func (w *nexusWorkspace) probeOllamaInBackground() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		cfg, _ := engineconfig.Load()
		database, _ := openCredentialsDB(cfg)
		if database != nil {
			defer database.Close()
		}

		baseURL := ollamaBaseURLFromDB(context.Background(), database)

		fetched, err := providers.FetchModels(ctx, "ollama", baseURL, "")
		if err != nil || len(fetched) == 0 {
			return
		}

		// Persist to DB cache.
		cached := make([]ollamaCachedModel, 0, len(fetched))
		for _, m := range fetched {
			cached = append(cached, ollamaCachedModel{ID: m.ModelID, Context: m.ContextWindow})
		}
		if data, jerr := json.Marshal(cached); jerr == nil && database != nil {
			_ = database.UpsertCredential(context.Background(), credKeyOllamaModels, string(data))
		}

		// Trigger a full model-list refresh so the picker updates if open.
		w.ListModels(context.Background())
	}()
}

// ollamaBaseURLFromDB returns the user-configured Ollama endpoint from the DB,
// or empty string (which makes FetchModels fall back to localhost:11434).
func ollamaBaseURLFromDB(ctx context.Context, database interface {
	GetCredential(ctx context.Context, key string) (string, bool, error)
}) string {
	if database == nil {
		return ""
	}
	if v, ok, _ := database.GetCredential(ctx, "provider_base_url:ollama"); ok && v != "" {
		return v
	}
	return ""
}

// newNexusWorkspace creates a nexusWorkspace. The workspace registers its own
// callbacks on the ClientConfig so that events are forwarded to the TUI.
func newNexusWorkspace(options runtimeOptions) (*nexusWorkspace, error) {
	_ = appdir.EnsureAppDirs()
	// Keep w.model as "" when no provider is configured — the TUI uses this
	// to detect first-run and auto-open the provider settings panel.
	modelStr := ""
	if options.Model.Provider != "" {
		modelStr = options.Model.String()
	}
	w := &nexusWorkspace{
		model:        modelStr,
		workDir:      options.WorkingDir,
		permMode:     string(options.PermissionMode),
		sqlitePath:   options.SQLitePath,
		clientOpts:   options,
		subagentLogs: make(map[string]string),
	}
	w.debounce = newChunkDebounce(33*time.Millisecond, func(text string) {
		w.send(tui.ChunkMsg{Text: text})
	})

	// When no provider is configured, fall back to the SDK default so the
	// client can be initialized. The user will configure the real provider
	// through the settings panel before submitting the first message.
	if options.Model.Provider == "" {
		options.Model = sdk.DefaultClientConfig().Model
	}

	client, err := newClient(
		options,
		w.promptFn,
		w.onProgress,
		w.onChunk,
		w.onRuntimeEvent,
	)
	if err != nil {
		return nil, err
	}
	w.client = client
	// Probe Ollama in background so model cache is warm by the time the picker opens.
	w.probeOllamaInBackground()
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

func (w *nexusWorkspace) reloadClient(ctx context.Context, modelOverride string) error {
	w.reloadMu.Lock()
	defer w.reloadMu.Unlock()

	if w.busy.Load() {
		return fmt.Errorf("cannot reload provider configuration while a turn is running")
	}

	overrides := runtimeOverrides{
		Model:          strings.TrimSpace(modelOverride),
		PermissionMode: w.permMode,
		WorkingDir:     w.workDir,
		SQLitePath:     w.sqlitePath,
	}
	if overrides.Model == "" {
		overrides.Model = strings.TrimSpace(w.model)
	}

	opts, err := loadRuntimeOptions(overrides)
	if err != nil {
		return err
	}
	opts.Monitoring = w.clientOpts.Monitoring

	displayModel := ""
	if opts.Model.Provider != "" {
		displayModel = opts.Model.String()
	}
	clientOpts := opts
	if clientOpts.Model.Provider == "" {
		clientOpts.Model = sdk.DefaultClientConfig().Model
	}

	client, err := newClient(clientOpts, w.promptFn, w.onProgress, w.onChunk, w.onRuntimeEvent)
	if err != nil {
		return err
	}

	activeID := w.ActiveSessionID()
	var session *sdk.Session
	if activeID != "" {
		session, err = client.LoadSession(ctx, sdk.SessionID(activeID))
		if err != nil {
			_ = client.Close()
			return fmt.Errorf("reload active session: %w", err)
		}
	}

	w.clientMu.Lock()
	oldClient := w.client
	w.client = client
	w.clientOpts = opts
	w.clientMu.Unlock()

	w.sessionMu.Lock()
	w.session = session
	w.sessionMu.Unlock()

	w.model = displayModel
	w.workDir = opts.WorkingDir
	w.permMode = string(opts.PermissionMode)
	w.sqlitePath = opts.SQLitePath

	if oldClient != nil {
		go func(c *sdk.Client) {
			_ = c.Close()
		}(oldClient)
	}
	return nil
}

func (w *nexusWorkspace) ListSessions(ctx context.Context) {
	go func() {
		w.clientMu.RLock()
		client := w.client
		infos, err := client.ListSessions()
		w.clientMu.RUnlock()
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
				ID:        id,
				ShortID:   shortIDStr(id),
				Turns:     info.TotalTurns,
				Tokens:    info.TotalTokens,
				UpdatedAt: time.Unix(info.UpdatedAt, 0),
				CreatedAt: time.Unix(info.CreatedAt, 0),
				Preview:   info.Preview,
			})
		}
		w.send(tui.SessionListMsg{Sessions: sessions})
	}()
}

func (w *nexusWorkspace) CreateSession(ctx context.Context) {
	go func() {
		// Wait for any in-flight provider reload to finish so the session is
		// always bound to the most recent (correctly keyed) client.
		w.reloadMu.Lock()
		w.reloadMu.Unlock() //nolint:staticcheck

		w.clientMu.RLock()
		client := w.client
		sess, err := client.CreateSession(ctx)
		w.clientMu.RUnlock()
		if err != nil {
			w.send(tui.SessionCreatedMsg{Err: err})
			return
		}
		_ = appdir.EnsureSessionDir(string(sess.GetID()))
		w.subagentMu.Lock()
		w.subagentLogs = make(map[string]string)
		w.subagentMu.Unlock()
		w.sessionMu.Lock()
		w.session = sess
		w.sessionMu.Unlock()
		w.send(tui.SessionCreatedMsg{ID: string(sess.GetID())})
	}()
}

func (w *nexusWorkspace) LoadSession(ctx context.Context, id string) {
	go func() {
		w.reloadMu.Lock()
		w.reloadMu.Unlock() //nolint:staticcheck

		w.clientMu.RLock()
		client := w.client
		sess, err := client.LoadSession(ctx, sdk.SessionID(id))
		w.clientMu.RUnlock()
		if err != nil {
			w.send(tui.SessionLoadedMsg{Err: err})
			return
		}
		_ = appdir.EnsureSessionDir(id)
		w.subagentMu.Lock()
		w.subagentLogs = make(map[string]string)
		w.subagentMu.Unlock()
		w.sessionMu.Lock()
		w.session = sess
		w.sessionMu.Unlock()
		messages := sess.GetMessages()
		history := buildSessionHistory(messages)
		w.send(tui.SessionLoadedMsg{ID: string(sess.GetID()), History: history})

		// Backfill session_files for sessions that predate live recording.
		go func(sessionID string, msgs []sdk.Message) {
			cfg, err := engineconfig.Load()
			if err != nil {
				return
			}
			database, err := openCredentialsDB(cfg)
			if err != nil {
				return
			}
			defer database.Close()
			ctx := context.Background()
			if already, _ := database.HasSessionFileEntry(ctx, sessionID); already {
				return // already populated, nothing to do
			}
			backfillSessionFiles(ctx, database, sessionID, msgs)
		}(string(sess.GetID()), messages)
	}()
}

// buildSessionHistory converts raw SDK messages into a flat list of HistoryEntry
// values suitable for replaying in the TUI chat component.
// ToolResultContent.Metadata already carries the full TUI metadata map
// (content, execution_duration_ms, lines_added, exit_code, …) written by
// buildToolResultMessages in the engine — no data is lost.
func buildSessionHistory(messages []sdk.Message) []tui.HistoryEntry {
	// Pre-pass: collect tool result metadata keyed by tool_use_id.
	// Both the raw content string and the full metadata map are captured.
	type toolResult struct {
		content  string
		metadata map[string]any
	}
	resultFor := make(map[string]toolResult, len(messages))
	for _, msg := range messages {
		if msg.Role != sdk.RoleUser {
			continue
		}
		for _, block := range msg.Content {
			if tr, ok := block.(sdk.ToolResultContent); ok {
				r := toolResult{content: tr.Content}
				if tr.Metadata != nil {
					r.metadata = *tr.Metadata
				}
				resultFor[tr.ToolUseID] = r
			}
		}
	}

	var entries []tui.HistoryEntry
	for _, msg := range messages {
		switch msg.Role {
		case sdk.RoleUser:
			var texts []string
			for _, block := range msg.Content {
				if t, ok := block.(sdk.TextContent); ok {
					if s := strings.TrimSpace(t.Text); s != "" {
						texts = append(texts, s)
					}
				}
			}
			if len(texts) > 0 {
				entries = append(entries, tui.HistoryEntry{
					Role: "user",
					Text: strings.Join(texts, "\n"),
				})
			}

		case sdk.RoleAssistant:
			entry := tui.HistoryEntry{Role: "assistant"}
			if msg.Metadata != nil {
				if msg.Metadata.Usage != nil {
					entry.InputTokens = msg.Metadata.Usage.InputTokens
					entry.OutputTokens = msg.Metadata.Usage.OutputTokens
				}
				entry.StopReason = msg.Metadata.StopReason
			}
			for _, block := range msg.Content {
				switch b := block.(type) {
				case sdk.ThinkingContent:
					entry.Thinking = b.Thinking
				case sdk.TextContent:
					if entry.Text != "" {
						entry.Text += "\n"
					}
					entry.Text += b.Text
				case sdk.ToolUseContent:
					tool := tui.HistoryTool{
						ID:    b.ID,
						Name:  b.Name,
						Input: b.Input,
					}
					if r, ok := resultFor[b.ID]; ok {
						// Use the persisted metadata map directly; fall back to
						// building a minimal one from the raw content string.
						if r.metadata != nil {
							tool.Metadata = r.metadata
						} else if r.content != "" {
							tool.Metadata = map[string]any{"content": r.content}
						}
					}
					entry.Tools = append(entry.Tools, tool)
				}
			}
			if entry.Text != "" || entry.Thinking != "" || len(entry.Tools) > 0 {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func (w *nexusWorkspace) DeleteSession(_ context.Context, id string) error {
	w.sessionMu.Lock()
	active := w.session != nil && string(w.session.GetID()) == id
	var sess *sdk.Session
	if active {
		sess = w.session
		w.session = nil
	}
	w.sessionMu.Unlock()
	if sess != nil {
		_ = sess.Interrupt()
		_ = sess.Close()
	}

	w.clientMu.RLock()
	err := w.client.DeleteSession(sdk.SessionID(id))
	w.clientMu.RUnlock()
	if err != nil {
		return err
	}
	// Remove the entire session directory: images, plans, tools, logs — one call.
	appdir.DeleteSessionDir(id)
	return nil
}

func (w *nexusWorkspace) Submit(ctx context.Context, prompt string) {
	w.sessionMu.Lock()
	sess := w.session
	w.sessionMu.Unlock()

	if sess == nil || w.busy.Load() {
		return
	}
	w.busy.Store(true)

	// Wrap with a per-submit cancel so Cancel() can interrupt the API call.
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
		}()
		w.send(tui.TurnStartMsg{
			SessionID: string(sess.GetID()),
		})
		resp, err := sess.SubmitMessage(submitCtx, prompt)
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

func (w *nexusWorkspace) cancelAsyncAgents() int {
	return coreagent.GetDefaultAsyncManager().CloseAllAgents()
}

func (w *nexusWorkspace) Cancel() {
	w.submitMu.Lock()
	cancel := w.submitCancel
	w.submitMu.Unlock()
	if cancel != nil {
		cancel()
	}

	w.sessionMu.Lock()
	sess := w.session
	w.sessionMu.Unlock()
	if sess != nil {
		_ = sess.Interrupt()
	}

	if closed := w.cancelAsyncAgents(); closed > 0 {
		log.Printf("[tui] cancelled %d async sub-agent(s)", closed)
	}

	// Safety net: unblock the UI immediately even if TurnDoneMsg is delayed.
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
		cfg, _ := engineconfig.Load()
		database, _ := openCredentialsDB(cfg)
		if database != nil {
			defer database.Close()
		}

		// Resolve which provider the global (non-scoped) api_key belongs to,
		// based on the persisted model selection.
		globalKeyProvider := sdk.APIProvider("")
		if database != nil {
			if modelStr, ok, _ := database.GetCredential(ctx, credKeyModel); ok && modelStr != "" {
				globalKeyProvider = engineconfig.ParseModelIdentifier(modelStr).Provider
			}
		}

		// isConfigured returns true when the provider has usable credentials
		// (env var, scoped DB key, or global DB key attributed to this provider).
		isConfigured := func(provider sdk.APIProvider) bool {
			for _, ev := range engineconfig.ProviderCredentialEnvVars(provider) {
				if strings.TrimSpace(os.Getenv(ev)) != "" {
					return true
				}
			}
			if database == nil {
				return false
			}
			pid := strings.ToLower(string(provider))
			// Scoped key written by the TUI config panel.
			if v, ok, _ := database.GetCredential(ctx, "api_key:"+pid); ok && v != "" {
				return true
			}
			// Global key written by `nexus config`, attributed to its provider.
			if provider == globalKeyProvider {
				if v, ok, _ := database.GetCredential(ctx, credKeyAPIKey); ok && v != "" {
					return true
				}
			}
			// Cloud providers (Bedrock, Vertex) store region/project instead of a key.
			if len(engineconfig.ProviderCredentialEnvVars(provider)) == 0 {
				for _, ck := range []string{"provider_region:" + pid, "provider_project_id:" + pid} {
					if v, ok, _ := database.GetCredential(ctx, ck); ok && v != "" {
						return true
					}
				}
			}
			return false
		}

		all := providers.AllProvidersInfo()
		var models []tui.ProviderModel

		for provider, info := range all {
			providerStr := string(provider)

			if provider == sdk.APIProviderOllama {
				// Quick live refresh — longer timeout since /api/show is called per model.
				// Falls back to startup-cached list on timeout or error.
				ollamaURL := ollamaBaseURLFromDB(ctx, database)
				liveCtx, liveCancel := context.WithTimeout(ctx, 8*time.Second)
				fetched, liveErr := providers.FetchModels(liveCtx, providerStr, ollamaURL, "")
				liveCancel()

				if liveErr == nil && len(fetched) > 0 {
					// Update the cache in background so future opens are fast.
					go func(list []providers.FetchedModel) {
						cached := make([]ollamaCachedModel, 0, len(list))
						for _, m := range list {
							cached = append(cached, ollamaCachedModel{ID: m.ModelID, Context: m.ContextWindow})
						}
						if data, err := json.Marshal(cached); err == nil {
							if cfg2, err := engineconfig.Load(); err == nil {
								if db2, err := openCredentialsDB(cfg2); err == nil {
									_ = db2.UpsertCredential(context.Background(), credKeyOllamaModels, string(data))
									_ = db2.Close()
								}
							}
						}
					}(fetched)
					for _, m := range fetched {
						desc := m.DisplayName
						if m.ContextWindow > 0 {
							desc = fmt.Sprintf("%s · %dk ctx", m.DisplayName, m.ContextWindow/1000)
						}
						models = append(models, tui.ProviderModel{
							Provider:    providerStr,
							Identifier:  m.ModelID,
							DisplayName: m.DisplayName,
							Description: desc,
							Context:     m.ContextWindow,
						})
					}
				} else if database != nil {
					// Live fetch failed or timed out — serve the startup cache.
					if raw, ok, _ := database.GetCredential(ctx, credKeyOllamaModels); ok && raw != "" {
						var cached []ollamaCachedModel
						if json.Unmarshal([]byte(raw), &cached) == nil {
							for _, m := range cached {
								desc := m.ID
								if m.Context > 0 {
									desc = fmt.Sprintf("%s · %dk ctx", m.ID, m.Context/1000)
								}
								models = append(models, tui.ProviderModel{
									Provider:    providerStr,
									Identifier:  m.ID,
									DisplayName: m.ID,
									Description: desc,
									Context:     m.Context,
								})
							}
						}
					}
				}
				continue
			}

			if !isConfigured(provider) {
				continue
			}

			for _, m := range info.Models {
				models = append(models, tui.ProviderModel{
					Provider:    providerStr,
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
	modelStr := providerID + ":" + modelID
	w.mu.Lock()
	w.model = modelStr
	w.mu.Unlock()
	// Persist asynchronously — DB I/O must not block the BubbleTea event loop.
	// Do NOT call w.send here: p.Send is a blocking channel write in BubbleTea v2,
	// so calling it from within Update (the event loop goroutine) causes a deadlock.
	// The header reads ModelString() directly, so no message is needed.
	go func() {
		if cfg, err := engineconfig.Load(); err == nil {
			if db, err := openCredentialsDB(cfg); err == nil {
				_ = db.UpsertCredential(context.Background(), credKeyModel, modelStr)
				_ = db.Close()
			}
		}
	}()
	go func() {
		if err := w.reloadClient(context.Background(), modelStr); err != nil {
			w.send(tui.ErrMsg{Err: err})
		}
	}()
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
	if err := database.UpsertCredential(ctx, providerCredKey(fieldKey, providerID), value); err != nil {
		return err
	}
	// Re-probe Ollama whenever its endpoint is saved so the model cache stays current.
	if strings.ToLower(providerID) == "ollama" && fieldKey == "provider_base_url" {
		w.probeOllamaInBackground()
	}

	// Always reload when saving a credential. If no model is selected yet (w.model == ""),
	// reloadClient will still build a keyed client using the default provider
	// (anthropic) or whatever provider was just configured, ensuring that the
	// first session creation has valid credentials.
	currentModel := strings.TrimSpace(w.model)
	if currentModel == "" {
		return w.reloadClient(ctx, "")
	}

	if parts := strings.SplitN(currentModel, ":", 2); strings.EqualFold(parts[0], providerID) {
		return w.reloadClient(ctx, "")
	}
	return nil
}

func (w *nexusWorkspace) DeleteProviderField(ctx context.Context, providerID, fieldKey string) error {
	cfg, _ := engineconfig.Load()
	database, err := openCredentialsDB(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.DeleteCredential(ctx, providerCredKey(fieldKey, providerID)); err != nil {
		return err
	}
	// Re-probe with default URL when the Ollama endpoint is cleared.
	if strings.ToLower(providerID) == "ollama" && fieldKey == "provider_base_url" {
		w.probeOllamaInBackground()
	}
	return w.reloadClient(ctx, "")
}

// searchProviderCatalog is the static metadata for each search provider.
const credKeySearXNG = "SEARXNG_BASE_URL"

var searchProviderCatalog = []tui.SearchKeyStatus{
	{ID: "tavily", DisplayName: "Tavily", Description: "AI-optimised search", EnvVar: "TAVILY_API_KEY", DBKey: "TAVILY_API_KEY", NeedsKey: true},
	{ID: "exa", DisplayName: "Exa", Description: "Neural search engine", EnvVar: "EXA_API_KEY", DBKey: "EXA_API_KEY", NeedsKey: true},
	{ID: "jina", DisplayName: "Jina AI", Description: "Reader-based web retrieval", EnvVar: "JINA_API_KEY", DBKey: "JINA_API_KEY", NeedsKey: true},
	{ID: "langsearch", DisplayName: "LangSearch", Description: "Free AI-optimised search", EnvVar: "LANGSEARCH_API_KEY", DBKey: "LANGSEARCH_API_KEY", NeedsKey: true},
	{ID: "searxng", DisplayName: "SearXNG", Description: "Self-hosted meta-search (needs instance URL)", EnvVar: "SEARXNG_BASE_URL", DBKey: credKeySearXNG, NeedsKey: true, FieldLabel: "Instance URL"},
	{ID: "ddg", DisplayName: "DuckDuckGo", Description: "Privacy-friendly fallback", NeedsKey: false},
}

func (w *nexusWorkspace) LoadSearchConfig(_ context.Context) tui.SearchConfig {
	cfg, _ := engineconfig.Load()
	database, dbErr := openCredentialsDB(cfg)
	if dbErr == nil {
		defer database.Close()
	}

	mode, _, _ := func() (string, bool, error) {
		if database == nil {
			return "", false, nil
		}
		return database.GetCredential(context.Background(), "WEB_SEARCH_PROVIDER")
	}()
	if mode == "" {
		mode = os.Getenv("WEB_SEARCH_PROVIDER")
	}
	if mode == "" {
		mode = "auto"
	}

	providers := make([]tui.SearchKeyStatus, len(searchProviderCatalog))
	copy(providers, searchProviderCatalog)
	for i, p := range providers {
		if !p.NeedsKey {
			continue
		}
		if database != nil {
			if _, ok, _ := database.GetCredential(context.Background(), p.DBKey); ok {
				providers[i].IsSet = true
				continue
			}
		}
		// Fallback: check env var (e.g. set by ApplySearchKeys).
		if os.Getenv(p.EnvVar) != "" {
			providers[i].IsSet = true
		}
	}
	return tui.SearchConfig{Mode: mode, Providers: providers}
}

func (w *nexusWorkspace) SaveSearchKey(ctx context.Context, dbKey, value string) error {
	cfg, _ := engineconfig.Load()
	database, err := openCredentialsDB(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.UpsertCredential(ctx, dbKey, value); err != nil {
		return err
	}
	// Apply immediately so the current process uses the new key.
	os.Setenv(strings.TrimPrefix(dbKey, "search:"), value)
	return w.reloadClient(ctx, "")
}

func (w *nexusWorkspace) SaveSearchMode(ctx context.Context, mode string) error {
	cfg, _ := engineconfig.Load()
	database, err := openCredentialsDB(cfg)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.UpsertCredential(ctx, "WEB_SEARCH_PROVIDER", mode); err != nil {
		return err
	}
	os.Setenv("WEB_SEARCH_PROVIDER", mode)
	return w.reloadClient(ctx, "")
}

func (w *nexusWorkspace) LoadToolCatalog(ctx context.Context) []tui.ToolInfo {
	w.clientMu.RLock()
	client := w.client
	surface, err := client.BuildToolSurface(ctx)
	w.clientMu.RUnlock()
	if err != nil || surface == nil {
		return nil
	}
	items := make([]tui.ToolInfo, 0, len(surface.Tools))
	for _, tool := range surface.Tools {
		items = append(items, tui.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Category:    tool.Category,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (w *nexusWorkspace) LoadMCPServers(_ context.Context) []tui.MCPServerInfo {
	w.clientMu.RLock()
	result := w.client.MCPResult()
	w.clientMu.RUnlock()
	if result == nil {
		return nil
	}
	items := make([]tui.MCPServerInfo, 0, len(result.ServerResults))
	for _, server := range result.ServerResults {
		status := "ready"
		errMsg := ""
		if server.Error != nil {
			status = "error"
			errMsg = server.Error.Error()
		}
		items = append(items, tui.MCPServerInfo{
			Name:            server.Name,
			ToolsRegistered: server.ToolsRegistered,
			Status:          status,
			Error:           errMsg,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (w *nexusWorkspace) LoadSkills(_ context.Context) []tui.SkillInfo {
	skills, err := skillspkg.All(w.workDir)
	if err != nil {
		return nil
	}
	items := make([]tui.SkillInfo, 0, len(skills))
	for _, skill := range skills {
		if !skill.UserInvocable {
			continue
		}
		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = strings.TrimSpace(skill.WhenToUse)
		}
		items = append(items, tui.SkillInfo{
			Name:        skill.Name,
			Description: description,
			WhenToUse:   strings.TrimSpace(skill.WhenToUse),
			Source:      string(skill.Source),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (w *nexusWorkspace) Close() {
	if closed := w.cancelAsyncAgents(); closed > 0 {
		log.Printf("[tui] closed %d async sub-agent(s) during shutdown", closed)
	}
	w.clientMu.RLock()
	client := w.client
	w.clientMu.RUnlock()
	if client != nil {
		_ = client.Close()
	}
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
		Metadata:  progress.Metadata,
	})
	// Record file operations in session_files as they complete.
	if string(progress.Stage) == "completed" {
		switch progress.ToolName {
		case "write_file", "edit_file", "apply_patch":
			w.recordSessionFile(progress)
		}
	}
}

func (w *nexusWorkspace) onRuntimeEvent(event sdk.RuntimeEvent) {
	if event.AgentToolUseID == "" {
		return
	}

	w.subagentMu.Lock()
	logText := w.subagentLogs[event.AgentToolUseID]
	var updated bool

	switch event.Type {
	case sdk.RuntimeEventTypeTurnStarted:
		logText += "\n### Sub-Agent Turn Started\n"
		updated = true

	case sdk.RuntimeEventTypeResponseChunk:
		if event.Chunk != nil && event.Chunk.Delta != "" {
			if event.Chunk.DeltaType == "thinking_delta" {
				// We can prepend a blockquote symbol if it's the start of thinking
				if !strings.HasSuffix(logText, "\n> ") && (logText == "" || strings.HasSuffix(logText, "\n")) {
					logText += "> "
				}
				// Replace newline with newline + blockquote symbol for markdown blockquote rendering
				delta := strings.ReplaceAll(event.Chunk.Delta, "\n", "\n> ")
				logText += delta
			} else {
				logText += event.Chunk.Delta
			}
			updated = true
		}

	case sdk.RuntimeEventTypeToolProgress:
		if event.ToolProgress != nil {
			status := string(event.ToolProgress.Stage)
			switch status {
			case "running":
				logText += fmt.Sprintf("\n* ▸ **Tool Call:** `%s` ...\n", event.ToolProgress.ToolName)
			case "completed":
				logText += fmt.Sprintf("* ✓ **Tool Completed:** `%s`\n", event.ToolProgress.ToolName)
			case "failed":
				logText += fmt.Sprintf("* ✗ **Tool Failed:** `%s` (%s)\n", event.ToolProgress.ToolName, event.ToolProgress.Message)
			}
			updated = true
		}

	case sdk.RuntimeEventTypeTurnCompleted:
		logText += "\n### Sub-Agent Turn Completed\n"
		updated = true
	}

	if updated {
		w.subagentLogs[event.AgentToolUseID] = logText
		w.subagentMu.Unlock()

		w.send(tui.ToolProgressMsg{
			ToolUseID: event.AgentToolUseID,
			ToolName:  "subagent_event",
			Status:    "running",
			Label:     "Sub-agent active",
			Metadata: map[string]any{
				"subagent_log": logText,
			},
		})
	} else {
		w.subagentMu.Unlock()
	}
}

// recordSessionFile persists a completed file-write operation to session_files.
// Runs asynchronously so it never blocks the TUI event loop.
func (w *nexusWorkspace) recordSessionFile(progress sdk.ToolProgress) {
	w.sessionMu.Lock()
	sess := w.session
	w.sessionMu.Unlock()
	if sess == nil {
		return
	}
	sessionID := string(sess.GetID())
	meta := progress.Metadata

	filePath, _ := meta["file_path"].(string)
	if filePath == "" {
		return
	}
	op := fileOperation(progress.ToolName, meta)
	linesAdded, _ := intFromAny(meta["lines_added"])
	linesRemoved, _ := intFromAny(meta["lines_removed"])

	go func() {
		cfg, err := engineconfig.Load()
		if err != nil {
			return
		}
		database, err := openCredentialsDB(cfg)
		if err != nil {
			return
		}
		defer database.Close()
		_ = database.UpsertSessionFile(context.Background(), db.SessionFile{
			SessionID:    sessionID,
			ToolUseID:    progress.ToolUseID,
			FilePath:     filePath,
			Operation:    op,
			LinesAdded:   linesAdded,
			LinesRemoved: linesRemoved,
		})
	}()
}

// backfillSessionFiles scans a transcript and populates session_files for any
// write_file, edit_file, or apply_patch tool results found there.
func backfillSessionFiles(ctx context.Context, database *db.DB, sessionID string, messages []sdk.Message) {
	// Build a map: tool_use_id → ToolResultContent.Metadata for file ops.
	type resultMeta struct {
		metadata map[string]any
		ts       int64
	}
	resultMap := make(map[string]resultMeta)
	for _, msg := range messages {
		ts := msg.Timestamp.Unix()
		if ts <= 0 {
			ts = time.Now().Unix()
		}
		for _, block := range msg.Content {
			if tr, ok := block.(sdk.ToolResultContent); ok && tr.Metadata != nil {
				resultMap[tr.ToolUseID] = resultMeta{metadata: *tr.Metadata, ts: ts}
			}
		}
	}

	for _, msg := range messages {
		if msg.Role != sdk.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			tu, ok := block.(sdk.ToolUseContent)
			if !ok {
				continue
			}
			switch tu.Name {
			case "write_file", "edit_file", "apply_patch":
			default:
				continue
			}
			r, hasResult := resultMap[tu.ID]
			var meta map[string]any
			if hasResult {
				meta = r.metadata
			} else {
				meta = map[string]any{}
			}
			filePath, _ := meta["file_path"].(string)
			if filePath == "" {
				filePath, _ = tu.Input["file_path"].(string)
			}
			if filePath == "" {
				continue
			}
			op := fileOperation(tu.Name, meta)
			linesAdded, _ := intFromAny(meta["lines_added"])
			linesRemoved, _ := intFromAny(meta["lines_removed"])
			ts := r.ts
			if ts == 0 {
				ts = time.Now().Unix()
			}
			_ = database.UpsertSessionFile(ctx, db.SessionFile{
				SessionID:     sessionID,
				ToolUseID:     tu.ID,
				FilePath:      filePath,
				Operation:     op,
				TimestampUnix: ts,
				LinesAdded:    linesAdded,
				LinesRemoved:  linesRemoved,
			})
		}
	}
}

func fileOperation(toolName string, meta map[string]any) string {
	switch toolName {
	case "write_file":
		if t, _ := meta["type"].(string); t != "" {
			return t // "create" or "update"
		}
		return "write"
	case "edit_file":
		return "edit"
	case "apply_patch":
		return "patch"
	}
	return toolName
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
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
		Metadata: req.Metadata,
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
	if lf := openCLILogFile(); lf != nil {
		log.SetOutput(lf)
	} else {
		log.SetOutput(io.Discard)
	}

	ws, err := newNexusWorkspace(options)
	if err != nil {
		return err
	}
	defer ws.Close()

	return tuiapp.Run(ws, ctx)
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

// openCLILogFile opens (or creates) ~/.config/nexus-cli/logs/cli.log for appending.
// Returns nil if the file cannot be created — caller falls back to io.Discard.
func openCLILogFile() *os.File {
	config, err := engineconfig.Load()
	if err != nil {
		return nil
	}
	logDir := filepath.Join(config.RuntimeRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(logDir, "cli.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil
	}
	return f
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func shortIDStr(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
