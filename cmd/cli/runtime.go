package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/providers"
	internalrag "github.com/EngineerProjects/nexus-engine/internal/rag"
	"github.com/EngineerProjects/nexus-engine/internal/rag/embedder"
	"github.com/EngineerProjects/nexus-engine/internal/vector"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

type runtimeOptions struct {
	Model                   sdk.ModelIdentifier
	PermissionMode          sdk.PermissionMode
	WorkingDir              string
	SQLitePath              string
	APIKey                  string
	ProviderBaseURL         string
	ProviderRegion          string
	ProviderProjectID       string
	ProviderResource        string
	BrowserRemoteControlURL string
	BrowserExecutablePath   string
	DoclingURL              string
	StorageGCEnabled        bool
	StorageGCInterval       time.Duration
	StorageGCLimit          int
	StorageGCNamespaces     []string
	Debug                   bool

	// RAGService is the embedded HNSW-backed RAG service.
	// Nil when the embedding provider is not configured (RAG_EMBEDDING_URL / RAG_EMBEDDING_MODEL absent).
	RAGService *sdk.RAGService

	// Monitoring is an optional pre-built monitoring system.
	// Set by runInteractive to redirect logs away from stdout/stderr when
	// running in TUI (alt-screen) mode.
	Monitoring *sdk.MonitoringSystem
}

type runtimeOverrides struct {
	Model          string
	PermissionMode string
	WorkingDir     string
	SQLitePath     string
	Debug          *bool
}

func loadRuntimeOptions(overrides runtimeOverrides) (runtimeOptions, error) {
	config, err := engineconfig.Load()
	if err != nil {
		return runtimeOptions{}, err
	}

	// Apply the model override before loading credentials so that
	// loadCredsIntoConfig resolves the API key for the correct provider.
	// Without this, loadCredsIntoConfig sees config.Model="" and falls back
	// to the default provider (anthropic), missing the scoped key for z-ai etc.
	if value := strings.TrimSpace(overrides.Model); value != "" {
		config.Model = value
	}

	// Overlay secrets from the credentials DB so that search keys and provider
	// API keys stored there take effect without being in the YAML file.
	if database, dbErr := openCredentialsDB(config); dbErr == nil {
		loadCredsIntoConfig(database, &config)
		_ = database.Close()
		engineconfig.ApplySearchKeys(config)
	}

	if overrides.Debug != nil {
		config.Debug = *overrides.Debug
	}
	if value := strings.TrimSpace(overrides.WorkingDir); value != "" {
		config.Cwd = value
	}
	if value := strings.TrimSpace(overrides.SQLitePath); value != "" {
		config.DBPath = value
	}

	permissionMode, err := parsePermissionMode(overrides.PermissionMode)
	if err != nil {
		return runtimeOptions{}, err
	}

	workingDir := strings.TrimSpace(config.Cwd)
	if workingDir == "" || workingDir == "." {
		workingDir, err = os.Getwd()
		if err != nil {
			return runtimeOptions{}, fmt.Errorf("resolve working directory: %w", err)
		}
	}
	workingDir, err = filepath.Abs(workingDir)
	if err != nil {
		return runtimeOptions{}, fmt.Errorf("resolve working directory: %w", err)
	}

	model := resolveModel(config)
	apiKey := engineconfig.ResolveAPIKey(config, model.Provider)

	hnswDir := runtimepath.HNSWDataDir(config.RuntimeRoot)

	return runtimeOptions{
		Model:                   model,
		PermissionMode:          permissionMode,
		WorkingDir:              workingDir,
		SQLitePath:              engineconfig.EffectiveSessionDBPath(config),
		APIKey:                  apiKey,
		ProviderBaseURL:         config.ProviderBaseURL,
		ProviderRegion:          config.ProviderRegion,
		ProviderProjectID:       config.ProviderProjectID,
		ProviderResource:        config.ProviderResource,
		BrowserRemoteControlURL: strings.TrimSpace(config.BrowserRemoteControlURL),
		BrowserExecutablePath:   strings.TrimSpace(config.BrowserExecutablePath),
		DoclingURL:              strings.TrimSpace(config.DoclingURL),
		StorageGCEnabled:        config.StorageGCEnabled,
		StorageGCInterval:       parseDurationOrDefault(config.StorageGCInterval, time.Hour),
		StorageGCLimit:          config.StorageGCLimit,
		StorageGCNamespaces:     splitCommaList(config.StorageGCNamespaces),
		Debug:                   config.Debug,
		RAGService:              buildRAGService(hnswDir),
	}, nil
}

