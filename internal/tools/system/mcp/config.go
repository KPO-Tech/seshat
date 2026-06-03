package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// validMCPServerNameRe validates MCP server names: letters, digits, hyphens, underscores only.
var validMCPServerNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type ConfigScope string

const (
	ScopeProject    ConfigScope = "project"
	ScopeUser       ConfigScope = "user"
	ScopeLocal      ConfigScope = "local"
	ScopeEnterprise ConfigScope = "enterprise"
)

type McpServerType string

const (
	ServerTypeStdio     McpServerType = "stdio"
	ServerTypeHTTP      McpServerType = "http"
	ServerTypeSSE       McpServerType = "sse"
	ServerTypeWebSocket McpServerType = "ws"
	ServerTypeSDK       McpServerType = "sdk"
)

type McpServerConfig struct {
	Type    McpServerType     `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
	Scope   ConfigScope       `json:"scope,omitempty"`
}

type McpJsonConfig struct {
	MCPServers map[string]McpServerConfig `json:"mcpServers,omitempty"`
}

type ScopedMcpServerConfig struct {
	McpServerConfig
	Scope ConfigScope
}

type ValidationError struct {
	File       string `json:"file,omitempty"`
	Path       string `json:"path,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	Severity   string `json:"severity"`
}

type ConfigLoadResult struct {
	Servers map[string]ScopedMcpServerConfig
	Errors  []ValidationError
}

func GetUserConfigDir() string {
	return runtimepath.ResolveRoot("")
}

func GetGlobalConfigPath() string {
	return filepath.Join(GetUserConfigDir(), "mcp.json")
}

func GetProjectConfigPath(cwd string) string {
	return filepath.Join(cwd, ".mcp.json")
}

func GetEnterpriseConfigPath() string {
	return filepath.Join(GetUserConfigDir(), "enterprise-mcp.json")
}

func DoesEnterpriseConfigExist() bool {
	_, err := os.Stat(GetEnterpriseConfigPath())
	return err == nil
}

func ParseMcpConfigFromFile(filePath string) (McpJsonConfig, []ValidationError) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return McpJsonConfig{}, []ValidationError{}
		}
		return McpJsonConfig{}, []ValidationError{{
			Message:  fmt.Sprintf("Failed to read file: %v", err),
			Severity: "fatal",
		}}
	}

	var config McpJsonConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return McpJsonConfig{}, []ValidationError{{
			File:     filePath,
			Message:  "Invalid JSON",
			Severity: "fatal",
		}}
	}

	errors := validateMcpConfig(&config)
	return config, errors
}

func validateMcpConfig(config *McpJsonConfig) []ValidationError {
	var errors []ValidationError

	if config.MCPServers == nil {
		return errors
	}

	for name, server := range config.MCPServers {
		if server.Type == ServerTypeStdio || server.Type == "" {
			if server.Command == "" {
				errors = append(errors, ValidationError{
					Path:     fmt.Sprintf("mcpServers.%s.command", name),
					Message:  "Command is required for stdio servers",
					Severity: "error",
				})
			}
		}

		if server.Type == ServerTypeHTTP || server.Type == ServerTypeSSE || server.Type == ServerTypeWebSocket {
			if server.URL == "" {
				errors = append(errors, ValidationError{
					Path:     fmt.Sprintf("mcpServers.%s.url", name),
					Message:  "URL is required for HTTP/SSE/WS servers",
					Severity: "error",
				})
			}
		}

		if server.Type == ServerTypeSDK {
			if name == "claude-in-chrome" {
				errors = append(errors, ValidationError{
					Path:     fmt.Sprintf("mcpServers.%s", name),
					Message:  "Cannot use reserved server name 'claude-in-chrome'",
					Severity: "error",
				})
			}
		}
	}

	return errors
}

func ExpandEnvVars(config McpServerConfig) (McpServerConfig, []string) {
	var missing []string

	expand := func(s string) string {
		result := os.ExpandEnv(s)
		if strings.Contains(result, "${") {
			parts := strings.Split(result, "${")
			for _, part := range parts[1:] {
				end := strings.Index(part, "}")
				if end > 0 {
					varName := part[:end]
					if os.Getenv(varName) == "" {
						missing = append(missing, varName)
					}
				}
			}
		}
		return result
	}

	config.Command = expand(config.Command)
	for i := range config.Args {
		config.Args[i] = expand(config.Args[i])
	}
	if config.URL != "" {
		config.URL = expand(config.URL)
	}
	for k, v := range config.Env {
		config.Env[k] = expand(v)
	}
	for k, v := range config.Headers {
		config.Headers[k] = expand(v)
	}

	return config, missing
}

