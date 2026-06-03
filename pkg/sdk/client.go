package sdk

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	coretasks "github.com/EngineerProjects/nexus-engine/internal/runtime/tasks"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	agentTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/agents"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

// Client provides a high-level SDK for headless AI operations.
type Client struct {
	queryEngine   *engine.Engine
	orchestrator  *execution.Orchestrator
	registry      *registry.Registry
	store         SessionStore
	config        *ClientConfig
	memoryInitErr error
	mcpResult     *MCPIntegrationResult
	mcpMu         sync.Mutex
	ownedStore    bool
	browser       browsercore.Manager
	artifacts     ArtifactStore
	reaper        *storage.Reaper
	closeOnce     sync.Once
	closeErr      error
}

type promptAwareTool interface {
	SetPromptFn(PromptFn)
}

// NewClient creates a new SDK client.
func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		config = DefaultClientConfig()
	}
	if config.WorkingDir == "" {
		workingDir, err := os.Getwd()
		if err != nil || workingDir == "" {
			workingDir = "."
		}
		config.WorkingDir = workingDir
	}

	// Resolve API key: CredentialResolver takes precedence over APIKey.
	apiKey := config.APIKey
	if config.CredentialResolver != nil {
		if resolved, resolveErr := config.CredentialResolver.ResolveAPIKey(context.Background(), string(config.Model.Provider)); resolveErr == nil && resolved != "" {
			apiKey = resolved
		} else if resolveErr != nil {
			log.Printf("[sdk] CredentialResolver failed for provider %s: %v", config.Model.Provider, resolveErr)
		}
	}

	// Provider
	var apiClient *providers.Client
	if config.ProviderConfig != nil {
		pc := *config.ProviderConfig
		if pc.Provider == "" {
			pc.Provider = config.Model.Provider
		}
		if pc.APIKey == "" {
			pc.APIKey = apiKey
		}
		apiClient = providers.NewClientWithConfig(apiKey, &pc)
	} else {
		apiClient = providers.NewClient(apiKey, config.Model.Provider)
	}

	artifactStore := initArtifactStore(config)
	browserManager, reaper := initBrowserManager(config, artifactStore)
	config.BrowserRemoteControlURL = strings.TrimSpace(config.BrowserRemoteControlURL)

	// Core subsystems
	orchestrator := execution.NewOrchestrator()
	compactor := compact.NewEngine(apiClient, compact.DefaultConfig())
	promptAssembler := prompt.NewAssembler()
	promptAssembler.SetDefaultSections(prompt.DefaultSystemPromptSections())
	permissionEngine := permissions.NewEngine()
	if err := permissionEngine.AddRules(permissions.NewDefaultRules()); err != nil {
		return nil, fmt.Errorf("add default permission rules: %w", err)
	}
	permissionIntegrator := permissions.NewIntegrator(permissionEngine)
	permissionIntegrator.SetAutoModeProviderClient(apiClient, config.Model)

	// Engine config
	queryConfig := buildEngineConfig(config)
	queryConfig.BrowserManager = browserManager

	// Tool registry
	reg, err := initBuiltinRegistry(config, browserManager, artifactStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool registry: %w", err)
	}

	// MCP servers
	var mcpResult *MCPIntegrationResult
	if len(config.MCPServers) > 0 {
		mcpResult = mcp.IntegrateMCPServersWithOptions(context.Background(), reg, config.MCPServers, nil)
	}

	store, ownedStore, err := initSessionStore(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}
	var sessionStore engine.SessionStore
	if store != nil {
		sessionStore = store
	}

	memSvc, memInitErr := initMemoryService(config)
	if memInitErr != nil && config.MemoryFailFast {
		return nil, fmt.Errorf("memory service initialization failed: %w", memInitErr)
	}

	monitoringSys := initMonitoringSystem(config)

	queryEngine := engine.NewEngine(
		apiClient, orchestrator, compactor, promptAssembler,
		permissionIntegrator, reg, sessionStore, queryConfig, memSvc, monitoringSys,
	)
	queryEngine.SetPromptFn(config.PromptFn)

	coretasks.NewDefaultManager(queryEngine, nil)

	agentToolInstance := agentTool.NewAgentTool(agentTool.DefaultAgentToolConfig())
	agentToolInstance.SetEngine(queryEngine)
	if config.EnableMemory || config.EnableHooks {
		agentRunner := coreagent.NewRunner(queryEngine)
		agentRunner.SetEnableMemory(config.EnableMemory)
		agentRunner.SetEnableHooks(config.EnableHooks)
		agentToolInstance.SetRunner(agentRunner)
	}
	if err := reg.Register(agentToolInstance); err != nil {
		return nil, fmt.Errorf("failed to register agent tool: %w", err)
	}

	// spawn_agent needs the live engine instance — registered here, not in builtin.go.
	// nil tools → sub-agent inherits all tools from the engine registry at call time.
	spawnAgentTool := agentTool.NewSpawnAgentTool(queryEngine, nil, coreagent.NewAgentRegistry())
	if err := reg.Register(spawnAgentTool); err != nil {
		return nil, fmt.Errorf("failed to register spawn_agent tool: %w", err)
	}

	return &Client{
		queryEngine:   queryEngine,
		orchestrator:  orchestrator,
		registry:      reg,
		store:         store,
		config:        config,
		memoryInitErr: memInitErr,
		mcpResult:     mcpResult,
		ownedStore:    ownedStore,
		browser:       browserManager,
		artifacts:     artifactStore,
		reaper:        reaper,
	}, nil
}

