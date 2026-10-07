package mcp

import (
	"context"

	internalmcp "github.com/KPO-Tech/seshat/internal/tools/system/mcp"
)

type (
	// ConfigScope says where an MCP server is defined: in the project, in the user's configuration, locally, or by the enterprise.
	ConfigScope = internalmcp.ConfigScope
	// ConnectServerConfig is the config for connecting to an MCP server
	ConnectServerConfig = internalmcp.ConnectServerConfig
	// IntegrationOptions controls how MCP servers are wrapped into the tool registry.
	IntegrationOptions = internalmcp.IntegrationOptions
	// IntegrationResult represents the result of MCP integration
	IntegrationResult = internalmcp.IntegrationResult
	// Manager manages MCP server connections
	Manager = internalmcp.MCPClientManager
	// McpJsonConfig is the content of an MCP configuration file: the servers under the "mcpServers" key, by name.
	McpJsonConfig = internalmcp.McpJsonConfig
	// McpServerConfig describes one MCP server: its transport Type, the Command, Args and Env that start it (stdio) or the URL and Headers that reach it (http, sse, ws), a Timeout and the Scope it was defined in.
	McpServerConfig = internalmcp.McpServerConfig
	// McpServerType is the transport of an MCP server: stdio, http, sse, ws or sdk.
	McpServerType = internalmcp.McpServerType
	// ScopedMcpServerConfig is a server configuration with the scope it was found in.
	ScopedMcpServerConfig = internalmcp.ScopedMcpServerConfig
	// ServerConfig represents configuration for an MCP server
	ServerConfig = internalmcp.ServerConfig
	// TransportType represents the type of MCP transport
	TransportType = internalmcp.TransportType
	// ValidationError is a problem found in an MCP configuration: where (File, Path), what (Message), how to fix it (Suggestion) and how serious it is (Severity).
	ValidationError = internalmcp.ValidationError
)

const (
	ScopeProject    = internalmcp.ScopeProject
	ScopeUser       = internalmcp.ScopeUser
	ScopeLocal      = internalmcp.ScopeLocal
	ScopeEnterprise = internalmcp.ScopeEnterprise

	ServerTypeStdio     = internalmcp.ServerTypeStdio
	ServerTypeHTTP      = internalmcp.ServerTypeHTTP
	ServerTypeSSE       = internalmcp.ServerTypeSSE
	ServerTypeWebSocket = internalmcp.ServerTypeWebSocket
	ServerTypeSDK       = internalmcp.ServerTypeSDK

	// TransportType* values are for ServerConfig.Transport specifically
	// (the low-level Client's transport selector) - a separate type from
	// ServerType* above (McpServerType, used by the higher-level DB-facing
	// McpServerConfig), even though the string values overlap.
	TransportTypeStdio     = internalmcp.TransportTypeStdio
	TransportTypeHTTP      = internalmcp.TransportTypeHTTP
	TransportTypeSSE       = internalmcp.TransportTypeSSE
	TransportTypeWebSocket = internalmcp.TransportTypeWebSocket
)

// GlobalManager returns the global MCP client manager
func GlobalManager() *Manager {
	return internalmcp.GlobalMCPManager()
}

// Client is a single MCP server connection, for callers that need a direct,
// deterministic tool call (e.g. connector.ActionConnector.Act) rather than
// registering a server's tools into an agent's tool registry for the LLM to
// decide when to call. Sequence: NewClient, Start, Initialize, then
// CallTool/ListTools/etc.; Close when done - see MCPClientManager.Connect
// (internal/tools/system/mcp/tool_manager.go) for the same sequence used
// internally.
type Client = internalmcp.Client

// NewClient creates a new MCP client for the given server config. It does
// not connect - call Start then Initialize on the result before use.
func NewClient(config ServerConfig) (*Client, error) {
	return internalmcp.NewClient(config)
}

// AddServer registers an MCP server under name in the configuration file of the given scope. The name may only contain letters, digits, hyphens and underscores, "claude-in-chrome" is reserved, and the transport type is inferred from config when it is not set.
func AddServer(name string, config McpServerConfig, scope ConfigScope) error {
	return internalmcp.AddMcpServer(name, config, scope)
}

// ReconnectServer disconnects the named server from manager and connects it again with the configuration found for cwd. It returns an error when no server of that name is configured.
func ReconnectServer(ctx context.Context, manager *Manager, serverName string, cwd string) error {
	return internalmcp.ReconnectMcpServer(ctx, manager, serverName, cwd)
}

// ParseMcpConfigFromFile reads an MCP configuration file. A missing file gives an empty configuration and no error; a file that cannot be read or parsed gives an empty configuration and validation errors of severity "fatal".
func ParseMcpConfigFromFile(filePath string) (McpJsonConfig, []ValidationError) {
	return internalmcp.ParseMcpConfigFromFile(filePath)
}
