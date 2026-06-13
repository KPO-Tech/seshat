package model

import (
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/session"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

func TestTodoPillUsesTasksLabel(t *testing.T) {
	sty := &styles.Styles{}
	rendered := todoPill([]session.Todo{{Content: "Implement sidebar", Status: session.TodoStatusInProgress}}, "…", false, true, sty)
	if !strings.Contains(rendered, "Tasks") {
		t.Fatalf("expected Tasks label, got %q", rendered)
	}
	if strings.Contains(rendered, "To-Do") {
		t.Fatalf("expected To-Do label to be gone, got %q", rendered)
	}
}

func TestCollapseAutoExpandedPillsIfDone(t *testing.T) {
	m := &UI{
		pillsExpanded:     true,
		pillsAutoExpanded: true,
		session:           &session.Session{Todos: []session.Todo{{Content: "Done", Status: session.TodoStatusCompleted}}},
	}
	if !m.collapseAutoExpandedPillsIfDone() {
		t.Fatal("expected auto-expanded pills to collapse when all tasks are done")
	}
	if m.pillsExpanded || m.pillsAutoExpanded {
		t.Fatalf("expected pills to be collapsed, got expanded=%v auto=%v", m.pillsExpanded, m.pillsAutoExpanded)
	}
}
