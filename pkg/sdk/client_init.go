package sdk

import (
	"context"
	"log"
	"strings"

	audioproviders "github.com/EngineerProjects/seshat/internal/audio/providers"
	"github.com/EngineerProjects/seshat/internal/audio/stt"
	"github.com/EngineerProjects/seshat/internal/audio/tts"
	"github.com/EngineerProjects/seshat/internal/image"
	imageproviders "github.com/EngineerProjects/seshat/internal/image/providers"
	"github.com/EngineerProjects/seshat/internal/memory"
	"github.com/EngineerProjects/seshat/internal/monitoring"
	"github.com/EngineerProjects/seshat/internal/storage"
	"github.com/EngineerProjects/seshat/internal/tools/builtin"
	"github.com/EngineerProjects/seshat/internal/tools/registry"
	browsercore "github.com/EngineerProjects/seshat/internal/web/browser"
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
		AutomationServiceURL:       config.AutomationServiceURL,
		AutomationAPIKey:           config.AutomationAPIKey,
		WebSearchKeys:              config.WebSearchKeys,
		ImageGenerator:             initImageGenerator(config),
		TTSGenerator:               initTextToSpeechGenerator(config),
		STTTranscriber:             initSpeechToTextTranscriber(config),
	})
}

func initImageGenerator(config *ClientConfig) image.Generation {
	cfg := config.ImageGeneration
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		return nil
	}
	providerID := strings.ToLower(strings.TrimSpace(cfg.Provider))
	apiKey := resolveCapabilityAPIKey(config, providerID, strings.TrimSpace(cfg.APIKey))
	baseURL := resolveCapabilityBaseURL(config, providerID, strings.TrimSpace(cfg.BaseURL))

	switch providerID {
	case "openai":
		if apiKey == "" {
			return nil
		}
		opts := []imageproviders.OpenAIOption{}
		if model := strings.TrimSpace(cfg.Model); model != "" {
			opts = append(opts, imageproviders.WithOpenAIModel(model))
		}
		if baseURL != "" {
			opts = append(opts, imageproviders.WithOpenAIBaseURL(ensureOpenAIBaseURL(baseURL)))
		}
		return imageproviders.NewOpenAI(apiKey, opts...)
	case "gemini":
		if apiKey == "" {
			return nil
		}
		opts := []imageproviders.GeminiOption{}
		if model := strings.TrimSpace(cfg.Model); model != "" {
			opts = append(opts, imageproviders.WithGeminiModel(model))
		}
		if baseURL != "" {
			opts = append(opts, imageproviders.WithGeminiBaseURL(ensureGeminiBaseURL(baseURL)))
		}
		return imageproviders.NewGemini(apiKey, opts...)
	default:
		return nil
	}
}

func initTextToSpeechGenerator(config *ClientConfig) tts.Generation {
	cfg := config.TextToSpeech
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		return nil
	}
	providerID := strings.ToLower(strings.TrimSpace(cfg.Provider))
	apiKey := resolveCapabilityAPIKey(config, providerID, strings.TrimSpace(cfg.APIKey))
	baseURL := resolveCapabilityBaseURL(config, providerID, strings.TrimSpace(cfg.BaseURL))

	switch providerID {
	case "openai":
		if apiKey == "" {
			return nil
		}
		opts := []audioproviders.OpenAITTSOption{}
		if model := strings.TrimSpace(cfg.Model); model != "" {
			opts = append(opts, audioproviders.WithTTSModel(model))
		}
		if voice := strings.TrimSpace(cfg.Voice); voice != "" {
			opts = append(opts, audioproviders.WithTTSVoice(voice))
		}
		if format := strings.TrimSpace(cfg.Format); format != "" {
			opts = append(opts, audioproviders.WithTTSFormat(format))
		}
		if baseURL != "" {
			opts = append(opts, audioproviders.WithTTSBaseURL(ensureOpenAIBaseURL(baseURL)))
		}
		return audioproviders.NewOpenAITTS(apiKey, opts...)
	default:
		return nil
	}
}

func initSpeechToTextTranscriber(config *ClientConfig) stt.SpeechToText {
	cfg := config.SpeechToText
	if cfg == nil || strings.TrimSpace(cfg.Provider) == "" {
		return nil
	}
	providerID := strings.ToLower(strings.TrimSpace(cfg.Provider))
	apiKey := resolveCapabilityAPIKey(config, providerID, strings.TrimSpace(cfg.APIKey))
	baseURL := resolveCapabilityBaseURL(config, providerID, strings.TrimSpace(cfg.BaseURL))

	switch providerID {
	case "openai":
		if apiKey == "" {
			return nil
		}
		opts := []audioproviders.OpenAISTTOption{}
		if model := strings.TrimSpace(cfg.Model); model != "" {
			opts = append(opts, audioproviders.WithSTTModel(model))
		}
		if language := strings.TrimSpace(cfg.Language); language != "" {
			opts = append(opts, audioproviders.WithSTTLanguage(language))
		}
		if baseURL != "" {
			opts = append(opts, audioproviders.WithSTTBaseURL(ensureOpenAIBaseURL(baseURL)))
		}
		return audioproviders.NewOpenAISTT(apiKey, opts...)
	default:
		return nil
	}
}

func resolveCapabilityAPIKey(config *ClientConfig, providerID, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if strings.EqualFold(providerID, string(config.Model.Provider)) {
		if config.ProviderConfig != nil && strings.TrimSpace(config.ProviderConfig.APIKey) != "" {
			return strings.TrimSpace(config.ProviderConfig.APIKey)
		}
		if strings.TrimSpace(config.APIKey) != "" {
			return strings.TrimSpace(config.APIKey)
		}
	}
	if config.CredentialResolver != nil {
		if key, err := config.CredentialResolver.ResolveAPIKey(context.Background(), providerID); err == nil {
			return strings.TrimSpace(key)
		}
	}
	return ""
}

func resolveCapabilityBaseURL(config *ClientConfig, providerID, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if strings.EqualFold(providerID, string(config.Model.Provider)) && config.ProviderConfig != nil {
		return strings.TrimSpace(config.ProviderConfig.BaseURL)
	}
	return ""
}

func ensureOpenAIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}

func ensureGeminiBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	if strings.Contains(baseURL, "/v1beta") {
		return baseURL
	}
	return baseURL + "/v1beta"
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
