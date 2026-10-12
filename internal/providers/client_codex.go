package providers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"context"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// ============================================================================
// Codex provider — chatgpt.com/backend-api/responses (OpenAI Responses API)
// ============================================================================
//
// Auth: Bearer token from OAuth (ChatGPT Pro subscription).
// Wire format: OpenAI Responses API, NOT Chat Completions.
//   - Request:  {"model":"...","instructions":"...","input":[...],"stream":true,"store":false}
//   - SSE events: response.output_text.delta, response.output_item.done, response.completed, response.failed

func (c *Client) buildCodexRequestBody(req types.APIRequest) (io.Reader, error) {
	// "store" MUST be false: confirmed live against a real ChatGPT-account
	// session, the chatgpt.com/backend-api/codex backend this provider talks
	// to rejects "store": true outright ({"detail":"Store must be set to
	// false"}) - unlike the standard OpenAI Responses API, which this
	// provider does not use (see the file header: this always targets the
	// ChatGPT-account backend, never api.openai.com).
	//
	// An earlier version of this function set "store": true to let each call
	// become the anchor a later iteration continues from via
	// previous_response_id, avoiding resending the whole growing message
	// history every iteration of a tool-use loop (the previous_response_id
	// fields on APIRequest/MutableState, and engine/loop.go's
	// recordPreviousResponse/buildAPIRequest wiring, are still present but
	// inert now - recordPreviousResponse never populates them for Codex, so
	// this function's previous_response_id branch below never fires. Kept
	// rather than torn out in case a store-compatible path for this backend
	// is found later.
	previousResponseID := ""
	if req.PreviousResponseID != "" && req.PreviousResponseMessageCount <= len(req.Messages) {
		previousResponseID = req.PreviousResponseID
		req.Messages = req.Messages[req.PreviousResponseMessageCount:]
	}

	input := c.buildCodexInput(req)

	body := map[string]any{
		"model":  req.Model.ProviderModelName(),
		"input":  input,
		"stream": true,
		"store":  false,
	}
	if previousResponseID != "" {
		body["previous_response_id"] = previousResponseID
	}

	systemPrompt := req.SystemPrompt
	if systemPrompt == "" && len(req.SystemPromptBlocks) > 0 {
		systemPrompt = types.FlattenSystemPromptBlocks(req.SystemPromptBlocks)
	}
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = "You are a helpful assistant."
	}
	body["instructions"] = systemPrompt

	// Every model behind this provider (gpt-5.6-sol, gpt-5.5, gpt-5.4-mini —
	// see registry.go's APIProviderCodex entry) is a reasoning model served
	// through the ChatGPT-account Responses API, which rejects `temperature`
	// outright ("Unsupported parameter: temperature") rather than just
	// ignoring it. Silently dropping it here regardless of what callers set
	// is scoped to this provider specifically — some other providers'
	// models do accept it.
	if len(req.Tools) > 0 {
		body["tools"] = codexTools(req.Tools)
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// buildCodexInput converts Seshat messages to the Responses API input item list.
// The Responses API uses a flat list of typed items:
//   - user/assistant text: {"type":"message","role":"user|assistant","content":[{"type":"input_text|output_text","text":"..."}]}
//   - function call (assistant):  {"type":"function_call","call_id":"...","name":"...","arguments":"..."}
//   - function result (user):     {"type":"function_call_output","call_id":"...","output":"..."}
func (c *Client) buildCodexInput(req types.APIRequest) []map[string]any {
	items := make([]map[string]any, 0, len(req.Messages))

	for _, msg := range req.Messages {
		switch msg.Role {
		case types.RoleUser:
			// Check if this message contains tool results.
			toolResults := toolResultsFromMessage(msg)
			if len(toolResults) > 0 {
				for _, result := range toolResults {
					output := result.Content
					items = append(items, map[string]any{
						"type":    "function_call_output",
						"call_id": result.ToolUseID,
						"output":  output,
					})
				}
			} else {
				text := joinTextBlocks(msg.Content)
				items = append(items, map[string]any{
					"type": "message",
					"role": "user",
					"content": []map[string]any{
						{"type": "input_text", "text": text},
					},
				})
			}

		case types.RoleAssistant:
			// Assistant turn may include text content and/or tool calls.
			// We emit function_call items first, then text.
			for _, block := range msg.Content {
				switch b := block.(type) {
				case types.ToolUseContent:
					args, _ := json.Marshal(b.Input)
					items = append(items, map[string]any{
						"type":      "function_call",
						"call_id":   b.ID,
						"name":      b.Name,
						"arguments": string(args),
					})
				case types.TextContent:
					if strings.TrimSpace(b.Text) != "" {
						items = append(items, map[string]any{
							"type": "message",
							"role": "assistant",
							"content": []map[string]any{
								{"type": "output_text", "text": b.Text},
							},
						})
					}
				}
			}

		case types.RoleSystem:
			// System messages in the conversation history (uncommon) — emit as user messages
			// because the Responses API uses the top-level `instructions` field for system prompts.
			text := joinTextBlocks(msg.Content)
			if strings.TrimSpace(text) != "" {
				items = append(items, map[string]any{
					"type": "message",
					"role": "user",
					"content": []map[string]any{
						{"type": "input_text", "text": text},
					},
				})
			}
		}
	}

	return items
}

// codexTools converts Seshat tool definitions to the Responses API flat tool format.
func codexTools(tools []types.APIToolDefinition) []map[string]any {
	defs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.InputSchema,
			"strict":      false,
		})
	}
	return defs
}

