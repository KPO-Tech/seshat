package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
	longtermStore "github.com/EngineerProjects/nexus-engine/internal/memory/longterm"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	slackgo "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const defaultModel = "mistral:mistral-small-latest"

// streamInterval is how often we push incremental text updates to Slack.
// Slack rate-limits UpdateMessage; 1.5s is safe.
const streamInterval = 1500 * time.Millisecond

// slackMaxLen is the Slack text field character limit.
const slackMaxLen = 2900

// requestState holds per-request live state for the Slack placeholder.
type requestState struct {
	mu         sync.Mutex
	statusLine string // from RuntimeEventFn (tool progress)
	accText    string // from ResponseChunkFn (streaming text)
}

func (r *requestState) setStatus(line string) {
	r.mu.Lock()
	r.statusLine = line
	r.mu.Unlock()
}

func (r *requestState) addChunk(delta string) {
	r.mu.Lock()
	r.accText += delta
	r.mu.Unlock()
}

func (r *requestState) snapshot() (status, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLine, r.accText
}

type bot struct {
	nexus *sdk.Client
	api   *slackgo.Client

	mu       sync.Mutex
	sessions map[string]sdk.SessionID // channelID → nexus sessionID

	// callbackMu serialises SetResponseChunkFn / SetRuntimeEventFn so concurrent
	// channel messages don't overwrite each other's per-request callbacks.
	callbackMu sync.Mutex

	// pendingMu guards pending and textPending — used by the Slack PromptFn.
	pendingMu   sync.Mutex
	pending     map[string]chan string // blockID → result (button clicks)
	textPending map[string]chan string // "channel:threadTS" → result (thread replies)
}

func main() {
	// Give the bot its own isolated runtime root so session artifacts,
	// cache, and logs go to ~/.config/nexus-slack/ and don't mix with
	// the CLI's ~/.config/nexus-cli/.
	if os.Getenv(runtimepath.EnvRuntimeRoot) == "" {
		os.Setenv(runtimepath.EnvRuntimeRoot, runtimepath.DefaultConfigDir("nexus-slack"))
	}

	botToken := mustEnv("NEXUS_SLACK_BOT_TOKEN")
	appToken := mustEnv("NEXUS_SLACK_APP_TOKEN")

	cfg, err := engineconfig.Load()
	if err != nil {
		log.Fatalf("[nexus-bot] config: %v", err)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultModel
	}

	model := resolveModel(cfg)
	apiKey := engineconfig.ResolveAPIKey(cfg, model.Provider)

	providerCfg := providers.GetProviderConfig(model.Provider)
	if providerCfg == nil {
		providerCfg = &providers.Config{Provider: model.Provider}
	}
	providerCfg.APIKey = apiKey
	if cfg.ProviderBaseURL != "" {
		providerCfg.BaseURL = cfg.ProviderBaseURL
	}

	// Long-term memory backed by SQLite (separate file from sessions).
	var ltMemory sdk.LongTermMemory
	memDBPath := memoryDBPath()
	if ltDB, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(memDBPath)); err == nil {
		ltMemory = longtermStore.NewSQLiteStore(ltDB.SQL())
		log.Printf("[nexus-bot] long-term memory: %s", memDBPath)
	} else {
		log.Printf("[nexus-bot] long-term memory unavailable: %v", err)
	}

	mcpServers := loadMCPServers(workdir())
	if len(mcpServers) > 0 {
		log.Printf("[nexus-bot] loaded %d MCP server(s)", len(mcpServers))
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	sysPrompt := buildSlackSystemPrompt()

	// b is used by PromptFn below; build it first then assign nexusClient.
	b := &bot{
		api:         slackgo.New(botToken, slackgo.OptionAppLevelToken(appToken)),
		sessions:    make(map[string]sdk.SessionID),
		pending:     make(map[string]chan string),
		textPending: make(map[string]chan string),
	}

	nexusClient, err := sdk.NewClient(&sdk.ClientConfig{
		APIKey:            apiKey,
		Model:             model,
		MaxTokens:         maxTokens,
		PermissionMode:    sdk.PermissionModeBypass,
		AutoCompact:       true,
		PersistSessions:   true,
		SessionSQLitePath: nexusDBPath(),
		WorkingDir:        workdir(),
		ProviderConfig:    providerCfg,
		MCPServers:        mcpServers,
		LongTermMemory:    ltMemory,
		PromptFn:          b.makeSlackPromptFn(),
		PromptConfig: &sdk.PromptConfig{
			SystemPrompt: &sysPrompt,
		},
		OnSessionTitled: func(id sdk.SessionID, title string) {
			log.Printf("[nexus-bot] session %s titled: %s", id, title)
		},
	})
	if err != nil {
		log.Fatalf("[nexus-bot] nexus client: %v", err)
	}
	defer nexusClient.Close()
	b.nexus = nexusClient

	sm := socketmode.New(b.api)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleSignals(cancel)
	go b.handleEvents(ctx, sm)

	log.Printf("[nexus-bot] ready — model: %s/%s  max_tokens: %d", model.Provider, model.Model, maxTokens)
	if err := sm.RunContext(ctx); err != nil && err != context.Canceled {
		log.Fatalf("[nexus-bot] socket mode: %v", err)
	}
}

