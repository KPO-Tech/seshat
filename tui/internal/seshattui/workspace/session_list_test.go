package workspace

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/runtime/state"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

func sdkID(id string) types.SessionID { return types.SessionID(id) }

func info(id string, updated int64, subagent bool, parent string) *state.SessionInfo {
	return &state.SessionInfo{ID: sdkID(id), Title: id, CreatedAt: updated - 10, UpdatedAt: updated, Subagent: subagent, ParentID: sdkID(parent)}
}

func TestConversationSessionsLeavesOutSubagentsAndSortsNewestFirst(t *testing.T) {
	infos := []*state.SessionInfo{
		info("old", 1000, false, ""),
		info("agent-a", 5000, true, "new"), // the most recent of all, but not a conversation
		info("new", 3000, false, ""),
		nil,
		info("legacy-agent", 4000, true, ""),
	}
	got := conversationSessions(infos, time.Unix(9999, 0))
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if want := []string{"new", "old"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("sessions = %v, want %v (conversations only, newest first, so \"continue\" takes the right one)", ids, want)
	}
	// Times are milliseconds: the session dialog reads them with time.UnixMilli.
	if got[0].UpdatedAt != 3000*1000 || got[0].CreatedAt != 2990*1000 {
		t.Fatalf("times = %d / %d, want milliseconds", got[0].UpdatedAt, got[0].CreatedAt)
	}
}

func TestConversationSessionsFillsAMissingTime(t *testing.T) {
	now := time.Unix(2000, 0)
	got := conversationSessions([]*state.SessionInfo{{ID: "x"}}, now)
	if len(got) != 1 || got[0].UpdatedAt != now.UnixMilli() || got[0].CreatedAt != now.UnixMilli() {
		t.Fatalf("got %+v", got)
	}
}

func TestDescendantSessionIDsFollowsSubagentsOfSubagents(t *testing.T) {
	infos := []*state.SessionInfo{
		info("conv", 1, false, ""),
		info("a", 2, true, "conv"),
		info("b", 3, true, "conv"),
		info("a1", 4, true, "a"), // launched by a sub-agent
		info("other", 5, false, ""),
		info("other-child", 6, true, "other"),
		info("loop1", 7, true, "loop2"), // a corrupt cycle must not hang
		info("loop2", 8, true, "loop1"),
	}
	got := descendantSessionIDs(infos, "conv")
	sort.Strings(got)
	if want := []string{"a", "a1", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("descendants = %v, want %v", got, want)
	}
	if got := descendantSessionIDs(infos, "loop1"); len(got) != 1 || got[0] != "loop2" {
		t.Fatalf("cycle: %v", got)
	}
	if got := descendantSessionIDs(infos, "nobody"); len(got) != 0 {
		t.Fatalf("no children: %v", got)
	}
}