func LoadMcpConfigs(cwd string) ConfigLoadResult {
	allServers := make(map[string]ScopedMcpServerConfig)
	var allErrors []ValidationError

	if DoesEnterpriseConfigExist() {
		enterpriseConfig, errors := ParseMcpConfigFromFile(GetEnterpriseConfigPath())
		for name, server := range enterpriseConfig.MCPServers {
			allServers[name] = ScopedMcpServerConfig{server, ScopeEnterprise}
		}
		allErrors = append(allErrors, errors...)
		return ConfigLoadResult{allServers, allErrors}
	}

	userConfig, userErrors := ParseMcpConfigFromFile(GetGlobalConfigPath())
	for name, server := range userConfig.MCPServers {
		server.Scope = ScopeUser
		allServers[name] = ScopedMcpServerConfig{server, ScopeUser}
	}
	allErrors = append(allErrors, userErrors...)

	projectConfigs := findProjectMcpConfigs(cwd)
	for _, configPath := range projectConfigs {
		projectConfig, projectErrors := ParseMcpConfigFromFile(configPath)
		for name, server := range projectConfig.MCPServers {
			server.Scope = ScopeProject
			allServers[name] = ScopedMcpServerConfig{server, ScopeProject}
		}
		allErrors = append(allErrors, projectErrors...)
	}

	localConfigPath := GetProjectConfigPath(cwd)
	localConfig, localErrors := ParseMcpConfigFromFile(localConfigPath)
	for name, server := range localConfig.MCPServers {
		server.Scope = ScopeLocal
		allServers[name] = ScopedMcpServerConfig{server, ScopeLocal}
	}
	allErrors = append(allErrors, localErrors...)

	return ConfigLoadResult{allServers, allErrors}
}

