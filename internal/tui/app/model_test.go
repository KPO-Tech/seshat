package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
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
	if cw != 80 {
		t.Fatalf("expected chat width 80, got %d", cw)
	}
	if ch != 18 {
		t.Fatalf("expected chat height 18, got %d", ch)
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
	m.input.SetValue("hello")
	m = m.resizeInput()

	view := m.inputView()
	if !strings.Contains(view, "chat") {
		t.Fatalf("expected input view to include composer badge, got %q", view)
	}
	if !strings.Contains(view, "enter send") {
		t.Fatalf("expected input view to include composer hint, got %q", view)
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
