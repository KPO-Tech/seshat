package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/db"
	engineconfig "github.com/KPO-Tech/seshat-runtime/pkg/config"
)

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_GEMINI_API_KEY", "ZHIPUAI_API_KEY", "Z_AI_API_KEY",
		"OPENROUTER_API_KEY", "MINIMAX_API_KEY", "DEEPSEEK_API_KEY", "MISTRAL_API_KEY", "MOONSHOT_API_KEY", "KIMI_API_KEY", "OPENCODE_API_KEY", "CODEX_API_KEY"} {
		t.Setenv(name, "")
	}
}

func credentialsDB(t *testing.T, creds map[string]string) *db.DB {
	t.Helper()
	database, err := db.Open(context.Background(), db.DefaultSQLiteConfig(filepath.Join(t.TempDir(), "seshat.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for key, value := range creds {
		if err := database.UpsertCredential(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func TestStartupModelFromAStoredProviderKey(t *testing.T) {
	clearProviderEnv(t)
	config := engineconfig.Config{}
	loadCredsIntoConfig(credentialsDB(t, map[string]string{"api_key:z-ai": "zk"}), &config)
	if config.Model != "z-ai:glm-5.3" {
		t.Fatalf("model = %q, want the default of the provider whose key is stored (z-ai:glm-5.3)", config.Model)
	}
	if config.APIKey != "zk" {
		t.Fatalf("the stored key must be loaded for that provider, got %q", config.APIKey)
	}
}

func TestStartupModelKeepsARememberedChoiceAndTheEnvironment(t *testing.T) {
	clearProviderEnv(t)
	config := engineconfig.Config{}
	loadCredsIntoConfig(credentialsDB(t, map[string]string{"api_key:z-ai": "zk", "api_key:openai": "ok", "model": "openai:gpt-6.1-sol"}), &config)
	if config.Model != "openai:gpt-6.1-sol" {
		t.Fatalf("a remembered model must win, got %q", config.Model)
	}

	t.Setenv("OPENAI_API_KEY", "env-key")
	config = engineconfig.Config{}
	loadCredsIntoConfig(credentialsDB(t, map[string]string{"api_key:z-ai": "zk"}), &config)
	if config.Model != "" {
		t.Fatalf("a provider named by the environment must not be overridden by a stored key, got %q", config.Model)
	}
}

func TestStartupModelStaysEmptyWithNothingConfigured(t *testing.T) {
	clearProviderEnv(t)
	config := engineconfig.Config{}
	loadCredsIntoConfig(credentialsDB(t, nil), &config)
	if config.Model != "" {
		t.Fatalf("model = %q, want none", config.Model)
	}
}