// ============================================================================
// Codex SSE streaming
// ============================================================================

func (c *Client) createCodexStreamResult(ctx context.Context, req types.APIRequest, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	req.Stream = true
	resp, err := c.sendMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp, nil)
	}

	return parseCodexSSEStream(ctx, resp.Body, req.Model, onChunk)
}

// parseCodexSSEStream reads a Codex Responses API body (the "event: ...\ndata:
// {...}" SSE shape) to completion and aggregates it into a single
// APIStreamResult. Shared by createCodexStreamResult above (onChunk fires as
// events arrive, for real streaming) and codexAdapter.decodeResponse below
// (onChunk is nil - emitStreamChunk no-ops on a nil callback). Codex's
// Responses API answers in this SSE shape unconditionally (see
// buildCodexRequestBody's hardcoded "stream": true and CreateMessage's own
// "Z.ai/GLM and Codex only support streaming" branch) - there is no separate
// plain-JSON wire format for non-streaming callers to decode, so
// non-streaming callers still need this same SSE reader, just without a
// live callback.
//
// response.completed's usage object also carries input_tokens_details.
// cached_tokens and output_tokens_details.reasoning_tokens (Responses API
// shape - distinct from Chat Completions' prompt_tokens_details that
// parseTokenUsage in helpers.go handles for other providers). Without
// reading these, a reasoning model's lopsided input:output token ratio is
// unexplainable from telemetry alone - there was previously no way to tell
// whether it came from resent context, a cache miss, or genuine hidden
// reasoning.
func parseCodexSSEStream(ctx context.Context, body io.Reader, model types.ModelIdentifier, onChunk func(types.APIResponseChunk)) (*types.APIStreamResult, error) {
	var text strings.Builder
	toolCalls := make(map[string]*openAIStreamToolCallState) // keyed by call_id
	callOrder := make([]string, 0)
	collected := make([]types.APIResponseChunk, 0)
	usage := types.TokenUsage{}
	responseID := "codex-stream"

	reader := bufio.NewReader(body)
streamLoop:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// Stream closed before response.completed — return what we have.
					break streamLoop
				}
				return nil, types.WrapError(types.ErrCodeAPIResponse, "failed to read codex stream", err)
			}
			line = strings.TrimSpace(line)
			if line == "" || line == "event: " {
				continue
			}
			// Skip "event: ..." lines — we parse the type from the "data: ..." JSON payload.
			if strings.HasPrefix(line, "event:") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" || data == "" {
				continue
			}

			var event struct {
				Type string `json:"type"`
				// response.output_text.delta
				Delta        string `json:"delta"`
				ContentIndex int    `json:"content_index"`
				ItemID       string `json:"item_id"`
				// response.output_item.done
				Item *struct {
					Type      string `json:"type"`
					Role      string `json:"role"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
					Content   []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
				// response.completed / response.failed
				Response *struct {
					ID    string `json:"id"`
					Error *struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
					Usage *struct {
						InputTokens        int `json:"input_tokens"`
						OutputTokens       int `json:"output_tokens"`
						InputTokensDetails *struct {
							CachedTokens int `json:"cached_tokens"`
						} `json:"input_tokens_details"`
						OutputTokensDetails *struct {
							ReasoningTokens int `json:"reasoning_tokens"`
						} `json:"output_tokens_details"`
					} `json:"usage"`
				} `json:"response"`
			}

			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "response.created":
				if event.Response != nil && event.Response.ID != "" {
					responseID = event.Response.ID
				}

			case "response.output_text.delta":
				if event.Delta != "" {
					text.WriteString(event.Delta)
					chunk := types.APIResponseChunk{
						Type:      types.APIChunkTypeContentBlockDelta,
						Delta:     event.Delta,
						DeltaType: "text_delta",
					}
					emitStreamChunk(onChunk, chunk)
					collected = append(collected, chunk)
				}

			case "response.output_item.done":
				if event.Item == nil {
					continue
				}
				switch event.Item.Type {
				case "function_call":
					callID := event.Item.CallID
					if callID == "" {
						callID = fmt.Sprintf("codex-call-%d", len(callOrder)+1)
					}
					state := &openAIStreamToolCallState{
						ID:   callID,
						Name: event.Item.Name,
					}
					state.Args.WriteString(event.Item.Arguments)
					toolCalls[callID] = state
					callOrder = append(callOrder, callID)

					startChunk := types.APIResponseChunk{
						Type: types.APIChunkTypeContentBlockStart,
						ContentBlock: types.ToolUseContent{
							ID:   callID,
							Name: event.Item.Name,
						},
					}
					emitStreamChunk(onChunk, startChunk)
					collected = append(collected, startChunk)

					if event.Item.Arguments != "" {
						deltaChunk := types.APIResponseChunk{
							Type:        types.APIChunkTypeContentBlockDelta,
							DeltaType:   "input_json_delta",
							PartialJSON: event.Item.Arguments,
						}
						emitStreamChunk(onChunk, deltaChunk)
						collected = append(collected, deltaChunk)
					}

					stopChunk := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockStop}
					emitStreamChunk(onChunk, stopChunk)
					collected = append(collected, stopChunk)
				}

			case "response.completed":
				if event.Response != nil {
					if event.Response.ID != "" {
						responseID = event.Response.ID
					}
					if event.Response.Usage != nil {
						usage.InputTokens = event.Response.Usage.InputTokens
						usage.OutputTokens = event.Response.Usage.OutputTokens
						if details := event.Response.Usage.InputTokensDetails; details != nil {
							usage.CachedTokens = details.CachedTokens
						}
						if details := event.Response.Usage.OutputTokensDetails; details != nil {
							usage.ReasoningTokens = details.ReasoningTokens
						}
					}
				}

				stopReason := "end_turn"
				if len(toolCalls) > 0 {
					stopReason = types.StopReasonToolUse
				}
				stopChunk := types.APIResponseChunk{
					Type:       types.APIChunkTypeMessageStop,
					StopReason: stringPtr(stopReason),
					Usage:      &usage,
				}
				emitStreamChunk(onChunk, stopChunk)
				collected = append(collected, stopChunk)
				return buildCodexStreamResult(model, responseID, text.String(), toolCalls, callOrder, usage, collected)

			case "response.failed":
				if event.Response != nil && event.Response.Error != nil {
					return nil, types.NewError(types.ErrCodeAPIResponse,
						fmt.Sprintf("codex response failed: %s: %s",
							event.Response.Error.Code, event.Response.Error.Message))
				}
				return nil, types.NewError(types.ErrCodeAPIResponse, "codex response.failed event received")

			case "response.incomplete":
				stopChunk := types.APIResponseChunk{
					Type:       types.APIChunkTypeMessageStop,
					StopReason: stringPtr(types.StopReasonMaxTokens),
					Usage:      &usage,
				}
				emitStreamChunk(onChunk, stopChunk)
				collected = append(collected, stopChunk)
				return buildCodexStreamResult(model, responseID, text.String(), toolCalls, callOrder, usage, collected)
			}
		}
	}
	// Reached after EOF break — return accumulated state.
	return buildCodexStreamResult(model, responseID, text.String(), toolCalls, callOrder, usage, collected)
}

func buildCodexStreamResult(
	model types.ModelIdentifier,
	responseID string,
	text string,
	toolCalls map[string]*openAIStreamToolCallState,
	callOrder []string,
	usage types.TokenUsage,
	collected []types.APIResponseChunk,
) (*types.APIStreamResult, error) {
	content := make([]types.ContentBlock, 0, 1+len(callOrder))
	if text != "" {
		content = append(content, types.TextContent{Text: text})
	}
	for _, callID := range callOrder {
		call := toolCalls[callID]
		if call == nil {
			continue
		}
		input := make(map[string]any)
		if raw := strings.TrimSpace(call.Args.String()); raw != "" {
			if err := json.Unmarshal([]byte(raw), &input); err != nil {
				return nil, types.WrapError(types.ErrCodeAPIResponse, "failed to decode codex tool arguments", err)
			}
		}
		content = append(content, types.ToolUseContent{ID: callID, Name: call.Name, Input: input})
	}

	stopReason := types.StopReasonEndTurn
	if types.ContentBlocksContainToolUse(content) {
		stopReason = types.StopReasonToolUse
	}

	response := types.APIResponse{
		Role:       types.RoleAssistant,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
		Model:      model,
		ID:         responseID,
	}
	return &types.APIStreamResult{Response: response, Chunks: collected}, nil
}
