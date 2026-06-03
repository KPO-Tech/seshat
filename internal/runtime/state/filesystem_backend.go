package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// FilesystemBackend stores each session as a directory with metadata,
// transcript, and checkpoint files.
type FilesystemBackend struct {
	baseDir string
}

// NewFilesystemBackend creates the default filesystem-backed session backend.
func NewFilesystemBackend(baseDir string) (*FilesystemBackend, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}
	return &FilesystemBackend{baseDir: baseDir}, nil
}

func (b *FilesystemBackend) SaveSession(sessionID types.SessionID, metadata *types.SessionMetadata) error {
	if err := b.ensureSessionDir(sessionID); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(b.metadataPath(sessionID), data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func (b *FilesystemBackend) LoadSession(sessionID types.SessionID) (*types.SessionMetadata, error) {
	data, err := os.ReadFile(b.metadataPath(sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata types.SessionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

func (b *FilesystemBackend) DeleteSession(sessionID types.SessionID) error {
	if err := os.RemoveAll(b.sessionDir(sessionID)); err != nil {
		return fmt.Errorf("failed to remove session directory: %w", err)
	}
	return nil
}

func (b *FilesystemBackend) ListSessions() ([]types.SessionID, error) {
	entries, err := os.ReadDir(b.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	sessions := make([]types.SessionID, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			sessions = append(sessions, types.SessionID(entry.Name()))
		}
	}

	return sessions, nil
}

func (b *FilesystemBackend) AppendTranscriptEntries(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := b.ensureSessionDir(sessionID); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}
	return b.appendTranscriptEntries(b.transcriptPath(sessionID), entries)
}

func (b *FilesystemBackend) ReplaceTranscript(sessionID types.SessionID, entries []types.TranscriptEntry) error {
	if err := b.ensureSessionDir(sessionID); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}
	return b.writeTranscriptEntries(b.transcriptPath(sessionID), entries)
}

// SearchTranscriptsByContent scans transcript files for needle (grep-style).
// This is O(sessions × file_size) but avoids full JSON deserialization.
func (b *FilesystemBackend) SearchTranscriptsByContent(needle string, limit int) ([]types.SessionID, error) {
	lowerNeedle := strings.ToLower(needle)
	sessionIDs, err := b.ListSessions()
	if err != nil {
		return nil, err
	}
	var matches []types.SessionID
	for _, sid := range sessionIDs {
		if limit > 0 && len(matches) >= limit {
			break
		}
		file, err := os.Open(b.transcriptPath(sid))
		if err != nil {
			continue
		}
		found := false
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if strings.Contains(strings.ToLower(scanner.Text()), lowerNeedle) {
				found = true
				break
			}
		}
		file.Close()
		if found {
			matches = append(matches, sid)
		}
	}
	return matches, nil
}

func (b *FilesystemBackend) LoadTranscript(sessionID types.SessionID) ([]types.TranscriptEntry, error) {
	file, err := os.Open(b.transcriptPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return []types.TranscriptEntry{}, nil
		}
		return nil, fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer file.Close()

	entries := make([]types.TranscriptEntry, 0)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry types.TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("%w for session %s at line %d: %v", ErrMalformedTranscriptEntry, sessionID, lineNumber, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan transcript file: %w", err)
	}

	return entries, nil
}

func (b *FilesystemBackend) SaveCheckpoint(sessionID types.SessionID, checkpoint *Checkpoint) error {
	if err := b.ensureSessionDir(sessionID); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(b.checkpointPath(sessionID), data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	return nil
}

func (b *FilesystemBackend) LoadCheckpoint(sessionID types.SessionID) (*Checkpoint, error) {
	data, err := os.ReadFile(b.checkpointPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

func (b *FilesystemBackend) transcriptPath(sessionID types.SessionID) string {
	return transcriptPathForSession(b.baseDir, sessionID)
}

func (b *FilesystemBackend) checkpointPath(sessionID types.SessionID) string {
	return checkpointPathForSession(b.baseDir, sessionID)
}

func (b *FilesystemBackend) metadataPath(sessionID types.SessionID) string {
	return metadataPathForSession(b.baseDir, sessionID)
}

func (b *FilesystemBackend) sessionDir(sessionID types.SessionID) string {
	return sessionDirFor(b.baseDir, sessionID)
}

func (b *FilesystemBackend) ensureSessionDir(sessionID types.SessionID) error {
	return os.MkdirAll(b.sessionDir(sessionID), 0755)
}

func (b *FilesystemBackend) writeTranscriptEntries(filePath string, entries []types.TranscriptEntry) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal transcript entry: %w", err)
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write transcript entry: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush transcript entries: %w", err)
	}
	return nil
}

func (b *FilesystemBackend) appendTranscriptEntries(filePath string, entries []types.TranscriptEntry) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal transcript entry: %w", err)
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write transcript entry: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush transcript entries: %w", err)
	}
	return nil
}

func transcriptPathForSession(baseDir string, sessionID types.SessionID) string {
	return filepath.Join(baseDir, string(sessionID), "transcript.jsonl")
}

func checkpointPathForSession(baseDir string, sessionID types.SessionID) string {
	return filepath.Join(baseDir, string(sessionID), "checkpoint.json")
}

func metadataPathForSession(baseDir string, sessionID types.SessionID) string {
	return filepath.Join(baseDir, string(sessionID), "metadata.json")
}

func sessionDirFor(baseDir string, sessionID types.SessionID) string {
	return filepath.Join(baseDir, string(sessionID))
}
