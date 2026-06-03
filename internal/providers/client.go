package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// codexJar is a process-global cookie jar for chatgpt.com Cloudflare cookies
// (cf_clearance, __cf_bm, __cflb, _cfuvid). Keeping it alive across requests
// lets the CF cookie flow work without re-triggering the bot challenge.
var (
	codexJarOnce sync.Once
	codexJar     http.CookieJar
)

func getCodexHTTPClient() *http.Client {
	codexJarOnce.Do(func() {
		if jar, err := cookiejar.New(nil); err == nil {
			codexJar = jar
		}
	})
	return &http.Client{
		Jar:     codexJar,
		Timeout: 120 * time.Second,
	}
}

// Client represents an API client
type Client struct {
	// apiKey is the API key
	apiKey string

	// baseURL is the base URL for the API
	baseURL string

	// httpClient is the HTTP client to use
	httpClient *http.Client

	// retryConfig is the retry configuration
	retryConfig types.RetryConfig

	// provider is the API provider
	provider types.APIProvider

	// providerConfig holds the provider-specific configuration
	providerConfig *Config

	// circuitBreaker provides fault tolerance
	circuitBreaker *CircuitBreaker

	// monitoring provides centralized metrics and logging
	monitoring *monitoring.System

	// adapter encapsulates the provider-specific wire format (request body,
	// endpoint, auth headers, response decode, streaming). It replaces the
	// per-provider switch statements that used to live in this file.
	adapter providerAdapter
}

// resolveAdapter returns the provider wire-format adapter, falling back to a
// provider-derived adapter if the client was constructed without one (defensive
// against struct-literal construction paths outside the standard constructors).
func (c *Client) resolveAdapter() providerAdapter {
	if c.adapter != nil {
		return c.adapter
	}
	return adapterForProvider(c.provider)
}

