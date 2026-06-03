package browser

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// SearchSnapshots searches previously captured snapshot content across pages in one session.
// This is a lightweight V3 primitive for agentic cross-tab recall without adding a vector store.
func (m *RodManager) SearchSnapshots(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]SnapshotSearchHit, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	session.lastActivity = time.Now().UTC()

	hits := make([]SnapshotSearchHit, 0, len(session.pages))
	for _, pageID := range session.pageOrder {
		state := session.pages[pageID]
		if state == nil {
			continue
		}
		hit, ok := snapshotSearchHit(state, query)
		if ok {
			hits = append(hits, hit)
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].LastSnapshottedAt.After(hits[j].LastSnapshottedAt)
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func snapshotSearchHit(state *pageState, query string) (SnapshotSearchHit, bool) {
	if state == nil || strings.TrimSpace(state.lastSnapshotText) == "" {
		return SnapshotSearchHit{}, false
	}
	score := 0
	score += weightedContains(state.info.Title, query, 5)
	score += weightedContains(state.info.URL, query, 3)
	score += weightedContains(state.lastSnapshotText, query, 1)
	for _, heading := range state.lastSnapshotHeadings {
		score += weightedContains(heading.Text, query, 3)
	}
	if score == 0 {
		return SnapshotSearchHit{}, false
	}
	return SnapshotSearchHit{
		PageID:            state.info.ID,
		URL:               state.info.URL,
		Title:             state.info.Title,
		Revision:          state.domRevision,
		Score:             score,
		Snippet:           snippetAround(state.lastSnapshotText, query, 220),
		LastSnapshottedAt: state.lastSnapshotAt,
	}, true
}

func weightedContains(haystack string, needle string, weight int) int {
	haystack = strings.ToLower(strings.TrimSpace(haystack))
	needle = strings.ToLower(strings.TrimSpace(needle))
	if haystack == "" || needle == "" || weight <= 0 {
		return 0
	}
	return strings.Count(haystack, needle) * weight
}

func snippetAround(text string, query string, maxLen int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxLen <= 0 {
		return ""
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	if lowerQuery == "" || len(text) <= maxLen {
		if len(text) <= maxLen {
			return text
		}
		return strings.TrimSpace(text[:maxLen]) + "..."
	}
	index := strings.Index(lowerText, lowerQuery)
	if index < 0 {
		if len(text) <= maxLen {
			return text
		}
		return strings.TrimSpace(text[:maxLen]) + "..."
	}
	start := index - maxLen/3
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet += "..."
	}
	return snippet
}
