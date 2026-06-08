package providers

import (
	"context"
	"io"
	"net/http"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// providerAdapter encapsulates the provider-specific wire format behind a
// polymorphic interface so the transport-level Client no longer dispatches on
// `c.provider` via scattered switch statements (one in buildRequestBody, one in
// buildModelEndpoint, one in setRequestHeaders, one in decodeProviderResponse,
// one in CreateMessageStreamResultWithCallback).
//
// Each adapter is stateless; the owning *Client is passed in so adapters can
// read baseURL/apiKey/providerConfig and reuse shared HTTP helpers
// (sendMessage, handleErrorResponse, monitoring). The canonical conversation
// format stays Anthropic-shaped (internal/types) — adapters only translate at
// the network edge, preserving the exact behaviour of the previous switches.
//
// Adding a provider becomes: reuse an existing adapter (e.g. openAICompatAdapter)
// or implement a new one, then map it in adapterForProvider.
type providerAdapter interface {
	// buildRequestBody serializes an APIRequest into the provider wire format.
	buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error)

	// modelEndpoint returns the default (non-stream) URL for a resolved model.
	modelEndpoint(c *Client, model string) string

	// requestEndpoint returns the URL for a specific request (stream-aware).
	requestEndpoint(c *Client, req types.APIRequest) string

	// applyAuthHeaders sets provider auth/version headers on the outgoing request.
	// The shared Content-Type and custom metadata headers are applied by the
	// caller (setRequestHeaders), so adapters only set provider-specific keys.
	applyAuthHeaders(c *Client, req *http.Request)

	// decodeResponse decodes a non-streaming response body into canonical form.
	decodeResponse(c *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error)

	// createStreamResult consumes the streaming response and returns the
	// canonical aggregated result, emitting normalized chunks to onChunk.
	createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error)
}

// adapterForProvider maps a provider to its wire-format adapter. The mapping
// mirrors exactly the branches of the former switch statements in client.go.
func adapterForProvider(p types.APIProvider) providerAdapter {
	switch p {
	case types.APIProviderZAi, "zai", "z.ai":
		return zAiAdapter{}
	case types.APIProviderOpenAI, types.APIProviderMiniMax, types.APIProviderOpenRouter, types.APIProviderMistral, types.APIProviderDeepSeek, types.APIProviderOpenCode:
		return openAICompatAdapter{}
	case types.APIProviderCodex:
		return codexAdapter{}
	case types.APIProviderOllama:
		return ollamaAdapter{}
	case types.APIProviderGemini:
		return geminiAdapter{}
	case types.APIProviderFoundry:
		return foundryAdapter{}
	default:
		// anthropic, bedrock, vertex, workers-ai, and any unknown provider use
		// the Anthropic Messages wire format (matches the prior switch default).
		return anthropicAdapter{}
	}
}

// ---------------------------------------------------------------------------
// Anthropic (and the default for bedrock/vertex/workers-ai/unknown)
// ---------------------------------------------------------------------------

type anthropicAdapter struct{}

func (anthropicAdapter) buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error) {
	return c.buildAnthropicRequestBody(req)
}

func (anthropicAdapter) modelEndpoint(c *Client, _ string) string {
	return c.baseURL + "/v1/messages"
}

func (anthropicAdapter) requestEndpoint(c *Client, req types.APIRequest) string {
	return c.buildModelEndpoint(req.Model.ProviderModelName())
}

func (anthropicAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (anthropicAdapter) decodeResponse(_ *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return decodeAnthropicResponse(body, model)
}

func (anthropicAdapter) createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	return c.createAnthropicStreamResult(ctx, req, onChunk)
}

// ---------------------------------------------------------------------------
// Azure AI Foundry — Anthropic wire format with Azure-style auth headers.
// ---------------------------------------------------------------------------

type foundryAdapter struct{ anthropicAdapter }

func (foundryAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	if c.providerConfig != nil && c.providerConfig.Region != "" {
		req.Header.Set("Anthropic-Foundry-Resource-Id", c.providerConfig.Region)
	}
}

