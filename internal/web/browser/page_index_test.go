package browser

import (
	"testing"
	"time"
)

func TestSessionAllocatePageIDIsMonotonic(t *testing.T) {
	session := &sessionState{
		pages: make(map[string]*pageState),
	}
	if got := session.allocatePageID(); got != "page-1" {
		t.Fatalf("unexpected first page id: %s", got)
	}
	_ = session.allocatePageID()
	if got := session.allocatePageID(); got != "page-3" {
		t.Fatalf("unexpected third page id: %s", got)
	}
}

func TestSessionRemovePageKeepsDeterministicActiveFallback(t *testing.T) {
	now := time.Now().UTC()
	session := &sessionState{
		pages: make(map[string]*pageState),
	}
	session.addPage(&pageState{info: PageInfo{ID: "page-1", CreatedAt: now, UpdatedAt: now}})
	session.addPage(&pageState{info: PageInfo{ID: "page-2", CreatedAt: now, UpdatedAt: now}})
	session.addPage(&pageState{info: PageInfo{ID: "page-3", CreatedAt: now, UpdatedAt: now}})
	session.setActivePage("page-2")

	session.removePage("page-2")

	if session.activePageID != "page-1" {
		t.Fatalf("expected first remaining page to become active, got %s", session.activePageID)
	}
	pages := session.orderedPageInfos()
	if len(pages) != 2 || pages[0].ID != "page-1" || pages[1].ID != "page-3" {
		t.Fatalf("unexpected page order after removal: %+v", pages)
	}
}
