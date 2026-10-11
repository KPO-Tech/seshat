package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These guard M4 in TODO.md: ExpandEnvVars existed but LoadMcpConfigs never
// called it, so a server config using the documented ${VAR} syntax reached
// the server process literally, unexpanded, instead of substituted.

func TestLoadMcpConfigs_ExpandsEnvVarsInLocalConfig(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	t.Setenv("MCP_TEST_TOKEN", "secret-value")

	cwd := t.TempDir()
	config := McpJsonConfig{
		MCPServers: map[string]McpServerConfig{
			"test-server": {
				Type:    ServerTypeStdio,
				Command: "npx",
				Args:    []string{"-y", "some-server", "--token=${MCP_TEST_TOKEN}"},
			},
		},
	}
	writeMcpJSON(t, GetProjectConfigPath(cwd), config)

	result := LoadMcpConfigs(cwd)

	server, ok := result.Servers["test-server"]
	if !ok {
		t.Fatalf("expected test-server to be loaded, got %#v", result.Servers)
	}
	want := "--token=secret-value"
	if server.Args[len(server.Args)-1] != want {
		t.Errorf("expected the ${VAR} reference to be expanded to %q, got %q", want, server.Args[len(server.Args)-1])
	}
}

func TestLoadMcpConfigs_WarnsOnMissingEnvVar(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	cwd := t.TempDir()
	config := McpJsonConfig{
		MCPServers: map[string]McpServerConfig{
			"test-server": {
				Type:    ServerTypeStdio,
				Command: "npx",
				Args:    []string{"--token=${MCP_TEST_TOKEN_DEFINITELY_NOT_SET}"},
			},
		},
	}
	writeMcpJSON(t, GetProjectConfigPath(cwd), config)

	result := LoadMcpConfigs(cwd)

	// The server still loads - a missing env var is a warning, not fatal.
	if _, ok := result.Servers["test-server"]; !ok {
		t.Fatalf("expected test-server to still load despite the missing env var, got %#v", result.Servers)
	}

	found := false
	for _, e := range result.Errors {
		if e.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning-severity validation error for the missing env var, got %#v", result.Errors)
	}
}

func writeMcpJSON(t *testing.T, path string, config McpJsonConfig) {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
