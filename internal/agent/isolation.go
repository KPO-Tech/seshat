package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Agent Permission Isolation System
// ---------------------------------------------------------------------------

// IsolationLevel represents the level of permission isolation for an agent
type IsolationLevel string

const (
	// IsolationLevelNone: No isolation, shares parent permissions
	IsolationLevelNone IsolationLevel = "none"

	// IsolationLevelBasic: Basic isolation with separate permission context
	IsolationLevelBasic IsolationLevel = "basic"

	// IsolationLevelStrict: Strict isolation with isolated resources
	IsolationLevelStrict IsolationLevel = "strict"

	// IsolationLevelSandbox: Full sandbox isolation
	IsolationLevelSandbox IsolationLevel = "sandbox"
)

// AgentPermissionContext represents an isolated permission context for an agent
type AgentPermissionContext struct {
	// AgentID is the unique identifier for this agent
	AgentID string `json:"agentId"`

	// AgentType is the type of agent
	AgentType string `json:"agentType"`

	// IsolationLevel is the isolation level for this agent
	IsolationLevel IsolationLevel `json:"isolationLevel"`

	// PermissionMode is the permission mode for this agent
	PermissionMode types.PermissionMode `json:"permissionMode"`

	// PermissionContext is the underlying permission context
	PermissionContext *types.PermissionContext `json:"permissionContext"`

	// GranularConfig is the granular permission configuration
	GranularConfig *types.GranularConfig `json:"granularConfig,omitempty"`

	// AllowedTools is the list of tools this agent can use
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DeniedTools is the list of tools this agent cannot use
	DeniedTools []string `json:"deniedTools,omitempty"`

	// ResourceLimits contains resource usage limits
	ResourceLimits *ResourceLimits `json:"resourceLimits,omitempty"`

	// ResourceUsage contains current resource usage
	ResourceUsage *ResourceUsage `json:"resourceUsage,omitempty"`

	// IsolationConstraints contains additional isolation constraints
	IsolationConstraints *IsolationConstraints `json:"isolationConstraints,omitempty"`

	// ParentAgentID is the ID of the parent agent (if any)
	ParentAgentID string `json:"parentAgentId,omitempty"`

	// ChildAgents is the list of child agent IDs
	ChildAgents []string `json:"childAgents,omitempty"`

	// CreatedAt is when this context was created
	CreatedAt time.Time `json:"createdAt"`

	// LastAccessed is when this context was last accessed
	LastAccessed time.Time `json:"lastAccessed"`
}

// ResourceLimits defines resource limits for an agent
type ResourceLimits struct {
	// MaxToolCalls limits the total number of tool calls
	MaxToolCalls int `json:"maxToolCalls,omitempty"`

	// MaxDuration limits the total execution duration
	MaxDuration time.Duration `json:"maxDuration,omitempty"`

	// MaxMemory limits the memory usage (in MB)
	MaxMemory int `json:"maxMemory,omitempty"`

	// MaxFileSize limits the maximum file size for operations (in MB)
	MaxFileSize int `json:"maxFileSize,omitempty"`

	// MaxNetworkRequests limits the number of network requests
	MaxNetworkRequests int `json:"maxNetworkRequests,omitempty"`

	// AllowedDirectories restricts file operations to specific directories
	AllowedDirectories []string `json:"allowedDirectories,omitempty"`

	// DeniedDirectories blocks file operations in specific directories
	DeniedDirectories []string `json:"deniedDirectories,omitempty"`
}

// ResourceUsage tracks current resource usage for an agent
type ResourceUsage struct {
	// ToolCalls is the number of tool calls made
	ToolCalls int `json:"toolCalls"`

	// Duration is the total execution duration
	Duration time.Duration `json:"duration"`

	// MemoryUsage is the current memory usage (in MB)
	MemoryUsage int `json:"memoryUsage"`

	// NetworkRequests is the number of network requests made
	NetworkRequests int `json:"networkRequests"`

	// FilesAccessed is the list of files accessed
	FilesAccessed []string `json:"filesAccessed,omitempty"`

	// DirectoriesCreated is the list of directories created
	DirectoriesCreated []string `json:"directoriesCreated,omitempty"`
}

