package config

import (
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	"os"
	"path/filepath"
	"testing"
)

func TestParseModelIdentifierKeepsColonModelIDs(t *testing.T) {
	model := ParseModelIdentifier("qwen2.5-coder:7b")
	if model.Provider != sdk.APIProviderAnthropic {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "qwen2.5-coder:7b" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

func TestHasExplicitProviderPrefix(t *testing.T) {
	if !HasExplicitProviderPrefix("openai:gpt-5.5") {
		t.Fatalf("expected explicit provider prefix")
	}
	if !HasExplicitProviderPrefix("deepseek:deepseek-chat") {
		t.Fatalf("expected explicit deepseek provider prefix")
	}
	if HasExplicitProviderPrefix("qwen2.5-coder:7b") {
		t.Fatalf("unexpected provider prefix for raw model id")
	}
}

func TestParseModelIdentifierRecognizesOpenAICompatibleAdditions(t *testing.T) {
	model := ParseModelIdentifier("deepseek:deepseek-chat")
	if model.Provider != sdk.APIProviderDeepSeek {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "deepseek-chat" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

func TestResolveAPIKeyUsesProviderSpecificEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")

	key := ResolveAPIKey(Config{APIKey: "fallback"}, sdk.APIProviderOpenAI)
	if key != "openai-key" {
		t.Fatalf("unexpected key: got %q", key)
	}
}

func TestApplyRuntimeEnvFromConfig(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_VERTEX_PROJECT_ID")
	_ = os.Unsetenv("CLOUD_ML_REGION")
	_ = os.Unsetenv("NEXUS_RUNTIME_ROOT")

	cfg := Config{
		RuntimeRoot:       "/tmp/nexus-runtime",
		Model:             "vertex:claude-3-5-sonnet@20241022",
		ProviderProjectID: "project-123",
		ProviderRegion:    "us-east5",
	}
	ApplyRuntimeEnv(cfg)

	if got := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID"); got != "project-123" {
		t.Fatalf("unexpected project id env: got %q", got)
	}
	if got := os.Getenv("CLOUD_ML_REGION"); got != "us-east5" {
		t.Fatalf("unexpected region env: got %q", got)
	}
	if got := os.Getenv("NEXUS_RUNTIME_ROOT"); got != "/tmp/nexus-runtime" {
		t.Fatalf("unexpected runtime root env: got %q", got)
	}
}

func TestSaveAtWritesLoadableConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".nexus.yaml")

	cfg := Config{
		Model:             "openai:gpt-5.5",
		APIKey:            "secret",
		ProviderBaseURL:   "https://example.invalid",
		ProviderRegion:    "us-east-1",
		ProviderProjectID: "demo-project",
		ProviderResource:  "demo-resource",
	}

	if err := SaveAt(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("expected config file content")
	}
}

func TestLoadIntoBindsBrowserRuntimeEnv(t *testing.T) {
	t.Setenv("NEXUS_BROWSER_REMOTE_CONTROL_URL", "ws://127.0.0.1:9222/devtools/browser/test")
	t.Setenv("NEXUS_BROWSER_EXECUTABLE_PATH", "/usr/bin/chromium")

	var cfg Config
	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.BrowserRemoteControlURL != "ws://127.0.0.1:9222/devtools/browser/test" {
		t.Fatalf("unexpected browser remote url: %q", cfg.BrowserRemoteControlURL)
	}
	if cfg.BrowserExecutablePath != "/usr/bin/chromium" {
		t.Fatalf("unexpected browser executable path: %q", cfg.BrowserExecutablePath)
	}
}

