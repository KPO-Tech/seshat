package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

type StorageProvider interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) error
	Download(ctx context.Context, key string) ([]byte, error)
	OpenReader(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	List(ctx context.Context, options ListOptions) ([]ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	GetURL(ctx context.Context, key string) (string, error)
	Exists(ctx context.Context, key string) (bool, error)
}

type ProviderType string

const (
	ProviderLocal ProviderType = "local"
	ProviderS3    ProviderType = "s3"
)

const DefaultS3Region = "us-east-1"

func DefaultLocalPath() string {
	return runtimepath.StorageDir("")
}

type Config struct {
	Provider          ProviderType
	LocalPath         string
	S3Endpoint        string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string
	S3KeyPrefix       string
}

func GetConfigFromEnv() Config {
	return Config{
		Provider:          GetProviderType(),
		LocalPath:         localPathFromEnv(),
		S3Endpoint:        os.Getenv("NEXUS_S3_ENDPOINT"),
		S3Bucket:          os.Getenv("NEXUS_S3_BUCKET"),
		S3AccessKeyID:     os.Getenv("NEXUS_S3_ACCESS_KEY_ID"),
		S3SecretAccessKey: os.Getenv("NEXUS_S3_SECRET_ACCESS_KEY"),
		S3Region:          os.Getenv("NEXUS_S3_REGION"),
		S3KeyPrefix:       os.Getenv("NEXUS_S3_KEY_PREFIX"),
	}
}

func localPathFromEnv() string {
	if value := os.Getenv("NEXUS_STORAGE_LOCAL_PATH"); value != "" {
		return value
	}
	return DefaultLocalPath()
}

func GetProviderType() ProviderType {
	provider := os.Getenv("NEXUS_STORAGE_PROVIDER")
	switch provider {
	case "s3":
		return ProviderS3
	default:
		return ProviderLocal
	}
}

var (
	providerInstance StorageProvider
	providerOnce     sync.Once
	providerErr      error
	userConfig       *Config
)

func SetConfig(cfg Config) {
	userConfig = &cfg
}

func GetProvider() (StorageProvider, error) {
	providerOnce.Do(func() {
		providerInstance, providerErr = newProvider()
	})
	return providerInstance, providerErr
}

func newProvider() (StorageProvider, error) {
	var cfg Config
	if userConfig != nil {
		cfg = *userConfig
	} else {
		cfg = GetConfigFromEnv()
	}
	return newProviderFromCfg(cfg)
}

func newProviderFromCfg(cfg Config) (StorageProvider, error) {
	switch cfg.Provider {
	case ProviderS3:
		return NewS3ProviderWithConfig(cfg)
	default:
		return NewLocalProviderWithConfig(cfg)
	}
}

// NewProviderFromConfig creates a StorageProvider from an explicit Config without
// touching the process-wide singleton. Use this when you need per-instance storage.
func NewProviderFromConfig(cfg Config) (StorageProvider, error) {
	return newProviderFromCfg(cfg)
}

func ResetProvider() {
	providerOnce = sync.Once{}
	providerInstance = nil
	providerErr = nil
}

func HealthCheck(ctx context.Context) error {
	provider, err := GetProvider()
	if err != nil {
		return fmt.Errorf("storage provider initialization failed: %w", err)
	}

	testKey := fmt.Sprintf("healthcheck/%d/test", os.Getpid())
	testData := []byte("healthcheck")

	if err := provider.Upload(ctx, testKey, testData, "text/plain"); err != nil {
		return fmt.Errorf("storage write test failed: %w", err)
	}

	exists, err := provider.Exists(ctx, testKey)
	if err != nil || !exists {
		return fmt.Errorf("storage exists check failed: %w", err)
	}
	info, err := provider.Stat(ctx, testKey)
	if err != nil {
		return fmt.Errorf("storage stat test failed: %w", err)
	}
	if info.Size != int64(len(testData)) {
		return fmt.Errorf("storage stat size mismatch: got %d want %d", info.Size, len(testData))
	}
	reader, _, err := provider.OpenReader(ctx, testKey)
	if err != nil {
		return fmt.Errorf("storage open reader test failed: %w", err)
	}
	_ = reader.Close()

	if err := provider.Delete(ctx, testKey); err != nil {
		return fmt.Errorf("storage delete test failed: %w", err)
	}

	return nil
}