// NewClient creates a new API client
func NewClient(apiKey string, providerType types.APIProvider) *Client {
	// Create provider config for model resolution
	providerConfig := GetProviderConfig(providerType)
	if providerConfig == nil {
		providerConfig = &Config{Provider: providerType}
	}
	providerConfig.APIKey = apiKey

	var httpClient *http.Client
	if providerType == types.APIProviderCodex {
		httpClient = getCodexHTTPClient()
	} else {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &Client{
		apiKey:         apiKey,
		baseURL:        providerConfig.GetBaseURL(),
		provider:       providerType,
		providerConfig: providerConfig,
		httpClient:     httpClient,
		retryConfig:    types.DefaultRetryConfig(),
		adapter:        adapterForProvider(providerType),
	}
}

// NewClientWithConfig creates a new API client with provider-specific configuration
func NewClientWithConfig(apiKey string, config *Config) *Client {
	return newClientWithConfig(apiKey, config)
}

// NewFallbackClient builds a provider client from the provider-local credential
// sources so the engine can follow configured inter-provider fallback chains.
func NewFallbackClient(ctx context.Context, provider types.APIProvider) (*Client, error) {
	config := GetProviderConfig(provider)
	if config == nil {
		return nil, fmt.Errorf("provider config not found for %q", provider)
	}

	apiKey, err := getAPIKeyForProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	config.APIKey = apiKey

	if err := ValidateProviderConfig(config); err != nil {
		return nil, err
	}

	return newClientWithConfig(apiKey, config), nil
}

func newClientWithConfig(apiKey string, config *Config) *Client {
	baseURL := config.GetBaseURL()
	if baseURL == "" {
		switch config.Provider {
		case types.APIProviderOpenAI:
			baseURL = "https://api.openai.com/v1"
		case types.APIProviderCodex:
			baseURL = "https://chatgpt.com/backend-api/codex"
		case types.APIProviderOllama:
			baseURL = "http://localhost:11434"
		default:
			baseURL = "https://api.anthropic.com"
		}
	}

	var httpClient *http.Client
	if config.Provider == types.APIProviderCodex {
		httpClient = getCodexHTTPClient()
	} else {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &Client{
		apiKey:         apiKey,
		baseURL:        baseURL,
		httpClient:     httpClient,
		retryConfig:    types.DefaultRetryConfig(),
		provider:       config.Provider,
		providerConfig: config,
		adapter:        adapterForProvider(config.Provider),
	}
}

// SetRetryConfig sets the retry configuration
func (c *Client) SetRetryConfig(config types.RetryConfig) {
	c.retryConfig = config
}

// SetMonitoring sets the monitoring system
func (c *Client) SetMonitoring(monitoringSys *monitoring.System) {
	c.monitoring = monitoringSys
}

// SetHTTPClient sets the HTTP client
func (c *Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// Config returns the provider configuration currently bound to the client.
func (c *Client) Config() *Config {
	return c.providerConfig
}

// CreateMessage sends a non-streaming message creation request
func (c *Client) CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error) {
	start := time.Now()

	// Record API request in monitoring
	if c.monitoring != nil {
		c.monitoring.RecordAPIRequest()
	}

	// Z.ai/GLM and Codex only support streaming
	if c.provider == types.APIProviderZAi || c.provider == types.APIProviderCodex {
		req.Stream = true
	} else {
		req.Stream = false
	}
	resp, err := c.sendMessageWithRetry(ctx, req)
	duration := time.Since(start)

	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		// Record API failure in monitoring
		if c.monitoring != nil {
			c.monitoring.RecordAPIFailure(err, duration)
		}
		return nil, err
	}
	defer resp.Body.Close()

	// Check status code first - if error, read error body before trying to decode
	if resp.StatusCode != http.StatusOK {
		// Record API failure in monitoring
		if c.monitoring != nil {
			c.monitoring.RecordAPIFailure(fmt.Errorf("HTTP status %d", resp.StatusCode), duration)
		}
		return nil, c.handleErrorResponse(resp, nil)
	}

	apiResp, err := c.decodeProviderResponse(resp.Body, req.Model)
	if err != nil {
		// Record API failure in monitoring
		if c.monitoring != nil {
			c.monitoring.RecordAPIFailure(err, duration)
		}
		return nil, types.WrapError(types.ErrCodeAPIResponse, "failed to decode response", err)
	}

	// Record API success in monitoring
	if c.monitoring != nil {
		c.monitoring.RecordAPISuccess(duration)
	}

	apiResp.StopReason = types.NormalizeStopReason(apiResp.StopReason, apiResp.Content)

	if c.monitoring != nil {
		c.monitoring.RecordCacheStats(
			apiResp.Usage.CacheReadInputTokens,
			apiResp.Usage.CacheCreationInputTokens,
		)
	}

	return &apiResp, nil
}

// CreateMessageStreamResult consumes the provider stream and returns a canonical aggregated response.
func (c *Client) CreateMessageStreamResult(ctx context.Context, req types.APIRequest) (*types.APIStreamResult, error) {
	return c.CreateMessageStreamResultWithCallback(ctx, req, nil)
}

// CreateMessageStreamResultWithCallback consumes the provider stream, emits
// normalized chunks to the callback, and returns a canonical aggregated response.
func (c *Client) CreateMessageStreamResultWithCallback(ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	req.Stream = true
	return c.resolveAdapter().createStreamResult(c, ctx, req, onChunk)
}

// createAnthropicStreamResult consumes an Anthropic SSE stream and returns the
// canonical aggregated response. It is the default streaming path (anthropic,
// bedrock, vertex, workers-ai) used by anthropicAdapter/foundryAdapter.
func (c *Client) createAnthropicStreamResult(ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	chunks, err := c.CreateMessageStream(ctx, req)
	if err != nil {
		return nil, err
	}

	accumulator := &types.APIChunkAccumulator{}
	collected := make([]types.APIResponseChunk, 0)
	for chunk := range chunks {
		if chunk.Type == types.APIChunkTypeError {
			if chunk.Error != nil {
				return nil, chunk.Error
			}
			return nil, types.NewError(types.ErrCodeAPIResponse, "stream ended with unknown error")
		}
		emitStreamChunk(onChunk, chunk)
		collected = append(collected, chunk)
		accumulator.AddChunk(chunk)
	}

	response := accumulator.Build(req.Model, "stream-response")
	response.StopReason = types.NormalizeStopReason(response.StopReason, response.Content)

	if c.monitoring != nil {
		c.monitoring.RecordCacheStats(
			response.Usage.CacheReadInputTokens,
			response.Usage.CacheCreationInputTokens,
		)
	}

	return &types.APIStreamResult{Response: response, Chunks: collected}, nil
}

func emitStreamChunk(onChunk func(types.APIResponseChunk), chunk types.APIResponseChunk) {
	if onChunk == nil {
		return
	}
	onChunk(chunk)
}

// CreateMessageStream sends a streaming message creation request
func (c *Client) CreateMessageStream(ctx context.Context, req types.APIRequest) (<-chan types.APIResponseChunk, error) {
	// Force streaming for this method
	req.Stream = true

	// Send request
	resp, err := c.sendMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.handleErrorResponse(resp, nil)
	}

	// Create channel for chunks
	chunkChan := make(chan types.APIResponseChunk, 10)

	// Start streaming in goroutine
	go c.streamAnthropicResponse(ctx, resp, chunkChan)

	return chunkChan, nil
}

