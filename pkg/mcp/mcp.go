package mcp

import (
	"context"

	internalmcp "github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
)

type (
	ConfigScope           = internalmcp.ConfigScope
	ConnectServerConfig   = internalmcp.ConnectServerConfig
	IntegrationOptions    = internalmcp.IntegrationOptions
	IntegrationResult     = internalmcp.IntegrationResult
	Manager               = internalmcp.MCPClientManager
	McpJsonConfig         = internalmcp.McpJsonConfig
	McpServerConfig       = internalmcp.McpServerConfig
	McpServerType         = internalmcp.McpServerType
	ScopedMcpServerConfig = internalmcp.ScopedMcpServerConfig
	ServerConfig          = internalmcp.ServerConfig
	TransportType         = internalmcp.TransportType
	ValidationError       = internalmcp.ValidationError
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
)

func GlobalManager() *Manager {
	return internalmcp.GlobalMCPManager()
}

func AddServer(name string, config McpServerConfig, scope ConfigScope) error {
	return internalmcp.AddMcpServer(name, config, scope)
}

func ReconnectServer(ctx context.Context, manager *Manager, serverName string, cwd string) error {
	return internalmcp.ReconnectMcpServer(ctx, manager, serverName, cwd)
}

func ParseMcpConfigFromFile(filePath string) (McpJsonConfig, []ValidationError) {
	return internalmcp.ParseMcpConfigFromFile(filePath)
}
