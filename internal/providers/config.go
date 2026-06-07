package providers

import (
	"fmt"
	"os"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// RoutingConfig defines provider routing and fallback behavior.
type RoutingConfig struct {
	// Priority determines which provider to try first (lower = higher priority).
	Priority int `json:"priority"`

	// FallbackModels defines a chain of models to try if the primary fails.
	FallbackModels []types.ModelIdentifier `json:"fallback_models,omitempty"`

	// FallbackProviders defines a chain of providers to try if all models fail.
	FallbackProviders []types.APIProvider `json:"fallback_providers,omitempty"`

	// CircuitBreaker tracks provider health.
	CircuitBreaker *CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
}

type Config struct {
	Provider          types.APIProvider
	APIKey            string
	BaseURL           string
	Region            string
	ProjectID         string
	ModelAliasMapping map[string]string
	CustomHeaders     map[string]string
	Routing           *RoutingConfig
}

func DefaultConfigs() map[types.APIProvider]*Config {
	return map[types.APIProvider]*Config{
		types.APIProviderAnthropic: {
			Provider: types.APIProviderAnthropic,
			BaseURL:  "https://api.anthropic.com",
			ModelAliasMapping: map[string]string{
				"sonnet":   "claude-sonnet-4-20250514",
				"haiku":    "claude-3-5-haiku-20241022",
				"opus":     "claude-3-opus-20240229",
				"sonnet-3": "claude-3-5-sonnet-20241022",
			},
			Routing: &RoutingConfig{
				Priority: 1,
				FallbackModels: []types.ModelIdentifier{
					{Provider: types.APIProviderAnthropic, Model: "claude-3-5-sonnet-20241022"},
					{Provider: types.APIProviderAnthropic, Model: "claude-3-5-haiku-20241022"},
				},
				FallbackProviders: []types.APIProvider{types.APIProviderOpenAI},
				CircuitBreaker:    DefaultCircuitBreakerConfig(),
			},
		},
		types.APIProviderOpenAI: {
			Provider: types.APIProviderOpenAI,
			BaseURL:  "https://api.openai.com/v1",
			ModelAliasMapping: map[string]string{
				"gpt-5.5":      "gpt-5.5",
				"gpt-5.4":      "gpt-5.4",
				"gpt-5.4-mini": "gpt-5.4-mini",
				"default":      "gpt-5.5",
				"mini":         "gpt-5.4-mini",
				"gpt-4o":       "gpt-4o",
				"gpt-4o-mini":  "gpt-4o-mini",
				"gpt-4-turbo":  "gpt-4-turbo",
				"o1":           "o1",
				"o1-mini":      "o1-mini",
				"o3":           "o3-mini",
				"o3-mini":      "o3-mini",
			},
		},
		types.APIProviderOllama: {
			Provider: types.APIProviderOllama,
			BaseURL:  "http://localhost:11434",
			ModelAliasMapping: map[string]string{
				"qwen-coder": "qwen2.5-coder:7b",
				"deepseek":   "deepseek-r1:14b",
				"llama":      "llama3.1:8b",
				"mistral":    "mistral:7b",
				"codellama":  "codellama:7b",
				"phi":        "phi3:3.8b",
			},
		},
		types.APIProviderBedrock: {
			Provider: types.APIProviderBedrock,
			Region:   getEnvOrDefault("AWS_REGION", "us-east-1"),
			ModelAliasMapping: map[string]string{
				"sonnet": "anthropic.claude-3-5-sonnet-20241022",
				"haiku":  "anthropic.claude-3-5-haiku-20241022",
				"opus":   "anthropic.claude-3-opus-20240229",
			},
		},
		types.APIProviderVertex: {
			Provider:  types.APIProviderVertex,
			ProjectID: os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID"),
			Region:    getEnvOrDefault("CLOUD_ML_REGION", "us-east5"),
			ModelAliasMapping: map[string]string{
				"sonnet": "claude-3-5-sonnet@20241022",
				"haiku":  "claude-3-5-haiku@20241022",
				"opus":   "claude-3-opus@20240229",
			},
		},
		types.APIProviderFoundry: {
			Provider: types.APIProviderFoundry,
			BaseURL:  getEnvOrDefault("ANTHROPIC_FOUNDRY_BASE_URL", "https://your-resource.services.ai.azure.com/anthropic/v1"),
			Region:   getEnvOrDefault("ANTHROPIC_FOUNDRY_RESOURCE", ""),
			ModelAliasMapping: map[string]string{
				"sonnet": "claude-3-5-sonnet-20241022",
				"haiku":  "claude-3-5-haiku-20241022",
			},
		},
		types.APIProviderGemini: {
			Provider: types.APIProviderGemini,
			BaseURL:  "https://generativelanguage.googleapis.com/v1beta",
			ModelAliasMapping: map[string]string{
				"gemini-pro":   "gemini-1.5-pro",
				"gemini-flash": "gemini-1.5-flash",
				"gemini-2":     "gemini-2.0-flash",
				"gemini-2-pro": "gemini-2.0-pro",
			},
		},
		types.APIProviderZAi: {
			Provider: types.APIProviderZAi,
			BaseURL:  "https://api.z.ai/api/paas/v4",
			ModelAliasMapping: map[string]string{
				"glm-5":       "glm-5.1",
				"glm-5-turbo": "glm-5-turbo",
				"glm-4.5":     "glm-4.5",
				"glm-4.7":     "glm-4.7",
				"glm-4.6":     "glm-4.6",
				"glm4.5":      "glm-4.5",
				"glm4":        "glm-4.5",
				"glm-4":       "glm-4-flash", // Try flash
				"glm4-flash":  "glm-4-flash",
			},
		},
		types.APIProviderOpenRouter: {
			Provider: types.APIProviderOpenRouter,
			BaseURL:  "https://openrouter.ai/api/v1",
			ModelAliasMapping: map[string]string{
				"opus":     "anthropic/claude-3-opus",
				"sonnet":   "anthropic/claude-3.5-sonnet",
				"haiku":    "anthropic/claude-3-haiku",
				"gpt-4o":   "openai/gpt-4o",
				"deepseek": "deepseek/deepseek-r1",
				"qwen":     "qwen/qwen2.5-coder-32b",
				"llama":    "meta-llama/llama-3.1-70b-instruct",
			},
		},
		types.APIProviderMiniMax: {
			Provider: types.APIProviderMiniMax,
			BaseURL:  "https://api.minimax.chat/v1/text/chatcompletion_v2",
			ModelAliasMapping: map[string]string{
				"m2.7":      "MiniMax-M2.7",
				"m2.7-fast": "MiniMax-M2.7-highspeed",
				"m2.5":      "MiniMax-M2.5",
				"m2.5-fast": "MiniMax-M2.5-highspeed",
				"m2.1":      "MiniMax-M2.1",
				"m2":        "MiniMax-M2",
			},
		},
		types.APIProviderWorkersAI: {
			Provider: types.APIProviderWorkersAI,
			BaseURL:  "https://workers.ai/v1/chat",
			ModelAliasMapping: map[string]string{
				"llama":    "@cf/meta/llama-3.1-70b-instruct",
				"deepseek": "@cf/deepseek-ai/deepseek-r1",
				"qwen":     "@cf/qwen/qwen2.5-coder-7b",
			},
		},
		types.APIProviderMistral: {
			Provider: types.APIProviderMistral,
			BaseURL:  "https://api.mistral.ai/v1",
			ModelAliasMapping: map[string]string{
				"large":  "mistral-large-latest",
				"small":  "mistral-small-latest",
				"7b":     "open-mistral-7b",
				"medium": "mistral-small-latest",
			},
		},
		types.APIProviderCodex: {
			Provider: types.APIProviderCodex,
			BaseURL:  "https://chatgpt.com/backend-api/codex",
			ModelAliasMapping: map[string]string{
				"default":    "gpt-5.3-codex",
				"codex":      "gpt-5.3-codex",
				"mini":       "gpt-5.4-mini",
				"codex-mini": "gpt-5.4-mini",
				"5.2":        "gpt-5.2-codex",
				"5.3":        "gpt-5.3-codex",
			},
		},
		types.APIProviderDeepSeek: {
			Provider: types.APIProviderDeepSeek,
			BaseURL:  "https://api.deepseek.com/v1",
			ModelAliasMapping: map[string]string{
				"default":  "deepseek-chat",
				"chat":     "deepseek-chat",
				"coder":    "deepseek-chat",
				"reasoner": "deepseek-reasoner",
				"r1":       "deepseek-reasoner",
			},
		},
		types.APIProviderOpenCode: {
			Provider: types.APIProviderOpenCode,
			BaseURL:  "https://opencode.ai/zen/v1",
			ModelAliasMapping: map[string]string{
				"default": "claude-sonnet-4",
				"sonnet":  "claude-sonnet-4",
				"codex":   "gpt-5.3-codex",
				"glm":     "glm-5.1",
			},
		},
	}
}

func GetProviderConfig(provider types.APIProvider) *Config {
	config, ok := DefaultConfigs()[provider]
	if !ok {
		return nil
	}
	// Return an isolated copy so callers can inject keys or overrides safely.
	return cloneConfig(config)
}

func (c *Config) ResolveModel(model string) string {
	if c == nil || c.ModelAliasMapping == nil {
		return model
	}
	if mapped, ok := c.ModelAliasMapping[model]; ok {
		return mapped
	}
	return model
}

func cloneConfig(config *Config) *Config {
	if config == nil {
		return nil
	}

	cloned := *config
	if config.ModelAliasMapping != nil {
		cloned.ModelAliasMapping = make(map[string]string, len(config.ModelAliasMapping))
		for key, value := range config.ModelAliasMapping {
			cloned.ModelAliasMapping[key] = value
		}
	}
	if config.CustomHeaders != nil {
		cloned.CustomHeaders = make(map[string]string, len(config.CustomHeaders))
		for key, value := range config.CustomHeaders {
			cloned.CustomHeaders[key] = value
		}
	}
	if config.Routing != nil {
		routing := *config.Routing
		if config.Routing.FallbackModels != nil {
			routing.FallbackModels = append([]types.ModelIdentifier(nil), config.Routing.FallbackModels...)
		}
		if config.Routing.FallbackProviders != nil {
			routing.FallbackProviders = append([]types.APIProvider(nil), config.Routing.FallbackProviders...)
		}
		if config.Routing.CircuitBreaker != nil {
			// Clone the circuit breaker config and create a new circuit breaker instance
			breaker := NewCircuitBreakerWithConfig(config.Routing.CircuitBreaker)
			routing.CircuitBreaker = &CircuitBreakerConfig{
				MaxFailures:      config.Routing.CircuitBreaker.MaxFailures,
				CallTimeout:      config.Routing.CircuitBreaker.CallTimeout,
				ResetTimeout:     config.Routing.CircuitBreaker.ResetTimeout,
				HalfOpenMaxCalls: config.Routing.CircuitBreaker.HalfOpenMaxCalls,
				ReadyToTrip:      config.Routing.CircuitBreaker.ReadyToTrip,
				OnStateChange:    config.Routing.CircuitBreaker.OnStateChange,
				CircuitBreaker:   breaker,
			}
		}
		cloned.Routing = &routing
	}

	return &cloned
}

func (c *Config) GetBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURLs[c.Provider]
}