// sendMessage sends a message request
func (c *Client) sendMessage(ctx context.Context, req types.APIRequest) (*http.Response, error) {
	// Build request body
	body, err := c.buildRequestBody(req)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to build request body", err)
	}

	// Create HTTP request
	endpoint := c.buildRequestEndpoint(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}

	// Set headers
	c.setRequestHeaders(httpReq, req)

	// Send request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}

	return resp, nil
}

// Helper methods

func (c *Client) buildURL(path string) string {
	return c.baseURL + path
}

func (c *Client) buildRequestEndpoint(req types.APIRequest) string {
	return c.resolveAdapter().requestEndpoint(c, req)
}

func (c *Client) buildModelEndpoint(model string) string {
	if c.providerConfig != nil {
		endpoint := c.providerConfig.GetEndpoint(model)
		if endpoint != "" {
			return endpoint
		}
	}
	return c.resolveAdapter().modelEndpoint(c, model)
}

func (c *Client) setRequestHeaders(req *http.Request, apiReq types.APIRequest) {
	req.Header.Set("Content-Type", "application/json")

	// Provider-specific auth/version headers are owned by the adapter.
	c.resolveAdapter().applyAuthHeaders(c, req)

	for key, value := range extractCustomHeaders(apiReq.Metadata) {
		if !isSafeMetadataHeader(key) {
			continue
		}
		req.Header.Set(key, value)
	}
}

func extractCustomHeaders(metadata map[string]any) map[string]string {
	if metadata == nil {
		return nil
	}
	if customHeaders, ok := metadata["headers"].(map[string]string); ok {
		return customHeaders
	}
	rawHeaders, ok := metadata["headers"].(map[string]any)
	if !ok {
		return nil
	}
	headers := make(map[string]string, len(rawHeaders))
	for key, rawValue := range rawHeaders {
		value, ok := rawValue.(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		headers[key] = value
	}
	return headers
}

// anthropicToolsWithCacheControl returns a copy of tools with cache_control
// set on the last entry for Anthropic-compatible providers. Anthropic caches
// all content up to the last cache_control breakpoint, so marking the final
// tool definition causes the entire tool list to be cached across turns.
// For other providers the input slice is returned as-is (no copy, no mutation).
func anthropicToolsWithCacheControl(tools []types.APIToolDefinition, provider types.APIProvider) []types.APIToolDefinition {
	if len(tools) == 0 {
		return tools
	}
	if provider != types.APIProviderAnthropic && provider != types.APIProviderFoundry {
		return tools
	}
	out := make([]types.APIToolDefinition, len(tools))
	copy(out, tools)
	out[len(out)-1].CacheControl = types.NewEphemeralPromptCacheControl()
	return out
}

func isSafeMetadataHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "content-type", "authorization", "x-api-key", "api-key", "anthropic-version":
		return false
	default:
		return key != ""
	}
}

