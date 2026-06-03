package browser

import (
	"context"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// V1 keeps an in-memory ring buffer of network activity per browser session.
// This is intentionally lightweight: enough for agent inspection and debugging
// without turning the runtime into a packet capture service.
func (m *RodManager) attachPageNetworkWatcherLocked(session *sessionState, state *pageState) {
	if session == nil || state == nil || state.page == nil || state.watchCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	state.watchCancel = cancel

	go state.page.Context(ctx).EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			session.mu.Lock()
			defer session.mu.Unlock()
			session.appendNetworkEntryLocked(NetworkEntry{
				PageID:       state.info.ID,
				URL:          e.Request.URL,
				Method:       e.Request.Method,
				ResourceType: string(e.Type),
				Stage:        "request",
				Timestamp:    time.Now().UTC(),
			})
		},
		func(e *proto.NetworkResponseReceived) {
			session.mu.Lock()
			defer session.mu.Unlock()
			session.appendNetworkEntryLocked(NetworkEntry{
				PageID:       state.info.ID,
				URL:          e.Response.URL,
				Method:       methodFromHeaders(e.Response.RequestHeaders),
				ResourceType: string(e.Type),
				StatusCode:   int(e.Response.Status),
				MimeType:     e.Response.MIMEType,
				Stage:        "response",
				Timestamp:    time.Now().UTC(),
			})
		},
		func(e *proto.NetworkLoadingFailed) {
			session.mu.Lock()
			defer session.mu.Unlock()
			session.appendNetworkEntryLocked(NetworkEntry{
				PageID:    state.info.ID,
				ErrorText: e.ErrorText,
				Stage:     "failed",
				Timestamp: time.Now().UTC(),
			})
		},
		func(e *proto.PageDownloadWillBegin) {
			session.mu.Lock()
			defer session.mu.Unlock()
			session.upsertDownloadLocked(DownloadEntry{
				GUID:              e.GUID,
				PageID:            state.info.ID,
				URL:               e.URL,
				SuggestedFilename: e.SuggestedFilename,
				State:             "starting",
				StartedAt:         time.Now().UTC(),
				UpdatedAt:         time.Now().UTC(),
			})
		},
		func(e *proto.PageDownloadProgress) {
			session.mu.Lock()
			entry := DownloadEntry{
				GUID:          e.GUID,
				PageID:        state.info.ID,
				State:         string(e.State),
				BytesReceived: int(e.ReceivedBytes),
				TotalBytes:    int(e.TotalBytes),
				UpdatedAt:     time.Now().UTC(),
			}
			session.upsertDownloadLocked(entry)
			if e.State == proto.PageDownloadProgressStateCompleted {
				index := session.downloadByID[e.GUID]
				completed := session.downloads[index]
				completed = m.persistDownload(context.Background(), session, completed)
				session.downloads[index] = completed
			}
			session.mu.Unlock()
		},
	)()
}

func methodFromHeaders(headers proto.NetworkHeaders) string {
	for key, value := range headers {
		if strings.EqualFold(key, ":method") || strings.EqualFold(key, "method") {
			return strings.TrimSpace(value.String())
		}
	}
	return ""
}

func (s *sessionState) appendNetworkEntryLocked(entry NetworkEntry) {
	if s == nil {
		return
	}
	s.nextNetSeq++
	entry.Seq = s.nextNetSeq
	s.networkLog = append(s.networkLog, entry)

	maxEntries := s.maxNetLog
	if maxEntries <= 0 {
		maxEntries = 256
	}
	if len(s.networkLog) > maxEntries {
		s.networkLog = append([]NetworkEntry(nil), s.networkLog[len(s.networkLog)-maxEntries:]...)
	}
}

func (s *sessionState) networkEntries(pageID string, limit int) []NetworkEntry {
	if s == nil || len(s.networkLog) == 0 {
		return nil
	}

	if limit <= 0 {
		limit = 25
	}

	filtered := make([]NetworkEntry, 0, min(limit, len(s.networkLog)))
	for i := len(s.networkLog) - 1; i >= 0; i-- {
		entry := s.networkLog[i]
		if pageID != "" && entry.PageID != pageID {
			continue
		}
		filtered = append(filtered, entry)
		if len(filtered) >= limit {
			break
		}
	}

	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	return filtered
}
