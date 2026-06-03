package sdk

import (
	"context"
	"log"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/memory"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/tools/builtin"
	"github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func initArtifactStore(config *ClientConfig) ArtifactStore {
	switch {
	case config.ArtifactStore != nil:
		return config.ArtifactStore
	case config.StorageConfig != nil:
		store, err := NewArtifactStoreFromConfig(*config.StorageConfig)
		if err != nil {
			log.Printf("[sdk] storage artifact store unavailable from config, continuing without persisted web artifacts: %v", err)
			return nil
		}
		return store
	default:
		store, err := DefaultArtifactStore()
		if err != nil {
			log.Printf("[sdk] storage artifact store unavailable, continuing without persisted web artifacts: %v", err)
			return nil
		}
		return store
	}
}

func initBrowserManager(config *ClientConfig, artifactStore ArtifactStore) (browsercore.Manager, *storage.Reaper) {
	browserConfig := browsercore.DefaultConfig()
	browserConfig.ArtifactStore = artifactStore
	browserConfig.RemoteControlURL = strings.TrimSpace(config.BrowserRemoteControlURL)
	browserConfig.ExecutablePath = strings.TrimSpace(config.BrowserExecutablePath)
	browserManager := browsercore.NewManager(browserConfig)

	var reaper *storage.Reaper
	if artifactStore != nil && config.StorageGCEnabled {
		reaper = storage.NewReaper(artifactStore, storage.ReaperConfig{
			Interval:   config.StorageGCInterval,
			Namespaces: parseStorageNamespaces(config.StorageGCNamespaces),
			Limit:      config.StorageGCLimit,
		})
		reaper.Start(context.Background())
	}
	return browserManager, reaper
}

func initSessionStore(config *ClientConfig) (SessionStore, bool, error) {
	switch {
	case config.SessionStore != nil:
		return config.SessionStore, false, nil
	case config.SessionBackend != nil:
		store, err := NewSessionStoreWithBackend(config.SessionBackend)
		if err != nil {
			return nil, false, err
		}
		return store, false, nil
	case config.SessionSQLitePath != "":
		backend, err := OpenSQLiteSessionBackend(config.SessionSQLitePath)
		if err != nil {
			return nil, false, err
		}
		store, err := NewSessionStoreWithBackend(backend)
		if err != nil {
			return nil, false, err
		}
		return store, true, nil
	case config.PersistSessions:
		store, err := NewSessionStore(config.SessionStorageDir)
		if err != nil {
			return nil, false, err
		}
		return store, true, nil
	default:
		return nil, false, nil
	}
}

func initMemoryService(config *ClientConfig) (*memory.Service, error) {
	if !config.EnableMemory {
		return nil, nil
	}
	svc, err := memory.NewService()
	if err != nil {
		if config.MemoryFailFast {
			return nil, err
		}
		log.Printf("[sdk] memory service unavailable, continuing without memory: %v", err)
		return nil, err
	}
	return svc, nil
}

func initMonitoringSystem(config *ClientConfig) *monitoring.System {
	if !config.EnableMonitoring {
		return nil
	}
	if config.Monitoring != nil {
		return config.Monitoring
	}
	return monitoring.NewSystem(nil)
}

func initBuiltinRegistry(config *ClientConfig, browserManager browsercore.Manager, artifactStore ArtifactStore) (*registry.Registry, error) {
	return builtin.NewBuiltinRegistryWithConfig(&builtin.Config{
		WorkingDir:                 config.WorkingDir,
		PromptFn:                   config.PromptFn,
		EnablePromptReaderFallback: config.EnablePromptReaderFallback,
		BrowserManager:             browserManager,
		ArtifactStore:              artifactStore,
		RAGService:                 config.RAGService,
		PlanStore:                  config.PlanStore,
		LongTermMemory:             config.LongTermMemory,
		DoclingURL:                 config.DoclingURL,
	})
}

func parseStorageNamespaces(raw []string) []storage.ArtifactNamespace {
	namespaces := make([]storage.ArtifactNamespace, 0, len(raw))
	for _, item := range raw {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		namespaces = append(namespaces, storage.ArtifactNamespace(value))
	}
	return namespaces
}
