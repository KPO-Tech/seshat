package builtin

import (
	"bufio"
	"context"
	"os"

	"github.com/EngineerProjects/nexus-engine/internal/audio/stt"
	"github.com/EngineerProjects/nexus-engine/internal/audio/tts"
	"github.com/EngineerProjects/nexus-engine/internal/fim"
	"github.com/EngineerProjects/nexus-engine/internal/image"
	longterm "github.com/EngineerProjects/nexus-engine/internal/memory/longterm"
	"github.com/EngineerProjects/nexus-engine/internal/rag"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

// PlanStore is the minimal interface the submit_plan tool needs to persist plan documents.
type PlanStore interface {
	// CreateOrUpdate upserts a plan document by ID. If planID is empty a new document is created.
	CreateOrUpdate(ctx context.Context, planID, sessionID, userID, slug, filename, content string) (id string, version int, err error)
	// SetStatus updates a plan document's status (pending/validated/rejected).
	SetStatus(ctx context.Context, planID string, status string) error
}

// Config controls how builtin tools are assembled for a specific host runtime.
type Config struct {
	WorkingDir                 string
	UserID                     string
	PromptFn                   types.PromptFn
	EnablePromptReaderFallback bool
	InputReader                *bufio.Reader
	MCPManager                 *mcp.MCPClientManager
	BrowserManager             browsercore.Manager
	ArtifactStore              storage.ArtifactStore
	// RAGService enables the rag_search and rag_ingest tools when set.
	// When nil those tools register but always return a "not configured" error.
	RAGService *rag.Service
	// PlanStore enables the submit_plan tool when set.
	PlanStore PlanStore
	// LongTermMemory enables the memory_* tools when set.
	// Provide any implementation that satisfies longterm.Store when wiring the runtime.
	LongTermMemory longterm.Store

	// DoclingURL is the base URL of a running docling-serve instance.
	// When set, the read_file tool converts PDFs to structured markdown.
	// Example: "http://localhost:5001"
	DoclingURL string

	// ImageGenerator enables the generate_image tool when set.
	// Use imageproviders.NewOpenAI(apiKey) or imageproviders.NewGemini(apiKey)
	// from internal/image/providers to create a provider client.
	ImageGenerator image.Generation

	// TTSGenerator enables the text_to_speech tool when set.
	// Use audioproviders.NewOpenAITTS(apiKey) from internal/audio/providers.
	TTSGenerator tts.Generation

	// STTTranscriber enables the speech_to_text tool when set.
	// Use audioproviders.NewOpenAISTT(apiKey) from internal/audio/providers.
	STTTranscriber stt.SpeechToText

	// FIMCompleter enables the code_complete tool when set.
	// Use fimproviders.NewMistral(apiKey) or fimproviders.NewDeepSeek(apiKey)
	// from internal/fim/providers to create a provider client.
	FIMCompleter fim.Completer
}

func DefaultConfig() *Config {
	workingDir, err := os.Getwd()
	if err != nil || workingDir == "" {
		workingDir = "."
	}
	artifactStore, _ := storage.DefaultArtifactStore()
	return &Config{
		WorkingDir:     workingDir,
		MCPManager:     mcp.GlobalMCPManager(),
		BrowserManager: browsercore.DefaultManager(),
		ArtifactStore:  artifactStore,
	}
}

func normalizeConfig(config *Config) *Config {
	if config == nil {
		config = DefaultConfig()
	}
	if config.WorkingDir == "" {
		config.WorkingDir = DefaultConfig().WorkingDir
	}
	if config.MCPManager == nil {
		config.MCPManager = mcp.GlobalMCPManager()
	}
	if config.BrowserManager == nil {
		config.BrowserManager = browsercore.DefaultManager()
	}
	if config.ArtifactStore == nil {
		config.ArtifactStore, _ = storage.DefaultArtifactStore()
	}
	return config
}
