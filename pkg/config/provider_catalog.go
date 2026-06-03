package config

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	internalproviders "github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

type ProviderSetupField struct {
	Key         string
	Label       string
	Description string
	EnvVar      string
	Secret      bool
	Required    bool
}

type ProviderInfo struct {
	Name         sdk.APIProvider
	DisplayName  string
	Description  string
	AuthType     string
	AuthTypes    []string
	SupportsCVMM bool
	SupportsPC   bool
	Models       []ModelInfo
	SetupFields  []ProviderSetupField
	SetupHint    string
}

type ModelInfo struct {
	Identifier    string
	ContextWindow int
	MaxOutput     int
	SupportsPC    bool
	Pricing       string
	Description   string
}

var providerOrder = []sdk.APIProvider{
	sdk.APIProviderAnthropic,
	sdk.APIProviderOpenAI,
	sdk.APIProviderCodex,
	sdk.APIProviderGemini,
	sdk.APIProviderZAi,
	sdk.APIProviderOpenRouter,
	sdk.APIProviderDeepSeek,
	sdk.APIProviderOpenCode,
	sdk.APIProviderMistral,
	sdk.APIProviderMiniMax,
	sdk.APIProviderWorkersAI,
	sdk.APIProviderOllama,
	sdk.APIProviderBedrock,
	sdk.APIProviderVertex,
	sdk.APIProviderFoundry,
}

func AvailableProviders() []ProviderInfo {
	raw := internalproviders.AllProvidersInfo()
	providers := make([]ProviderInfo, 0, len(raw))

	for _, provider := range providerOrder {
		info, ok := raw[provider]
		if !ok {
			continue
		}
		providers = append(providers, providerInfoFromInternal(provider, info))
	}

	return providers
}

func GetProviderInfo(provider sdk.APIProvider) (ProviderInfo, bool) {
	info, ok := internalproviders.GetProviderInfo(provider)
	if !ok {
		return ProviderInfo{}, false
	}
	return providerInfoFromInternal(provider, info), true
}

func ResolveProvider(raw string) sdk.APIProvider {
	provider, ok := parseProvider(raw)
	if !ok {
		return ""
	}
	return provider
}

func ProviderForModel(model string) sdk.APIProvider {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return ""
	}

	for _, provider := range AvailableProviders() {
		for _, candidate := range provider.Models {
			if strings.EqualFold(candidate.Identifier, normalized) {
				return provider.Name
			}
		}
	}

	return ""
}

func ProviderCredentialEnvVars(provider sdk.APIProvider) []string {
	switch provider {
	case sdk.APIProviderAnthropic:
		return []string{"ANTHROPIC_API_KEY"}
	case sdk.APIProviderOpenAI:
		return []string{"OPENAI_API_KEY"}
	case sdk.APIProviderCodex:
		return []string{"CODEX_API_KEY"}
	case sdk.APIProviderGemini:
		return []string{"GOOGLE_API_KEY", "GOOGLE_GEMINI_API_KEY"}
	case sdk.APIProviderZAi:
		return []string{"ZHIPUAI_API_KEY", "Z_AI_API_KEY"}
	case sdk.APIProviderOpenRouter:
		return []string{"OPENROUTER_API_KEY"}
	case sdk.APIProviderDeepSeek:
		return []string{"DEEPSEEK_API_KEY"}
	case sdk.APIProviderOpenCode:
		return []string{"OPENCODE_API_KEY"}
	case sdk.APIProviderMistral:
		return []string{"MISTRAL_API_KEY"}
	case sdk.APIProviderMiniMax:
		return []string{"MINIMAX_API_KEY"}
	case sdk.APIProviderWorkersAI:
		return []string{"CLOUDFLARE_API_KEY"}
	case sdk.APIProviderFoundry:
		return []string{"ANTHROPIC_FOUNDRY_API_KEY"}
	case sdk.APIProviderOllama, sdk.APIProviderBedrock, sdk.APIProviderVertex:
		return nil
	default:
		return nil
	}
}

func ResolveAPIKey(config Config, provider sdk.APIProvider) string {
	if key := resolveAPIKeyFromEnv(provider); key != "" {
		return key
	}
	return strings.TrimSpace(config.APIKey)
}

