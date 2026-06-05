package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

type mockWorkspace struct{}

func (mockWorkspace) ListSessions(context.Context)                {}
func (mockWorkspace) CreateSession(context.Context)               {}
func (mockWorkspace) LoadSession(context.Context, string)         {}
func (mockWorkspace) DeleteSession(context.Context, string) error { return nil }
func (mockWorkspace) Submit(context.Context, string)              {}
func (mockWorkspace) Cancel()                                     {}
func (mockWorkspace) ActiveSessionID() string                     { return "" }
func (mockWorkspace) IsBusy() bool                                { return false }
func (mockWorkspace) ModelString() string                         { return "test/model" }
func (mockWorkspace) WorkingDir() string                          { return "/tmp" }
func (mockWorkspace) PermissionMode() string                      { return "default" }
func (mockWorkspace) ListModels(context.Context)                  {}
func (mockWorkspace) SetModel(string, string)                     {}
func (mockWorkspace) Subscribe(*tea.Program)                      {}
func (mockWorkspace) Close()                                      {}
func (mockWorkspace) LoadProviderConfig(context.Context) []tui.ProviderStatus {
	return nil
}
func (mockWorkspace) LoadToolCatalog(context.Context) []tui.ToolInfo {
	return []tui.ToolInfo{{Name: "bash", Description: "Run shell commands", Category: "system"}}
}
func (mockWorkspace) LoadMCPServers(context.Context) []tui.MCPServerInfo {
	return []tui.MCPServerInfo{{Name: "github", ToolsRegistered: 3, Status: "ready"}}
}
func (mockWorkspace) LoadSkills(context.Context) []tui.SkillInfo {
	return []tui.SkillInfo{{Name: "summarise-pr", Description: "Summarise a pull request", Source: "bundled"}}
}
func (mockWorkspace) SaveProviderField(context.Context, string, string, string) error {
	return nil
}
func (mockWorkspace) DeleteProviderField(context.Context, string, string) error {
	return nil
}

func TestModelRelayoutPropagatesChildSizes(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 80
	m.height = 24

	m = m.relayout()

	cw, ch := m.chat.Size()
	if cw != 76 {
		t.Fatalf("expected chat width 76, got %d", cw)
	}
	if ch != 19 {
		t.Fatalf("expected chat height 19, got %d", ch)
	}
	sw, sh := m.sessions.Size()
	if sw != 80 {
		t.Fatalf("expected session width 80, got %d", sw)
	}
	if sh != 24 {
		t.Fatalf("expected session height 24, got %d", sh)
	}
}

func TestModelPrevChatStateDependsOnActiveSession(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())

	if got := m.prevChatState(); got != stateWelcome {
		t.Fatalf("expected welcome state without active session, got %v", got)
	}

	m.activeSession = "sess-1"
	if got := m.prevChatState(); got != stateChat {
		t.Fatalf("expected chat state with active session, got %v", got)
	}
}

func TestModelInputViewUsesPromptStyleAndHint(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 100
	m.height = 30
	m = m.relayout()
	view := m.inputView()
	if strings.Contains(view, "enter send") || strings.Contains(view, "chat") {
		t.Fatalf("expected input view to avoid inline helper chrome, got %q", view)
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╯") {
		t.Fatalf("expected input view to render a compact bordered composer, got %q", view)
	}
	if !strings.Contains(view, ">") {
		t.Fatalf("expected input view to include prompt marker, got %q", view)
	}
}

func TestModelResizeInputUsesTextareaLineCount(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.input.SetValue(strings.Repeat("line\n", 20))
	m = m.resizeInput()
	if got := m.input.Height(); got != inputMaxH {
		t.Fatalf("expected input height %d, got %d", inputMaxH, got)
	}
}

func TestModelHeaderRendersModelAndStatusPills(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 100
	m.activeSession = "session-1234567890abcdef"
	header := m.header()
	if !strings.Contains(header, "NEXUS") {
		t.Fatalf("expected header to include wordmark, got %q", header)
	}
	if !strings.Contains(header, "test/model") {
		t.Fatalf("expected header to include model pill, got %q", header)
	}
	if !strings.Contains(header, "live") {
		t.Fatalf("expected header to include session status pill, got %q", header)
	}
	if !strings.Contains(header, common.ShortID(m.activeSession)) {
		t.Fatalf("expected header to include short session id, got %q", header)
	}
}

