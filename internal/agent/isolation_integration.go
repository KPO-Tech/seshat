package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Integration with Async Agent System
// ---------------------------------------------------------------------------

// IsolatedAsyncAgent extends AsyncAgent with permission isolation
type IsolatedAsyncAgent struct {
	*AsyncAgent

	// Permission context for this agent
	permissionContext *AgentPermissionContext

	// Isolation manager
	isolationManager *PermissionIsolationManager

	// Permission resolver
	permissionResolver types.PermissionResolver

	// Original tools
	originalTools []tool.Tool

	// Modified tools list
	modifiedTools []tool.Tool
}

// NewIsolatedAsyncAgent creates a new isolated async agent
func NewIsolatedAsyncAgent(
	asyncAgent *AsyncAgent,
	isolationManager *PermissionIsolationManager,
) *IsolatedAsyncAgent {
	return &IsolatedAsyncAgent{
		AsyncAgent:       asyncAgent,
		isolationManager: isolationManager,
		originalTools:    asyncAgent.Config.Tools,
		modifiedTools:    asyncAgent.Config.Tools,
	}
}

// InitializePermissionContext initializes the permission context for this agent
func (a *IsolatedAsyncAgent) InitializePermissionContext(parentAgentID string) error {
	a.permissionContext = a.isolationManager.CreateAgentPermissionContext(
		a.ID,
		a.Config.AgentType,
		parentAgentID,
	)

	// Apply permission constraints based on isolation level
	a.applyPermissionConstraints()

	return nil
}

// applyPermissionConstraints applies permission constraints based on isolation level
func (a *IsolatedAsyncAgent) applyPermissionConstraints() {
	if a.permissionContext == nil {
		return
	}

	switch a.permissionContext.IsolationLevel {
	case IsolationLevelStrict:
		a.applyStrictIsolation()

	case IsolationLevelSandbox:
		a.applySandboxIsolation()

	case IsolationLevelBasic:
		a.applyBasicIsolation()

	case IsolationLevelNone:
		// No isolation, keep original tools

	default:
		a.applyBasicIsolation()
	}
}

// applyBasicIsolation applies basic isolation constraints
func (a *IsolatedAsyncAgent) applyBasicIsolation() {
	// Basic isolation: Separate permission context, allow most tools
	// Modify tools based on agent type
	a.modifiedTools = a.filterToolsByAgentType(a.originalTools)

	// Set resource limits
	if a.permissionContext.ResourceLimits == nil {
		a.permissionContext.ResourceLimits = &ResourceLimits{
			MaxToolCalls:       50, // Reasonable default
			MaxDuration:        10 * time.Minute,
			MaxFileSize:        10, // 10 MB
			MaxNetworkRequests: 100,
		}
	}

	// Set isolation constraints
	if a.permissionContext.IsolationConstraints == nil {
		sharedResources := a.Config.AgentType == AgentTypeGeneralPurpose
		a.permissionContext.IsolationConstraints = &IsolationConstraints{
			NetworkAccess:     true,
			FileSystemAccess:  true,
			ProcessExecution:  true,
			EnvironmentAccess: false, // No env access by default
			SystemCommands:    false, // No system commands by default
			SharedResources:   sharedResources,
		}
	}
}

// applyStrictIsolation applies strict isolation constraints
func (a *IsolatedAsyncAgent) applyStrictIsolation() {
	// Strict isolation: More restrictive tool access, separate resources
	a.modifiedTools = a.filterToolsForStrictIsolation(a.originalTools)

	// More restrictive resource limits
	a.permissionContext.ResourceLimits = &ResourceLimits{
		MaxToolCalls:       25, // More restrictive
		MaxDuration:        5 * time.Minute,
		MaxMemory:          100, // 100 MB
		MaxFileSize:        5,   // 5 MB
		MaxNetworkRequests: 50,
	}

	// More restrictive isolation constraints
	a.permissionContext.IsolationConstraints = &IsolationConstraints{
		NetworkAccess:     true,
		FileSystemAccess:  true,
		ProcessExecution:  false, // No process execution
		EnvironmentAccess: false,
		SystemCommands:    false,
		SharedResources:   false, // No shared resources
	}
}