func (c *Client) buildRequestBody(req types.APIRequest) (io.Reader, error) {
	return c.resolveAdapter().buildRequestBody(c, req)
}

// buildAnthropicRequestBody serializes an APIRequest into the Anthropic Messages
// wire format. It is the default request builder (anthropic, foundry, bedrock,
// vertex, workers-ai) used by anthropicAdapter/foundryAdapter.
func (c *Client) buildAnthropicRequestBody(req types.APIRequest) (io.Reader, error) {
	body := map[string]any{
		"model":      req.Model.ProviderModelName(),
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
	}

	// Anthropic-style providers can consume structured system prompt blocks.
	// Other providers currently fall back to the flattened prompt string so the
	// runtime keeps one canonical prompt representation while the transport stays simple.
	if len(req.SystemPromptBlocks) > 0 && c.provider == types.APIProviderAnthropic {
		body["system"] = req.SystemPromptBlocks
	} else if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	} else if len(req.SystemPromptBlocks) > 0 {
		body["system"] = types.FlattenSystemPromptBlocks(req.SystemPromptBlocks)
	}

	if len(req.Tools) > 0 {
		body["tools"] = anthropicToolsWithCacheControl(req.Tools, c.provider)
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopK != nil {
		body["top_k"] = *req.TopK
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if len(req.StopSequences) > 0 {
		body["stop_sequences"] = req.StopSequences
	}
	if req.Stream {
		body["stream"] = true
	}
	// Anthropic has no native structured-output API: inject the schema constraint
	// as a system message suffix so the model knows the required JSON shape.
	if req.OutputSchema != nil {
		hint := req.OutputSchema.SystemPromptHint()
		if existing, ok := body["system"].(string); ok {
			body["system"] = existing + "\n\n" + hint
		} else {
			body["system"] = hint
		}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (c *Client) decodeProviderResponse(body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	return c.resolveAdapter().decodeResponse(c, body, model)
}

// decodeAnthropicResponse decodes an Anthropic Messages JSON response into the
// canonical APIResponse. It is the default decoder (anthropic, foundry, bedrock,
// vertex, workers-ai) used by anthropicAdapter/foundryAdapter.
func decodeAnthropicResponse(body io.Reader, _ types.ModelIdentifier) (types.APIResponse, error) {
	var apiResp types.APIResponse
	if err := json.NewDecoder(body).Decode(&apiResp); err != nil {
		return types.APIResponse{}, err
	}
	return apiResp, nil
}

func (c *Client) handleErrorResponse(resp *http.Response, apiResp *types.APIResponse) error {
	// Read error body
	body, _ := io.ReadAll(resp.Body)

	// Parse error
	var errResp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	_ = json.Unmarshal(body, &errResp)

	// Determine error code
	var errCode types.ErrorCode
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		errCode = types.ErrCodeAPIAuth
	case http.StatusTooManyRequests:
		errCode = types.ErrCodeAPIRateLimit
	case http.StatusBadRequest:
		errCode = types.ErrCodeAPIInvalid
	case http.StatusServiceUnavailable, // 503 — provider down or overloaded
		http.StatusGatewayTimeout,      // 504
		http.StatusBadGateway,          // 502
		http.StatusInternalServerError, // 500
		http.StatusRequestTimeout:      // 408
		errCode = types.ErrCodeAPITimeout
	default:
		if resp.StatusCode == 529 {
			// 529 is Anthropic's non-standard "overloaded" status
			errCode = types.ErrCodeAPITimeout
		} else {
			errCode = types.ErrCodeAPIRequest
		}
	}

	// Create error message
	message := errResp.Error.Message
	if message == "" {
		message = string(body)
	}

	return types.NewError(errCode, message)
}
