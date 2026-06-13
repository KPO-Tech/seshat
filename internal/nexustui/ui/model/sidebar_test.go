package model

import (
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/session"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

func TestEnsureSidebarTaskSelectionPrefersInProgress(t *testing.T) {
	m := &UI{session: &session.Session{Todos: []session.Todo{
		{ID: "done", Content: "Done", Status: session.TodoStatusCompleted},
		{ID: "active", Content: "Active", Status: session.TodoStatusInProgress},
		{ID: "pending", Content: "Pending", Status: session.TodoStatusPending},
	}}}
	m.ensureSidebarTaskSelection()
	if m.selectedSidebarTaskID != "active" {
		t.Fatalf("expected active task selected, got %q", m.selectedSidebarTaskID)
	}
}

func TestEnsureSidebarTaskSelectionKeepsExistingWhenPresent(t *testing.T) {
	m := &UI{selectedSidebarTaskID: "pending", session: &session.Session{Todos: []session.Todo{
		{ID: "pending", Content: "Pending", Status: session.TodoStatusPending},
		{ID: "done", Content: "Done", Status: session.TodoStatusCompleted},
	}}}
	m.ensureSidebarTaskSelection()
	if m.selectedSidebarTaskID != "pending" {
		t.Fatalf("expected pending task to remain selected, got %q", m.selectedSidebarTaskID)
	}
}

func TestMoveSidebarTaskSelectionFollowsSortedOrder(t *testing.T) {
	m := &UI{selectedSidebarTaskID: "active", session: &session.Session{Todos: []session.Todo{
		{ID: "done", Content: "Done", Status: session.TodoStatusCompleted},
		{ID: "pending", Content: "Pending", Status: session.TodoStatusPending},
		{ID: "active", Content: "Active", Status: session.TodoStatusInProgress},
	}}}
	if !m.moveSidebarTaskSelection(1) {
		t.Fatal("expected selection to move")
	}
	if m.selectedSidebarTaskID != "pending" {
		t.Fatalf("expected pending selected next, got %q", m.selectedSidebarTaskID)
	}
	if !m.moveSidebarTaskSelection(1) {
		t.Fatal("expected selection to move to completed")
	}
	if m.selectedSidebarTaskID != "done" {
		t.Fatalf("expected done selected last, got %q", m.selectedSidebarTaskID)
	}
}

func TestRenderSidebarTaskDescriptionTruncatesWhenCollapsed(t *testing.T) {
	sty := &styles.Styles{}
	sty.Files.TruncationHint = sty.Files.TruncationHint.UnsetWidth()
	rendered := renderSidebarTaskDescription(sty, `1
2
3
4
5
6
7
8`, 40, false)
	if !strings.Contains(rendered, "space to expand") {
		t.Fatalf("expected truncation hint, got %q", rendered)
	}
}
