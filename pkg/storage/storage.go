package storage

import (
	"context"

	internalstorage "github.com/EngineerProjects/nexus-engine/internal/storage"
)

type (
	ArtifactMetadata       = internalstorage.ArtifactMetadata
	ArtifactNamespace      = internalstorage.ArtifactNamespace
	ArtifactPutRequest     = internalstorage.ArtifactPutRequest
	ArtifactRef            = internalstorage.ArtifactRef
	ArtifactRetentionClass = internalstorage.ArtifactRetentionClass
	ArtifactStore          = internalstorage.ArtifactStore
	Config                 = internalstorage.Config
	GCOptions              = internalstorage.GCOptions
	GCReport               = internalstorage.GCReport
	ListOptions            = internalstorage.ListOptions
	LocalProvider          = internalstorage.LocalProvider
	ProviderType           = internalstorage.ProviderType
	StorageProvider        = internalstorage.StorageProvider
)

const (
	ProviderLocal = internalstorage.ProviderLocal
	ProviderS3    = internalstorage.ProviderS3
)

func SetConfig(cfg Config) {
	internalstorage.SetConfig(cfg)
}

func HealthCheck(ctx context.Context) error {
	return internalstorage.HealthCheck(ctx)
}

func GetProviderType() ProviderType {
	return internalstorage.GetProviderType()
}

func DefaultArtifactStore() (ArtifactStore, error) {
	return internalstorage.DefaultArtifactStore()
}

func NewArtifactStore(provider StorageProvider) ArtifactStore {
	return internalstorage.NewArtifactStore(provider)
}

func NewArtifactStoreFromConfig(cfg Config) (ArtifactStore, error) {
	return internalstorage.NewArtifactStoreFromConfig(cfg)
}

func NewLocalProviderWithConfig(cfg Config) (*LocalProvider, error) {
	return internalstorage.NewLocalProviderWithConfig(cfg)
}

func DetectContentType(filename string) string {
	return internalstorage.DetectContentType(filename)
}