// IsolationConstraints defines additional isolation constraints
type IsolationConstraints struct {
	// NetworkAccess controls whether the agent can access the network
	NetworkAccess bool `json:"networkAccess"`

	// FileSystemAccess controls whether the agent can access the file system
	FileSystemAccess bool `json:"fileSystemAccess"`

	// ProcessExecution controls whether the agent can execute processes
	ProcessExecution bool `json:"processExecution"`

	// EnvironmentAccess controls whether the agent can access environment variables
	EnvironmentAccess bool `json:"environmentAccess"`

	// SystemCommands controls whether the agent can run system commands
	SystemCommands bool `json:"systemCommands"`

	// SharedResources controls whether the agent can access shared resources
	SharedResources bool `json:"sharedResources"`
}

// PermissionIsolationManager manages isolated permission contexts for agents
type PermissionIsolationManager struct {
	// Active permission contexts by agent ID
	permissionContexts   map[string]*AgentPermissionContext
	permissionContextsMu sync.RWMutex

	// Global default configuration
	defaultIsolationLevel       IsolationLevel
	defaultPermissionMode       types.PermissionMode
	defaultResourceLimits       *ResourceLimits
	defaultIsolationConstraints *IsolationConstraints

	// Resource tracking
	resourceTracker *ResourceTracker

	// Context cleanup
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker
	cleanupDone     chan struct{}
}

// ResourceTracker tracks resource usage across all agents
type ResourceTracker struct {
	// Global resource limits
	globalLimits *ResourceLimits

	// Per-agent resource usage
	agentUsage   map[string]*ResourceUsage
	agentUsageMu sync.RWMutex

	// Shared resources
	sharedResources   map[string]interface{}
	sharedResourcesMu sync.RWMutex
}

// NewPermissionIsolationManager creates a new permission isolation manager
func NewPermissionIsolationManager() *PermissionIsolationManager {
	tracker := &ResourceTracker{
		globalLimits:    &ResourceLimits{},
		agentUsage:      make(map[string]*ResourceUsage),
		sharedResources: make(map[string]interface{}),
	}

	manager := &PermissionIsolationManager{
		permissionContexts:    make(map[string]*AgentPermissionContext),
		resourceTracker:       tracker,
		defaultIsolationLevel: IsolationLevelBasic,
		defaultPermissionMode: types.PermissionModeOnRequest,
		cleanupInterval:       30 * time.Minute,
		cleanupDone:           make(chan struct{}),
	}

	// Start cleanup goroutine
	manager.startCleanup()

	return manager
}

// SetDefaultIsolationLevel sets the default isolation level
func (m *PermissionIsolationManager) SetDefaultIsolationLevel(level IsolationLevel) {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()
	m.defaultIsolationLevel = level
}

// SetDefaultPermissionMode sets the default permission mode
func (m *PermissionIsolationManager) SetDefaultPermissionMode(mode types.PermissionMode) {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()
	m.defaultPermissionMode = mode
}

// SetDefaultResourceLimits sets the default resource limits
func (m *PermissionIsolationManager) SetDefaultResourceLimits(limits *ResourceLimits) {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()
	m.defaultResourceLimits = limits
}

// SetDefaultIsolationConstraints sets the default isolation constraints
func (m *PermissionIsolationManager) SetDefaultIsolationConstraints(constraints *IsolationConstraints) {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()
	m.defaultIsolationConstraints = constraints
}

// CreateAgentPermissionContext creates an isolated permission context for an agent
func (m *PermissionIsolationManager) CreateAgentPermissionContext(
	agentID string,
	agentType string,
	parentAgentID string,
) *AgentPermissionContext {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()

	now := time.Now()

	context := &AgentPermissionContext{
		AgentID:              agentID,
		AgentType:            agentType,
		IsolationLevel:       m.defaultIsolationLevel,
		PermissionMode:       m.defaultPermissionMode,
		PermissionContext:    m.createBasePermissionContext(),
		ResourceLimits:       m.cloneDefaultResourceLimits(),
		ResourceUsage:        &ResourceUsage{},
		IsolationConstraints: m.cloneDefaultConstraints(),
		ParentAgentID:        parentAgentID,
		ChildAgents:          []string{},
		CreatedAt:            now,
		LastAccessed:         now,
	}

	// Store the context
	m.permissionContexts[agentID] = context

	// Add child to parent's child list if parent exists
	if parentAgentID != "" {
		if parentContext, parentExists := m.permissionContexts[parentAgentID]; parentExists {
			parentContext.ChildAgents = append(parentContext.ChildAgents, agentID)
		}
	}

	// Initialize resource tracking
	m.resourceTracker.agentUsageMu.Lock()
	m.resourceTracker.agentUsage[agentID] = &ResourceUsage{}
	m.resourceTracker.agentUsageMu.Unlock()

	return context
}

