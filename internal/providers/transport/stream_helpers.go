package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// streamSSEResponse consumes an Anthropic SSE response body and forwards parsed chunks
// to ch, then closes ch. Designed to run in a goroutine; closes body on return.
func streamSSEResponse(ctx context.Context, body io.ReadCloser, ch chan<- types.APIResponseChunk) {
	defer close(ch)
	defer body.Close()

	reader := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				ch <- errChunk("stream read error", err)
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

		chunk, err := parseSSEChunk([]byte(data))
		if err != nil {
			ch <- errChunk("failed to parse chunk", err)
			return
		}
		ch <- chunk
	}
}

// readBedrockEventStream decodes AWS EventStream frames from body and sends the
// embedded Anthropic SSE chunks to ch. Closes ch and body on return.
//
// EventStream frame layout (all big-endian):
//
//	[4] total_len  [4] headers_len  [4] prelude_crc
//	[headers_len] headers
//	[total_len - 12 - headers_len - 4] payload (JSON)
//	[4] message_crc
//
// The payload JSON has a "bytes" field containing a base64-encoded Anthropic SSE JSON chunk.
func readBedrockEventStream(ctx context.Context, body io.ReadCloser, ch chan<- types.APIResponseChunk) {
	defer close(ch)
	defer body.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 12-byte prelude
		var prelude [12]byte
		if _, err := io.ReadFull(body, prelude[:]); err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				ch <- errChunk("eventstream: failed to read prelude", err)
			}
			return
		}

		totalLen := int(binary.BigEndian.Uint32(prelude[0:4]))
		headersLen := int(binary.BigEndian.Uint32(prelude[4:8]))
		// prelude CRC at [8:12] — not verified here

		payloadLen := totalLen - 12 - headersLen - 4
		if payloadLen < 0 || headersLen < 0 {
			ch <- errChunk("eventstream: malformed frame", nil)
			return
		}

		// Skip headers
		if headersLen > 0 {
			if _, err := io.ReadFull(body, make([]byte, headersLen)); err != nil {
				ch <- errChunk("eventstream: failed to read headers", err)
				return
			}
		}

		// Read payload
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(body, payload); err != nil {
			ch <- errChunk("eventstream: failed to read payload", err)
			return
		}

		// Discard message CRC
		if _, err := io.ReadFull(body, make([]byte, 4)); err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				ch <- errChunk("eventstream: failed to read CRC", err)
			}
			return
		}

		// Payload is {"bytes":"<base64>"} where the decoded value is an Anthropic SSE JSON chunk.
		var envelope struct {
			Bytes string `json:"bytes"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Bytes == "" {
			// May be an error event from Bedrock
			var errEvent struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(payload, &errEvent) == nil && errEvent.Message != "" {
				ch <- errChunk(errEvent.Message, nil)
			}
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(envelope.Bytes)
		if err != nil {
			ch <- errChunk("eventstream: base64 decode failed", err)
			return
		}

		chunk, err := parseSSEChunk(decoded)
		if err != nil {
			ch <- errChunk("eventstream: parse chunk failed", err)
			return
		}

		ch <- chunk

		if chunk.Type == types.APIChunkTypeMessageStop {
			return
		}
	}
}

// buildAnthropicStreamBody builds a standard Anthropic streaming request body usable
// by Foundry, Vertex, and Bedrock (all expose the Anthropic messages API).
func buildAnthropicStreamBody(req types.APIRequest) io.Reader {
	body := map[string]any{
		"model":      req.Model.ProviderModelName(),
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
		"stream":     true,
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

// parseSSEChunk parses a single Anthropic SSE data-line JSON into an APIResponseChunk.
func parseSSEChunk(data []byte) (types.APIResponseChunk, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return types.APIResponseChunk{}, err
	}

	chunkType, _ := raw["type"].(string)
	chunk := types.APIResponseChunk{Type: types.APIChunkType(chunkType)}

	switch chunk.Type {
	case types.APIChunkTypeContentBlockStart:
		if cb, ok := raw["content_block"].(map[string]any); ok {
			chunk.ContentBlock = parseSSEContentBlock(cb)
		}

	case types.APIChunkTypeContentBlockDelta:
		if delta, ok := raw["delta"].(map[string]any); ok {
			chunk.DeltaType, _ = delta["type"].(string)
			chunk.Delta, _ = delta["text"].(string)
			chunk.PartialJSON, _ = delta["partial_json"].(string)
		}
		if u, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseSSEUsage(u)
		}

	case types.APIChunkTypeContentBlockStop:
		// no extra fields

	case types.APIChunkTypeMessageDelta:
		if delta, ok := raw["delta"].(map[string]any); ok {
			if s, ok := delta["stop_reason"].(string); ok {
				chunk.StopReason = &s
			}
			if s, ok := delta["stop_sequence"].(string); ok {
				chunk.StopSequence = &s
			}
		}
		if u, ok := raw["usage"].(map[string]any); ok {
			chunk.Usage = parseSSEUsage(u)
		}

	case types.APIChunkTypeMessageStop:
		if s, ok := raw["stop_reason"].(string); ok {
			chunk.StopReason = &s
		}
	}

	return chunk, nil
}

func parseSSEContentBlock(raw map[string]any) types.ContentBlock {
	blockType, _ := raw["type"].(string)
	switch types.ContentType(blockType) {
	case types.ContentTypeText:
		text, _ := raw["text"].(string)
		return types.TextContent{Text: text}
	case types.ContentTypeToolUse:
		c := types.ToolUseContent{}
		c.ID, _ = raw["id"].(string)
		c.Name, _ = raw["name"].(string)
		if input, ok := raw["input"].(map[string]any); ok {
			c.Input = input
		}
		return c
	}
	return nil
}

func parseSSEUsage(raw map[string]any) *types.TokenUsage {
	usage := &types.TokenUsage{}
	if v, ok := raw["input_tokens"].(float64); ok {
		usage.InputTokens = int(v)
	}
	if v, ok := raw["output_tokens"].(float64); ok {
		usage.OutputTokens = int(v)
	}
	return usage
}

func errChunk(msg string, err error) types.APIResponseChunk {
	return types.APIResponseChunk{
		Type:  types.APIChunkTypeError,
		Error: types.WrapError(types.ErrCodeAPIResponse, msg, err),
	}
}