func newClient(
	options runtimeOptions,
	promptFn sdk.PromptFn,
	progressFn func(sdk.ToolProgress),
	chunkFn func(sdk.ResponseChunk),
) (*sdk.Client, error) {
	// Load pre_tool_use hooks from config if any are defined.
	var preToolHooks []sdk.PreToolHookConfig
	if rawCfg, err := engineconfig.Load(); err == nil {
		for _, entry := range rawCfg.Hooks["pre_tool_use"] {
			preToolHooks = append(preToolHooks, sdk.PreToolHookConfig{
				Matcher: entry.Matcher,
				Command: entry.Command,
				Timeout: entry.Timeout,
			})
		}
	}

	// Build the provider configuration.
	providerConfig := providers.GetProviderConfig(options.Model.Provider)
	if providerConfig == nil {
		providerConfig = &providers.Config{Provider: options.Model.Provider}
	}
	providerConfig.APIKey = options.APIKey
	if options.ProviderBaseURL != "" {
		providerConfig.BaseURL = options.ProviderBaseURL
	}
	if options.ProviderRegion != "" {
		providerConfig.Region = options.ProviderRegion
	}
	if options.ProviderProjectID != "" {
		providerConfig.ProjectID = options.ProviderProjectID
	}
	if options.ProviderResource != "" && options.Model.Provider == sdk.APIProviderFoundry {
		providerConfig.Region = options.ProviderResource
	}

	// EnableMonitoring must be true so initMonitoringSystem honours
	// options.Monitoring (the TUI file logger) instead of short-circuiting.
	enableMonitoring := options.Monitoring != nil
	client, err := sdk.NewClient(&sdk.ClientConfig{
		APIKey:                  options.APIKey,
		Model:                   options.Model,
		PermissionMode:          options.PermissionMode,
		AutoCompact:             true,
		PersistSessions:         true,
		SessionSQLitePath:       options.SQLitePath,
		PromptFn:                promptFn,
		ProgressFn:              progressFn,
		ResponseChunkFn:         chunkFn,
		WorkingDir:              options.WorkingDir,
		BrowserRemoteControlURL: options.BrowserRemoteControlURL,
		BrowserExecutablePath:   options.BrowserExecutablePath,
		DoclingURL:              options.DoclingURL,
		StorageGCEnabled:        options.StorageGCEnabled,
		StorageGCInterval:       options.StorageGCInterval,
		StorageGCLimit:          options.StorageGCLimit,
		StorageGCNamespaces:     options.StorageGCNamespaces,
		PreToolHooks:            preToolHooks,
		EnableMonitoring:        enableMonitoring,
		Monitoring:              options.Monitoring,
		RAGService:              options.RAGService,
		ProviderConfig:          providerConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("create SDK client: %w", err)
	}
	return client, nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parsePermissionMode(raw string) (sdk.PermissionMode, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return sdk.PermissionModeOnRequest, nil
	}
	if strings.EqualFold(value, "plan") {
		return "", fmt.Errorf("unsupported permission mode %q: plan is now an execution mode, not a permission mode", raw)
	}

	switch {
	case strings.EqualFold(value, string(sdk.PermissionModeOnRequest)):
		return sdk.PermissionModeOnRequest, nil
	case strings.EqualFold(value, string(sdk.PermissionModeAuto)):
		return sdk.PermissionModeAuto, nil
	case strings.EqualFold(value, "acceptEdits") || strings.EqualFold(value, "acceptedits"):
		return sdk.PermissionMode("acceptEdits"), nil
	case strings.EqualFold(value, string(sdk.PermissionModeBypass)):
		return sdk.PermissionModeBypass, nil
	case strings.EqualFold(value, string(sdk.PermissionModeNever)):
		return sdk.PermissionModeNever, nil
	default:
		return "", fmt.Errorf("unsupported permission mode %q", raw)
	}
}

// buildRAGService creates an HNSW-backed RAG service when an embedding provider
// is configured via env vars (RAG_EMBEDDING_URL + RAG_EMBEDDING_MODEL).
// Returns nil when embedding is not configured — rag_ingest / rag_search tools
// will then be unavailable but all other tools continue working normally.
func buildRAGService(hnswDir string) *sdk.RAGService {
	emb := embedder.NewFromEnv()
	if emb == nil {
		return nil
	}
	store, err := vector.NewHNSWStore(hnswDir)
	if err != nil {
		log.Printf("[cli] hnsw vector store unavailable, rag disabled: %v", err)
		return nil
	}
	return internalrag.NewService(nil, store, emb, nil)
}

func resolveModel(config engineconfig.Config) sdk.ModelIdentifier {
	raw := strings.TrimSpace(config.Model)
	model := engineconfig.ParseModelIdentifier(raw)
	if engineconfig.HasExplicitProviderPrefix(raw) {
		return model
	}

	provider := engineconfig.DetectProviderFromModel(raw)
	if provider == "" {
		_, provider = engineconfig.EffectiveAPIKeyAndProvider(config)
	}
	if provider == "" {
		provider = model.Provider
	}
	model.Provider = provider
	return model
}
