package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Config is the transport-facing provider configuration subset.
type Config struct {
	Provider  types.APIProvider
	BaseURL   string
	Region    string
	ProjectID string
}

// Transport is an interface for provider-specific API transports.
type Transport interface {
	CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error)
	CreateMessageStream(ctx context.Context, req types.APIRequest) (<-chan types.APIResponseChunk, error)
	Close() error
}

// NewTransport creates a transport for the given provider.
func NewTransport(apiKey string, config *Config) (Transport, error) {
	if config == nil {
		config = &Config{Provider: types.APIProviderAnthropic}
	}

	switch config.Provider {
	case types.APIProviderBedrock:
		return NewBedrockTransport(apiKey, config)
	case types.APIProviderVertex:
		return NewVertexTransport(apiKey, config)
	case types.APIProviderFoundry:
		return NewFoundryTransport(apiKey, config)
	default:
		return nil, nil
	}
}

// BedrockTransport uses AWS Bedrock for Anthropic models.
type BedrockTransport struct {
	apiKey     string
	config     *Config
	httpClient *http.Client
}

func NewBedrockTransport(apiKey string, config *Config) (*BedrockTransport, error) {
	return &BedrockTransport{
		apiKey:     apiKey,
		config:     config,
		httpClient: NewHTTPClient(nil, 10*time.Minute),
	}, nil
}

func (t *BedrockTransport) CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error) {
	body := t.buildRequestBody(req)
	awsCreds := t.getAWSCredentials()
	endpoint := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke", regionFromConfig(t.config), req.Model.ProviderModelName())

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}

	t.signRequest(httpReq, awsCreds, body)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, t.handleErrorResponse(resp)
	}

	return t.parseResponse(resp.Body)
}

func (t *BedrockTransport) buildRequestBody(req types.APIRequest) io.Reader {
	body := map[string]any{
		"modelId":   req.Model.ProviderModelName(),
		"messages":  req.Messages,
		"maxTokens": req.MaxTokens,
	}

	if len(req.SystemPromptBlocks) > 0 {
		body["system"] = types.FlattenSystemPromptBlocks(req.SystemPromptBlocks)
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}

	data, _ := json.Marshal(body)
	return bytes.NewReader(data)
}

func (t *BedrockTransport) getAWSCredentials() *awsCredentials {
	return &awsCredentials{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
	}
}

type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

func (t *BedrockTransport) signRequest(req *http.Request, creds *awsCredentials, body io.Reader) {
	req.Header.Set("Content-Type", "application/json")

	if bearerToken := os.Getenv("AWS_BEARER_TOKEN_BEDROCK"); bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
		return
	}

	if creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		// AWS SigV4 signing is not yet implemented.
		// Setting AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY without
		// AWS_BEARER_TOKEN_BEDROCK will produce unsigned requests that AWS
		// rejects with HTTP 403. Rather than silently failing, we inject a
		// header that makes the misconfiguration visible in logs.
		req.Header.Set("X-Nexus-Bedrock-Auth-Warning", "SigV4-not-implemented: set AWS_BEARER_TOKEN_BEDROCK or remove AWS credentials")
	}
}

func (t *BedrockTransport) parseResponse(body io.Reader) (*types.APIResponse, error) {
	var resp struct {
		OutputText string `json:"outputText"`
		StopReason string `json:"stopReason"`
		Usage      struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}

	return &types.APIResponse{
		Content: []types.ContentBlock{
			types.TextContent{Text: resp.OutputText},
		},
		StopReason: resp.StopReason,
		Usage: types.TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func (t *BedrockTransport) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return types.NewError(types.ErrCodeAPIRequest, fmt.Sprintf("Bedrock error: %s", string(body)))
}

func (t *BedrockTransport) CreateMessageStream(ctx context.Context, req types.APIRequest) (<-chan types.APIResponseChunk, error) {
	body := buildAnthropicStreamBody(req)
	awsCreds := t.getAWSCredentials()
	endpoint := fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke-with-response-stream",
		regionFromConfig(t.config),
		req.Model.ProviderModelName(),
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}
	httpReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	t.signRequest(httpReq, awsCreds, body)

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, t.handleErrorResponse(resp)
	}

	ch := make(chan types.APIResponseChunk, 16)
	go readBedrockEventStream(ctx, resp.Body, ch)
	return ch, nil
}

