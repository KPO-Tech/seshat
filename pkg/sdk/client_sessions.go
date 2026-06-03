package sdk

import (
	"context"
	"fmt"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	websearchtool "github.com/EngineerProjects/nexus-engine/internal/tools/web/search"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// LoadSessionWithAdditional loads an existing session and merges additional metadata keys.
// Keys already present in the session are not overwritten.
func (c *Client) LoadSessionWithAdditional(ctx context.Context, sessionID SessionID, additional map[string]any) (*Session, error) {
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
	if len(additional) > 0 {
		if metadata.Additional == nil {
			metadata.Additional = make(map[string]any)
		}
		for k, v := range additional {
			if _, exists := metadata.Additional[k]; !exists {
				metadata.Additional[k] = v
			}
		}
	}
	querySession, err := c.queryEngine.NewSessionFromState(ctx, sessionID, metadata, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to restore session: %w", err)
	}
	return newSDKSession(c, querySession), nil
}

// CreateSessionWithAdditional creates a new session with pre-populated additional metadata.
func (c *Client) CreateSessionWithAdditional(ctx context.Context, additional map[string]any) (*Session, error) {
	if len(additional) == 0 {
		return c.CreateSession(ctx)
	}
	meta := &types.SessionMetadata{
		Additional:    additional,
		SchemaVersion: types.SessionMetadataSchemaVersion,
	}
	querySession, err := c.queryEngine.NewSessionFromState(ctx, types.SessionID(""), meta, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return newSDKSession(c, querySession), nil
}

// ReloadMCPServers hot-reloads MCP server integrations without restarting the client.
func (c *Client) ReloadMCPServers(ctx context.Context, servers []MCPServerConfig) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	c.mcpMu.Lock()
	defer c.mcpMu.Unlock()

	if c.mcpResult != nil {
		for _, t := range c.mcpResult.MCPTools {
			_ = c.registry.Unregister(t.Definition().Name)
		}
		_ = c.mcpResult.Close()
		c.mcpResult = nil
	}

	if len(servers) == 0 {
		return nil
	}

	result := mcp.IntegrateMCPServersWithOptions(ctx, c.registry, servers, nil)
	c.mcpResult = result
	return result.Error
}

type webSearchAwareTool interface {
	SetRunner(websearchtool.RunnerFn)
}

// SetWebSearchRunner installs a per-request runner on the web_search tool.
func (c *Client) SetWebSearchRunner(fn websearchtool.RunnerFn) {
	if c.registry == nil {
		return
	}
	t, ok := c.registry.Get("web_search")
	if !ok {
		return
	}
	if aware, ok := t.(webSearchAwareTool); ok {
		aware.SetRunner(fn)
	}
}

// BuildToolSurface builds the current external tool surface for the client.
func (c *Client) BuildToolSurface(ctx context.Context) (*Surface, error) {
	if c.registry == nil {
		return &Surface{Tools: []ToolDefinition{}}, nil
	}
	builder := registry.NewSurfaceBuilder(c.registry)
	return builder.Build(ctx, registry.SurfaceBuildRequest{
		IncludeReadOnly:    true,
		IncludeDestructive: true,
	})
}

// ToolNames returns the registered primary tool names in stable order.
func (c *Client) ToolNames() []string {
	if c.registry == nil {
		return nil
	}
	return c.registry.GetToolNames()
}

// MemoryInitError returns the error from memory service initialization, if any.
func (c *Client) MemoryInitError() error {
	return c.memoryInitErr
}

// MCPResult returns the outcome of MCP server integration.
func (c *Client) MCPResult() *MCPIntegrationResult {
	return c.mcpResult
}

// SetPromptFn updates the runtime prompt bridge.
func (c *Client) SetPromptFn(promptFn PromptFn) {
	c.config.PromptFn = promptFn
	if c.queryEngine != nil {
		c.queryEngine.SetPromptFn(promptFn)
	}
}

// SetProgressFn updates the host callback for live tool execution progress.
func (c *Client) SetProgressFn(progressFn func(ToolProgress)) {
	c.config.ProgressFn = progressFn
}

// SetResponseChunkFn updates the host callback for live model stream chunks.
func (c *Client) SetResponseChunkFn(chunkFn func(ResponseChunk)) {
	c.config.ResponseChunkFn = chunkFn
}

// SetRuntimeEventFn updates the host callback for structured runtime events.
func (c *Client) SetRuntimeEventFn(runtimeEventFn func(RuntimeEvent)) {
	c.config.RuntimeEventFn = runtimeEventFn
}

// GetMonitoring returns the underlying monitoring system.
func (c *Client) GetMonitoring() *MonitoringSystem {
	if c == nil || c.queryEngine == nil {
		return nil
	}
	return c.queryEngine.GetMonitoring()
}

// GetSessionStore returns the configured SDK-visible session store.
func (c *Client) GetSessionStore() SessionStore {
	if c == nil {
		return nil
	}
	return c.store
}

// GetArtifactStore returns the configured SDK-visible artifact store.
func (c *Client) GetArtifactStore() ArtifactStore {
	if c == nil {
		return nil
	}
	return c.artifacts
}

// AddToolHook registers a host-side tool hook on the shared orchestrator.
func (c *Client) AddToolHook(hook ToolHook) {
	if c == nil || c.orchestrator == nil {
		return
	}
	c.orchestrator.AddHook(hook)
}

// HookRegistry returns the engine's lifecycle hook registry.
func (c *Client) HookRegistry() *HookRegistry {
	if c == nil || c.queryEngine == nil {
		return nil
	}
	return c.queryEngine.HookRegistry()
}

// RegisterHook registers a handler for a single lifecycle event.
func (c *Client) RegisterHook(event HookEvent, handler HookHandler) string {
	r := c.HookRegistry()
	if r == nil {
		return ""
	}
	id := fmt.Sprintf("hook-%d", time.Now().UnixNano())
	r.Add(HookRegistration{
		ID:      id,
		Event:   event,
		Handler: handler,
		State:   HookStateActive,
	})
	return id
}

// AutoModeAvailable reports whether the permission integrator has an AI-backed
// auto-mode classifier wired and ready.
func (c *Client) AutoModeAvailable() bool {
	if c == nil || c.queryEngine == nil {
		return false
	}
	return c.queryEngine.AutoModeAvailable()
}