func (c *Config) BuildAuthHeaders() map[string]string {
	headers := make(map[string]string)

	switch c.Provider {
	case types.APIProviderAnthropic, types.APIProviderBedrock, types.APIProviderVertex:
		if c.APIKey != "" {
			headers["x-api-key"] = c.APIKey
		}
		headers["anthropic-version"] = "2023-06-01"

	case types.APIProviderOpenAI, types.APIProviderGemini, types.APIProviderOpenRouter, types.APIProviderMiniMax, types.APIProviderMistral, types.APIProviderDeepSeek, types.APIProviderOpenCode:
		if c.APIKey != "" {
			headers["Authorization"] = "Bearer " + c.APIKey
		}

	case types.APIProviderFoundry:
		if c.APIKey != "" {
			headers["api-key"] = c.APIKey
		}
		if c.Region != "" {
			headers["Anthropic-Foundry-Resource-Id"] = c.Region
		}

	case types.APIProviderOllama:
		if c.APIKey != "" {
			headers["Authorization"] = "Bearer " + c.APIKey
		}

	case types.APIProviderZAi:
		if c.APIKey != "" {
			headers["x-api-key"] = c.APIKey
		}
		headers["Content-Type"] = "application/json"
		headers["Accept-Language"] = "en-US,en"

	case types.APIProviderWorkersAI:
		if c.APIKey != "" {
			headers["Authorization"] = "Bearer " + c.APIKey
		}

	case types.APIProviderCodex:
		if c.APIKey != "" {
			headers["Authorization"] = "Bearer " + c.APIKey
		}
	}

	for k, v := range c.CustomHeaders {
		headers[k] = v
	}

	return headers
}