func (b *bot) handleEvents(ctx context.Context, sm *socketmode.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sm.Events:
			if !ok {
				return
			}
			switch evt.Type {
			case socketmode.EventTypeConnecting,
				socketmode.EventTypeConnectionError,
				socketmode.EventTypeConnected,
				socketmode.EventTypeHello,
				socketmode.EventTypeInvalidAuth,
				socketmode.EventTypeDisconnect:
				log.Printf("[nexus-bot] socket: %s", evt.Type)

			case socketmode.EventTypeEventsAPI:
				ev, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok || evt.Request == nil {
					continue
				}
				if err := sm.Ack(*evt.Request); err != nil {
					log.Printf("[nexus-bot] ack error: %v", err)
				}
				b.dispatch(ctx, ev)

			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slackgo.InteractionCallback)
				if !ok || evt.Request == nil {
					continue
				}
				if err := sm.Ack(*evt.Request); err != nil {
					log.Printf("[nexus-bot] ack error (interactive): %v", err)
				}
				b.handleInteraction(callback)

			default:
				if evt.Request != nil {
					if err := sm.Ack(*evt.Request); err != nil {
						log.Printf("[nexus-bot] ack error: %v", err)
					}
				}
			}
		}
	}
}

func (b *bot) dispatch(ctx context.Context, ev slackevents.EventsAPIEvent) {
	switch inner := ev.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		if inner.BotID != "" {
			return
		}
		replyTS := inner.ThreadTimeStamp
		if replyTS == "" {
			replyTS = inner.TimeStamp
		}
		// If a text prompt is waiting for a reply in this thread, route to it.
		threadKey := inner.Channel + ":" + replyTS
		b.pendingMu.Lock()
		textCh, waiting := b.textPending[threadKey]
		b.pendingMu.Unlock()
		if waiting {
			select {
			case textCh <- stripMention(inner.Text):
			default:
			}
			return
		}
		go b.onMessage(ctx, inner.Channel, replyTS, inner.Text)

	case *slackevents.MessageEvent:
		if inner.BotID != "" || inner.SubType != "" {
			return
		}
		replyTS := inner.ThreadTimeStamp
		if replyTS == "" {
			replyTS = inner.TimeStamp
		}
		threadKey := inner.Channel + ":" + replyTS
		b.pendingMu.Lock()
		textCh, waiting := b.textPending[threadKey]
		b.pendingMu.Unlock()
		if waiting {
			select {
			case textCh <- stripMention(inner.Text):
			default:
			}
			return
		}
		if ev.InnerEvent.Type == "message" {
			go b.onMessage(ctx, inner.Channel, replyTS, inner.Text)
		}
	}
}

