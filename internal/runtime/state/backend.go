package state

import (
	"errors"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ErrMalformedTranscriptEntry indicates a persisted transcript entry could not
// be decoded and the session state cannot be restored safely.
var ErrMalformedTranscriptEntry = errors.New("malformed transcript entry")

// Backend persists session artifacts independently from the higher-level store
// logic so Nexus can support filesystem, in-memory, or database-backed
// implementations behind the same restore/save contract.
type Backend interface {
	SaveSession(sessionID types.SessionID, metadata *types.SessionMetadata) error
	LoadSession(sessionID types.SessionID) (*types.SessionMetadata, error)
	DeleteSession(sessionID types.SessionID) error
	ListSessions() ([]types.SessionID, error)
	AppendTranscriptEntries(sessionID types.SessionID, entries []types.TranscriptEntry) error
	ReplaceTranscript(sessionID types.SessionID, entries []types.TranscriptEntry) error
	LoadTranscript(sessionID types.SessionID) ([]types.TranscriptEntry, error)
	SaveCheckpoint(sessionID types.SessionID, checkpoint *Checkpoint) error
	LoadCheckpoint(sessionID types.SessionID) (*Checkpoint, error)
	// SearchTranscriptsByContent returns IDs of sessions whose transcript JSON
	// contains needle (case-insensitive). limit <= 0 means no cap.
	// This replaces the N+1 pattern of ListSessions + LoadTranscript per session.
	SearchTranscriptsByContent(needle string, limit int) ([]types.SessionID, error)
}
