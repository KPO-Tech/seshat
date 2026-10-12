package providers

import (
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/model"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

type ProviderInfo struct {
	Name         string
	DisplayName  string
	Description  string
	AuthType     string
	AuthTypes    []string
	SupportsCVMM bool
	SupportsPC   bool
	Models       []ModelInfo
}

// ModelInfo describes a single model offered by a provider.
// Capabilities contains fine-grained per-model feature flags.
// The Pricing field is a legacy string; structured pricing lives in model.Metadata.
type ModelInfo struct {
	Identifier         string
	ContextWindow      int
	MaxOutput          int
	DefaultTemperature float64
	SupportsPC         bool
	Pricing            string
	Description        string
	Capabilities       model.Capabilities
}

func AllProvidersInfo() map[types.APIProvider]ProviderInfo {
	return map[types.APIProvider]ProviderInfo{
		types.APIProviderAnthropic: {
			Name:         "anthropic",
			DisplayName:  "Anthropic",
			Description:  "Direct API - Claude models",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "claude-sonnet-4-20250514", ContextWindow: 200000, MaxOutput: 64000, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude Sonnet 4 — Latest flagship model. Best balance of speed and quality for agentic tasks.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true, StructuredOutput: true}},
				{Identifier: "claude-3-5-sonnet-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Sonnet — Excellent coding and reasoning. Ideal for complex analysis and generation.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true, StructuredOutput: true}},
				{Identifier: "claude-3-5-haiku-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Haiku — Fastest Claude model. Best for high-volume, low-latency tasks.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
			},
		},
		types.APIProviderOpenAI: {
			Name:         "openai",
			DisplayName:  "OpenAI",
			Description:  "Direct API - GPT models",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "gpt-5.5", ContextWindow: 272000, MaxOutput: 32768, DefaultTemperature: 1.0, SupportsPC: true, Description: "GPT-5.5 — Frontier model for complex coding, research, and real-world work.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true, StructuredOutput: true}},
				{Identifier: "gpt-5.4-mini", ContextWindow: 272000, MaxOutput: 16384, DefaultTemperature: 1.0, SupportsPC: true, Description: "GPT-5.4 Mini — Small, fast, and cost-efficient model for simpler coding tasks.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true, StructuredOutput: true}},
				{Identifier: "gpt-4o", ContextWindow: 128000, MaxOutput: 16384, DefaultTemperature: 1.0, SupportsPC: true, Description: "GPT-4o — Multimodal flagship. Fast, vision-capable, and great for general-purpose tasks.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true, StructuredOutput: true}},
			},
		},
		types.APIProviderCodex: {
			Name:         "codex",
			DisplayName:  "Codex",
			Description:  "ChatGPT Pro subscription — Responses API via chatgpt.com",
			AuthType:     "oauth",
			AuthTypes:    []string{"oauth"},
			SupportsCVMM: false,
			SupportsPC:   false,
			// gpt-5.3-codex/gpt-5.2-codex/gpt-5.4-codex all confirmed broken by
			// a real ChatGPT-account session ("The 'gpt-5.X-codex' model is
			// not supported when using Codex with a ChatGPT account"). The
			// three below are confirmed working via that same live session
			// (both from the account's own VS Code Codex picker and a direct
			// API smoke test, 2026-08-28).
			//
			// Also confirmed working live (2026-08-28) but not yet added as
			// full catalog entries below, pending real context/output-limit
			// numbers rather than a guess: gpt-5.6-terra, gpt-5.6-luna (same
			// gpt-5.6 family as gpt-5.6-sol) and gpt-5.4 (no "-mini"). Usable
			// today via an explicit "codex:<id>" override even though they
			// aren't listed here — this repo's IsSupportedModel never gates
			// on catalog membership.
			Models: []ModelInfo{
				{Identifier: "gpt-5.6-sol", ContextWindow: 200000, MaxOutput: 100000, DefaultTemperature: 1.0, Description: "GPT-5.6-Sol — Flagship Codex model via ChatGPT Pro subscription.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "gpt-5.5", ContextWindow: 200000, MaxOutput: 100000, DefaultTemperature: 1.0, Description: "GPT-5.5 — Codex model available via ChatGPT Pro subscription.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "gpt-5.4-mini", ContextWindow: 272000, MaxOutput: 16384, DefaultTemperature: 1.0, Description: "GPT-5.4-Mini — Fast, cost-efficient model via Codex (ChatGPT Pro subscription).",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderMistral: {
			Name:         "mistral",
			DisplayName:  "Mistral AI",
			Description:  "Direct API - Mistral models (OpenAI-compatible)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   false,
			Models: []ModelInfo{
				{Identifier: "mistral-large-latest", ContextWindow: 131072, MaxOutput: 16384, DefaultTemperature: 0.7, Description: "Mistral Large — Top-tier reasoning model. Ideal for complex tasks requiring deep understanding and multi-step reasoning.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "mistral-small-latest", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 0.7, Description: "Mistral Small — Efficient and capable. Strong performance for most tasks at lower cost and latency.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "open-mistral-7b", ContextWindow: 32768, MaxOutput: 4096, DefaultTemperature: 0.7, Description: "Mistral 7B — Lightweight open-source model. Best for high-throughput use cases where speed is critical.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderGemini: {
			Name:         "gemini",
			DisplayName:  "Google Gemini",
			Description:  "Google's Gemini models",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   false,
			Models: []ModelInfo{
				{Identifier: "gemini-2.0-pro", ContextWindow: 2000000, MaxOutput: 8192, DefaultTemperature: 1.0, Description: "Gemini 2.0 Pro — Google's most capable model. Excels at complex reasoning with an unmatched 2M token context window.",
					Capabilities: model.Capabilities{Vision: true, Audio: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "gemini-2.0-flash", ContextWindow: 1000000, MaxOutput: 8192, DefaultTemperature: 1.0, Description: "Gemini 2.0 Flash — High-speed model with a 1M context window. Best for fast multimodal tasks.",
					Capabilities: model.Capabilities{Vision: true, Audio: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "gemini-1.5-flash", ContextWindow: 1000000, MaxOutput: 8192, DefaultTemperature: 1.0, Description: "Gemini 1.5 Flash — Cost-effective and fast. Handles long documents, audio, and video efficiently.",
					Capabilities: model.Capabilities{Vision: true, Audio: true, FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderMiniMax: {
			Name:         "minimax",
			DisplayName:  "MiniMax",
			Description:  "Chinese AI platform (OpenAI-compatible chat completions)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "MiniMax-M2.7", ContextWindow: 204800, MaxOutput: 128000, DefaultTemperature: 1.0, SupportsPC: true, Description: "MiniMax M2.7 — Latest flagship with recursive self-improvement. Excels at long-horizon agentic tasks.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "MiniMax-M2.5", ContextWindow: 204800, MaxOutput: 128000, DefaultTemperature: 1.0, SupportsPC: true, Description: "MiniMax M2.5 — Peak performance for complex multi-step tasks. Strong coding and reasoning capabilities.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "MiniMax-M2.1", ContextWindow: 204800, MaxOutput: 8192, DefaultTemperature: 1.0, Description: "MiniMax M2.1 — Powerful multilingual model. Optimized for coding, instruction following, and analysis.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderOpenRouter: {
			Name:         "openrouter",
			DisplayName:  "OpenRouter",
			Description:  "Unified API for 300+ models",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			// OpenRouter does support prompt caching for some underlying
			// models (e.g. Anthropic's cache_control), but only when the
			// request is sent in the native Anthropic wire shape — this
			// codebase routes OpenRouter through the OpenAI-compatible
			// adapter (chat/completions body), which has no cache_control
			// field at all, so no request this codebase sends actually gets
			// cached today. false reflects what's actually implemented,
			// not what OpenRouter could theoretically support.
			SupportsPC: false,
			Models: []ModelInfo{
				{Identifier: "anthropic/claude-3.5-sonnet", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, Description: "Claude 3.5 Sonnet via OpenRouter — Top-quality model accessible through a single unified API key.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "openai/gpt-4o", ContextWindow: 128000, MaxOutput: 16384, DefaultTemperature: 1.0, Description: "GPT-4o via OpenRouter — OpenAI's flagship model without a direct OpenAI subscription.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "deepseek/deepseek-r1", ContextWindow: 64000, MaxOutput: 32000, DefaultTemperature: 1.0, Description: "DeepSeek-R1 via OpenRouter — Open-source reasoning model competitive with o1. Exceptional for math and code.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderDeepSeek: {
			Name:         "deepseek",
			DisplayName:  "DeepSeek",
			Description:  "Direct API - DeepSeek models (OpenAI-compatible)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   false,
			// deepseek-chat/deepseek-reasoner/deepseek-coder-v2 (the
			// previous catalog here) are legacy V3.1-era model IDs;
			// DeepSeek's own pricing docs (api-docs.deepseek.com, checked
			// 2026-08-28) list only the V4 lineup below, with no mention of
			// the old names at all.
			Models: []ModelInfo{
				{Identifier: "deepseek-v4-flash", ContextWindow: 1048576, MaxOutput: 393216, DefaultTemperature: 0.7, Description: "DeepSeek V4 Flash — Fast, cost-effective default. 1M context, supports both thinking and non-thinking modes.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "deepseek-v4-pro", ContextWindow: 1048576, MaxOutput: 393216, DefaultTemperature: 0.7, Description: "DeepSeek V4 Pro — Flagship, highest-capability model. Same 1M context/384K output as Flash, ~3x the cost, for demanding reasoning and coding tasks.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "deepseek-v4-flash-vision-exp", ContextWindow: 1048576, MaxOutput: 393216, DefaultTemperature: 0.7, Description: "DeepSeek V4 Flash Vision (experimental) — Same specs as Flash, adds image understanding. No FIM completion support.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderOpenCode: {
			Name:         "opencode",
			DisplayName:  "OpenCode Zen",
			Description:  "Curated model gateway (OpenAI-compatible)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   false,
			Models: []ModelInfo{
				{Identifier: "claude-sonnet-4", ContextWindow: 200000, MaxOutput: 64000, DefaultTemperature: 1.0, Description: "Claude Sonnet 4 via OpenCode Zen.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "gpt-5.3-codex", ContextWindow: 200000, MaxOutput: 100000, DefaultTemperature: 1.0, Description: "GPT-5.3-Codex via OpenCode Zen.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true}},
				{Identifier: "glm-5.1", ContextWindow: 200000, MaxOutput: 128000, DefaultTemperature: 0.7, Description: "GLM-5.1 via OpenCode Zen.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
			},
		},
		types.APIProviderZAi: {
			Name:         "z-ai",
			DisplayName:  "Z.ai",
			Description:  "GLM models (Chinese AI, OpenAI-compatible)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "glm-5.1", ContextWindow: 200000, MaxOutput: 128000, DefaultTemperature: 0.7, SupportsPC: true, Description: "GLM-5.1 — Latest flagship GLM. Designed for 8h+ autonomous execution with advanced tool use.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "glm-4.7", ContextWindow: 1000000, MaxOutput: 8192, DefaultTemperature: 0.7, Description: "GLM-4.7 — SOTA performance with 1M token context. Ideal for long-document analysis.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
				{Identifier: "glm-4.5", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 0.7, Description: "GLM-4.5 — Hybrid reasoning model. Balances fast inference with strong analytical capabilities.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true}},
			},
		},
		// Ollama: no static model list — models are discovered dynamically from the
		// local Ollama API (/api/tags). The frontend shows an empty list until the
		// user triggers a sync, which populates the DB with locally installed models.
		types.APIProviderOllama: {
			Name:         "ollama",
			DisplayName:  "Ollama",
			Description:  "Local LLM inference — auto-detected from localhost:11434",
			AuthType:     "none",
			AuthTypes:    []string{"none"},
			SupportsCVMM: false,
			SupportsPC:   false,
			Models:       nil,
		},
		types.APIProviderBedrock: {
			Name:         "bedrock",
			DisplayName:  "AWS Bedrock",
			Description:  "Claude via Amazon AWS",
			AuthType:     "aws",
			AuthTypes:    []string{"aws"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "anthropic.claude-3-5-sonnet-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Sonnet via Bedrock",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "anthropic.claude-3-5-haiku-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Haiku via Bedrock",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "anthropic.claude-3-opus-20240229", ContextWindow: 200000, MaxOutput: 4096, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3 Opus via Bedrock",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
			},
		},
		types.APIProviderVertex: {
			Name:         "vertex",
			DisplayName:  "Google Cloud Vertex AI",
			Description:  "Claude via Google Cloud",
			AuthType:     "gcp",
			AuthTypes:    []string{"gcp"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "claude-3-5-sonnet@20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Sonnet via Vertex",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "claude-3-5-haiku@20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Haiku via Vertex",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "claude-3-opus@20240229", ContextWindow: 200000, MaxOutput: 4096, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3 Opus via Vertex",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
			},
		},
		types.APIProviderFoundry: {
			Name:         "foundry",
			DisplayName:  "Azure AI Foundry",
			Description:  "Claude via Microsoft Azure",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			SupportsPC:   true,
			Models: []ModelInfo{
				{Identifier: "claude-3-5-sonnet-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Sonnet via Azure Foundry",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "claude-3-5-haiku-20241022", ContextWindow: 200000, MaxOutput: 8192, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3.5 Haiku via Azure Foundry",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "claude-3-opus-20240229", ContextWindow: 200000, MaxOutput: 4096, DefaultTemperature: 1.0, SupportsPC: true, Description: "Claude 3 Opus via Azure Foundry",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
			},
		},
		types.APIProviderKimi: {
			Name:         "kimi",
			DisplayName:  "Kimi (Moonshot AI)",
			Description:  "Direct API - Kimi/Moonshot models (OpenAI-compatible)",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: true,
			// Moonshot caches automatically server-side (same-prefix requests
			// hit the cache with no cache_control marker or other client-side
			// action required), unlike Anthropic's opt-in mechanism — so this
			// is accurate regardless of what this codebase's own request
			// building does. See
			// https://platform.moonshot.ai/docs/guide/use-context-caching-feature-of-kimi-api.
			SupportsPC: true,
			// The moonshot-v1-* generation and kimi-k2.5 are being sunset by
			// Moonshot (new registrations already blocked, full platform
			// sunset ~Aug 31 2026) in favor of the kimi-k2.6/k2.7/k3 lineup -
			// see https://platform.kimi.ai/docs/models.
			Models: []ModelInfo{
				{Identifier: "kimi-k3", ContextWindow: 1048576, MaxOutput: 1048576, DefaultTemperature: 0.7, SupportsPC: true, Description: "Kimi K3 — Flagship, 2.8T-parameter native multimodal agentic model. 1M-token context, vision, and a reasoning/thinking mode for long-horizon coding and knowledge work.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "kimi-k2.7-code", ContextWindow: 262144, MaxOutput: 262144, DefaultTemperature: 0.7, SupportsPC: true, Description: "Kimi K2.7 Code — Coding specialist, always runs in thinking mode. Best for long-horizon agentic coding over large repositories.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "kimi-k2.7-code-highspeed", ContextWindow: 262144, MaxOutput: 262144, DefaultTemperature: 0.7, SupportsPC: true, Description: "Kimi K2.7 Code (high-speed) — Same coding specialist as K2.7 Code, tuned for faster (~180 tok/s) output when latency matters more than squeezing out the last bit of quality.",
					Capabilities: model.Capabilities{FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "kimi-k2.6", ContextWindow: 262144, MaxOutput: 262144, DefaultTemperature: 0.7, SupportsPC: true, Description: "Kimi K2.6 — Multimodal model with vision, thinking mode, and agent orchestration for long-horizon coding and UI/UX generation.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
				{Identifier: "kimi-k2.5", ContextWindow: 262144, MaxOutput: 262144, DefaultTemperature: 0.7, SupportsPC: true, Description: "Kimi K2.5 — Vision, thinking mode, agents, and coding. Being sunset by Moonshot (~Aug 31 2026); prefer K2.6, K2.7 Code, or K3 for new work.",
					Capabilities: model.Capabilities{Vision: true, FunctionCalling: true, Streaming: true, PromptCaching: true}},
			},
		},
		types.APIProviderWorkersAI: {
			Name:         "workers-ai",
			DisplayName:  "Cloudflare Workers AI",
			Description:  "Edge AI inference",
			AuthType:     "api_key",
			AuthTypes:    []string{"api_key"},
			SupportsCVMM: false,
			SupportsPC:   false,
			Models: []ModelInfo{
				{Identifier: "@cf/meta/llama-3.1-70b-instruct", ContextWindow: 128000, MaxOutput: 4096, DefaultTemperature: 0.7, Description: "Llama 3.1 70B — Edge inference via Cloudflare.",
					Capabilities: model.Capabilities{FunctionCalling: false, Streaming: true}},
				{Identifier: "@cf/deepseek-ai/deepseek-r1", ContextWindow: 46080, MaxOutput: 4096, DefaultTemperature: 0.7, Description: "DeepSeek-R1 — Reasoning model at the edge.",
					Capabilities: model.Capabilities{FunctionCalling: false, Streaming: true}},
				{Identifier: "@cf/qwen/qwen2.5-coder-7b", ContextWindow: 32768, MaxOutput: 4096, DefaultTemperature: 0.7, Description: "Qwen2.5-Coder 7B — Code model at the edge.",
					Capabilities: model.Capabilities{FunctionCalling: false, Streaming: true}},
			},
		},
	}
}

// OllamaModelLookup returns a name → ModelInfo map of context-window hints for
// popular Ollama models. It is used as a best-effort fallback when /api/show
// does not return context length data. The authoritative dynamic model list
// comes from providers.FetchModels (internal/providers/fetch.go).
func OllamaModelLookup() map[string]ModelInfo {
	return map[string]ModelInfo{
		"qwen2.5-coder:32b": {Identifier: "qwen2.5-coder:32b", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 0.7},
		"qwen2.5-coder:7b":  {Identifier: "qwen2.5-coder:7b", ContextWindow: 32768, MaxOutput: 8192, DefaultTemperature: 0.7},
		"llama3.2":          {Identifier: "llama3.2", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 0.8},
		"llama3.1:8b":       {Identifier: "llama3.1:8b", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 0.8},
		"mistral:7b":        {Identifier: "mistral:7b", ContextWindow: 32768, MaxOutput: 4096, DefaultTemperature: 0.7},
		"deepseek-r1:7b":    {Identifier: "deepseek-r1:7b", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 0.6},
		"phi4":              {Identifier: "phi4", ContextWindow: 16384, MaxOutput: 4096, DefaultTemperature: 0.7},
		"gemma3:12b":        {Identifier: "gemma3:12b", ContextWindow: 131072, MaxOutput: 8192, DefaultTemperature: 1.0},
	}
}

func ResolveProviderFromString(s string) types.APIProvider {
	switch strings.ToLower(s) {
	case "anthropic", "claude":
		return types.APIProviderAnthropic
	case "openai", "gpt":
		return types.APIProviderOpenAI
	case "ollama":
		return types.APIProviderOllama
	case "bedrock", "aws":
		return types.APIProviderBedrock
	case "vertex", "gcp", "google-cloud":
		return types.APIProviderVertex
	case "foundry", "azure":
		return types.APIProviderFoundry
	case "gemini", "google-ai":
		return types.APIProviderGemini
	case "z-ai", "z.ai", "zai":
		return types.APIProviderZAi
	case "openrouter":
		return types.APIProviderOpenRouter
	case "minimax":
		return types.APIProviderMiniMax
	case "workers-ai", "workers", "cloudflare":
		return types.APIProviderWorkersAI
	case "mistral", "mistral-ai", "mistralai":
		return types.APIProviderMistral
	case "codex":
		return types.APIProviderCodex
	case "deepseek", "deep-seek":
		return types.APIProviderDeepSeek
	case "opencode", "opencode-zen", "opencode_zen":
		return types.APIProviderOpenCode
	case "kimi", "moonshot", "moonshot-ai", "moonshotai":
		return types.APIProviderKimi
	default:
		return types.APIProviderAnthropic
	}
}

func GetProviderInfo(provider types.APIProvider) (ProviderInfo, bool) {
	info, ok := AllProvidersInfo()[provider]
	return info, ok
}

func GetModelInfo(provider types.APIProvider, model string) (ModelInfo, bool) {
	info, ok := AllProvidersInfo()[provider]
	if !ok {
		return ModelInfo{}, false
	}
	for _, m := range info.Models {
		if m.Identifier == model {
			return m, true
		}
	}
	return ModelInfo{}, false
}

func DefaultModelIdentifier(provider types.APIProvider) (types.ModelIdentifier, bool) {
	info, ok := GetProviderInfo(provider)
	if !ok || len(info.Models) == 0 {
		return types.ModelIdentifier{}, false
	}
	return types.ModelIdentifier{
		Provider: provider,
		Model:    info.Models[0].Identifier,
	}, true
}
