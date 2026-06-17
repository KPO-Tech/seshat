package lspServerManager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tools/special/lsp/lspClient"
)

// ServerConfig represents LSP server configuration
type ServerConfig struct {
	// Command is the command to start the LSP server
	Command string

	// Args are the command-line arguments
	Args []string

	// Env is the environment variables
	Env []string

	// RootPatterns are the root marker patterns
	RootPatterns []string

	// ExtensionToLanguage maps file extensions to language IDs
	ExtensionToLanguage map[string]string

	// Languages is the list of languages this server supports
	Languages []string

	// Timeout is the initialization timeout
	Timeout time.Duration
}

// DefaultServerConfigs returns default LSP server configurations
func DefaultServerConfigs() map[string]ServerConfig {
	return map[string]ServerConfig{
		"gopls": {
			Command: "gopls",
			Args:    []string{"serve"},
			RootPatterns: []string{
				"go.mod",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".go": "go",
			},
			Languages: []string{"go"},
			Timeout:   60 * time.Second,
		},
		"rust-analyzer": {
			Command: "rust-analyzer",
			Args:    []string{},
			RootPatterns: []string{
				"Cargo.toml",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".rs": "rust",
			},
			Languages: []string{"rust"},
			Timeout:   60 * time.Second,
		},
		"pyright": {
			Command: "pyright-langserver",
			Args:    []string{"--stdio"},
			RootPatterns: []string{
				"pyproject.toml",
				"setup.py",
				"requirements.txt",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".py": "python",
			},
			Languages: []string{"python"},
			Timeout:   60 * time.Second,
		},
		"typescript-language-server": {
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			RootPatterns: []string{
				"package.json",
				"tsconfig.json",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".ts":  "typescript",
				".tsx": "typescript",
				".js":  "javascript",
				".jsx": "javascript",
			},
			Languages: []string{"typescript", "javascript"},
			Timeout:   60 * time.Second,
		},
		"clangd": {
			Command: "clangd",
			Args:    []string{},
			RootPatterns: []string{
				"compile_commands.json",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".c":   "c",
				".cpp": "cpp",
				".cc":  "cpp",
				".cxx": "cpp",
				".h":   "c",
				".hpp": "cpp",
			},
			Languages: []string{"c", "cpp"},
			Timeout:   60 * time.Second,
		},
		"jedi-language-server": {
			Command: "jedi-language-server",
			Args:    []string{},
			RootPatterns: []string{
				"pyproject.toml",
				"setup.py",
				".git",
			},
			ExtensionToLanguage: map[string]string{
				".py": "python",
			},
			Languages: []string{"python"},
			Timeout:   60 * time.Second,
		},
		"tailwindcss-language-server": {
			Command: "tailwindcss-language-server",
			Args:    []string{"--stdio"},
			RootPatterns: []string{
				"tailwind.config.js",
				"tailwind.config.ts",
				"package.json",
			},
			ExtensionToLanguage: map[string]string{
				".css":  "css",
				".html": "html",
			},
			Languages: []string{"css", "html"},
			Timeout:   30 * time.Second,
		},
	}
}

// ServerInstance represents a running LSP server instance
type ServerInstance struct {
	Name   string
	Config ServerConfig
	Client *lspClient.Client
	State  ServerState

	mu      sync.RWMutex
	stdin   io.Writer
	stdout  io.Reader
	process *os.Process
	ctx     context.Context
	cancel  context.CancelFunc
}

// ServerState represents the state of a server
type ServerState string

const (
	ServerStateStopped  ServerState = "stopped"
	ServerStateStarting ServerState = "starting"
	ServerStateRunning  ServerState = "running"
	ServerStateError    ServerState = "error"
	ServerStateStopping ServerState = "stopping"
)

// Manager manages multiple LSP server instances
type Manager struct {
	mu           sync.RWMutex
	servers      map[string]*ServerInstance
	configs      map[string]ServerConfig
	extensionMap map[string]string
	openedFiles  map[string]string // URI -> server name
	workingDir   string
}

// NewManager creates a new LSP server manager
func NewManager(workingDir string) *Manager {
	m := &Manager{
		servers:      make(map[string]*ServerInstance),
		configs:      DefaultServerConfigs(),
		extensionMap: make(map[string]string),
		openedFiles:  make(map[string]string),
		workingDir:   workingDir,
	}
	m.rebuildExtensionMap()
	return m
}