// applySandboxIsolation applies sandbox isolation constraints
func (a *IsolatedAsyncAgent) applySandboxIsolation() {
	// Sandbox isolation: Maximum restrictions
	a.modifiedTools = a.filterToolsForSandbox(a.originalTools)

	// Very restrictive resource limits
	a.permissionContext.ResourceLimits = &ResourceLimits{
		MaxToolCalls:       10, // Very restrictive
		MaxDuration:        2 * time.Minute,
		MaxMemory:          50, // 50 MB
		MaxFileSize:        1,  // 1 MB
		MaxNetworkRequests: 10,
	}

	// Maximum isolation constraints
	a.permissionContext.IsolationConstraints = &IsolationConstraints{
		NetworkAccess:     false, // No network access
		FileSystemAccess:  false, // No file system access
		ProcessExecution:  false,
		EnvironmentAccess: false,
		SystemCommands:    false,
		SharedResources:   false,
	}
}

// filterToolsByAgentType filters tools based on agent type
func (a *IsolatedAsyncAgent) filterToolsByAgentType(tools []tool.Tool) []tool.Tool {
	if tools == nil {
		return nil
	}

	switch a.Config.AgentType {
	case AgentTypeExplore:
		// Explore agent: Read-only tools only
		return a.filterReadOnlyTools(tools)

	case AgentTypeVerify:
		// Verify agent: Only read-only analysis tools
		return a.filterAnalysisTools(tools)

	case AgentTypePlan:
		// Plan agent: Read tools + write tools for planning
		return a.filterPlanningTools(tools)

	case AgentTypeGeneralPurpose:
		// General purpose: All tools allowed
		return tools

	default:
		return tools
	}
}

// filterReadOnlyTools returns only read-only tools
func (a *IsolatedAsyncAgent) filterReadOnlyTools(tools []tool.Tool) []tool.Tool {
	readOnlyToolNames := []string{
		"read_file", "glob", "grep", "tree",
		"web_fetch", "web_search", "web_crawl", "web_map", "wikipedia", "scholarly_search",
	}

	filtered := []tool.Tool{}
	for _, tool := range tools {
		toolName := tool.Definition().Name
		for _, readOnlyToolName := range readOnlyToolNames {
			if toolName == readOnlyToolName {
				filtered = append(filtered, tool)
				break
			}
		}
	}

	return filtered
}

// filterAnalysisTools returns only analysis tools for verify agent
func (a *IsolatedAsyncAgent) filterAnalysisTools(tools []tool.Tool) []tool.Tool {
	analysisToolNames := []string{
		"read_file", "glob", "grep",
	}

	filtered := []tool.Tool{}
	for _, tool := range tools {
		toolName := tool.Definition().Name
		for _, analysisToolName := range analysisToolNames {
			if toolName == analysisToolName {
				filtered = append(filtered, tool)
				break
			}
		}
	}

	return filtered
}

// filterPlanningTools returns tools suitable for planning
func (a *IsolatedAsyncAgent) filterPlanningTools(tools []tool.Tool) []tool.Tool {
	planningToolNames := []string{
		"read_file", "glob", "grep",
		"write_file", "edit_file", // Plan agents can write for planning
	}

	filtered := []tool.Tool{}
	for _, tool := range tools {
		toolName := tool.Definition().Name
		for _, planningToolName := range planningToolNames {
			if toolName == planningToolName {
				filtered = append(filtered, tool)
				break
			}
		}
	}

	return filtered
}

