package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// MemoryBackend is a simple in-memory backend useful for tests and as a
// reference implementation for future database adapters.
type MemoryBackend struct {
	mu          sync.RWMutex
	metadata    map[types.SessionID]*types.SessionMetadata
	transcripts map[types.SessionID][]types.TranscriptEntry
	checkpoints map[types.SessionID]*Checkpoint
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		metadata:    make(map[types.SessionID]*types.SessionMetadata),
		transcripts: make(map[types.SessionID][]types.TranscriptEntry),
		checkpoints: make(map[types.SessionID]*Checkpoint),
	}
}

func (b *MemoryBackend) SaveSession(sessionID types.SessionID, metadata *types.SessionMetadata) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cloned, err := cloneViaJSON(metadata)
	if err != nil {
		return fmt.Errorf("failed to clone metadata: %w", err)
	}
	b.metadata[sessionID] = cloned
	return nil
}

func (b *MemoryBackend) LoadSession(sessionID types.SessionID) (*types.SessionMetadata, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	metadata, ok := b.metadata[sessionID]
	if !ok {
		return nil, fmt.Errorf("failed to read metadata: session %s not found", sessionID)
	}

	cloned, err := cloneViaJSON(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to clone metadata: %w", err)
	}
	return cloned, nil
}

func (b *MemoryBackend) DeleteSession(sessionID types.SessionID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.metadata, sessionID)
	delete(b.transcripts, sessionID)
	delete(b.checkpoints, sessionID)
	return nil
}

func (b *MemoryBackend) ListSessions() ([]types.SessionID, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sessions := make([]types.SessionID, 0, len(b.metadata))
	for sessionID := range b.metadata {
		sessions = append(sessions, sessionID)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i] < sessions[j]
	})
	return sessions, nil
}

func (b *MemoryBackend) AppendTranscriptEntries(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cloned, err := cloneViaJSON(entries)
	if err != nil {
		return fmt.Errorf("failed to clone transcript entries: %w", err)
	}
	b.transcripts[sessionID] = append(b.transcripts[sessionID], cloned...)
	return nil
}

func (b *MemoryBackend) ReplaceTranscript(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cloned, err := cloneViaJSON(entries)
	if err != nil {
		return fmt.Errorf("failed to clone transcript entries: %w", err)
	}
	b.transcripts[sessionID] = cloned
	return nil
}

// SearchTranscriptsByContent scans in-memory transcripts for needle.
func (b *MemoryBackend) SearchTranscriptsByContent(needle string, limit int) ([]types.SessionID, error) {
	lowerNeedle := strings.ToLower(needle)
	b.mu.RLock()
	defer b.mu.RUnlock()
	var ids []types.SessionID
	for sessionID, entries := range b.transcripts {
		for _, entry := range entries {
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(string(data)), lowerNeedle) {
				ids = append(ids, sessionID)
				break
			}
		}
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	return ids, nil
}

func (b *MemoryBackend) LoadTranscript(sessionID types.SessionID) ([]types.TranscriptEntry, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries := b.transcripts[sessionID]
	cloned, err := cloneViaJSON(entries)
	if err != nil {
		return nil, fmt.Errorf("failed to clone transcript entries: %w", err)
	}
	return cloned, nil
}

func (b *MemoryBackend) SaveCheckpoint(sessionID types.SessionID, checkpoint *Checkpoint) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cloned, err := cloneViaJSON(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to clone checkpoint: %w", err)
	}
	b.checkpoints[sessionID] = cloned
	return nil
}

func (b *MemoryBackend) LoadCheckpoint(sessionID types.SessionID) (*Checkpoint, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	checkpoint, ok := b.checkpoints[sessionID]
	if !ok {
		return nil, nil
	}

	cloned, err := cloneViaJSON(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to clone checkpoint: %w", err)
	}
	return cloned, nil
}

func cloneViaJSON[T any](value T) (T, error) {
	var zero T

	data, err := json.Marshal(value)
	if err != nil {
		return zero, err
	}
	if string(data) == "null" {
		return zero, nil
	}

	var cloned T
	if err := json.Unmarshal(data, &cloned); err != nil {
		return zero, err
	}
	return cloned, nil
}