// SetConfigs sets custom server configurations
func (m *Manager) SetConfigs(configs map[string]ServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs = configs
	m.rebuildExtensionMap()
}

// rebuildExtensionMap rebuilds the extension to server mapping
func (m *Manager) rebuildExtensionMap() {
	m.extensionMap = make(map[string]string)
	for name, config := range m.configs {
		for ext := range config.ExtensionToLanguage {
			m.extensionMap[ext] = name
		}
	}
}

// Initialize initializes all configured LSP servers
func (m *Manager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build extension map first
	m.rebuildExtensionMap()

	// Initialize each server
	for name, config := range m.configs {
		server, err := m.createServer(ctx, name, config)
		if err != nil {
			continue // Continue with other servers
		}
		m.servers[name] = server
	}

	return nil
}

// InitializeForFile initializes the appropriate LSP server for a file
func (m *Manager) InitializeForFile(ctx context.Context, filePath string) (*ServerInstance, error) {
	ext := filepath.Ext(filePath)

	m.mu.RLock()
	serverName := m.extensionMap[ext]
	m.mu.RUnlock()

	if serverName == "" {
		return nil, fmt.Errorf("no LSP server for file type: %s", ext)
	}

	return m.InitializeServer(ctx, serverName)
}

// InitializeServer initializes a specific LSP server
func (m *Manager) InitializeServer(ctx context.Context, serverName string) (*ServerInstance, error) {
	m.mu.RLock()
	server, exists := m.servers[serverName]
	m.mu.RUnlock()

	if exists && server.State == ServerStateRunning {
		return server, nil
	}

	if !exists {
		config, ok := func() (ServerConfig, bool) {
			m.mu.RLock()
			defer m.mu.RUnlock()
			c, ok := m.configs[serverName]
			return c, ok
		}()

		if !ok {
			return nil, fmt.Errorf("unknown server: %s", serverName)
		}

		var err error
		server, err = m.createServer(ctx, serverName, config)
		if err != nil {
			return nil, err
		}

		m.mu.Lock()
		m.servers[serverName] = server
		m.mu.Unlock()
	}

	// Start the server if not running
	if server.State != ServerStateRunning {
		if err := m.startServer(ctx, server); err != nil {
			return nil, err
		}
	}

	return server, nil
}