// createBasePermissionContext creates a base permission context
func (m *PermissionIsolationManager) createBasePermissionContext() *types.PermissionContext {
	return &types.PermissionContext{
		Mode:          m.defaultPermissionMode,
		ExecutionMode: "execute",
	}
}

// cloneDefaultResourceLimits clones the default resource limits
func (m *PermissionIsolationManager) cloneDefaultResourceLimits() *ResourceLimits {
	if m.defaultResourceLimits == nil {
		return nil
	}

	return &ResourceLimits{
		MaxToolCalls:       m.defaultResourceLimits.MaxToolCalls,
		MaxDuration:        m.defaultResourceLimits.MaxDuration,
		MaxMemory:          m.defaultResourceLimits.MaxMemory,
		MaxFileSize:        m.defaultResourceLimits.MaxFileSize,
		MaxNetworkRequests: m.defaultResourceLimits.MaxNetworkRequests,
		AllowedDirectories: append([]string{}, m.defaultResourceLimits.AllowedDirectories...),
		DeniedDirectories:  append([]string{}, m.defaultResourceLimits.DeniedDirectories...),
	}
}

// cloneDefaultConstraints clones the default isolation constraints
func (m *PermissionIsolationManager) cloneDefaultConstraints() *IsolationConstraints {
	if m.defaultIsolationConstraints == nil {
		return nil
	}

	return &IsolationConstraints{
		NetworkAccess:     m.defaultIsolationConstraints.NetworkAccess,
		FileSystemAccess:  m.defaultIsolationConstraints.FileSystemAccess,
		ProcessExecution:  m.defaultIsolationConstraints.ProcessExecution,
		EnvironmentAccess: m.defaultIsolationConstraints.EnvironmentAccess,
		SystemCommands:    m.defaultIsolationConstraints.SystemCommands,
		SharedResources:   m.defaultIsolationConstraints.SharedResources,
	}
}

// GetAgentPermissionContext retrieves an agent's permission context
func (m *PermissionIsolationManager) GetAgentPermissionContext(agentID string) (*AgentPermissionContext, error) {
	m.permissionContextsMu.RLock()
	defer m.permissionContextsMu.RUnlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return nil, fmt.Errorf("agent permission context not found: %s", agentID)
	}

	// Update last accessed time
	context.LastAccessed = time.Now()

	return context, nil
}

// UpdateAgentPermissionContext updates an agent's permission context
func (m *PermissionIsolationManager) UpdateAgentPermissionContext(
	agentID string,
	updates func(*AgentPermissionContext),
) error {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return fmt.Errorf("agent permission context not found: %s", agentID)
	}

	// Apply updates
	updates(context)
	context.LastAccessed = time.Now()

	return nil
}

// DeleteAgentPermissionContext removes an agent's permission context
func (m *PermissionIsolationManager) DeleteAgentPermissionContext(agentID string) error {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return fmt.Errorf("agent permission context not found: %s", agentID)
	}

	// Remove child relationships
	for _, childID := range context.ChildAgents {
		if childCtx, childExists := m.permissionContexts[childID]; childExists {
			childCtx.ParentAgentID = ""
		}
	}

	// Remove from parent
	if context.ParentAgentID != "" {
		if parentCtx, parentExists := m.permissionContexts[context.ParentAgentID]; parentExists {
			for i, childID := range parentCtx.ChildAgents {
				if childID == agentID {
					parentCtx.ChildAgents = append(parentCtx.ChildAgents[:i], parentCtx.ChildAgents[i+1:]...)
					break
				}
			}
		}
	}

	// Delete the context
	delete(m.permissionContexts, agentID)

	// Cleanup resource tracking
	m.resourceTracker.agentUsageMu.Lock()
	delete(m.resourceTracker.agentUsage, agentID)
	m.resourceTracker.agentUsageMu.Unlock()

	return nil
}

