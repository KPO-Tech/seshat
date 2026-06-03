package storage

import (
	"context"
	"io"
	"time"
)

// ArtifactNamespace groups stored objects by runtime-level responsibility.
// These prefixes are intentionally generic so the core can stay reusable while
// backend layers add higher-level document, RAG, or tenant semantics on top.
type ArtifactNamespace string

const (
	NamespaceDocuments          ArtifactNamespace = "documents"
	NamespaceWebArtifacts       ArtifactNamespace = "artifacts/web"
	NamespaceBrowserScreenshots ArtifactNamespace = "artifacts/browser/screenshots"
	NamespaceBrowserDownloads   ArtifactNamespace = "artifacts/browser/downloads"
	NamespaceRAGDocuments       ArtifactNamespace = "rag/documents"
)

// ArtifactRetentionClass controls lifecycle defaults for persisted artifacts.
type ArtifactRetentionClass string

const (
	RetentionDurable   ArtifactRetentionClass = "durable"
	RetentionTemporary ArtifactRetentionClass = "temporary"
	RetentionSession   ArtifactRetentionClass = "session"
)

// ListOptions bounds provider-side listings so callers can enumerate namespaces
// without loading the full storage tree into memory.
type ListOptions struct {
	Prefix string
	Limit  int
}

// ObjectInfo is the provider-level description of a stored object.
type ObjectInfo struct {
	Key         string    `json:"key"`
	URL         string    `json:"url,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Size        int64     `json:"size,omitempty"`
	ModifiedAt  time.Time `json:"modified_at,omitempty"`
	ETag        string    `json:"etag,omitempty"`
}

// ArtifactRef is the stable runtime-facing handle returned after persisting a blob.
// Callers should pass these refs around instead of assuming a local path or S3 URL.
type ArtifactRef struct {
	Key            string    `json:"key"`
	URL            string    `json:"url"`
	ContentType    string    `json:"content_type,omitempty"`
	Size           int64     `json:"size,omitempty"`
	ModifiedAt     time.Time `json:"modified_at,omitempty"`
	ChecksumSHA256 string    `json:"checksum_sha256,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	Namespace      string    `json:"namespace,omitempty"`
}

// ArtifactPutRequest describes a runtime-level persisted artifact.
// The artifact store derives the storage key from this struct so callers do not
// duplicate key layout logic across browser, fetch, and future RAG layers.
type ArtifactPutRequest struct {
	Namespace      ArtifactNamespace
	Filename       string
	SessionID      string
	PageID         string
	ContentType    string
	Timestamp      time.Time
	RetentionClass ArtifactRetentionClass
	TTL            time.Duration
}

// ArtifactMetadata is the persisted control-plane record associated with a blob.
// It lets the runtime run GC, checks integrity, and prepare future document/RAG
// features without coupling this package to a specific database.
type ArtifactMetadata struct {
	Key            string                 `json:"key"`
	Namespace      string                 `json:"namespace,omitempty"`
	Filename       string                 `json:"filename,omitempty"`
	SessionID      string                 `json:"session_id,omitempty"`
	PageID         string                 `json:"page_id,omitempty"`
	ContentType    string                 `json:"content_type,omitempty"`
	Size           int64                  `json:"size,omitempty"`
	ChecksumSHA256 string                 `json:"checksum_sha256,omitempty"`
	CreatedAt      time.Time              `json:"created_at,omitempty"`
	ModifiedAt     time.Time              `json:"modified_at,omitempty"`
	ExpiresAt      time.Time              `json:"expires_at,omitempty"`
	RetentionClass ArtifactRetentionClass `json:"retention_class,omitempty"`
}

// GCOptions controls artifact garbage collection.
type GCOptions struct {
	Now        time.Time
	Namespaces []ArtifactNamespace
	Limit      int
	DryRun     bool
}

// GCReport summarizes one garbage collection pass.
type GCReport struct {
	Scanned     int      `json:"scanned"`
	Deleted     int      `json:"deleted"`
	Kept        int      `json:"kept"`
	Errors      []string `json:"errors,omitempty"`
	DeletedKeys []string `json:"deleted_keys,omitempty"`
}

// ReaperConfig controls background garbage collection for expiring artifacts.
type ReaperConfig struct {
	Interval   time.Duration
	Namespaces []ArtifactNamespace
	Limit      int
}

// ArtifactStore is the narrow abstraction consumed by browser/fetch/document layers.
// It deliberately hides provider-specific details so the runtime can switch between
// local and object storage without leaking infra concerns into web packages.
type ArtifactStore interface {
	Put(ctx context.Context, key string, body []byte, contentType string) (ArtifactRef, error)
	PutArtifact(ctx context.Context, request ArtifactPutRequest, body []byte) (ArtifactRef, error)
	Get(ctx context.Context, key string) ([]byte, error)
	OpenReader(ctx context.Context, key string) (io.ReadCloser, ArtifactRef, error)
	Stat(ctx context.Context, key string) (ArtifactRef, error)
	List(ctx context.Context, options ListOptions) ([]ArtifactRef, error)
	Metadata(ctx context.Context, key string) (ArtifactMetadata, error)
	ListMetadata(ctx context.Context, options ListOptions) ([]ArtifactMetadata, error)
	GarbageCollect(ctx context.Context, options GCOptions) (GCReport, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	URL(ctx context.Context, key string) (string, error)
}