// buildEngineConfig converts ClientConfig into engine.Config.
func buildEngineConfig(config *ClientConfig) *engine.Config {
	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	qc := &engine.Config{
		MaxTurns:                config.MaxTurns,
		AutoCompact:             config.AutoCompact,
		PermissionMode:          config.PermissionMode,
		Model:                   config.Model,
		MaxTokens:               maxTokens,
		WorkingDirectory:        config.WorkingDir,
		SystemPromptTemplate:    config.SystemPromptTemplate,
		MCPServers:              config.MCPServers,
		EnableMemory:            config.EnableMemory,
		EnableMonitoring:        config.EnableMonitoring,
		MaxIterations:           config.MaxIterations,
		TurnTokenBudget:         config.TurnTokenBudget,
		BudgetContinuationLimit: config.BudgetContinuationLimit,
		ContinuationNudgeLimit:  config.ContinuationNudgeLimit,
		StopHooks:               config.StopHooks,
		MaxConsecutiveDenials:   config.MaxConsecutiveDenials,
	}
	if pc := config.PromptConfig; pc != nil {
		if pc.SystemPrompt != nil {
			qc.SystemPromptTemplate = *pc.SystemPrompt
		}
		if pc.AppendSystemPrompt != nil {
			qc.AppendSystemPrompt = *pc.AppendSystemPrompt
		}
		qc.PromptStage = pc.Stage
		qc.PromptStageOverrides = pc.StageOverrides
		qc.PromptToolHints = pc.ToolHints
	}
	return qc
}

// Ask performs a single-turn query (convenience method).
func (c *Client) Ask(ctx context.Context, prompt string, tools []Tool) (*AskResponse, error) {
	session, err := c.CreateSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	for _, t := range tools {
		if err := session.RegisterTool(t); err != nil {
			return nil, fmt.Errorf("failed to register tool %q: %w", t.Definition().Name, err)
		}
	}

	response, err := session.SubmitMessage(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to submit message: %w", err)
	}

	lastMessage, ok := lastAssistantMessage(response.Messages)
	if !ok {
		return nil, fmt.Errorf("no assistant message in response")
	}

	return &AskResponse{
		Content:     extractTextContent(lastMessage),
		Thinking:    extractThinkingFromMessages(assistantMessagesForLatestTurn(response.Messages)),
		ToolUses:    response.ToolUses,
		ToolResults: response.ToolResults,
		Usage:       response.Usage,
		IsComplete:  response.IsComplete,
	}, nil
}

// CreateSession creates a new session for multi-turn conversations.
func (c *Client) CreateSession(ctx context.Context) (*Session, error) {
	querySession, err := c.queryEngine.NewSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return newSDKSession(c, querySession), nil
}

// LoadSession loads an existing session.
func (c *Client) LoadSession(ctx context.Context, sessionID SessionID) (*Session, error) {
	if c.store == nil {
		return nil, fmt.Errorf("session persistence not enabled")
	}
	metadata, messages, err := c.store.RestoreSessionState(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore session state: %w", err)
	}
	if metadata.Status == SessionStatusClosed {
		return nil, fmt.Errorf("session is closed")
	}
	querySession, err := c.queryEngine.NewSessionFromState(ctx, sessionID, metadata, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to restore session: %w", err)
	}
	return newSDKSession(c, querySession), nil
}

// RegisterTool registers a tool globally.
func (c *Client) RegisterTool(tool Tool) error {
	if promptAware, ok := tool.(promptAwareTool); ok {
		promptAware.SetPromptFn(c.config.PromptFn)
	}
	return c.registry.Register(tool)
}

// RegisterTools registers multiple tools globally.
func (c *Client) RegisterTools(tools []Tool) error {
	for _, t := range tools {
		if err := c.RegisterTool(t); err != nil {
			return err
		}
	}
	return nil
}

// ListSessions lists all sessions.
func (c *Client) ListSessions() ([]*SessionInfo, error) {
	if c.store == nil {
		return nil, fmt.Errorf("session persistence not enabled")
	}
	return c.store.GetAllSessionsInfo()
}

// DeleteSession deletes a session.
func (c *Client) DeleteSession(sessionID SessionID) error {
	if c.store == nil {
		return fmt.Errorf("session persistence not enabled")
	}
	return c.store.DeleteSession(sessionID)
}

// Close releases SDK-owned resources. Safe to call multiple times.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		var errs []error
		if c.mcpResult != nil {
			if err := c.mcpResult.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if c.ownedStore && c.store != nil {
			if err := c.store.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if c.reaper != nil {
			c.reaper.Stop()
		}
		if c.browser != nil {
			if err := c.browser.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		c.closeErr = errors.Join(errs...)
	})
	return c.closeErr
}