// IsToolAllowed checks if a tool is allowed for an agent based on isolation
func (m *PermissionIsolationManager) IsToolAllowed(agentID, toolName string) (bool, string) {
	m.permissionContextsMu.RLock()
	defer m.permissionContextsMu.RUnlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return false, "agent permission context not found"
	}

	// Check denied tools
	for _, deniedTool := range context.DeniedTools {
		if deniedTool == toolName {
			return false, fmt.Sprintf("tool %s is explicitly denied for agent %s", toolName, agentID)
		}
	}

	// If allowed tools are specified, check if tool is in the list
	if len(context.AllowedTools) > 0 {
		allowed := false
		for _, allowedTool := range context.AllowedTools {
			if allowedTool == toolName {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("tool %s is not in allowed tools for agent %s", toolName, agentID)
		}
	}

	return true, ""
}

// CheckResourceLimits checks if an action would exceed resource limits
func (m *PermissionIsolationManager) CheckResourceLimits(
	agentID string,
	action string,
) (bool, string) {
	m.permissionContextsMu.RLock()
	defer m.permissionContextsMu.RUnlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return false, "agent permission context not found"
	}

	if context.ResourceLimits == nil {
		return true, ""
	}

	// Get current usage
	m.resourceTracker.agentUsageMu.RLock()
	usage, usageExists := m.resourceTracker.agentUsage[agentID]
	m.resourceTracker.agentUsageMu.RUnlock()

	if !usageExists {
		return true, ""
	}

	// Check tool call limit
	if context.ResourceLimits.MaxToolCalls > 0 && usage.ToolCalls >= context.ResourceLimits.MaxToolCalls {
		return false, fmt.Sprintf("tool call limit reached: %d/%d", usage.ToolCalls, context.ResourceLimits.MaxToolCalls)
	}

	// Check duration limit
	if context.ResourceLimits.MaxDuration > 0 && usage.Duration >= context.ResourceLimits.MaxDuration {
		return false, fmt.Sprintf("duration limit reached: %v/%v", usage.Duration, context.ResourceLimits.MaxDuration)
	}

	// Check network request limit
	if context.ResourceLimits.MaxNetworkRequests > 0 && usage.NetworkRequests >= context.ResourceLimits.MaxNetworkRequests {
		return false, fmt.Sprintf("network request limit reached: %d/%d", usage.NetworkRequests, context.ResourceLimits.MaxNetworkRequests)
	}

	return true, ""
}

// RecordToolCall records a tool call for resource tracking
func (m *PermissionIsolationManager) RecordToolCall(agentID, toolName string) {
	m.resourceTracker.agentUsageMu.Lock()
	defer m.resourceTracker.agentUsageMu.Unlock()

	usage, exists := m.resourceTracker.agentUsage[agentID]
	if !exists {
		return
	}

	usage.ToolCalls++
}

// RecordDuration records execution duration for resource tracking
func (m *PermissionIsolationManager) RecordDuration(agentID string, duration time.Duration) {
	m.resourceTracker.agentUsageMu.Lock()
	defer m.resourceTracker.agentUsageMu.Unlock()

	usage, exists := m.resourceTracker.agentUsage[agentID]
	if !exists {
		return
	}

	usage.Duration += duration
}

// RecordNetworkRequest records a network request for resource tracking
func (m *PermissionIsolationManager) RecordNetworkRequest(agentID string) {
	m.resourceTracker.agentUsageMu.Lock()
	defer m.resourceTracker.agentUsageMu.Unlock()

	usage, exists := m.resourceTracker.agentUsage[agentID]
	if !exists {
		return
	}

	usage.NetworkRequests++
}

// RecordFileAccess records a file access for resource tracking
func (m *PermissionIsolationManager) RecordFileAccess(agentID, filePath string) {
	m.resourceTracker.agentUsageMu.Lock()
	defer m.resourceTracker.agentUsageMu.Unlock()

	usage, exists := m.resourceTracker.agentUsage[agentID]
	if !exists {
		return
	}

	// Add to files accessed if not already present
	for _, accessedFile := range usage.FilesAccessed {
		if accessedFile == filePath {
			return
		}
	}

	usage.FilesAccessed = append(usage.FilesAccessed, filePath)
}

