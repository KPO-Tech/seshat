package browser

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
)

func (s *sessionState) upsertDownloadLocked(entry DownloadEntry) {
	if s == nil || entry.GUID == "" {
		return
	}
	now := time.Now().UTC()
	entry.UpdatedAt = now
	if entry.StartedAt.IsZero() {
		entry.StartedAt = now
	}
	if index, ok := s.downloadByID[entry.GUID]; ok && index >= 0 && index < len(s.downloads) {
		current := s.downloads[index]
		if entry.PageID == "" {
			entry.PageID = current.PageID
		}
		if entry.URL == "" {
			entry.URL = current.URL
		}
		if entry.SuggestedFilename == "" {
			entry.SuggestedFilename = current.SuggestedFilename
		}
		if entry.StartedAt.IsZero() {
			entry.StartedAt = current.StartedAt
		}
		s.downloads[index] = entry
		return
	}
	s.downloads = append(s.downloads, entry)
	s.downloadByID[entry.GUID] = len(s.downloads) - 1
	maxEntries := s.maxDownloads
	if maxEntries <= 0 {
		maxEntries = 64
	}
	if len(s.downloads) > maxEntries {
		s.downloads = append([]DownloadEntry(nil), s.downloads[len(s.downloads)-maxEntries:]...)
		s.reindexDownloadsLocked()
	}
}

func (s *sessionState) reindexDownloadsLocked() {
	s.downloadByID = make(map[string]int, len(s.downloads))
	for i, entry := range s.downloads {
		s.downloadByID[entry.GUID] = i
	}
}

func (s *sessionState) downloadEntries(pageID string, limit int) []DownloadEntry {
	if s == nil || len(s.downloads) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 25
	}
	filtered := make([]DownloadEntry, 0, min(limit, len(s.downloads)))
	for i := len(s.downloads) - 1; i >= 0; i-- {
		entry := s.downloads[i]
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

func (m *RodManager) persistDownload(ctx context.Context, session *sessionState, entry DownloadEntry) DownloadEntry {
	if session == nil || m == nil || m.config == nil || m.config.ArtifactStore == nil {
		return entry
	}
	filename := entry.SuggestedFilename
	if filename == "" {
		filename = entry.GUID
	}
	path := filepath.Join(session.downloadDir, entry.GUID)
	body, err := os.ReadFile(path)
	if err != nil {
		entry.ErrorText = err.Error()
		return entry
	}
	key := storage.DownloadKey(string(session.id), entry.PageID, filename, time.Now().UTC())
	ref, err := m.config.ArtifactStore.Put(ctx, key, body, storage.DetectContentType(filename))
	if err != nil {
		entry.ErrorText = err.Error()
		return entry
	}
	entry.PersistedPath = ref.URL
	entry.PersistedSize = len(body)
	return entry
}
