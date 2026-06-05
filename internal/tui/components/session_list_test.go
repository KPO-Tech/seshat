package components

import (
	"testing"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

func TestSessionListFilterAndDeleteSelected(t *testing.T) {
	now := time.Now()
	s := NewSessionList(common.DefaultStyles())
	s.SetSessions([]tui.SessionInfo{
		{ID: "sess-1", ShortID: "alpha123", UpdatedAt: now, Turns: 1},
		{ID: "sess-2", ShortID: "beta456", UpdatedAt: now, Turns: 2},
		{ID: "sess-3", ShortID: "beta789", UpdatedAt: now, Turns: 3},
	})

	s.TypeFilter("b")
	s.TypeFilter("e")
	s.TypeFilter("t")
	s.TypeFilter("a")

	if got := len(s.list.FilteredItems()); got != 2 {
		t.Fatalf("expected 2 filtered sessions, got %d", got)
	}
	if got := s.Selected(); got != "sess-2" {
		t.Fatalf("expected first filtered session to be selected, got %q", got)
	}

	s.Down()
	if got := s.Selected(); got != "sess-3" {
		t.Fatalf("expected second filtered session to be selected after Down, got %q", got)
	}

	deleted := s.DeleteSelected()
	if deleted != "sess-3" {
		t.Fatalf("expected deleted session sess-3, got %q", deleted)
	}

	if got := len(s.sessions); got != 2 {
		t.Fatalf("expected 2 sessions after delete, got %d", got)
	}
	if got := len(s.list.FilteredItems()); got != 1 {
		t.Fatalf("expected 1 filtered session after delete, got %d", got)
	}
	if got := s.Selected(); got != "sess-2" {
		t.Fatalf("expected cursor to fall back to sess-2, got %q", got)
	}
}

func TestSessionListClearFilterRestoresAllSessions(t *testing.T) {
	now := time.Now()
	s := NewSessionList(common.DefaultStyles())
	s.SetSessions([]tui.SessionInfo{
		{ID: "sess-1", ShortID: "alpha123", UpdatedAt: now},
		{ID: "sess-2", ShortID: "beta456", UpdatedAt: now},
	})

	s.TypeFilter("z")
	if got := len(s.list.FilteredItems()); got != 0 {
		t.Fatalf("expected no filtered sessions, got %d", got)
	}

	s.ClearFilter()
	if got := len(s.list.FilteredItems()); got != 2 {
		t.Fatalf("expected all sessions after clearing filter, got %d", got)
	}
}
