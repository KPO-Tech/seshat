package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	dbpkg "github.com/KPO-Tech/seshat/internal/db"
	longtermStore "github.com/KPO-Tech/seshat/internal/memory/longterm"
	"github.com/KPO-Tech/seshat/internal/providers"
	internalrag "github.com/KPO-Tech/seshat/internal/rag"
	"github.com/KPO-Tech/seshat/internal/rag/embedder"
	"github.com/KPO-Tech/seshat/internal/rag/reranker"
	"github.com/KPO-Tech/seshat/internal/storage"
	"github.com/KPO-Tech/seshat/internal/vector"
	"github.com/KPO-Tech/seshat/pkg/companion"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
	"github.com/KPO-Tech/seshat/pkg/runtimepath"
	"github.com/KPO-Tech/seshat/pkg/sdk"
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

	// MCPServers are the MCP server configs to wire into the SDK client so
	// the agent can call MCP tools. Populated from seshat.json before newClient().
	MCPServers []sdk.MCPServerConfig

	ImageGeneration *sdk.ImageGenerationConfig
	TextToSpeech    *sdk.TextToSpeechConfig
	SpeechToText    *sdk.SpeechToTextConfig
	Companion       *companion.Profile
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
	ragSQLitePath := runtimepath.RAGSQLiteDBPath(config.RuntimeRoot)
	companionProfile, companionErr := companion.Load(config.RuntimeRoot)
	var companionPtr *companion.Profile
	if companionErr == nil && companionProfile.Enabled {
		companionPtr = &companionProfile
	}

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
		RAGService:              buildRAGService(config, hnswDir, ragSQLitePath),
		Companion:               companionPtr,
	}, nil
}

