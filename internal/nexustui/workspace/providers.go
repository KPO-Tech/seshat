package workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/EngineerProjects/nexus-engine/internal/db"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

// SetStartupConfig stores the options needed to rebuild the SDK client when
// the user switches provider or model from within the TUI.
// Must be called before the Bubble Tea program starts.
func (w *NexusWorkspace) SetStartupConfig(sqlitePath string, permMode sdk.PermissionMode, monitoring *sdk.MonitoringSystem) {
	w.sqlitePath = sqlitePath
	w.permMode = permMode
	w.monitoring = monitoring
}

// DetectProviders probes env vars, the credentials DB, and Ollama to populate
// the workspace's provider key store. Run this in a background goroutine
// immediately after SetStartupConfig — it blocks on network I/O.
func (w *NexusWorkspace) DetectProviders() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Env vars (highest priority — already present when the TUI starts).
	for _, p := range engineconfig.AvailableProviders() {
		if _, already := w.providerKeys.Load(string(p.Name)); already {
			continue
		}
		for _, envVar := range engineconfig.ProviderCredentialEnvVars(p.Name) {
			if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
				w.providerKeys.Store(string(p.Name), v)
				break
			}
		}
	}

	// 2. Credentials DB (persisted by previous TUI sessions or `nexus config`).
	if engCfg, err := engineconfig.Load(); err == nil {
		dbPath := engineconfig.EffectiveSessionDBPath(engCfg)
		if database, err := db.Open(context.Background(), db.DefaultSQLiteConfig(dbPath)); err == nil {
			defer database.Close()
			dbCtx := context.Background()
			for _, p := range engineconfig.AvailableProviders() {
				pid := string(p.Name)
				if _, already := w.providerKeys.Load(pid); already {
					continue // env var wins
				}
				key := "api_key:" + strings.ToLower(pid)
				if val, ok, _ := database.GetCredential(dbCtx, key); ok && strings.TrimSpace(val) != "" {
					w.providerKeys.Store(pid, val)
				}
			}
		}
	}

	// 3. Ollama — no API key required; just check connectivity + fetch models.
	ollamaBase := strings.TrimRight(os.Getenv("OLLAMA_HOST"), "/")
	if ollamaBase == "" {
		ollamaBase = "http://localhost:11434"
	}
	if models := fetchOllamaModels(ctx, ollamaBase); len(models) > 0 {
		w.ollamaMu.Lock()
		w.ollamaModels = models
		w.ollamaMu.Unlock()
		// Mark as configured even without a key.
		w.providerKeys.Store("ollama", "")
	}

	// Invalidate the cached TUI config so Config() rebuilds with the new data.
	w.cfgMu.Lock()
	w.cfg = nil
	w.cfgMu.Unlock()
}

// resolveAPIKey returns the stored API key for providerID, falling back to env.
func (w *NexusWorkspace) resolveAPIKey(providerID string) string {
	if v, ok := w.providerKeys.Load(providerID); ok {
		if key, _ := v.(string); key != "" {
			return key
		}
	}
	for _, envVar := range engineconfig.ProviderCredentialEnvVars(engineconfig.ResolveProvider(providerID)) {
		if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
			return v
		}
	}
	return ""
}

// persistProviderAPIKey saves providerID's API key to the credentials DB.
// Best-effort: errors are logged but not propagated.
func (w *NexusWorkspace) persistProviderAPIKey(providerID, apiKey string) {
	engCfg, err := engineconfig.Load()
	if err != nil {
		return
	}
	database, err := db.Open(context.Background(), db.DefaultSQLiteConfig(engineconfig.EffectiveSessionDBPath(engCfg)))
	if err != nil {
		return
	}
	defer database.Close()
	key := "api_key:" + strings.ToLower(providerID)
	_ = database.UpsertCredential(context.Background(), key, apiKey)
}

// fetchOllamaModels queries Ollama's /api/tags endpoint and converts the
// response to a catwalk model list.
func fetchOllamaModels(ctx context.Context, baseURL string) []catwalk.Model {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}

	models := make([]catwalk.Model, 0, len(payload.Models))
	for _, m := range payload.Models {
		displayName := m.Name
		// Strip ":latest" suffix for cleaner display ("llama3.2" not "llama3.2:latest").
		if tag := strings.LastIndex(displayName, ":"); tag > 0 && displayName[tag+1:] == "latest" {
			displayName = displayName[:tag]
		}
		models = append(models, catwalk.Model{
			ID:            m.Name,
			Name:          displayName,
			ContextWindow: 128000,
		})
	}
	return models
}

// catwalkTypeFor maps a nexus-engine provider ID to the matching catwalk.Type
// used by the TUI's API key verification flow.
func catwalkTypeFor(providerID string) catwalk.Type {
	switch providerID {
	case "anthropic":
		return catwalk.TypeAnthropic
	case "gemini":
		return catwalk.TypeGoogle
	case "bedrock":
		return catwalk.TypeBedrock
	case "vertex":
		return "google-vertex"
	default:
		// openai, openrouter, deepseek, mistral, minimax, workers-ai, opencode, ollama
		return catwalk.TypeOpenAI
	}
}

// displayNameFor returns the human-readable display name for a provider ID.
func displayNameFor(providerID string) string {
	for _, p := range engineconfig.AvailableProviders() {
		if string(p.Name) == providerID {
			return p.DisplayName
		}
	}
	return providerID
}