func findProjectMcpConfigs(cwd string) []string {
	var configs []string

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return configs
	}

	current := absCwd
	for {
		configPath := filepath.Join(current, ".mcp.json")
		if _, err := os.Stat(configPath); err == nil {
			configs = append(configs, configPath)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return configs
}

func AddMcpServer(name string, config McpServerConfig, scope ConfigScope) error {
	if !validMCPServerNameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q: can only contain letters, numbers, hyphens, and underscores", name)
	}

	if name == "claude-in-chrome" {
		return fmt.Errorf("cannot use reserved server name 'claude-in-chrome'")
	}

	config.Type = getServerType(&config)

	var configPath string
	switch scope {
	case ScopeProject, ScopeLocal:
		configPath = GetProjectConfigPath(".")
		config.Scope = ScopeProject
	case ScopeUser:
		configPath = GetGlobalConfigPath()
		config.Scope = ScopeUser
	default:
		return fmt.Errorf("unsupported scope: %s", scope)
	}

	existingConfig, _ := ParseMcpConfigFromFile(configPath)
	if existingConfig.MCPServers == nil {
		existingConfig.MCPServers = make(map[string]McpServerConfig)
	}

	if _, exists := existingConfig.MCPServers[name]; exists {
		return fmt.Errorf("server '%s' already exists", name)
	}

	existingConfig.MCPServers[name] = config

	data, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func RemoveMcpServer(name string, scope ConfigScope) error {
	var configPath string
	switch scope {
	case ScopeProject, ScopeLocal:
		configPath = GetProjectConfigPath(".")
	case ScopeUser:
		configPath = GetGlobalConfigPath()
	default:
		return fmt.Errorf("unsupported scope: %s", scope)
	}

	existingConfig, _ := ParseMcpConfigFromFile(configPath)
	if existingConfig.MCPServers == nil {
		return fmt.Errorf("no server '%s' found", name)
	}

	if _, exists := existingConfig.MCPServers[name]; !exists {
		return fmt.Errorf("no server '%s' found", name)
	}

	delete(existingConfig.MCPServers, name)

	data, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func getServerType(config *McpServerConfig) McpServerType {
	if config.Type != "" {
		return config.Type
	}
	if config.Command != "" {
		return ServerTypeStdio
	}
	if config.URL != "" {
		if strings.HasPrefix(config.URL, "ws://") || strings.HasPrefix(config.URL, "wss://") {
			return ServerTypeWebSocket
		}
		return ServerTypeHTTP
	}
	return ServerTypeStdio
}

func GetMcpServerConfigByName(name string, cwd string) *ScopedMcpServerConfig {
	configs := LoadMcpConfigs(cwd)
	if server, ok := configs.Servers[name]; ok {
		return &server
	}
	return nil
}

func IsMcpServerDisabled(name string, cwd string) bool {
	projectConfigPath := filepath.Join(cwd, ".nexus", "config.json")
	data, err := os.ReadFile(projectConfigPath)
	if err != nil {
		return false
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	disabled, ok := config["disabledMcpServers"].([]interface{})
	if !ok {
		return false
	}

	for _, s := range disabled {
		if s == name {
			return true
		}
	}
	return false
}

func SetMcpServerEnabled(name string, enabled bool, cwd string) error {
	projectConfigPath := filepath.Join(cwd, ".nexus", "config.json")

	var config map[string]interface{}
	data, err := os.ReadFile(projectConfigPath)
	if err == nil {
		json.Unmarshal(data, &config)
	} else {
		config = make(map[string]interface{})
	}

	var disabledList []interface{}
	if d, ok := config["disabledMcpServers"].([]interface{}); ok {
		disabledList = d
	} else {
		disabledList = []interface{}{}
	}

	found := false
	for i, s := range disabledList {
		if s == name {
			found = true
			if enabled {
				disabledList = append(disabledList[:i], disabledList[i+1:]...)
			}
			break
		}
	}

	if !found && !enabled {
		disabledList = append(disabledList, name)
	}

	config["disabledMcpServers"] = disabledList

	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(projectConfigPath)
	os.MkdirAll(dir, 0755)

	return os.WriteFile(projectConfigPath, data, 0644)
}

func ConnectMcpServers(ctx context.Context, manager *MCPClientManager, cwd string) error {
	result := LoadMcpConfigs(cwd)

	for _, err := range result.Errors {
		slog.Warn("MCP config warning", "message", err.Message)
	}

	for name, serverConfig := range result.Servers {
		if IsMcpServerDisabled(name, cwd) {
			slog.Debug("MCP: skipping disabled server", "server", name)
			continue
		}

		mcpConfig := ConnectServerConfig{
			Name:      name,
			Command:   serverConfig.Command,
			Args:      serverConfig.Args,
			URL:       serverConfig.URL,
			Transport: string(serverConfig.Type),
			Env:       serverConfig.Env,
			Timeout:   serverConfig.Timeout,
			Headers:   serverConfig.Headers,
		}

		if mcpConfig.Timeout == 0 {
			mcpConfig.Timeout = 30
		}

		if err := manager.Connect(ctx, mcpConfig); err != nil {
			slog.Warn("MCP: failed to connect server", "server", name, "err", err)
		} else {
			slog.Info("MCP: connected", "server", name)
		}
	}

	return nil
}

type MCPOAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	Scopes       []string
}

func GetOAuthConfig() *MCPOAuthConfig {
	return &MCPOAuthConfig{
		ClientID:     os.Getenv("NEXUS_MCP_CLIENT_ID"),
		ClientSecret: os.Getenv("NEXUS_MCP_CLIENT_SECRET"),
		AuthURL:      os.Getenv("NEXUS_MCP_AUTH_URL"),
		TokenURL:     os.Getenv("NEXUS_MCP_TOKEN_URL"),
		Scopes:       strings.Split(os.Getenv("NEXUS_MCP_SCOPES"), ","),
	}
}

func ShouldUseOAuth() bool {
	cfg := GetOAuthConfig()
	return cfg.ClientID != "" && cfg.AuthURL != ""
}

func (m *MCPClientManager) ConnectWithOAuth(ctx context.Context, config ConnectServerConfig) error {
	if !ShouldUseOAuth() {
		return m.Connect(ctx, config)
	}

	oauthCfg := GetOAuthConfig()
	slog.Info("MCP OAuth: authenticating", "server", config.Name, "client_id", oauthCfg.ClientID)

	return m.Connect(ctx, config)
}

type PluginMcpServer struct {
	Name   string
	Source string
	Config McpServerConfig
}

var pluginMcpServers []PluginMcpServer

func RegisterPluginMcpServer(name string, source string, config McpServerConfig) {
	pluginMcpServers = append(pluginMcpServers, PluginMcpServer{
		Name:   fmt.Sprintf("plugin:%s:%s", source, name),
		Source: source,
		Config: config,
	})
}

func GetPluginMcpServers() []PluginMcpServer {
	return pluginMcpServers
}

type MCPServerStatus struct {
	Name        string       `json:"name"`
	Status      ClientStatus `json:"status"`
	Scope       ConfigScope  `json:"scope"`
	Type        string       `json:"type"`
	LastError   string       `json:"last_error,omitempty"`
	ConnectedAt time.Time    `json:"connected_at,omitempty"`
	ToolCount   int          `json:"tool_count"`
}

func (m *MCPClientManager) GetServerStatuses() []MCPServerStatus {
	statuses := make([]MCPServerStatus, 0)

	for name, client := range m.clients {
		metadata := client.Metadata()
		toolCount := 0
		if tools, err := client.ListTools(context.Background()); err == nil {
			toolCount = len(tools)
		}

		statuses = append(statuses, MCPServerStatus{
			Name:      name,
			Status:    metadata.Status,
			Scope:     ScopeUser,
			Type:      "stdio",
			LastError: metadata.LastError,
			ToolCount: toolCount,
		})
	}

	return statuses
}

func ReconnectMcpServer(ctx context.Context, manager *MCPClientManager, serverName string, cwd string) error {
	config := GetMcpServerConfigByName(serverName, cwd)
	if config == nil {
		return fmt.Errorf("server '%s' not found", serverName)
	}

	manager.Disconnect(serverName)

	connectConfig := ConnectServerConfig{
		Name:      serverName,
		Command:   config.Command,
		Args:      config.Args,
		URL:       config.URL,
		Transport: string(config.Type),
		Env:       config.Env,
		Timeout:   config.Timeout,
		Headers:   config.Headers,
	}

	return manager.Connect(ctx, connectConfig)
}
