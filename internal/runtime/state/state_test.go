package state

import (
	"context"
	"errors"
	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- from sqlite_backend_test.go ---

func TestSQLiteBackendRoundTrip(t *testing.T) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(filepath.Join(t.TempDir(), "state.db")))
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer database.Close()

	backend, err := NewSQLiteBackend(database)
	if err != nil {
		t.Fatalf("NewSQLiteBackend failed: %v", err)
	}

	sessionID := types.SessionID("sqlite-roundtrip")
	metadata := &types.SessionMetadata{
		ID:          sessionID,
		Status:      types.SessionStatusActive,
		CreatedAt:   time.Unix(1700001000, 0).UTC(),
		UpdatedAt:   time.Unix(1700001001, 0).UTC(),
		TotalTurns:  2,
		TotalTokens: 73,
	}
	if err := backend.SaveSession(sessionID, metadata); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	transcript := []types.TranscriptEntry{
		{
			ID:   types.MessageID("msg-1"),
			Type: types.EntryTypeMessage,
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextContent{Text: "hello"},
			},
			Timestamp: time.Unix(1700001002, 0).UTC(),
		},
	}
	if err := backend.ReplaceTranscript(sessionID, transcript); err != nil {
		t.Fatalf("ReplaceTranscript failed: %v", err)
	}

	checkpoint := &Checkpoint{
		SessionID:    sessionID,
		TurnNumber:   2,
		MessagesHash: "hash-1",
		Timestamp:    time.Unix(1700001003, 0).UTC().Unix(),
	}
	if err := backend.SaveCheckpoint(sessionID, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loadedMetadata, err := backend.LoadSession(sessionID)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loadedMetadata.ID != sessionID {
		t.Fatalf("expected session id %q, got %q", sessionID, loadedMetadata.ID)
	}

	loadedTranscript, err := backend.LoadTranscript(sessionID)
	if err != nil {
		t.Fatalf("LoadTranscript failed: %v", err)
	}
	if len(loadedTranscript) != 1 {
		t.Fatalf("expected 1 transcript entry, got %d", len(loadedTranscript))
	}

	loadedCheckpoint, err := backend.LoadCheckpoint(sessionID)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loadedCheckpoint == nil || loadedCheckpoint.MessagesHash != "hash-1" {
		t.Fatalf("expected checkpoint hash %q, got %#v", "hash-1", loadedCheckpoint)
	}

	sessions, err := backend.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 1 || sessions[0] != sessionID {
		t.Fatalf("expected one session %q, got %#v", sessionID, sessions)
	}
}

// --- from store_test.go ---

func TestStoreSaveAndRestoreSessionStateWithMemoryBackend(t *testing.T) {
	store, err := NewStoreWithBackend(NewMemoryBackend())
	if err != nil {
		t.Fatalf("NewStoreWithBackend failed: %v", err)
	}

	sessionID := types.SessionID("session-memory-roundtrip")
	createdAt := time.Unix(1700000000, 0).UTC()
	updatedAt := createdAt.Add(2 * time.Minute)
	metadata := &types.SessionMetadata{
		ID:          sessionID,
		Status:      types.SessionStatusActive,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		RootPath:    "/repo",
		Model:       "openai:gpt-test",
		TotalTurns:  1,
		TotalTokens: 42,
		Additional: map[string]any{
			"permission_context": map[string]any{
				"mode": "default",
			},
		},
	}

	previousMessages := []types.Message{
		types.UserMessage("msg-1", "hello"),
	}
	currentMessages := []types.Message{
		types.UserMessage("msg-1", "hello"),
		types.AssistantMessage("msg-2", []types.ContentBlock{
			types.TextContent{Text: "world"},
		}),
	}

	if err := store.SaveSessionState(sessionID, metadata, nil, previousMessages); err != nil {
		t.Fatalf("initial SaveSessionState failed: %v", err)
	}

	if err := store.SaveSessionState(sessionID, metadata, previousMessages, currentMessages); err != nil {
		t.Fatalf("SaveSessionState failed: %v", err)
	}

	restoredMetadata, restoredMessages, err := store.RestoreSessionState(sessionID)
	if err != nil {
		t.Fatalf("RestoreSessionState failed: %v", err)
	}

	if restoredMetadata.ID != sessionID {
		t.Fatalf("expected restored metadata ID %q, got %q", sessionID, restoredMetadata.ID)
	}
	if restoredMetadata.TotalTurns != 1 {
		t.Fatalf("expected restored total turns 1, got %d", restoredMetadata.TotalTurns)
	}
	summary, ok := restoredMetadata.Additional["canonical_transcript"].(map[string]any)
	if !ok {
		t.Fatalf("expected canonical transcript summary in metadata, got %#v", restoredMetadata.Additional["canonical_transcript"])
	}
	if got := summary["message_count"]; got != float64(2) && got != 2 {
		t.Fatalf("expected message_count 2, got %#v", got)
	}
	if len(restoredMessages) != 2 {
		t.Fatalf("expected 2 restored messages, got %d", len(restoredMessages))
	}
	if restoredMessages[1].Role != types.RoleAssistant {
		t.Fatalf("expected second restored role assistant, got %q", restoredMessages[1].Role)
	}
	content, ok := restoredMessages[1].Content[0].(types.TextContent)
	if !ok {
		t.Fatalf("expected assistant text content, got %#v", restoredMessages[1].Content[0])
	}
	if content.Text != "world" {
		t.Fatalf("expected assistant content %q, got %q", "world", content.Text)
	}
}

func TestRestoreSessionStateFailsOnMalformedTranscriptEntry(t *testing.T) {
	baseDir := t.TempDir()
	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sessionID := types.SessionID("session-corrupt-transcript")
	metadata := &types.SessionMetadata{
		ID:         sessionID,
		Status:     types.SessionStatusActive,
		CreatedAt:  time.Unix(1700000000, 0).UTC(),
		UpdatedAt:  time.Unix(1700000001, 0).UTC(),
		TotalTurns: 1,
	}
	if err := store.SaveSession(sessionID, metadata); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	transcriptPath := filepath.Join(baseDir, sessionID.String(), "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("{not valid json}\n"), 0644); err != nil {
		t.Fatalf("failed to seed malformed transcript: %v", err)
	}

	_, _, err = store.RestoreSessionState(sessionID)
	if err == nil {
		t.Fatal("expected restore to fail on malformed transcript entry")
	}
	if !errors.Is(err, ErrMalformedTranscriptEntry) {
		t.Fatalf("expected ErrMalformedTranscriptEntry, got %v", err)
	}
}