func (b *bot) onMessage(ctx context.Context, channel, replyTS, text string) {
	query := stripMention(text)
	if query == "" {
		return
	}

	log.Printf("[nexus-bot] message channel=%s query=%q", channel, query)

	_, thinkTS, err := b.api.PostMessageContext(ctx, channel,
		slackgo.MsgOptionText(":hourglass_flowing_sand: _Nexus is thinking..._", false),
		slackgo.MsgOptionTS(replyTS),
		slackgo.MsgOptionDisableLinkUnfurl(),
	)
	if err != nil {
		log.Printf("[nexus-bot] post placeholder: %v", err)
		return
	}

	session, err := b.getOrCreateSession(ctx, channel)
	if err != nil {
		b.updateMsg(ctx, channel, thinkTS, fmt.Sprintf(":x: Could not start session: %v", err))
		return
	}

	// ── Live callbacks ─────────────────────────────────────────────────────────
	b.callbackMu.Lock()
	state := &requestState{}

	b.nexus.SetResponseChunkFn(func(chunk sdk.ResponseChunk) {
		if chunk.Delta != "" {
			state.addChunk(chunk.Delta)
		}
	})

	b.nexus.SetRuntimeEventFn(func(evt sdk.RuntimeEvent) {
		switch evt.Type {
		case sdk.RuntimeEventTypeToolProgress:
			tp := evt.ToolProgress
			if tp != nil && tp.Stage == sdk.ToolProgressStageRunning && !isPlanModeTool(tp.ToolName) {
				icon := toolIcon(tp.ToolName)
				msg := tp.Message
				if msg == "" {
					msg = tp.ToolName + "..."
				}
				state.setStatus(fmt.Sprintf("%s _%s_", icon, msg))
				log.Printf("[nexus-bot] tool: %s — %s", tp.ToolName, msg)
			}

		case sdk.RuntimeEventTypePlanSubmitted:
			state.setStatus("📋 _Planning..._")

		case sdk.RuntimeEventTypeExecutionModeChanged:
			if evt.ExecutionMode == "execute" {
				state.setStatus("⚡ _Executing plan..._")
			}
		}
	})
	b.callbackMu.Unlock()

	// ── Ticker: update placeholder every streamInterval ────────────────────────
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(streamInterval)
		defer ticker.Stop()
		var lastDisplay string
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				status, text := state.snapshot()
				var display string
				if text != "" {
					display = slackTrunc(text, slackMaxLen-1) + "▌"
				} else if status != "" {
					display = ":hourglass_flowing_sand: " + status
				}
				if display != "" && display != lastDisplay {
					lastDisplay = display
					b.updateMsg(ctx, channel, thinkTS, display)
				}
			}
		}
	}()

	// Inject channel context so the Slack PromptFn knows where to post questions.
	msgCtx := context.WithValue(ctx, channelCtxKey{}, channelCtxVal{
		Channel:  channel,
		ThreadTS: replyTS,
	})

	startTime := time.Now()
	resp, err := session.SubmitMessage(msgCtx, query)

	close(done)
	b.callbackMu.Lock()
	b.nexus.SetResponseChunkFn(nil)
	b.nexus.SetRuntimeEventFn(nil)
	b.callbackMu.Unlock()

	if err != nil {
		b.updateMsg(ctx, channel, thinkTS, fmt.Sprintf(":x: Agent error: %v", err))
		return
	}

	answer := mdToMrkdwn(extractAnswer(resp))
	if answer == "" {
		answer = "_No response generated._"
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	footer := fmt.Sprintf("\n\n_— Nexus for Slack · %dms_", elapsed.Milliseconds())

	if tools := extractToolsUsed(resp); len(tools) > 0 {
		log.Printf("[nexus-bot] tools used: %s", strings.Join(tools, ", "))
		footer += fmt.Sprintf(" · 🔧 _%s_", strings.Join(tools, ", "))
	}

	chunks := splitForSlack(answer, footer, slackMaxLen)
	b.updateMsg(ctx, channel, thinkTS, chunks[0])
	for _, extra := range chunks[1:] {
		if _, _, err := b.api.PostMessageContext(ctx, channel,
			slackgo.MsgOptionText(extra, false),
			slackgo.MsgOptionTS(replyTS),
			slackgo.MsgOptionDisableLinkUnfurl(),
		); err != nil {
			log.Printf("[nexus-bot] post continuation: %v", err)
		}
	}

	// Upload files produced during this turn (workspace + generated images/audio).
	go b.uploadTurnArtifacts(ctx, channel, replyTS, session.GetID(), startTime)
}