func newClient(
	options runtimeOptions,
	promptFn sdk.PromptFn,
	progressFn func(sdk.ToolProgress),
	chunkFn func(sdk.ResponseChunk),
	runtimeEventFn func(sdk.RuntimeEvent),
	titledFn func(sdk.SessionID, string),
	planStore sdk.PlanStore,
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

	// Initialize the longterm knowledge-graph store backed by the same SQLite DB.
	// Non-fatal: if this fails the memory_* tools remain disabled rather than
	// blocking the whole TUI startup.
	var ltMemory sdk.LongTermMemory
	if options.SQLitePath != "" {
		if ltDB, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(options.SQLitePath)); err == nil {
			ltMemory = longtermStore.NewSQLiteStore(ltDB.SQL())
		} else {
			log.Printf("[runtime] longterm memory store unavailable: %v", err)
		}
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
		RuntimeEventFn:          runtimeEventFn,
		OnSessionTitled:         titledFn,
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
		PlanStore:               planStore,
		LongTermMemory:          ltMemory,
		MCPServers:              options.MCPServers,
		ImageGeneration:         options.ImageGeneration,
		TextToSpeech:            options.TextToSpeech,
		SpeechToText:            options.SpeechToText,
		Companion:               options.Companion,
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

// buildRAGService creates a RAG service. It no longer requires an embedding
// provider to be configured: without one, rag_ingest/rag_search still work
// in vectorless mode (pure BM25/keyword ranking via the vector store's
// full-text index - real FTS5 on SQLite, a coarser keyword-overlap score on
// HNSW). Configuring RAG_EMBEDDING_URL + RAG_EMBEDDING_MODEL upgrades to
// semantic (embedding-based) search automatically, no other change needed.
// Returns nil only when no vector store backend could be constructed at all.
//
// Vector storage prefers the embedded HNSW backend, falling back to the
// SQLite backend at sqliteFallbackPath when HNSW isn't available - notably
// on Windows, where github.com/coder/hnsw's atomic-write dependency
// (google/renameio) doesn't build (see internal/vector/hnsw_store_windows.go).
// Without this fallback, RAG was silently disabled on every Windows install
// regardless of embedding configuration.
func buildRAGService(config engineconfig.Config, hnswDir, sqliteFallbackPath string) *sdk.RAGService {
	emb := embedder.NewFromEnv() // nil is fine - vectorless mode covers it

	store := buildVectorStore(config, hnswDir, sqliteFallbackPath)
	if store == nil {
		return nil
	}

	// Best-effort: RAG still works without artifact storage (chunk text is
	// preserved in the vector store either way), it just loses the ability
	// to retrieve the original un-chunked document later.
	artifacts, _ := storage.DefaultArtifactStore()

	// SemanticChunker groups sentences by embedding similarity instead of
	// blind paragraph splitting - meaningfully better retrieval quality, at
	// the cost of one extra embedding call per sentence during ingest. Needs
	// a real embedder to work at all; without one NewService defaults to the
	// plain ParagraphChunker (nil chunker).
	//
	// embForService must be a genuinely nil Embedder INTERFACE when emb is
	// nil, not just a nil *embedder.Embedder boxed into one - Service's own
	// `s.embedder != nil` checks compare the interface, and a nil pointer
	// wrapped in a non-nil interface value would make that check true, then
	// panic dereferencing the nil receiver on the first EmbedTexts call.
	var chunker internalrag.Chunker
	var embForService internalrag.Embedder
	if emb != nil {
		chunker = internalrag.NewSemanticChunker(emb, 0)
		embForService = emb
	}

	svc := internalrag.NewService(artifacts, store, embForService, chunker)

	// reranker.NewFromEnv prefers a self-hosted RAG_RERANK_URL (e.g. a local
	// TEI/vLLM instance serving BAAI/bge-reranker-v2-m3 - free, no API key,
	// no external network call) and falls back to LangSearch's hosted API
	// when only LANGSEARCH_API_KEY is set. No-ops (IsConfigured() == false)
	// when neither is configured, so it's always safe to attach. It scores
	// raw text against the query text, not embeddings, so it works in
	// vectorless mode too.
	svc.SetReranker(reranker.NewFromEnv())

	return svc
}

func buildVectorStore(config engineconfig.Config, hnswDir, sqliteFallbackPath string) vector.Store {
	if strings.EqualFold(strings.TrimSpace(config.VectorStore), string(vector.StoreOpenSearch)) {
		createIndex := config.OpenSearchCreateIndex
		store, err := vector.NewStore(context.Background(), vector.Config{
			StoreKind:                    vector.StoreOpenSearch,
			Dim:                          1536,
			OpenSearchAddresses:          splitCommaList(config.OpenSearchAddresses),
			OpenSearchUsername:           strings.TrimSpace(config.OpenSearchUsername),
			OpenSearchPassword:           strings.TrimSpace(config.OpenSearchPassword),
			OpenSearchAPIKey:             strings.TrimSpace(config.OpenSearchAPIKey),
			OpenSearchIndexPrefix:        strings.TrimSpace(config.OpenSearchIndexPrefix),
			OpenSearchCreateIndex:        &createIndex,
			OpenSearchKNN:                config.OpenSearchKNN,
			OpenSearchInsecureSkipVerify: config.OpenSearchInsecureSkipVerify,
			OpenSearchBulkSize:           config.OpenSearchBulkSize,
		})
		if err == nil {
			return store
		}
		log.Printf("[cli] opensearch vector store unavailable, rag disabled: %v", err)
		return nil
	}

	if hnswStore, err := vector.NewHNSWStore(hnswDir); err == nil {
		return hnswStore
	} else {
		log.Printf("[cli] hnsw vector store unavailable (%v), falling back to sqlite", err)
		sqliteStore, sqliteErr := vector.OpenSQLiteStore(sqliteFallbackPath)
		if sqliteErr != nil {
			log.Printf("[cli] sqlite vector store unavailable, rag disabled: %v", sqliteErr)
			return nil
		}
		return sqliteStore
	}
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
