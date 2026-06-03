package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// streamAnthropicResponse consumes an Anthropic SSE response and forwards
// normalized chunks to chunkChan. The Anthropic event stream uses
// content_block_start / content_block_delta / content_block_stop /
// message_delta / message_stop events.
func (c *Client) streamAnthropicResponse(ctx context.Context, resp *http.Response, chunkChan chan<- types.APIResponseChunk) {
	defer close(chunkChan)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					chunkChan <- types.APIResponseChunk{
						Type:  types.APIChunkTypeError,
						Error: types.WrapError(types.ErrCodeAPIResponse, "failed to read stream", err),
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			chunk, err := c.parseAnthropicChunk([]byte(data))
			if err != nil {
				chunkChan <- types.APIResponseChunk{
					Type:  types.APIChunkTypeError,
					Error: types.WrapError(types.ErrCodeAPIResponse, "failed to parse stream chunk", err),
				}
				return
			}

			chunkChan <- chunk
		}
	}
}

// parseAnthropicChunk parses a single Anthropic SSE data line into an APIResponseChunk.
func (c *Client) parseAnthropicChunk(data []byte) (types.APIResponseChunk, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return types.APIResponseChunk{}, err
	}

	chunk := types.APIResponseChunk{}

	chunkType, _ := raw["type"].(string)
	switch types.APIChunkType(chunkType) {
	case types.APIChunkTypeContentBlockStart:
		chunk.Type = types.APIChunkTypeContentBlockStart
		if contentBlock, ok := raw["content_block"].(map[string]any); ok {
			chunk.ContentBlock = c.parseAnthropicContentBlock(contentBlock)
		}

	case types.APIChunkTypeContentBlockDelta:
		chunk.Type = types.APIChunkTypeContentBlockDelta
		if delta, ok := raw["delta"].(map[string]any); ok {
			if deltaType, ok := delta["type"].(string); ok {
				chunk.DeltaType = deltaType
			}
			if text, ok := delta["text"].(string); ok {
				chunk.Delta = text
			}
			if partialJSON, ok := delta["partial_json"].(string); ok {
				chunk.PartialJSON = partialJSON
			}
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseTokenUsage(usage)
		}

	case types.APIChunkTypeContentBlockStop:
		chunk.Type = types.APIChunkTypeContentBlockStop
		if usage, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseTokenUsage(usage)
		}

	case types.APIChunkTypeMessageDelta:
		chunk.Type = types.APIChunkTypeMessageDelta
		if delta, ok := raw["delta"].(map[string]any); ok {
			if stopReason, ok := delta["stop_reason"].(string); ok {
				chunk.StopReason = &stopReason
			}
			if stopSequence, ok := delta["stop_sequence"].(string); ok {
				chunk.StopSequence = &stopSequence
			}
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseTokenUsage(usage)
		}

	case types.APIChunkTypeMessageStop:
		chunk.Type = types.APIChunkTypeMessageStop
		if stopReason, ok := raw["stop_reason"].(string); ok {
			chunk.StopReason = &stopReason
		}
		if stopSequence, ok := raw["stop_sequence"].(string); ok {
			chunk.StopSequence = &stopSequence
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseTokenUsage(usage)
		}

	default:
		chunk.Type = types.APIChunkType(chunkType)
	}

	return chunk, nil
}

// parseAnthropicContentBlock parses a content block from an Anthropic API response.
func (c *Client) parseAnthropicContentBlock(raw map[string]any) types.ContentBlock {
	blockType, _ := raw["type"].(string)
	switch types.ContentType(blockType) {
	case types.ContentTypeText:
		if text, ok := raw["text"].(string); ok {
			return types.TextContent{Text: text}
		}
	case types.ContentTypeToolUse:
		content := types.ToolUseContent{}
		if id, ok := raw["id"].(string); ok {
			content.ID = id
		}
		if name, ok := raw["name"].(string); ok {
			content.Name = name
		}
		if input, ok := raw["input"].(map[string]any); ok {
			content.Input = input
		}
		return content
	}
	return nil
}