func (b *bot) getOrCreateSession(ctx context.Context, channelID string) (*sdk.Session, error) {
	b.mu.Lock()
	sessionID, exists := b.sessions[channelID]
	b.mu.Unlock()

	if exists {
		s, err := b.nexus.LoadSession(ctx, sessionID)
		if err == nil {
			return s, nil
		}
		log.Printf("[nexus-bot] reload session %s failed (%v) — creating new", sessionID, err)
	}

	s, err := b.nexus.CreateSessionWithAdditional(ctx, map[string]any{
		"slack_channel": channelID,
		"source":        "nexus-slack-bot",
	})
	if err != nil {
		return nil, err
	}

	// Each session gets its own workspace inside the session dir.
	// This keeps the agent's file work co-located with the session and makes
	// cleanup trivial (os.RemoveAll(sessions/{id}/)).
	workspace := sessionWorkspaceDir(s.GetID())
	if mkErr := os.MkdirAll(workspace, 0o755); mkErr == nil {
		s.SetWorkingDirectory(workspace)
	}

	b.mu.Lock()
	b.sessions[channelID] = s.GetID()
	b.mu.Unlock()

	log.Printf("[nexus-bot] new session %s for channel %s workspace: %s", s.GetID(), channelID, workspace)
	return s, nil
}

func (b *bot) updateMsg(ctx context.Context, channel, ts, text string) {
	_, _, _, err := b.api.UpdateMessageContext(ctx, channel, ts,
		slackgo.MsgOptionText(text, false),
		slackgo.MsgOptionDisableLinkUnfurl(),
	)
	if err != nil {
		log.Printf("[nexus-bot] update message: %v", err)
	}
}

// toolIcon returns a Slack emoji for a tool name.
func toolIcon(name string) string {
	switch {
	case strings.Contains(name, "search"):
		return "🔍"
	case strings.Contains(name, "browser"), strings.Contains(name, "web_fetch"):
		return "🌐"
	case strings.Contains(name, "file"), strings.Contains(name, "read"), strings.Contains(name, "write"):
		return "📄"
	case strings.Contains(name, "memory"):
		return "🧠"
	case strings.Contains(name, "linkedin"):
		return "💼"
	case strings.Contains(name, "agent"):
		return "🤖"
	case strings.Contains(name, "code"), strings.Contains(name, "exec"):
		return "⚙️"
	default:
		return "🔧"
	}
}