// createServer creates a new server instance
func (m *Manager) createServer(ctx context.Context, name string, config ServerConfig) (*ServerInstance, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &ServerInstance{
		Name:   name,
		Config: config,
		Client: lspClient.NewClient(name),
		State:  ServerStateStopped,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// startServer starts an LSP server
func (m *Manager) startServer(ctx context.Context, server *ServerInstance) error {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.State == ServerStateRunning {
		return nil
	}

	server.State = ServerStateStarting

	// Find root directory
	rootDir := m.findRootDir(server.Config.RootPatterns)
	if rootDir == "" {
		rootDir = m.workingDir
	}

	// Create command
	cmd := exec.CommandContext(server.ctx, server.Config.Command, server.Config.Args...)
	cmd.Dir = rootDir

	// Set environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, server.Config.Env...)

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		server.State = ServerStateError
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		server.State = ServerStateError
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		server.State = ServerStateError
		return fmt.Errorf("failed to start server: %w", err)
	}

	server.process = cmd.Process
	server.stdin = stdin
	server.stdout = stdout
	server.State = ServerStateRunning

	// Connect the client
	if err := server.Client.ConnectProcess(stdin, stdout); err != nil {
		server.State = ServerStateError
		return fmt.Errorf("failed to connect client: %w", err)
	}

	// Initialize the server
	timeout := server.Config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	initCtx, cancel := context.WithTimeout(server.ctx, timeout)
	defer cancel()

	_, err = server.Client.Initialize(initCtx, rootDir)
	if err != nil {
		server.State = ServerStateError
		return fmt.Errorf("failed to initialize server: %w", err)
	}

	return nil
}

// findRootDir finds the root directory based on root patterns
func (m *Manager) findRootDir(patterns []string) string {
	dir := m.workingDir
	for dir != "/" && dir != "" {
		for _, pattern := range patterns {
			if _, err := os.Stat(filepath.Join(dir, pattern)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return m.workingDir
}

// SendRequest sends a request to the appropriate LSP server for a file
func (m *Manager) SendRequest(ctx context.Context, filePath string, method string, params interface{}) (interface{}, error) {
	server, err := m.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	server.mu.RLock()
	client := server.Client
	server.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	var result json.RawMessage
	err = client.Request(ctx, method, params, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// OpenFile opens a file in the appropriate LSP server
func (m *Manager) OpenFile(ctx context.Context, filePath string) error {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Check file size (10MB limit like OpenClaude)
	if len(content) > 10_000_000 {
		return fmt.Errorf("file too large for LSP analysis (%dMB exceeds 10MB limit)", len(content)/1_000_000)
	}

	// Get the server
	server, err := m.InitializeForFile(ctx, filePath)
	if err != nil {
		return err
	}

	server.mu.RLock()
	client := server.Client
	server.mu.RUnlock()

	uri := lspClient.URIFromPath(filePath)
	languageID := m.getLanguageForFile(filePath)

	err = client.TextDocumentOpen(ctx, uri, string(content), languageID)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	// Track opened file
	m.mu.Lock()
	m.openedFiles[uri] = server.Name
	m.mu.Unlock()

	return nil
}

// CloseFile closes a file in the LSP server
func (m *Manager) CloseFile(ctx context.Context, filePath string) error {
	server, err := m.InitializeForFile(ctx, filePath)
	if err != nil {
		return err
	}

	server.mu.RLock()
	client := server.Client
	server.mu.RUnlock()

	uri := lspClient.URIFromPath(filePath)
	err = client.TextDocumentClose(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Remove from opened files
	m.mu.Lock()
	delete(m.openedFiles, uri)
	m.mu.Unlock()

	return nil
}

// IsFileOpen checks if a file is open in any LSP server
func (m *Manager) IsFileOpen(filePath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uri := lspClient.URIFromPath(filePath)
	_, ok := m.openedFiles[uri]
	return ok
}

// GetServerForFile returns the server for a given file
func (m *Manager) GetServerForFile(filePath string) *ServerInstance {
	ext := filepath.Ext(filePath)

	m.mu.RLock()
	serverName := m.extensionMap[ext]
	m.mu.RUnlock()

	if serverName == "" {
		return nil
	}

	m.mu.RLock()
	server := m.servers[serverName]
	m.mu.RUnlock()

	return server
}

// Shutdown shuts down all LSP servers
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		if server.State == ServerStateRunning {
			server.State = ServerStateStopping
			server.cancel() // Cancel the context

			// Try to gracefully shutdown
			if server.Client != nil {
				shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				_ = server.Client.Shutdown(shutdownCtx)
				server.Client.Close()
			}

			// Kill the process if still running
			if server.process != nil {
				_ = server.process.Kill()
			}

			server.State = ServerStateStopped
		}
		delete(m.servers, name)
	}

	m.openedFiles = make(map[string]string)

	return nil
}

// GetAllServers returns all running servers
func (m *Manager) GetAllServers() map[string]*ServerInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ServerInstance, len(m.servers))
	for k, v := range m.servers {
		result[k] = v
	}
	return result
}

// getLanguageForFile returns the language ID for a file
func (m *Manager) getLanguageForFile(filePath string) string {
	ext := filepath.Ext(filePath)

	m.mu.RLock()
	lang, ok := func() (string, bool) {
		configs := m.configs
		for _, config := range configs {
			if lang, exists := config.ExtensionToLanguage[ext]; exists {
				return lang, true
			}
		}
		return "", false
	}()
	m.mu.RUnlock()

	if ok {
		return lang
	}

	// Default mappings
	switch ext {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"
	default:
		return "plaintext"
	}
}

// GetAvailableServers returns a list of available server names
func (m *Manager) GetAvailableServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	servers := make([]string, 0, len(m.servers))
	for name := range m.servers {
		servers = append(servers, name)
	}
	return servers
}

// GetStatus returns the status of all servers
func (m *Manager) GetStatus() map[string]ServerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]ServerState)
	for name, server := range m.servers {
		server.mu.RLock()
		status[name] = server.State
		server.mu.RUnlock()
	}
	return status
}