func (c *Config) GetEndpoint(model string) string {
	resolvedModel := c.ResolveModel(model)

	switch c.Provider {
	case types.APIProviderAnthropic, types.APIProviderVertex, types.APIProviderFoundry:
		return c.GetBaseURL() + "/v1/messages"

	case types.APIProviderOpenAI, types.APIProviderOpenRouter, types.APIProviderMiniMax, types.APIProviderZAi, types.APIProviderMistral, types.APIProviderDeepSeek, types.APIProviderOpenCode:
		return c.GetBaseURL() + "/chat/completions"

	case types.APIProviderGemini:
		return c.GetBaseURL() + "/models/" + resolvedModel + ":generateContent"

	case types.APIProviderOllama:
		return c.GetBaseURL() + "/api/chat"

	case types.APIProviderWorkersAI:
		return c.BaseURL + "/" + resolvedModel

	case types.APIProviderCodex:
		return c.GetBaseURL() + "/responses"

	case types.APIProviderBedrock:
		return ""

	default:
		return c.GetBaseURL() + "/v1/messages"
	}
}

func (c *Config) IsSupportedModel(model string) bool {
	return true
}

var defaultBaseURLs = map[types.APIProvider]string{
	types.APIProviderAnthropic:  "https://api.anthropic.com",
	types.APIProviderOpenAI:     "https://api.openai.com/v1",
	types.APIProviderOllama:     "http://localhost:11434",
	types.APIProviderGemini:     "https://generativelanguage.googleapis.com/v1beta",
	types.APIProviderZAi:        "https://api.z.ai/api/paas/v4",
	types.APIProviderOpenRouter: "https://openrouter.ai/api/v1",
	types.APIProviderMiniMax:    "https://api.minimax.chat/v1/text/chatcompletion_v2",
	types.APIProviderWorkersAI:  "https://workers.ai/v1/chat",
	types.APIProviderFoundry:    "https://your-resource.services.ai.azure.com/anthropic/v1",
	types.APIProviderMistral:    "https://api.mistral.ai/v1",
	types.APIProviderCodex:      "https://chatgpt.com/backend-api/codex",
	types.APIProviderDeepSeek:   "https://api.deepseek.com/v1",
	types.APIProviderOpenCode:   "https://opencode.ai/zen/v1",
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func ValidateProviderConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("provider config is nil")
	}

	switch config.Provider {
	case types.APIProviderBedrock:
		if config.Region == "" {
			return fmt.Errorf("Bedrock requires AWS_REGION to be set")
		}

	case types.APIProviderVertex:
		if config.ProjectID == "" {
			return fmt.Errorf("Vertex requires ANTHROPIC_VERTEX_PROJECT_ID to be set")
		}
		if config.Region == "" {
			return fmt.Errorf("Vertex requires CLOUD_ML_REGION to be set")
		}

	case types.APIProviderFoundry:
		if config.BaseURL == "" && config.Region == "" {
			return fmt.Errorf("Foundry requires ANTHROPIC_FOUNDRY_BASE_URL or ANTHROPIC_FOUNDRY_RESOURCE to be set")
		}

	case types.APIProviderGemini:
		if config.APIKey == "" {
			return fmt.Errorf("Gemini requires GOOGLE_GEMINI_API_KEY to be set")
		}

	case types.APIProviderZAi:
		if config.APIKey == "" {
			return fmt.Errorf("Z.ai requires Z_AI_API_KEY to be set")
		}

	case types.APIProviderMiniMax:
		if config.APIKey == "" {
			return fmt.Errorf("MiniMax requires MINIMAX_API_KEY to be set")
		}

	case types.APIProviderOpenRouter:
		if config.APIKey == "" {
			return fmt.Errorf("OpenRouter requires OPENROUTER_API_KEY to be set")
		}

	case types.APIProviderWorkersAI:
		if config.APIKey == "" {
			return fmt.Errorf("Workers AI requires CLOUDFLARE_API_KEY to be set")
		}

	case types.APIProviderMistral:
		if config.APIKey == "" {
			return fmt.Errorf("Mistral requires MISTRAL_API_KEY to be set")
		}

	case types.APIProviderDeepSeek:
		if config.APIKey == "" {
			return fmt.Errorf("DeepSeek requires DEEPSEEK_API_KEY to be set")
		}

	case types.APIProviderOpenCode:
		if config.APIKey == "" {
			return fmt.Errorf("OpenCode requires OPENCODE_API_KEY to be set")
		}
	}

	return nil
}
