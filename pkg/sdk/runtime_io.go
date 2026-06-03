package sdk

import (
	"context"
	"io"

	"github.com/EngineerProjects/nexus-engine/internal/runtime/state"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
)

// SessionBackend is the SDK-owned low-level persistence backend contract.
// It matches the runtime's session backend responsibilities without forcing
// SDK consumers to depend on internal packages.
type SessionBackend interface {
	SaveSession(sessionID SessionID, metadata *SessionMetadata) error
	LoadSession(sessionID SessionID) (*SessionMetadata, error)
	DeleteSession(sessionID SessionID) error
	ListSessions() ([]SessionID, error)
	AppendTranscriptEntries(sessionID SessionID, entries []TranscriptEntry) error
	ReplaceTranscript(sessionID SessionID, entries []TranscriptEntry) error
	LoadTranscript(sessionID SessionID) ([]TranscriptEntry, error)
	SaveCheckpoint(sessionID SessionID, checkpoint *Checkpoint) error
	LoadCheckpoint(sessionID SessionID) (*Checkpoint, error)
	SearchTranscriptsByContent(needle string, limit int) ([]SessionID, error)
}

// SessionStore is the SDK-owned contract for persisted runtime sessions.
// It keeps callers on pkg/sdk while remaining compatible with the runtime's
// internal store implementations.
type SessionStore interface {
	SaveSession(sessionID SessionID, metadata *SessionMetadata) error
	LoadSession(sessionID SessionID) (*SessionMetadata, error)
	DeleteSession(sessionID SessionID) error
	ListSessions() ([]SessionID, error)
	AppendTranscriptEntries(sessionID SessionID, entries []TranscriptEntry) error
	ReplaceTranscript(sessionID SessionID, entries []TranscriptEntry) error
	LoadTranscript(sessionID SessionID) ([]TranscriptEntry, error)
	SaveCheckpoint(sessionID SessionID, checkpoint *Checkpoint) error
	LoadCheckpoint(sessionID SessionID) (*Checkpoint, error)
	LoadCanonicalMessages(sessionID SessionID) ([]Message, error)
	SaveCanonicalMessages(sessionID SessionID, messages []Message) error
	RestoreSessionState(sessionID SessionID) (*SessionMetadata, []Message, error)
	SaveSessionState(sessionID SessionID, metadata *SessionMetadata, previousMessages []Message, currentMessages []Message) error
	GetSessionInfo(sessionID SessionID) (*SessionInfo, error)
	GetAllSessionsInfo() ([]*SessionInfo, error)
	Close() error
}

// StorageProviderType selects the underlying artifact storage backend.
type StorageProviderType string

const (
	StorageProviderLocal StorageProviderType = "local"
	StorageProviderS3    StorageProviderType = "s3"
)

// StorageConfig is the SDK-owned artifact storage configuration.
type StorageConfig struct {
	Provider          StorageProviderType
	LocalPath         string
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3KeyPrefix       string
}

// ArtifactStore is the SDK-owned artifact storage contract.
type ArtifactStore interface {
	Put(ctx context.Context, key string, body []byte, contentType string) (ArtifactRef, error)
	PutArtifact(ctx context.Context, request ArtifactPutRequest, body []byte) (ArtifactRef, error)
	Get(ctx context.Context, key string) ([]byte, error)
	OpenReader(ctx context.Context, key string) (io.ReadCloser, ArtifactRef, error)
	Stat(ctx context.Context, key string) (ArtifactRef, error)
	List(ctx context.Context, options ArtifactListOptions) ([]ArtifactRef, error)
	Metadata(ctx context.Context, key string) (ArtifactMetadata, error)
	ListMetadata(ctx context.Context, options ArtifactListOptions) ([]ArtifactMetadata, error)
	GarbageCollect(ctx context.Context, options ArtifactGCOptions) (ArtifactGCReport, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	URL(ctx context.Context, key string) (string, error)
}

func (cfg StorageConfig) toInternal() storage.Config {
	return storage.Config{
		Provider:          storage.ProviderType(cfg.Provider),
		LocalPath:         cfg.LocalPath,
		S3Endpoint:        cfg.S3Endpoint,
		S3Bucket:          cfg.S3Bucket,
		S3AccessKeyID:     cfg.S3AccessKeyID,
		S3SecretAccessKey: cfg.S3SecretAccessKey,
		S3Region:          cfg.S3Region,
		S3KeyPrefix:       cfg.S3KeyPrefix,
	}
}

var _ SessionBackend = (*state.MemoryBackend)(nil)
var _ SessionStore = (*state.Store)(nil)
var _ ArtifactStore = storage.ArtifactStore(nil)

// NewMemorySessionBackend returns an in-memory persistence backend suitable for
// tests or fully in-process runtimes.
func NewMemorySessionBackend() SessionBackend {
	return state.NewMemoryBackend()
}

// NewFilesystemSessionBackend returns a filesystem-backed session backend rooted
// at baseDir.
func NewFilesystemSessionBackend(baseDir string) (SessionBackend, error) {
	return state.NewFilesystemBackend(baseDir)
}

// OpenSQLiteSessionBackend opens a SQLite-backed session backend using the
// shared runtime DB initialization path.
func OpenSQLiteSessionBackend(path string) (SessionBackend, error) {
	return state.OpenSQLiteBackend(path)
}

// NewSessionStore creates the default filesystem-backed session store rooted at
// baseDir.
func NewSessionStore(baseDir string) (SessionStore, error) {
	return state.NewStore(baseDir)
}

// NewSessionStoreWithBackend adapts a caller-provided backend into the SDK's
// public SessionStore contract.
func NewSessionStoreWithBackend(backend SessionBackend) (SessionStore, error) {
	return state.NewStoreWithBackend(backend)
}

// DefaultArtifactStore returns the process-wide artifact store configured for
// the runtime.
func DefaultArtifactStore() (ArtifactStore, error) {
	return storage.DefaultArtifactStore()
}

// NewArtifactStoreFromConfig constructs an artifact store from an explicit SDK
// storage config without touching the process-wide singleton.
func NewArtifactStoreFromConfig(cfg StorageConfig) (ArtifactStore, error) {
	return storage.NewArtifactStoreFromConfig(cfg.toInternal())
}
