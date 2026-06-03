package engine

import (
	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/hooks"
	"github.com/EngineerProjects/nexus-engine/internal/memory"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

// SessionStore persists canonical session metadata and transcript state.
type SessionStore interface {
	SaveSessionState(sessionID types.SessionID, metadata *types.SessionMetadata, previousMessages []types.Message, currentMessages []types.Message) error
}

// Engine orchestrates query sessions.
type Engine struct {
	apiClient            *providers.Client
	orchestrator         *execution.Orchestrator
	compactor            *compact.Engine
	promptAssembler      *prompt.Assembler
	promptBuilder        *prompt.Builder
	loop                 *Loop
	permissionIntegrator *permissions.Integrator
	toolRegistry         *tool.Registry
	monitoring           *monitoring.System
	sessionStore         SessionStore
	config               *Config
	hookRegistry         *hooks.Registry
	hookExecutor         *hooks.Executor
	promptFn             types.PromptFn
	memoryService        *memory.Service
	browserManager       browsercore.Manager
}

// NewEngine creates a new query engine.
func NewEngine(
	apiClient *providers.Client,
	orchestrator *execution.Orchestrator,
	compactor *compact.Engine,
	promptAssembler *prompt.Assembler,
	permissionIntegrator *permissions.Integrator,
	toolRegistry *tool.Registry,
	sessionStore SessionStore,
	config *Config,
	memoryService *memory.Service,
	monitoringSys *monitoring.System,
) *Engine {
	if config == nil {
		config = DefaultConfig()
	}
	config.WorkingDirectory = resolveWorkingDirectory(config.WorkingDirectory)

	if monitoringSys == nil {
		monitoringSys = monitoring.NewSystem(nil)
	}

	promptBuilder := prompt.NewBuilder(promptAssembler, prompt.DefaultBuilderConfig())

	hookRegistry := hooks.NewRegistry()
	hookExecutor := hooks.NewExecutor(hookRegistry)

	if orchestrator != nil && config.MaxConsecutiveDenials > 0 {
		orchestrator.SetDenialLimitConfig(types.DenialLimitConfig{MaxConsecutiveDenials: config.MaxConsecutiveDenials})
	}

	var providerConfig *providers.Config
	if apiClient != nil {
		providerConfig = apiClient.Config()
		apiClient.SetMonitoring(monitoringSys)
	}
	loop := NewLoop(
		apiClient,
		orchestrator,
		compactor,
		promptAssembler,
		permissionIntegrator,
		hookExecutor,
		loopConfigFromConfig(config),
		monitoringSys,
		providerConfig,
	)

	if orchestrator != nil {
		orchestrator.SetMonitoring(monitoringSys)
	}

	return &Engine{
		apiClient:            apiClient,
		orchestrator:         orchestrator,
		compactor:            compactor,
		promptAssembler:      promptAssembler,
		promptBuilder:        promptBuilder,
		loop:                 loop,
		permissionIntegrator: permissionIntegrator,
		toolRegistry:         toolRegistry,
		sessionStore:         sessionStore,
		config:               config,
		hookRegistry:         hookRegistry,
		hookExecutor:         hookExecutor,
		memoryService:        memoryService,
		monitoring:           monitoringSys,
		browserManager:       config.BrowserManager,
	}
}

// SetAPIClient swaps the loop-facing API client.
func (e *Engine) SetAPIClient(apiClient *providers.Client) {
	e.apiClient = apiClient
	if e.loop != nil {
		e.loop.apiClient = apiClient
		if apiClient != nil {
			e.loop.providerConfig = apiClient.Config()
		} else {
			e.loop.providerConfig = nil
		}
	}
	if e.permissionIntegrator != nil {
		e.permissionIntegrator.SetAutoModeProviderClient(apiClient, e.config.Model)
	}
}

// HookRegistry returns the engine's hook registry for external hook registration.
func (e *Engine) HookRegistry() *hooks.Registry {
	return e.hookRegistry
}

// HookExecutor returns the engine's hook executor for external use.
func (e *Engine) HookExecutor() *hooks.Executor {
	return e.hookExecutor
}

type promptAwareTool interface {
	SetPromptFn(types.PromptFn)
}

// SetPromptFn wires the runtime prompt bridge into permissions and prompt-aware tools.
func (e *Engine) SetPromptFn(fn types.PromptFn) {
	e.promptFn = fn
	if e.permissionIntegrator != nil {
		e.permissionIntegrator.SetPromptFn(fn)
	}
	if e.toolRegistry == nil {
		return
	}
	for _, registeredTool := range e.toolRegistry.List() {
		if promptAware, ok := registeredTool.(promptAwareTool); ok {
			promptAware.SetPromptFn(fn)
		}
	}
}

// AutoModeAvailable reports whether the permission integrator has an AI-backed
// auto-mode classifier wired up and ready.
func (e *Engine) AutoModeAvailable() bool {
	if e == nil || e.permissionIntegrator == nil {
		return false
	}
	return e.permissionIntegrator.AutoModeAvailable()
}

// GetMonitoring returns the monitoring system for external integration.
func (e *Engine) GetMonitoring() *monitoring.System {
	return e.monitoring
}

func (e *Engine) workingDirectory() string {
	if e == nil || e.config == nil {
		return "."
	}
	return resolveWorkingDirectory(e.config.WorkingDirectory)
}

func (e *Engine) defaultPermissionContext() *types.PermissionContext {
	mode := e.config.PermissionMode
	if mode == "" {
		mode = types.PermissionModeOnRequest
	}
	return &types.PermissionContext{
		Mode:                             mode,
		IsBypassPermissionsModeAvailable: mode == types.PermissionModeBypass,
		IsAutoModeAvailable:              e.permissionIntegrator != nil && e.permissionIntegrator.AutoModeAvailable(),
	}
}

func (e *Engine) memoryContext() string {
	if e == nil || e.memoryService == nil {
		return ""
	}
	return e.memoryService.Context()
}