// extractAnswer pulls only the current-turn assistant text from the session response.
func extractAnswer(resp *sdk.SessionResponse) string {
	lastUserIdx := -1
	for i, msg := range resp.Messages {
		if msg.Role == sdk.RoleUser {
			lastUserIdx = i
		}
	}
	var sb strings.Builder
	for i, msg := range resp.Messages {
		if i <= lastUserIdx {
			continue
		}
		if msg.Role != sdk.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if tc, ok := block.(sdk.TextContent); ok && tc.Text != "" {
				sb.WriteString(tc.Text)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// isPlanModeTool returns true for tools that are internal plan-mode mechanics
// and should not be surfaced to users in the Slack UI.
func isPlanModeTool(name string) bool {
	switch name {
	case "enter_plan_mode", "exit_plan_mode", "submit_plan", "request_permissions":
		return true
	}
	return false
}

// extractToolsUsed returns deduplicated visible tool names called during the response.
// Internal plan-mode and permission tools are excluded.
func extractToolsUsed(resp *sdk.SessionResponse) []string {
	seen := map[string]bool{}
	var tools []string
	for _, msg := range resp.Messages {
		for _, block := range msg.Content {
			if tu, ok := block.(sdk.ToolUseContent); ok && !seen[tu.Name] && !isPlanModeTool(tu.Name) {
				seen[tu.Name] = true
				tools = append(tools, tu.Name)
			}
		}
	}
	return tools
}

// mdToMrkdwn converts standard Markdown to Slack mrkdwn.
var (
	reMdBoldItalic = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	reMdBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMdStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reMdLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reMdHeader     = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reMdHRule      = regexp.MustCompile(`(?m)^---+\s*$`)
)

func mdToMrkdwn(s string) string {
	s = reMdBoldItalic.ReplaceAllString(s, "*_$1_*")
	s = reMdBold.ReplaceAllString(s, "*$1*")
	s = reMdStrike.ReplaceAllString(s, "~$1~")
	s = reMdLink.ReplaceAllString(s, "<$2|$1>")
	s = reMdHeader.ReplaceAllString(s, "*$1*")
	s = reMdHRule.ReplaceAllString(s, "")
	return s
}

// splitForSlack splits answer+footer into chunks that fit within maxLen.
func splitForSlack(answer, footer string, maxLen int) []string {
	if len(answer)+len(footer) <= maxLen {
		return []string{answer + footer}
	}
	var chunks []string
	remaining := answer
	for len(remaining) > 0 {
		isLast := len(remaining) <= maxLen
		if isLast {
			if len(remaining)+len(footer) <= maxLen {
				chunks = append(chunks, remaining+footer)
			} else {
				chunks = append(chunks, remaining)
				chunks = append(chunks, footer)
			}
			break
		}
		cut := maxLen
		for i := cut; i > maxLen-300 && i > 0; i-- {
			if remaining[i] == '\n' {
				cut = i + 1
				break
			}
		}
		chunks = append(chunks, remaining[:cut])
		remaining = strings.TrimSpace(remaining[cut:])
	}
	return chunks
}

// slackTrunc truncates s to max bytes, cutting at a word boundary when possible.
func slackTrunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - 1
	for cut > max-50 && cut > 0 && s[cut] != ' ' && s[cut] != '\n' {
		cut--
	}
	return s[:cut] + "…"
}

// stripMention removes <@UXXXXXXX> Slack mention syntax from text.
func stripMention(text string) string {
	s := text
	for strings.Contains(s, "<@") {
		start := strings.Index(s, "<@")
		end := strings.Index(s[start:], ">")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	return strings.TrimSpace(s)
}

func resolveModel(cfg engineconfig.Config) sdk.ModelIdentifier {
	raw := strings.TrimSpace(cfg.Model)
	model := engineconfig.ParseModelIdentifier(raw)
	if engineconfig.HasExplicitProviderPrefix(raw) {
		return model
	}
	provider := engineconfig.DetectProviderFromModel(raw)
	if provider == "" {
		_, provider = engineconfig.EffectiveAPIKeyAndProvider(cfg)
	}
	if provider == "" {
		provider = model.Provider
	}
	model.Provider = provider
	return model
}

// loadMCPServers reads MCP configs and converts them to sdk.MCPServerConfig.
// It loads from the bot runtime root (NEXUS_RUNTIME_ROOT = ~/.config/nexus-slack/)
// and falls back to the CLI config (~/.config/nexus-cli/mcp.json) so users
// don't have to duplicate their MCP setup for the bot.
func loadMCPServers(cwd string) []sdk.MCPServerConfig {
	result := mcp.LoadMcpConfigs(cwd)

	// Pull in the CLI's mcp.json for servers not already defined in the bot config.
	cliMcpPath := filepath.Join(runtimepath.DefaultConfigDir("nexus-cli"), "mcp.json")
	if cliCfg, _ := mcp.ParseMcpConfigFromFile(cliMcpPath); len(cliCfg.MCPServers) > 0 {
		for name, srv := range cliCfg.MCPServers {
			if _, exists := result.Servers[name]; !exists {
				result.Servers[name] = mcp.ScopedMcpServerConfig{McpServerConfig: srv}
			}
		}
	}

	var servers []sdk.MCPServerConfig
	for name, scoped := range result.Servers {
		cfg := scoped.McpServerConfig
		srv := sdk.MCPServerConfig{
			Name:    name,
			Command: cfg.Command,
			Args:    cfg.Args,
			URL:     cfg.URL,
			Env:     cfg.Env,
			Headers: cfg.Headers,
		}
		switch cfg.Type {
		case mcp.ServerTypeHTTP:
			srv.Transport = sdk.MCPTransportHTTP
		case mcp.ServerTypeSSE:
			srv.Transport = sdk.MCPTransportSSE
		case mcp.ServerTypeWebSocket:
			srv.Transport = sdk.MCPTransportWebSocket
		default:
			srv.Transport = sdk.MCPTransportStdio
		}
		servers = append(servers, srv)
	}
	return servers
}

func nexusDBPath() string {
	if p := os.Getenv("NEXUS_SLACK_DB_PATH"); p != "" {
		return p
	}
	return runtimepath.Join("", "sessions.db")
}

func memoryDBPath() string {
	return runtimepath.Join("", "memory.db")
}

func workdir() string {
	wd, _ := os.Getwd()
	return wd
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("[nexus-bot] required env var %s is not set", key)
	}
	return v
}

func handleSignals(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("[nexus-bot] shutting down...")
	cancel()
}
