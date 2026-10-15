package state

import (
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// A sub-agent runs in a session of its own. The session list has to tell it from a
// conversation, and to know which conversation launched it.
func TestSessionInfoTellsASubagentSessionFromAConversation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0).UTC()

	conversation := &types.SessionMetadata{ID: "conv", Status: types.SessionStatusActive, CreatedAt: now, UpdatedAt: now}
	child := types.NewSubagentSessionMetadata("subagent", "conv", now)
	child.ID = "child"
	legacy := types.NewSubagentSessionMetadata("subagent", "", now) // saved before the parent was recorded
	legacy.ID = "legacy"
	for _, m := range []*types.SessionMetadata{conversation, child, legacy} {
		if err := store.SaveSession(m.ID, m); err != nil {
			t.Fatal(err)
		}
	}

	info := func(id types.SessionID) *SessionInfo {
		t.Helper()
		got, err := store.GetSessionInfo(id)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := info("conv"); got.Subagent || got.ParentID != "" {
		t.Errorf("a conversation must not look like a sub-agent session: %+v", got)
	}
	if got := info("child"); !got.Subagent || got.ParentID != "conv" {
		t.Errorf("a sub-agent session must keep its parent: %+v", got)
	}
	if got := info("legacy"); !got.Subagent || got.ParentID != "" {
		t.Errorf("a sub-agent session saved without a parent is still a sub-agent session: %+v", got)
	}
}