// ---------------------------------------------------------------------------
// z-ai (api.z.ai) — OpenAI-compat body format but x-api-key auth header.
// The api.z.ai endpoint uses Anthropic-style authentication (x-api-key)
// while the request/response body follows the OpenAI /chat/completions format.
// ---------------------------------------------------------------------------

type zAiAdapter struct{ openAICompatAdapter }

func (zAiAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
}

// ---------------------------------------------------------------------------
// OpenAI-compatible /chat/completions (openai, minimax, openrouter, mistral)
// ---------------------------------------------------------------------------

type openAICompatAdapter struct{}

func (openAICompatAdapter) buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error) {
	return c.buildOpenAIRequestBody(req)
}

func (openAICompatAdapter) modelEndpoint(c *Client, _ string) string {
	return c.baseURL + "/chat/completions"
}

func (openAICompatAdapter) requestEndpoint(c *Client, req types.APIRequest) string {
	return c.buildModelEndpoint(req.Model.ProviderModelName())
}

func (openAICompatAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func (openAICompatAdapter) decodeResponse(_ *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return decodeOpenAIResponse(body, model)
}

func (openAICompatAdapter) createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	return c.createOpenAIStreamResult(ctx, req, onChunk)
}

// ---------------------------------------------------------------------------
// Codex — OpenAI Responses API via chatgpt.com (OAuth bearer).
// ---------------------------------------------------------------------------

type codexAdapter struct{}

func (codexAdapter) buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error) {
	return c.buildCodexRequestBody(req)
}

func (codexAdapter) modelEndpoint(c *Client, _ string) string {
	return c.baseURL + "/responses"
}

func (codexAdapter) requestEndpoint(c *Client, req types.APIRequest) string {
	return c.buildModelEndpoint(req.Model.ProviderModelName())
}

func (codexAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("User-Agent", "codex_cli_rs/0.1.0 (Linux; x86_64) xterm")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("Accept", "text/event-stream")
}

// decodeResponse mirrors the former decodeProviderResponse switch, which grouped
// codex with the OpenAI decoder. In practice codex is always streamed.
func (codexAdapter) decodeResponse(_ *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return decodeOpenAIResponse(body, model)
}

func (codexAdapter) createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	return c.createCodexStreamResult(ctx, req, onChunk)
}

// ---------------------------------------------------------------------------
// Ollama — local /api/chat.
// ---------------------------------------------------------------------------

type ollamaAdapter struct{}

func (ollamaAdapter) buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error) {
	return c.buildOllamaRequestBody(req)
}

func (ollamaAdapter) modelEndpoint(c *Client, _ string) string {
	return c.baseURL + "/api/chat"
}

func (ollamaAdapter) requestEndpoint(c *Client, req types.APIRequest) string {
	return c.buildModelEndpoint(req.Model.ProviderModelName())
}

func (ollamaAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func (ollamaAdapter) decodeResponse(_ *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return decodeOllamaResponse(body, model)
}

func (ollamaAdapter) createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	return c.createOllamaStreamResult(ctx, req, onChunk)
}

// ---------------------------------------------------------------------------
// Gemini — generativelanguage.googleapis.com (distinct stream endpoint).
// ---------------------------------------------------------------------------

type geminiAdapter struct{}

func (geminiAdapter) buildRequestBody(c *Client, req types.APIRequest) (io.Reader, error) {
	return c.buildGeminiRequestBody(req)
}

func (geminiAdapter) modelEndpoint(c *Client, model string) string {
	return c.baseURL + "/models/" + model + ":generateContent"
}

func (geminiAdapter) requestEndpoint(c *Client, req types.APIRequest) string {
	model := req.Model.ProviderModelName()
	if req.Stream {
		return c.baseURL + "/models/" + model + ":streamGenerateContent?alt=sse"
	}
	return c.buildModelEndpoint(model)
}

func (geminiAdapter) applyAuthHeaders(c *Client, req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

func (geminiAdapter) decodeResponse(_ *Client, body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return decodeGeminiResponse(body, model)
}

func (geminiAdapter) createStreamResult(c *Client, ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	return c.createGeminiStreamResult(ctx, req, onChunk)
}