func TestLoadIntoBindsStorageGCEnv(t *testing.T) {
	t.Setenv("NEXUS_STORAGE_GC_ENABLED", "true")
	t.Setenv("NEXUS_STORAGE_GC_INTERVAL", "30m")
	t.Setenv("NEXUS_STORAGE_GC_LIMIT", "128")
	t.Setenv("NEXUS_STORAGE_GC_NAMESPACES", "artifacts/web,artifacts/browser/downloads")

	var cfg Config
	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.StorageGCEnabled {
		t.Fatal("expected storage GC enabled")
	}
	if cfg.StorageGCInterval != "30m" {
		t.Fatalf("unexpected storage gc interval: %q", cfg.StorageGCInterval)
	}
	if cfg.StorageGCLimit != 128 {
		t.Fatalf("unexpected storage gc limit: %d", cfg.StorageGCLimit)
	}
	if cfg.StorageGCNamespaces != "artifacts/web,artifacts/browser/downloads" {
		t.Fatalf("unexpected storage gc namespaces: %q", cfg.StorageGCNamespaces)
	}
}

func TestEffectiveSessionDBPathFallsBackToDBPath(t *testing.T) {
	cfg := Config{DBPath: "/tmp/backend.sqlite"}
	if got := EffectiveSessionDBPath(cfg); got != "/tmp/backend.sqlite" {
		t.Fatalf("unexpected session db path: %q", got)
	}
}

func TestEffectiveRuntimePathsUseUnifiedRoot(t *testing.T) {
	cfg := Config{RuntimeRoot: "/tmp/nexus-runtime"}
	if got := EffectiveRuntimeRoot(cfg); got != "/tmp/nexus-runtime" {
		t.Fatalf("unexpected runtime root: %q", got)
	}
	if got := EffectiveDBPath(cfg); got != "/tmp/nexus-runtime/nexus.db" {
		t.Fatalf("unexpected db path: %q", got)
	}
	if got := EffectiveStorageLocalPath(cfg); got != "/tmp/nexus-runtime/storage" {
		t.Fatalf("unexpected storage path: %q", got)
	}
}

func TestEffectiveSessionDBPathPrefersExplicitSessionPath(t *testing.T) {
	cfg := Config{
		DBPath:        "/tmp/backend.sqlite",
		SessionDBPath: "/tmp/runtime.sqlite",
	}
	if got := EffectiveSessionDBPath(cfg); got != "/tmp/runtime.sqlite" {
		t.Fatalf("unexpected session db path: %q", got)
	}
}

func TestLoadIntoBindsBackendAndSessionDBEnv(t *testing.T) {
	t.Setenv("NEXUS_DB_DRIVER", "postgres")
	t.Setenv("NEXUS_DB_DSN", "postgres://user:pass@localhost:5432/nexus")
	t.Setenv("NEXUS_DB_AUTO_MIGRATE", "false")
	t.Setenv("NEXUS_SESSION_DB_PATH", "/tmp/runtime.sqlite")

	var cfg Config
	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.DBDriver != "postgres" {
		t.Fatalf("unexpected db driver: %q", cfg.DBDriver)
	}
	if cfg.DBDSN != "postgres://user:pass@localhost:5432/nexus" {
		t.Fatalf("unexpected db dsn: %q", cfg.DBDSN)
	}
	if cfg.DBAutoMigrate {
		t.Fatal("expected db_auto_migrate=false from env")
	}
	if cfg.SessionDBPath != "/tmp/runtime.sqlite" {
		t.Fatalf("unexpected session db path: %q", cfg.SessionDBPath)
	}
}