func (t *BedrockTransport) Close() error {
	return nil
}

func regionFromConfig(cfg *Config) string {
	if cfg != nil && cfg.Region != "" {
		return cfg.Region
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		return "us-east-1"
	}
	return region
}

// FoundryTransport uses Azure Foundry for Anthropic models.
type FoundryTransport struct {
	apiKey     string
	config     *Config
	httpClient *http.Client
}

func NewFoundryTransport(apiKey string, config *Config) (*FoundryTransport, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("ANTHROPIC_FOUNDRY_BASE_URL")
		if baseURL == "" {
			baseURL = "https://your-resource.services.ai.azure.com/anthropic/v1"
		}
	}

	config = cloneConfig(config)
	config.BaseURL = baseURL

	return &FoundryTransport{
		apiKey:     apiKey,
		config:     config,
		httpClient: NewHTTPClient(nil, 10*time.Minute),
	}, nil
}

func (t *FoundryTransport) CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error) {
	body := t.buildRequestBody(req)

	resourceID := t.config.Region
	if resourceID == "" {
		resourceID = os.Getenv("ANTHROPIC_FOUNDRY_RESOURCE")
	}

	endpoint := fmt.Sprintf("%s/messages", t.config.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	if t.apiKey != "" {
		httpReq.Header.Set("api-key", t.apiKey)
	} else if apiKey := os.Getenv("ANTHROPIC_FOUNDRY_API_KEY"); apiKey != "" {
		httpReq.Header.Set("api-key", apiKey)
	}

	if resourceID != "" {
		httpReq.Header.Set("Anthropic-Foundry-Resource-Id", resourceID)
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, t.handleErrorResponse(resp)
	}

	return t.parseResponse(resp.Body)
}

func (t *FoundryTransport) buildRequestBody(req types.APIRequest) io.Reader {
	body := map[string]any{
		"model":      req.Model.ProviderModelName(),
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
	}

	if len(req.SystemPromptBlocks) > 0 {
		body["system"] = types.FlattenSystemPromptBlocks(req.SystemPromptBlocks)
	}

	data, _ := json.Marshal(body)
	return bytes.NewReader(data)
}

func (t *FoundryTransport) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return types.NewError(types.ErrCodeAPIRequest, fmt.Sprintf("Foundry error: %s", string(body)))
}