func TestModelStatusLineShowsBusyAndUsage(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 100
	m.busy = true
	busy := m.statusLine()
	if !strings.Contains(busy, "working") {
		t.Fatalf("expected busy status line to mention working, got %q", busy)
	}

	m.busy = false
	m.busy = false
	m.lastTurnErr = "boom"
	errLine := m.statusLine()
	if !strings.Contains(errLine, "failed") {
		t.Fatalf("expected error status line to mention failed, got %q", errLine)
	}
}

func TestModelFooterSimplifiesPrimaryActions(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 100
	m.sessionInputTokens = 10
	m.sessionOutputTokens = 5
	footer := m.footer()
	if strings.Contains(footer, "ctrl+e") || strings.Contains(footer, "chat/tools") {
		t.Fatalf("expected footer to remove old select/tools hints, got %q", footer)
	}
	if !strings.Contains(footer, "ctrl+p") || !strings.Contains(footer, "settings") || !strings.Contains(footer, "15 total") {
		t.Fatalf("expected footer to include settings and total token usage, got %q", footer)
	}
}

func TestModelViewChatIncludesStatusLine(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 120
	m.height = 30
	m.activeSession = "session-123"
	m.state = stateChat
	m.busy = true
	m = m.relayout()
	m.chat.AddUserMessage("hello")
	view := m.viewChat()
	if !strings.Contains(view, "working") {
		t.Fatalf("expected chat view to include busy status line, got %q", view)
	}
}

func TestModelSlashKeyFallsThroughToInput(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateChat
	consumed, cmd := m.handleKey(tea.KeyPressMsg{Text: "/"})
	if consumed {
		t.Fatalf("expected slash to fall through to the textarea for skill input")
	}
	if cmd != nil {
		t.Fatalf("expected slash key handling not to emit a command")
	}
	if got := m.state; got != stateChat {
		t.Fatalf("expected slash to keep chat state, got %v", got)
	}
}

func TestModelCtrlShiftCCopiesActiveSelection(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 120
	m.height = 30
	m.state = stateChat
	m.activeSession = "session-123"
	m = m.relayout()
	m.chat.AddUserMessage("hello world")
	m.chat.HandleMouseDown(0, 0)
	m.chat.HandleMouseDrag(8, 0)
	m.chat.HandleMouseUp(8, 0)

	consumed, cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c', ShiftedCode: 'C', Mod: tea.ModCtrl | tea.ModShift}))
	if !consumed {
		t.Fatalf("expected ctrl+shift+c to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected ctrl+shift+c to return a clipboard command")
	}
	if m.copyNotice == "" {
		t.Fatalf("expected a copy notice")
	}
}

func TestModelRightClickCopiesActiveSelection(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.width = 120
	m.height = 30
	m.state = stateChat
	m.activeSession = "session-123"
	m = m.relayout()
	m.chat.AddUserMessage("hello world")
	m.chat.HandleMouseDown(0, 0)
	m.chat.HandleMouseDrag(8, 0)
	m.chat.HandleMouseUp(8, 0)

	layout := m.currentChatLayout()
	cmd := m.handleMouseClick(tea.MouseClickMsg(tea.Mouse{X: layout.chatX + 2, Y: layout.chatY, Button: tea.MouseRight}))
	if cmd == nil {
		t.Fatalf("expected right click to return a clipboard command")
	}
	if m.copyNotice == "" {
		t.Fatalf("expected a copy notice")
	}
}

func TestClipboardNoticeReflectsCapabilities(t *testing.T) {
	if got := clipboardNotice("Selection copied", true, false); got != "Selection copied" {
		t.Fatalf("expected native clipboard success notice, got %q", got)
	}
	if got := clipboardNotice("Selection copied", false, true); got != "Selection copied (terminal clipboard requested)" {
		t.Fatalf("expected terminal clipboard fallback notice, got %q", got)
	}
	if got := clipboardNotice("Selection copied", false, false); got != "Clipboard unavailable: install wl-clipboard or xclip" {
		t.Fatalf("expected unavailable notice, got %q", got)
	}
}

func TestModelCtrlPOpensSettingsRoot(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateChat
	consumed, cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))
	if !consumed {
		t.Fatalf("expected ctrl+p to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected ctrl+p not to emit an async command")
	}
	if got := m.state; got != stateCommands {
		t.Fatalf("expected settings hub state, got %v", got)
	}
	if sel := m.commands.Selected(); sel == nil || sel.Name != "Commands" {
		t.Fatalf("expected settings root to start on Commands, got %+v", sel)
	}
}

