package engine

import (
	"context"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/memory"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
)

// NewEngineWithRegistry creates a new query engine with a custom tool registry
func NewEngineWithRegistry(
	ctx context.Context,
	apiClient *providers.Client,
	toolRegistry *tool.Registry,
	config *Config,
) (*Engine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Integrate MCP servers if configured
	if len(config.MCPServers) > 0 {
		mcp.IntegrateMCPServersWithOptions(ctx, toolRegistry, config.MCPServers, nil)
	}

	// Create orchestrator
	orchestrator := execution.NewOrchestrator()

	// Create compactor
	compactor := compact.NewEngine(apiClient, compact.DefaultConfig())

	// Create prompt assembler
	promptAssembler := prompt.NewAssembler()
	promptAssembler.SetDefaultSections(prompt.DefaultSystemPromptSections())

	// Create permission engine with default safety rules
	permissionEngine := permissions.NewEngine()
	if err := permissionEngine.AddRules(permissions.NewDefaultRules()); err != nil {
		return nil, fmt.Errorf("add default permission rules: %w", err)
	}

	// Create permission integrator; wire auto-mode classifier when an API client
	// is available so PermissionModeAuto works outside the SDK path as well.
	permissionIntegrator := permissions.NewIntegrator(permissionEngine)
	permissionIntegrator.SetAutoModeProviderClient(apiClient, config.Model)

	// Create memory service (optional - can be nil)
	var memoryService *memory.Service
	if config.EnableMemory {
		var err error
		memoryService, err = memory.NewService()
		if err != nil {
			// Log warning but don't fail engine creation
			// return nil, err
			memoryService = nil
		}
	}

	// Create monitoring system — nil passed to NewEngine means discard logger,
	// so only create a real system when explicitly requested.
	var monitoringSys *monitoring.System
	if config.EnableMonitoring {
		monitoringSys = monitoring.NewSystem(nil)
	}

	return NewEngine(
		apiClient,
		orchestrator,
		compactor,
		promptAssembler,
		permissionIntegrator,
		toolRegistry,
		nil,
		config,
		memoryService,
		monitoringSys,
	), nil
}