func (t *FoundryTransport) parseResponse(body io.Reader) (*types.APIResponse, error) {
	var resp struct {
		Content    []map[string]any `json:"content"`
		StopReason string           `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}

	content := make([]types.ContentBlock, 0)
	for _, c := range resp.Content {
		if c["type"] == "text" {
			if text, ok := c["text"].(string); ok {
				content = append(content, types.TextContent{Text: text})
			}
		}
	}

	return &types.APIResponse{
		Content:    content,
		StopReason: resp.StopReason,
		Usage: types.TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

func (t *FoundryTransport) CreateMessageStream(ctx context.Context, req types.APIRequest) (<-chan types.APIResponseChunk, error) {
	body := buildAnthropicStreamBody(req)

	resourceID := t.config.Region
	if resourceID == "" {
		resourceID = os.Getenv("ANTHROPIC_FOUNDRY_RESOURCE")
	}

	endpoint := fmt.Sprintf("%s/messages", t.config.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if t.apiKey != "" {
		httpReq.Header.Set("api-key", t.apiKey)
	} else if apiKey := os.Getenv("ANTHROPIC_FOUNDRY_API_KEY"); apiKey != "" {
		httpReq.Header.Set("api-key", apiKey)
	}
	if resourceID != "" {
		httpReq.Header.Set("Anthropic-Foundry-Resource-Id", resourceID)
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, t.handleErrorResponse(resp)
	}

	ch := make(chan types.APIResponseChunk, 16)
	go streamSSEResponse(ctx, resp.Body, ch)
	return ch, nil
}

func (t *FoundryTransport) Close() error {
	return nil
}

// VertexTransport uses Google Vertex AI for Anthropic models.
type VertexTransport struct {
	apiKey     string
	config     *Config
	httpClient *http.Client
}

func NewVertexTransport(apiKey string, config *Config) (*VertexTransport, error) {
	projectID := config.ProjectID
	if projectID == "" {
		projectID = os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	}

	region := config.Region
	if region == "" {
		region = os.Getenv("CLOUD_ML_REGION")
		if region == "" {
			region = "us-east5"
		}
	}

	config = cloneConfig(config)
	config.ProjectID = projectID
	config.Region = region

	return &VertexTransport{
		apiKey:     apiKey,
		config:     config,
		httpClient: NewHTTPClient(nil, 10*time.Minute),
	}, nil
}

func (t *VertexTransport) CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error) {
	projectID := t.config.ProjectID
	if projectID == "" {
		projectID = os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	}

	body := t.buildRequestBody(req)
	modelName := req.Model.ProviderModelName()

	endpoint := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:predict",
		regionFromConfig(t.config),
		projectID,
		regionFromConfig(t.config),
		modelName,
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, t.handleErrorResponse(resp)
	}

	return t.parseResponse(resp.Body)
}

func (t *VertexTransport) buildRequestBody(req types.APIRequest) io.Reader {
	body := map[string]any{
		"instances": []map[string]any{
			{"messages": req.Messages},
		},
		"parameters": map[string]any{
			"maxTokens":   req.MaxTokens,
			"temperature": req.Temperature,
		},
	}

	data, _ := json.Marshal(body)
	return bytes.NewReader(data)
}

func (t *VertexTransport) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return types.NewError(types.ErrCodeAPIRequest, fmt.Sprintf("Vertex error: %s", string(body)))
}

func (t *VertexTransport) parseResponse(body io.Reader) (*types.APIResponse, error) {
	var resp struct {
		Predictions []struct {
			Content string `json:"content"`
		} `json:"predictions"`
		Metadata struct {
			TokenUsage struct {
				InputTokenCount  int `json:"totalTokenCount"`
				OutputTokenCount int `json:"outputTokenCount"`
			} `json:"tokenUsage"`
		} `json:"metadata"`
	}

	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}

	content := make([]types.ContentBlock, 0)
	for _, p := range resp.Predictions {
		content = append(content, types.TextContent{Text: p.Content})
	}

	return &types.APIResponse{
		Content:    content,
		StopReason: "end_turn",
		Usage: types.TokenUsage{
			InputTokens:  resp.Metadata.TokenUsage.InputTokenCount,
			OutputTokens: resp.Metadata.TokenUsage.OutputTokenCount,
		},
	}, nil
}

func (t *VertexTransport) CreateMessageStream(ctx context.Context, req types.APIRequest) (<-chan types.APIResponseChunk, error) {
	projectID := t.config.ProjectID
	if projectID == "" {
		projectID = os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	}

	body := buildAnthropicStreamBody(req)
	region := regionFromConfig(t.config)
	endpoint := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:streamRawPredict",
		region, projectID, region, req.Model.ProviderModelName(),
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to create HTTP request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if t.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)
	} else if apiKey := os.Getenv("ANTHROPIC_VERTEX_API_KEY"); apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeAPIRequest, "failed to send request", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, t.handleErrorResponse(resp)
	}

	ch := make(chan types.APIResponseChunk, 16)
	go streamSSEResponse(ctx, resp.Body, ch)
	return ch, nil
}

func (t *VertexTransport) Close() error {
	return nil
}

func cloneConfig(config *Config) *Config {
	if config == nil {
		return &Config{}
	}
	cloned := *config
	return &cloned
}