func ValidateProviderSetup(config Config, provider sdk.APIProvider) error {
	switch provider {
	case sdk.APIProviderOllama:
		return nil
	case sdk.APIProviderBedrock:
		if effectiveProviderRegion(config, provider) == "" {
			return fmt.Errorf("provider %s requires AWS region", provider)
		}
		return nil
	case sdk.APIProviderVertex:
		if effectiveProviderProjectID(config, provider) == "" {
			return fmt.Errorf("provider %s requires project id", provider)
		}
		if effectiveProviderRegion(config, provider) == "" {
			return fmt.Errorf("provider %s requires region", provider)
		}
		return nil
	case sdk.APIProviderFoundry:
		if ResolveAPIKey(config, provider) == "" {
			return fmt.Errorf("provider %s requires API key", provider)
		}
		if effectiveProviderBaseURL(config, provider) == "" && effectiveProviderResource(config, provider) == "" {
			return fmt.Errorf("provider %s requires base URL or resource id", provider)
		}
		return nil
	default:
		if len(ProviderCredentialEnvVars(provider)) == 0 {
			return nil
		}
		if ResolveAPIKey(config, provider) == "" {
			return fmt.Errorf("provider %s requires API key", provider)
		}
		return nil
	}
}

func ApplyRuntimeEnv(config Config) {
	setEnvIfPresent(runtimepath.EnvRuntimeRoot, EffectiveRuntimeRoot(config))

	provider := preferredProvider(config)
	if provider == "" {
		return
	}

	switch provider {
	case sdk.APIProviderBedrock:
		setEnvIfPresent("AWS_REGION", config.ProviderRegion)
	case sdk.APIProviderVertex:
		setEnvIfPresent("ANTHROPIC_VERTEX_PROJECT_ID", config.ProviderProjectID)
		setEnvIfPresent("CLOUD_ML_REGION", config.ProviderRegion)
	case sdk.APIProviderFoundry:
		setEnvIfPresent("ANTHROPIC_FOUNDRY_BASE_URL", config.ProviderBaseURL)
		setEnvIfPresent("ANTHROPIC_FOUNDRY_RESOURCE", config.ProviderResource)
	}
}

func parseProvider(raw string) (sdk.APIProvider, bool) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "anthropic", "claude":
		return sdk.APIProviderAnthropic, true
	case "openai", "gpt":
		return sdk.APIProviderOpenAI, true
	case "codex":
		return sdk.APIProviderCodex, true
	case "ollama":
		return sdk.APIProviderOllama, true
	case "bedrock", "aws":
		return sdk.APIProviderBedrock, true
	case "vertex", "gcp", "google-cloud":
		return sdk.APIProviderVertex, true
	case "foundry", "azure":
		return sdk.APIProviderFoundry, true
	case "gemini", "google-ai":
		return sdk.APIProviderGemini, true
	case "z-ai", "z.ai", "zai":
		return sdk.APIProviderZAi, true
	case "openrouter":
		return sdk.APIProviderOpenRouter, true
	case "deepseek", "deep-seek":
		return sdk.APIProviderDeepSeek, true
	case "opencode", "opencode-zen", "opencode_zen":
		return sdk.APIProviderOpenCode, true
	case "mistral", "mistral-ai", "mistralai":
		return sdk.APIProviderMistral, true
	case "minimax":
		return sdk.APIProviderMiniMax, true
	case "workers-ai", "workers", "cloudflare":
		return sdk.APIProviderWorkersAI, true
	default:
		return "", false
	}
}