func TestLoadIntoBindsPgVectorEnv(t *testing.T) {
	t.Setenv("NEXUS_PGVECTOR_CREATE_EXTENSION", "false")
	t.Setenv("NEXUS_PGVECTOR_DSN", "postgres://vector:test@localhost:5432/vectors")
	t.Setenv("NEXUS_PGVECTOR_INDEX_METHOD", "ivfflat")
	t.Setenv("NEXUS_PGVECTOR_HNSW_M", "24")
	t.Setenv("NEXUS_PGVECTOR_HNSW_EF_CONSTRUCTION", "96")
	t.Setenv("NEXUS_PGVECTOR_IVFFLAT_LISTS", "256")

	var cfg Config
	if err := LoadInto(&cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.PgVectorCreateExtension {
		t.Fatal("expected pgvector_create_extension=false from env")
	}
	if cfg.PgVectorIndexMethod != "ivfflat" {
		t.Fatalf("unexpected pgvector index method: %q", cfg.PgVectorIndexMethod)
	}
	if cfg.PgVectorDSN != "postgres://vector:test@localhost:5432/vectors" {
		t.Fatalf("unexpected pgvector dsn: %q", cfg.PgVectorDSN)
	}
	if cfg.PgVectorHNSWM != 24 {
		t.Fatalf("unexpected pgvector hnsw m: %d", cfg.PgVectorHNSWM)
	}
	if cfg.PgVectorHNSWEF != 96 {
		t.Fatalf("unexpected pgvector hnsw ef construction: %d", cfg.PgVectorHNSWEF)
	}
	if cfg.PgVectorIVFFlatLists != 256 {
		t.Fatalf("unexpected pgvector ivfflat lists: %d", cfg.PgVectorIVFFlatLists)
	}
}

func TestAvailableProvidersIncludesOpenAI(t *testing.T) {
	providers := AvailableProviders()
	if len(providers) == 0 {
		t.Fatalf("expected providers")
	}

	found := false
	for _, provider := range providers {
		if provider.Name == sdk.APIProviderOpenAI {
			found = true
			if len(provider.Models) == 0 {
				t.Fatalf("expected openai models")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected openai provider")
	}
}

func TestAvailableProvidersIncludesOpenAICompatibleAdditions(t *testing.T) {
	for _, provider := range []sdk.APIProvider{
		sdk.APIProviderCodex,
		sdk.APIProviderDeepSeek,
		sdk.APIProviderOpenCode,
		sdk.APIProviderMistral,
	} {
		info, ok := GetProviderInfo(provider)
		if !ok {
			t.Fatalf("expected provider info for %q", provider)
		}
		if len(info.Models) == 0 {
			t.Fatalf("expected models for %q", provider)
		}
	}
}

func TestProviderForModelUsesCatalog(t *testing.T) {
	if provider := ProviderForModel("gpt-5.5"); provider != sdk.APIProviderOpenAI {
		t.Fatalf("unexpected provider: got %q", provider)
	}
	if provider := ProviderForModel("@cf/meta/llama-3.1-70b-instruct"); provider != sdk.APIProviderWorkersAI {
		t.Fatalf("unexpected provider: got %q", provider)
	}
	if provider := ProviderForModel("deepseek-chat"); provider != sdk.APIProviderDeepSeek {
		t.Fatalf("unexpected provider: got %q", provider)
	}
	if provider := ProviderForModel("claude-sonnet-4"); provider != sdk.APIProviderOpenCode {
		t.Fatalf("unexpected provider: got %q", provider)
	}
}

func TestResolveProviderOpenAICompatibleAdditions(t *testing.T) {
	cases := map[string]sdk.APIProvider{
		"codex":        sdk.APIProviderCodex,
		"deepseek":     sdk.APIProviderDeepSeek,
		"deep-seek":    sdk.APIProviderDeepSeek,
		"opencode":     sdk.APIProviderOpenCode,
		"opencode-zen": sdk.APIProviderOpenCode,
		"mistral":      sdk.APIProviderMistral,
		"mistral-ai":   sdk.APIProviderMistral,
	}
	for raw, want := range cases {
		if got := ResolveProvider(raw); got != want {
			t.Fatalf("ResolveProvider(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestProviderCredentialEnvVarsOpenAICompatibleAdditions(t *testing.T) {
	cases := map[sdk.APIProvider]string{
		sdk.APIProviderCodex:    "CODEX_API_KEY",
		sdk.APIProviderDeepSeek: "DEEPSEEK_API_KEY",
		sdk.APIProviderOpenCode: "OPENCODE_API_KEY",
		sdk.APIProviderMistral:  "MISTRAL_API_KEY",
	}
	for provider, want := range cases {
		vars := ProviderCredentialEnvVars(provider)
		if len(vars) == 0 || vars[0] != want {
			t.Fatalf("ProviderCredentialEnvVars(%q) = %v, want first %q", provider, vars, want)
		}
	}
}

func TestValidateProviderSetup(t *testing.T) {
	err := ValidateProviderSetup(Config{}, sdk.APIProviderOpenAI)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	err = ValidateProviderSetup(Config{APIKey: "secret"}, sdk.APIProviderOpenAI)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