func TestModelSettingsEnterCommandsStaysInHub(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateCommands
	m.commands.Open("")
	cmd := m.activateSettingsSelection()
	if cmd != nil {
		t.Fatalf("expected opening commands section not to emit a command")
	}
	if sel := m.commands.Selected(); sel == nil || sel.Name != "New Session" {
		t.Fatalf("expected commands section to open on first command, got %+v", sel)
	}
}

func TestModelSettingsProvidersRouteToConfig(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateCommands
	m.commands.Open("")
	m.commands.Down()
	cmd := m.activateSettingsSelection()
	if cmd == nil {
		t.Fatalf("expected providers route to emit a load command")
	}
	if got := m.state; got != stateProviderConfig {
		t.Fatalf("expected provider config state, got %v", got)
	}
	if got := m.returnState; got != stateCommands {
		t.Fatalf("expected return state to point back to settings, got %v", got)
	}
}

func TestModelCtrlPLoadsLiveSettingsSections(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateChat
	consumed, cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))
	if !consumed {
		t.Fatalf("expected ctrl+p to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected ctrl+p not to emit an async command")
	}
	if !m.commands.OpenSection("tools") {
		t.Fatalf("expected tools section to open")
	}
	if sel := m.commands.Selected(); sel == nil || sel.Name != "bash" {
		t.Fatalf("expected live tools section to include bash, got %+v", sel)
	}
	if !m.commands.Back() {
		t.Fatalf("expected to return to settings root from tools")
	}
	m.commands.Down()
	m.commands.Down()
	m.commands.Down()
	m.commands.OpenSection("mcp")
	if sel := m.commands.Selected(); sel == nil || sel.Name != "github" {
		t.Fatalf("expected live mcp section to include github, got %+v", sel)
	}
	if !m.commands.Back() {
		t.Fatalf("expected to return to settings root from mcp")
	}
	m.commands.Down()
	m.commands.Down()
	m.commands.OpenSection("skills")
	if sel := m.commands.Selected(); sel == nil || sel.Name != "/summarise-pr" {
		t.Fatalf("expected live skills section to include /summarise-pr, got %+v", sel)
	}
}

func TestModelSettingsSkillSelectionPrimesComposer(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateCommands
	m.activeSession = "session-123"
	m.refreshSettingsHubData()
	if !m.commands.OpenSection("skills") {
		t.Fatalf("expected skills section to open")
	}
	cmd := m.activateSettingsSelection()
	if cmd == nil {
		t.Fatalf("expected skill selection to focus the composer")
	}
	if got := m.state; got != stateChat {
		t.Fatalf("expected state to return to chat, got %v", got)
	}
	if got := m.input.Value(); got != "/summarise-pr " {
		t.Fatalf("expected skill to be inserted into composer, got %q", got)
	}
	if got := m.focus; got != uiFocusEditor {
		t.Fatalf("expected editor focus after inserting skill, got %v", got)
	}
}

func TestModelSlashSkillPopupOpensWhileTyping(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateChat
	m.width = 100
	m.height = 30
	m = m.relayout()
	m.input.SetValue("/sum")
	m.syncComposerAssist()
	if !m.skillCompletions.IsOpen() {
		t.Fatalf("expected slash skill popup to open for /sum")
	}
	view := m.inputView()
	if !strings.Contains(view, "/summarise-pr") {
		t.Fatalf("expected skill popup to render matching skill, got %q", view)
	}
}

func TestModelSlashSkillPopupSelectionPrimesComposer(t *testing.T) {
	m := New(mockWorkspace{}, context.Background())
	m.state = stateChat
	m.width = 100
	m.height = 30
	m = m.relayout()
	m.input.SetValue("/sum")
	m.syncComposerAssist()
	consumed, cmd := m.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !consumed {
		t.Fatalf("expected enter to be handled by slash skill popup")
	}
	if cmd != nil {
		t.Fatalf("expected skill popup selection not to emit a command")
	}
	if got := m.input.Value(); got != "/summarise-pr " {
		t.Fatalf("expected selected skill to replace typed query, got %q", got)
	}
	if m.skillCompletions.IsOpen() {
		t.Fatalf("expected slash skill popup to close after selection")
	}
}
