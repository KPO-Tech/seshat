package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (c *Client) buildOllamaRequestBody(req types.APIRequest) (io.Reader, error) {
	body := map[string]any{
		"model":    req.Model.ProviderModelName(),
		"messages": c.buildOpenAIMessages(req),
		"stream":   req.Stream,
	}
	if len(req.Tools) > 0 {
		body["tools"] = openAITools(req.Tools)
	}
	options := map[string]any{}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if req.Temperature != nil {
		options["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		options["top_p"] = *req.TopP
	}
	if len(options) > 0 {
		body["options"] = options
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func decodeOllamaResponse(body io.Reader, model types.ModelIdentifier) (types.APIResponse, error) {
	var resp struct {
		Model   string `json:"model"`
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return types.APIResponse{}, err
	}
	content := make([]types.ContentBlock, 0, 1+len(resp.Message.ToolCalls))
	if resp.Message.Content != "" {
		content = append(content, types.TextContent{Text: resp.Message.Content})
	}
	for idx, call := range resp.Message.ToolCalls {
		content = append(content, types.ToolUseContent{
			ID:    fmt.Sprintf("ollama-tool-%d", idx+1),
			Name:  call.Function.Name,
			Input: call.Function.Arguments,
		})
	}
	return types.APIResponse{
		Role:       types.RoleAssistant,
		Content:    content,
		StopReason: types.NormalizeStopReason("", content),
		Usage: types.TokenUsage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
		Model: model,
		ID:    "ollama-response",
	}, nil
}

func (c *Client) createOllamaStreamResult(ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	req.Stream = true
	resp, err := c.sendMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp, nil)
	}

	var text strings.Builder
	content := make([]types.ContentBlock, 0)
	collected := make([]types.APIResponseChunk, 0)
	toolIndex := 0
	usage := types.TokenUsage{}
	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if text.Len() > 0 {
						content = append([]types.ContentBlock{types.TextContent{Text: text.String()}}, content...)
					}
					stopChunk := types.APIResponseChunk{
						Type:       types.APIChunkTypeMessageStop,
						StopReason: stringPtr(types.NormalizeStopReason("", content)),
						Usage:      &usage,
					}
					emitStreamChunk(onChunk, stopChunk)
					collected = append(collected, stopChunk)
					response := types.APIResponse{
						Role:       types.RoleAssistant,
						Content:    content,
						StopReason: types.NormalizeStopReason("", content),
						Usage:      usage,
						Model:      req.Model,
						ID:         "ollama-stream-response",
					}
					return &types.APIStreamResult{Response: response, Chunks: collected}, nil
				}
				return nil, types.WrapError(types.ErrCodeAPIResponse, "failed to read ollama stream", err)
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var chunk struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Function struct {
							Name      string         `json:"name"`
							Arguments map[string]any `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
				PromptEvalCount int `json:"prompt_eval_count"`
				EvalCount       int `json:"eval_count"`
			}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				return nil, types.WrapError(types.ErrCodeAPIResponse, "failed to decode ollama stream chunk", err)
			}
			if chunk.Message.Content != "" {
				text.WriteString(chunk.Message.Content)
				streamChunk := types.APIResponseChunk{
					Type:      types.APIChunkTypeContentBlockDelta,
					Delta:     chunk.Message.Content,
					DeltaType: "text_delta",
				}
				emitStreamChunk(onChunk, streamChunk)
				collected = append(collected, streamChunk)
			}
			if chunk.PromptEvalCount > 0 {
				usage.InputTokens = chunk.PromptEvalCount
			}
			if chunk.EvalCount > 0 {
				usage.OutputTokens = chunk.EvalCount
			}
			for _, toolCall := range chunk.Message.ToolCalls {
				toolIndex++
				toolUse := types.ToolUseContent{
					ID:    fmt.Sprintf("ollama-tool-%d", toolIndex),
					Name:  toolCall.Function.Name,
					Input: toolCall.Function.Arguments,
				}
				content = append(content, toolUse)
				streamChunk := types.APIResponseChunk{
					Type:         types.APIChunkTypeContentBlockStart,
					ContentBlock: toolUse,
				}
				emitStreamChunk(onChunk, streamChunk)
				collected = append(collected, streamChunk)
			}
		}
	}
}
