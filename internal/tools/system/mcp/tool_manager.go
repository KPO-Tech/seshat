package mcp

import (
	"context"
	"fmt"
	"time"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// ToolServerConfig represents MCP server configuration for the MCP tool surface.
// Kept separate from the transport-level ServerConfig to avoid conflating
// runtime connection intent with the canonical MCP client config.
type ToolServerConfig struct {
	// Name is the server name
	Name string

	// Command is the command to start the server (for stdio)
	Command string

	// Args are the command arguments (for stdio)
	Args []string

	// URL is the server URL (for http)
	URL string

	// Transport is the transport type (stdio or http)
	Transport string

	// Env are environment variables
	Env map[string]string

	// Headers are HTTP headers (for http transport)
	Headers map[string]string
}

// ConnectServerConfig is the config for connecting to an MCP server
type ConnectServerConfig struct {
	// Name is the server name
	Name string `json:"name"`

	// Command is the command to start the server (for stdio)
	Command string `json:"command,omitempty"`

	// Args are the command arguments (for stdio)
	Args []string `json:"args,omitempty"`

	// URL is the server URL (for http)
	URL string `json:"url,omitempty"`

	// Transport is the transport type ("stdio" or "http")
	Transport string `json:"transport"`

	// Env are environment variables
	Env map[string]string `json:"env,omitempty"`

	// Timeout is the timeout in seconds
	Timeout int `json:"timeout"`

	// Headers are HTTP headers (for http transport)
	Headers map[string]string `json:"headers,omitempty"`
}

// Connect connects to an MCP server and registers its tools
func (m *MCPClientManager) Connect(ctx context.Context, config ConnectServerConfig) error {
	// Validate config
	if config.Name == "" {
		return fmt.Errorf("server name is required")
	}

	if config.Command == "" && config.URL == "" {
		return fmt.Errorf("either command or URL is required")
	}

	// Determine transport type
	transportType := TransportTypeStdio
	if config.Transport == "http" || config.URL != "" {
		transportType = TransportTypeHTTP
	}

	// Create server config
	serverConfig := ServerConfig{
		Name:      config.Name,
		Command:   config.Command,
		Args:      config.Args,
		URL:       config.URL,
		Transport: transportType,
		Env:       config.Env,
		Timeout:   time.Duration(config.Timeout) * time.Second,
		Headers:   config.Headers,
	}

	// Create MCP client
	client, err := NewClient(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create MCP client: %w", err)
	}

	// Start the client
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Initialize the client
	_, err = client.Initialize(ctx)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	// Store the client
	m.mu.Lock()
	m.clients[config.Name] = client
	m.mu.Unlock()

	return nil
}

// Disconnect disconnects from an MCP server
func (m *MCPClientManager) Disconnect(serverName string) error {
	m.mu.Lock()
	client, ok := m.clients[serverName]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("server '%s' not connected", serverName)
	}
	delete(m.clients, serverName)
	m.mu.Unlock()

	if err := client.Close(); err != nil {
		return fmt.Errorf("failed to close MCP client: %w", err)
	}
	return nil
}

// GetConnectedServers returns all connected server names
func (m *MCPClientManager) GetConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	servers := make([]string, 0, len(m.clients))
	for name := range m.clients {
		servers = append(servers, name)
	}
	return servers
}

// GetServerTools returns all tools from a connected server
func (m *MCPClientManager) GetServerTools(ctx context.Context, serverName string) ([]Tool, error) {
	m.mu.RLock()
	client, ok := m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server '%s' not connected", serverName)
	}

	return client.ListTools(ctx)
}

// AllTools returns all tools from all connected servers as native tools
func (m *MCPClientManager) AllTools(ctx context.Context, registry *tool.Registry) ([]tool.Tool, error) {
	m.mu.RLock()
	snapshot := make(map[string]*Client, len(m.clients))
	for name, c := range m.clients {
		snapshot[name] = c
	}
	m.mu.RUnlock()

	allTools := make([]tool.Tool, 0)

	for serverName, client := range snapshot {
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			continue
		}

		wrapper := NewWrapper(client, serverName, nil)
		wrapped, err := wrapper.WrapTools(mcpTools)
		if err != nil {
			continue
		}

		allTools = append(allTools, wrapped...)
	}

	return allTools, nil
}