// filterToolsForStrictIsolation filters tools for strict isolation
func (a *IsolatedAsyncAgent) filterToolsForStrictIsolation(tools []tool.Tool) []tool.Tool {
	// Remove potentially dangerous tools
	dangerousToolNames := []string{
		"bash",       // Shell execution
		"edit_file",  // File modifications
		"write_file", // File modifications
	}

	filtered := []tool.Tool{}
	for _, tool := range tools {
		toolName := tool.Definition().Name
		allowed := true
		for _, dangerousToolName := range dangerousToolNames {
			if toolName == dangerousToolName {
				allowed = false
				break
			}
		}
		if allowed {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}

// filterToolsForSandbox filters tools for sandbox isolation
func (a *IsolatedAsyncAgent) filterToolsForSandbox(tools []tool.Tool) []tool.Tool {
	// Sandbox: Only read-only tools, no file system access
	safeToolNames := []string{
		"read_file", "glob", "grep",
	}

	filtered := []tool.Tool{}
	for _, tool := range tools {
		toolName := tool.Definition().Name
		for _, safeToolName := range safeToolNames {
			if toolName == safeToolName {
				filtered = append(filtered, tool)
				break
			}
		}
	}

	return filtered
}

// CreatePermissionResolver creates a permission resolver with isolation
func (a *IsolatedAsyncAgent) CreatePermissionResolver(
	sessionID types.SessionID,
	turnID types.TurnID,
) types.PermissionResolver {
	if a.permissionContext == nil {
		return nil
	}

	return types.CanUseToolFn(func(ctx context.Context, request types.ToolPermissionRequest) types.PermissionResult {
		// Update last accessed time
		a.permissionContext.LastAccessed = a.AsyncAgent.EndTime

		// Check tool permission
		allowed, reason := a.isolationManager.IsToolAllowed(a.ID, request.ToolName)
		if !allowed {
			return types.Deny(reason)
		}

		// Check resource limits
		allowed, reason = a.isolationManager.CheckResourceLimits(a.ID, "tool_call")
		if !allowed {
			return types.Deny(reason)
		}

		// Check isolation constraints
		if a.permissionContext.IsolationConstraints != nil {
			if err := a.checkIsolationConstraints(request); err != nil {
				return types.Deny(err.Error())
			}
		}

		// Record tool call
		a.isolationManager.RecordToolCall(a.ID, request.ToolName)

		// Allow the tool use
		return types.AllowWithDecisionReason(
			"Tool allowed with isolation checks",
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonOther,
				Source: "agent_isolation",
				Reason: "Tool permitted by agent isolation rules",
			},
		)
	})
}

// checkIsolationConstraints checks if action violates isolation constraints
func (a *IsolatedAsyncAgent) checkIsolationConstraints(request types.ToolPermissionRequest) error {
	if a.permissionContext.IsolationConstraints == nil {
		return nil
	}

	constraints := a.permissionContext.IsolationConstraints

	// Check file system access
	if !constraints.FileSystemAccess {
		fileSystemTools := []string{"write_file", "edit_file", "bash"}
		for _, fsTool := range fileSystemTools {
			if request.ToolName == fsTool {
				return fmt.Errorf("file system access not allowed by isolation constraints")
			}
		}
	}

	// Check network access
	if !constraints.NetworkAccess {
		networkTools := []string{"web_fetch", "web_search", "web_crawl", "web_map", "wikipedia", "scholarly_search"}
		for _, netTool := range networkTools {
			if request.ToolName == netTool {
				return fmt.Errorf("network access not allowed by isolation constraints")
			}
		}
	}

	// Check process execution
	if !constraints.ProcessExecution {
		processTools := []string{"bash"}
		for _, procTool := range processTools {
			if request.ToolName == procTool {
				return fmt.Errorf("process execution not allowed by isolation constraints")
			}
		}
	}

	return nil
}

// GetModifiedConfig returns the modified config with isolated tools
func (a *IsolatedAsyncAgent) GetModifiedConfig() *RunConfig {
	modifiedConfig := *a.Config
	modifiedConfig.Tools = a.modifiedTools
	return &modifiedConfig
}

// GetPermissionContext returns the permission context
func (a *IsolatedAsyncAgent) GetPermissionContext() *AgentPermissionContext {
	return a.permissionContext
}

// UpdatePermissionContext updates the permission context
func (a *IsolatedAsyncAgent) UpdatePermissionContext(updates func(*AgentPermissionContext)) error {
	if a.permissionContext == nil {
		return fmt.Errorf("permission context not initialized")
	}

	return a.isolationManager.UpdateAgentPermissionContext(a.ID, updates)
}

// ---------------------------------------------------------------------------
// Integration Manager
// ---------------------------------------------------------------------------

// IsolationIntegrationManager integrates isolation with async agents
type IsolationIntegrationManager struct {
	isolationManager *PermissionIsolationManager
	asyncManager     *AsyncAgentManager
	isolatedAgents   map[string]*IsolatedAsyncAgent
	isolatedAgentsMu sync.RWMutex
}

// NewIsolationIntegrationManager creates a new isolation integration manager
func NewIsolationIntegrationManager(
	isolationManager *PermissionIsolationManager,
	asyncManager *AsyncAgentManager,
) *IsolationIntegrationManager {
	return &IsolationIntegrationManager{
		isolationManager: isolationManager,
		asyncManager:     asyncManager,
		isolatedAgents:   make(map[string]*IsolatedAsyncAgent),
	}
}

// StartIsolatedAgent starts an agent with permission isolation
func (m *IsolationIntegrationManager) StartIsolatedAgent(
	config *RunConfig,
	isolationLevel IsolationLevel,
	parentAgentID string,
) (*IsolatedAsyncAgent, error) {
	// Start the async agent
	asyncAgent, err := m.asyncManager.StartAgent(config)
	if err != nil {
		return nil, err
	}

	// Create isolated agent wrapper
	isolatedAgent := NewIsolatedAsyncAgent(asyncAgent, m.isolationManager)

	// Initialize permission context first
	if err := isolatedAgent.InitializePermissionContext(parentAgentID); err != nil {
		return nil, fmt.Errorf("failed to initialize permission context: %w", err)
	}

	// Set isolation level
	if err := m.isolationManager.UpdateAgentPermissionContext(asyncAgent.ID, func(ctx *AgentPermissionContext) {
		ctx.IsolationLevel = isolationLevel
	}); err != nil {
		return nil, fmt.Errorf("failed to set isolation level: %w", err)
	}

	// Store isolated agent
	m.isolatedAgentsMu.Lock()
	m.isolatedAgents[asyncAgent.ID] = isolatedAgent
	m.isolatedAgentsMu.Unlock()

	return isolatedAgent, nil
}

// GetIsolatedAgent retrieves an isolated agent by ID
func (m *IsolationIntegrationManager) GetIsolatedAgent(agentID string) (*IsolatedAsyncAgent, error) {
	m.isolatedAgentsMu.RLock()
	defer m.isolatedAgentsMu.RUnlock()

	agent, exists := m.isolatedAgents[agentID]
	if !exists {
		return nil, fmt.Errorf("isolated agent not found: %s", agentID)
	}

	return agent, nil
}

// CancelIsolatedAgent cancels an isolated agent and cleans up isolation
func (m *IsolationIntegrationManager) CancelIsolatedAgent(agentID string) error {
	m.isolatedAgentsMu.Lock()
	defer m.isolatedAgentsMu.Unlock()

	// Cancel the async agent
	if err := m.asyncManager.CancelAgent(agentID); err != nil {
		return err
	}

	// Clean up permission context
	if err := m.isolationManager.DeleteAgentPermissionContext(agentID); err != nil {
		return err
	}

	// Remove from isolated agents map
	delete(m.isolatedAgents, agentID)

	return nil
}

// CleanupCompletedAgents cleans up completed isolated agents
func (m *IsolationIntegrationManager) CleanupCompletedAgents() {
	m.isolatedAgentsMu.Lock()
	defer m.isolatedAgentsMu.Unlock()

	for agentID, isolatedAgent := range m.isolatedAgents {
		if isolatedAgent.IsComplete() {
			// Clean up permission context
			m.isolationManager.DeleteAgentPermissionContext(agentID) //nolint:errcheck // cleanup
			// Remove from map
			delete(m.isolatedAgents, agentID)
		}
	}
}

// GetAllIsolatedAgents returns all isolated agents
func (m *IsolationIntegrationManager) GetAllIsolatedAgents() []*IsolatedAsyncAgent {
	m.isolatedAgentsMu.RLock()
	defer m.isolatedAgentsMu.RUnlock()

	agents := make([]*IsolatedAsyncAgent, 0, len(m.isolatedAgents))
	for _, agent := range m.isolatedAgents {
		agents = append(agents, agent)
	}

	return agents
}

// Shutdown shuts down the isolation integration manager
func (m *IsolationIntegrationManager) Shutdown() {
	// Cancel all isolated agents
	m.isolatedAgentsMu.Lock()
	agentIDs := make([]string, 0, len(m.isolatedAgents))
	for agentID := range m.isolatedAgents {
		agentIDs = append(agentIDs, agentID)
	}
	m.isolatedAgentsMu.Unlock()

	// Cancel each agent and clean up
	for _, agentID := range agentIDs {
		m.CancelIsolatedAgent(agentID) //nolint:errcheck // shutdown cleanup
	}
}
