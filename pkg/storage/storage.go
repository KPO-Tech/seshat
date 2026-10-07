package storage

import (
	"context"

	internalstorage "github.com/KPO-Tech/seshat/internal/storage"
)

type (
	// ArtifactMetadata is the persisted control-plane record associated with a blob.
	// It lets the runtime run GC, checks integrity, and prepare future document/RAG
	// features without coupling this package to a specific database.
	ArtifactMetadata = internalstorage.ArtifactMetadata
	// ArtifactNamespace groups stored objects by runtime-level responsibility.
	// These prefixes are intentionally generic so the core can stay reusable while
	// backend layers add higher-level document, RAG, or tenant semantics on top.
	ArtifactNamespace = internalstorage.ArtifactNamespace
	// ArtifactPutRequest describes a runtime-level persisted artifact.
	// The artifact store derives the storage key from this struct so callers do not
	// duplicate key layout logic across browser, fetch, and future RAG layers.
	ArtifactPutRequest = internalstorage.ArtifactPutRequest
	// ArtifactRef is the stable runtime-facing handle returned after persisting a blob.
	// Callers should pass these refs around instead of assuming a local path or S3 URL.
	ArtifactRef = internalstorage.ArtifactRef
	// ArtifactRetentionClass controls lifecycle defaults for persisted artifacts.
	ArtifactRetentionClass = internalstorage.ArtifactRetentionClass
	// ArtifactStore is the narrow abstraction consumed by browser/fetch/document layers.
	// It deliberately hides provider-specific details so the runtime can switch between
	// local and object storage without leaking infra concerns into web packages.
	ArtifactStore = internalstorage.ArtifactStore
	// Config describes where artifacts are stored: the provider (local directory or S3) and its settings (the local path; the endpoint, bucket, credentials, region and key prefix of S3).
	Config = internalstorage.Config
	// GCOptions controls artifact garbage collection.
	GCOptions = internalstorage.GCOptions
	// GCReport summarizes one garbage collection pass.
	GCReport = internalstorage.GCReport
	// ListOptions bounds provider-side listings so callers can enumerate namespaces
	// without loading the full storage tree into memory.
	ListOptions = internalstorage.ListOptions
	// LocalProvider is a StorageProvider that keeps the objects as files under a base directory.
	LocalProvider = internalstorage.LocalProvider
	// ProviderType names a kind of storage backend: local or s3.
	ProviderType = internalstorage.ProviderType
	// StorageProvider is a store of binary objects addressed by key: upload, download, stream, stat, list, delete, produce a URL, and test for existence.
	StorageProvider = internalstorage.StorageProvider
)

const (
	ProviderLocal = internalstorage.ProviderLocal
	ProviderS3    = internalstorage.ProviderS3
)

// SetConfig sets the storage configuration used when the shared provider is first created. Call it before the first use of the storage; it does not replace a provider that already exists.
func SetConfig(cfg Config) {
	internalstorage.SetConfig(cfg)
}

// HealthCheck verifies that the shared storage provider can be created and used: it writes a small test object, checks that it exists, has the right size and can be opened, then deletes it. The error says which step failed.
func HealthCheck(ctx context.Context) error {
	return internalstorage.HealthCheck(ctx)
}

// GetProviderType returns the provider chosen by the SESHAT_STORAGE_PROVIDER environment variable: s3 for "s3", local otherwise.
func GetProviderType() ProviderType {
	return internalstorage.GetProviderType()
}

// DefaultArtifactStore returns the process-wide artifact store backed by the configured provider.
func DefaultArtifactStore() (ArtifactStore, error) {
	return internalstorage.DefaultArtifactStore()
}

// NewArtifactStore adapts an existing provider into the narrower artifact interface.
func NewArtifactStore(provider StorageProvider) ArtifactStore {
	return internalstorage.NewArtifactStore(provider)
}

// NewArtifactStoreFromConfig creates an artifact store from an explicit Config,
// bypassing the process-wide provider singleton. Use when different SDK clients
// need independent storage backends.
func NewArtifactStoreFromConfig(cfg Config) (ArtifactStore, error) {
	return internalstorage.NewArtifactStoreFromConfig(cfg)
}

// NewLocalProviderWithConfig creates a LocalProvider on cfg.LocalPath, or on the default local path when it is empty, creating the directory if needed.
func NewLocalProviderWithConfig(cfg Config) (*LocalProvider, error) {
	return internalstorage.NewLocalProviderWithConfig(cfg)
}

// DetectContentType returns the MIME type for a file name from its extension, or application/octet-stream when the extension is unknown.
func DetectContentType(filename string) string {
	return internalstorage.DetectContentType(filename)
}