func providerInfoFromInternal(provider sdk.APIProvider, info internalproviders.ProviderInfo) ProviderInfo {
	models := make([]ModelInfo, 0, len(info.Models))
	for _, model := range info.Models {
		models = append(models, ModelInfo{
			Identifier:    model.Identifier,
			ContextWindow: model.ContextWindow,
			MaxOutput:     model.MaxOutput,
			SupportsPC:    model.SupportsPC,
			Pricing:       model.Pricing,
			Description:   model.Description,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return strings.ToLower(models[i].Identifier) < strings.ToLower(models[j].Identifier)
	})

	return ProviderInfo{
		Name:         provider,
		DisplayName:  info.DisplayName,
		Description:  info.Description,
		AuthType:     info.AuthType,
		AuthTypes:    append([]string(nil), info.AuthTypes...),
		SupportsCVMM: info.SupportsCVMM,
		SupportsPC:   info.SupportsPC,
		Models:       models,
		SetupFields:  setupFieldsForProvider(provider),
		SetupHint:    setupHintForProvider(provider),
	}
}

func setupFieldsForProvider(provider sdk.APIProvider) []ProviderSetupField {
	fields := make([]ProviderSetupField, 0, 4)
	if envVars := ProviderCredentialEnvVars(provider); len(envVars) > 0 {
		fields = append(fields, ProviderSetupField{
			Key:         "api_key",
			Label:       "API key",
			Description: "Credential used for requests",
			EnvVar:      envVars[0],
			Secret:      true,
			Required:    true,
		})
	}

	switch provider {
	case sdk.APIProviderBedrock:
		fields = append(fields, ProviderSetupField{
			Key:         "provider_region",
			Label:       "AWS region",
			Description: "Example: us-east-1",
			EnvVar:      "AWS_REGION",
			Required:    true,
		})
	case sdk.APIProviderVertex:
		fields = append(fields,
			ProviderSetupField{
				Key:         "provider_project_id",
				Label:       "Vertex project id",
				Description: "GCP project used by Anthropic Vertex",
				EnvVar:      "ANTHROPIC_VERTEX_PROJECT_ID",
				Required:    true,
			},
			ProviderSetupField{
				Key:         "provider_region",
				Label:       "Vertex region",
				Description: "Example: us-east5",
				EnvVar:      "CLOUD_ML_REGION",
				Required:    true,
			},
		)
	case sdk.APIProviderFoundry:
		fields = append(fields,
			ProviderSetupField{
				Key:         "provider_base_url",
				Label:       "Foundry base URL",
				Description: "Optional if you provide a Foundry resource id",
				EnvVar:      "ANTHROPIC_FOUNDRY_BASE_URL",
			},
			ProviderSetupField{
				Key:         "provider_resource",
				Label:       "Foundry resource id",
				Description: "Optional if you provide a Foundry base URL",
				EnvVar:      "ANTHROPIC_FOUNDRY_RESOURCE",
			},
		)
	}

	return fields
}

func setupHintForProvider(provider sdk.APIProvider) string {
	switch provider {
	case sdk.APIProviderOllama:
		return "Uses the default local Ollama endpoint at http://localhost:11434."
	case sdk.APIProviderBedrock:
		return "Requires AWS credentials in your environment or profile in addition to the region."
	case sdk.APIProviderVertex:
		return "Requires Google Cloud application credentials in addition to project and region."
	case sdk.APIProviderFoundry:
		return "Set at least one of base URL or resource id for Azure Foundry."
	default:
		return ""
	}
}

func resolveAPIKeyFromEnv(provider sdk.APIProvider) string {
	for _, envVar := range ProviderCredentialEnvVars(provider) {
		if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
			return value
		}
	}
	return ""
}

func effectiveProviderRegion(config Config, provider sdk.APIProvider) string {
	if value := strings.TrimSpace(config.ProviderRegion); value != "" {
		return value
	}
	switch provider {
	case sdk.APIProviderBedrock:
		return strings.TrimSpace(os.Getenv("AWS_REGION"))
	case sdk.APIProviderVertex:
		return strings.TrimSpace(os.Getenv("CLOUD_ML_REGION"))
	default:
		return ""
	}
}

func effectiveProviderProjectID(config Config, provider sdk.APIProvider) string {
	if provider != sdk.APIProviderVertex {
		return ""
	}
	if value := strings.TrimSpace(config.ProviderProjectID); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID"))
}

func effectiveProviderBaseURL(config Config, provider sdk.APIProvider) string {
	if provider != sdk.APIProviderFoundry {
		return ""
	}
	if value := strings.TrimSpace(config.ProviderBaseURL); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("ANTHROPIC_FOUNDRY_BASE_URL"))
}

func effectiveProviderResource(config Config, provider sdk.APIProvider) string {
	if provider != sdk.APIProviderFoundry {
		return ""
	}
	if value := strings.TrimSpace(config.ProviderResource); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("ANTHROPIC_FOUNDRY_RESOURCE"))
}

func setEnvIfPresent(key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	_ = os.Setenv(key, value)
}

func ProviderNames() []sdk.APIProvider {
	out := make([]sdk.APIProvider, 0, len(providerOrder))
	for _, provider := range providerOrder {
		if _, ok := GetProviderInfo(provider); ok {
			out = append(out, provider)
		}
	}
	return out
}

func IsKnownProvider(provider sdk.APIProvider) bool {
	return slices.Contains(ProviderNames(), provider)
}