// IsDirectoryAllowed checks if directory access is allowed based on isolation
func (m *PermissionIsolationManager) IsDirectoryAllowed(agentID, directory string) (bool, string) {
	m.permissionContextsMu.RLock()
	defer m.permissionContextsMu.RUnlock()

	context, exists := m.permissionContexts[agentID]
	if !exists {
		return false, "agent permission context not found"
	}

	if context.ResourceLimits == nil {
		return true, ""
	}

	// Check denied directories - check if path starts with denied directory
	for _, deniedDir := range context.ResourceLimits.DeniedDirectories {
		if directory == deniedDir || len(directory) > len(deniedDir) && directory[:len(deniedDir)+1] == deniedDir+"/" {
			return false, fmt.Sprintf("directory %s is explicitly denied", directory)
		}
	}

	// If allowed directories are specified, check if directory is allowed
	if len(context.ResourceLimits.AllowedDirectories) > 0 {
		allowed := false
		for _, allowedDir := range context.ResourceLimits.AllowedDirectories {
			if directory == allowedDir || len(directory) > len(allowedDir) && directory[:len(allowedDir)+1] == allowedDir+"/" {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("directory %s is not in allowed directories", directory)
		}
	}

	return true, ""
}

// GetResourceUsage returns current resource usage for an agent
func (m *PermissionIsolationManager) GetResourceUsage(agentID string) (*ResourceUsage, error) {
	m.resourceTracker.agentUsageMu.RLock()
	defer m.resourceTracker.agentUsageMu.RUnlock()

	usage, exists := m.resourceTracker.agentUsage[agentID]
	if !exists {
		return nil, fmt.Errorf("agent resource usage not found: %s", agentID)
	}

	// Return a copy to avoid race conditions
	return &ResourceUsage{
		ToolCalls:          usage.ToolCalls,
		Duration:           usage.Duration,
		MemoryUsage:        usage.MemoryUsage,
		NetworkRequests:    usage.NetworkRequests,
		FilesAccessed:      append([]string{}, usage.FilesAccessed...),
		DirectoriesCreated: append([]string{}, usage.DirectoriesCreated...),
	}, nil
}

// GetAllAgentContexts returns all active agent permission contexts
func (m *PermissionIsolationManager) GetAllAgentContexts() []*AgentPermissionContext {
	m.permissionContextsMu.RLock()
	defer m.permissionContextsMu.RUnlock()

	contexts := make([]*AgentPermissionContext, 0, len(m.permissionContexts))
	for _, context := range m.permissionContexts {
		contexts = append(contexts, context)
	}

	return contexts
}

// startCleanup starts the periodic cleanup of inactive contexts
func (m *PermissionIsolationManager) startCleanup() {
	m.cleanupTicker = time.NewTicker(m.cleanupInterval)

	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupInactiveContexts()

			case <-m.cleanupDone:
				return
			}
		}
	}()
}

// cleanupInactiveContexts removes inactive agent permission contexts
func (m *PermissionIsolationManager) cleanupInactiveContexts() {
	m.permissionContextsMu.Lock()
	defer m.permissionContextsMu.Unlock()

	now := time.Now()
	inactiveThreshold := 2 * m.cleanupInterval // 2x the cleanup interval

	for agentID, context := range m.permissionContexts {
		// Don't cleanup if agent is still active
		if now.Sub(context.LastAccessed) < inactiveThreshold {
			continue
		}

		// Only cleanup if agent is complete
		m.DeleteAgentPermissionContext(agentID) //nolint:errcheck // periodic cleanup
	}
}

// Shutdown gracefully shuts down the isolation manager
func (m *PermissionIsolationManager) Shutdown() {
	// Stop cleanup ticker
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
	}

	// Signal cleanup goroutine to stop
	close(m.cleanupDone)

	// Clean up all contexts
	// Get all agent IDs while holding the lock
	m.permissionContextsMu.Lock()
	agentIDs := make([]string, 0, len(m.permissionContexts))
	for agentID := range m.permissionContexts {
		agentIDs = append(agentIDs, agentID)
	}
	m.permissionContextsMu.Unlock()

	// Delete each context without holding the lock
	for _, agentID := range agentIDs {
		m.DeleteAgentPermissionContext(agentID) //nolint:errcheck // shutdown cleanup
	}
}

// Default permission isolation manager instance
var defaultPermissionIsolationManager *PermissionIsolationManager
var defaultPermissionIsolationManagerOnce sync.Once

// GetDefaultPermissionIsolationManager returns the default permission isolation manager instance
func GetDefaultPermissionIsolationManager() *PermissionIsolationManager {
	defaultPermissionIsolationManagerOnce.Do(func() {
		defaultPermissionIsolationManager = NewPermissionIsolationManager()
	})
	return defaultPermissionIsolationManager
}
