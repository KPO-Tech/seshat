package workspace

import (
	"sort"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/runtime/state"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/session"
)

// conversationSessions turns the stored session infos into the list the user
// picks a conversation from: the sessions they opened, most recently updated first.
//
// A sub-agent or a background agent runs in a session of its own. Listed as if
// they were conversations, they filled the session list with one entry per agent
// the model launched, and "continue the last session" could land in one of them.
// They are left out here; they belong to the session that launched them.
//
// Times are in milliseconds, the unit every session time of the workspace uses.
func conversationSessions(infos []*state.SessionInfo, now time.Time) []session.Session {
	result := make([]session.Session, 0, len(infos))
	for _, info := range infos {
		if info == nil || info.Subagent {
			continue
		}
		updatedAt := info.UpdatedAt * 1000
		if updatedAt == 0 {
			updatedAt = now.UnixMilli()
		}
		createdAt := info.CreatedAt * 1000
		if createdAt == 0 {
			createdAt = now.UnixMilli()
		}
		result = append(result, session.Session{
			ID:        string(info.ID),
			Title:     info.Title,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result
}

// descendantSessionIDs returns the ids of every session launched, directly or
// through another sub-agent, by the given session.
func descendantSessionIDs(infos []*state.SessionInfo, parent string) []string {
	children := map[string][]string{}
	for _, info := range infos {
		if info != nil && info.ParentID != "" {
			children[string(info.ParentID)] = append(children[string(info.ParentID)], string(info.ID))
		}
	}
	var out []string
	seen := map[string]bool{parent: true}
	queue := []string{parent}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}
