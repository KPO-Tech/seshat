package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

func TestParseOpenAISSEStreamSilentEOFNoOutputReturnsError(t *testing.T) {
	// Connection closes cleanly (io.EOF, not a read error) without ever
	// sending a finish_reason or any content - the silent-truncation case.
	body := "data: {\"choices\":[{\"delta\":{}}]}\n\n"
	_, err := parseOpenAISSEStream(context.Background(), strings.NewReader(body), types.ModelIdentifier{Model: "test-model"}, nil)
	if err == nil {
		t.Fatal("expected an error for a silently interrupted stream, got nil")
	}
	if !errors.Is(err, errStreamInterrupted) {
		t.Fatalf("expected errStreamInterrupted, got %v", err)
	}
}

func TestParseOpenAISSEStreamInBandErrorReturnsError(t *testing.T) {
	body := `data: {"error":{"message":"upstream failed","code":502}}` + "\n\n"
	_, err := parseOpenAISSEStream(context.Background(), strings.NewReader(body), types.ModelIdentifier{Model: "test-model"}, nil)
	if err == nil {
		t.Fatal("expected an error for an in-band provider error, got nil")
	}
	if !errors.Is(err, errProviderStreamError) {
		t.Fatalf("expected errProviderStreamError, got %v", err)
	}
}

func TestParseOpenAISSEStreamNormalCompletionStillWorks(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"hello"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}` + "\n\n" +
		"data: [DONE]\n\n"
	result, err := parseOpenAISSEStream(context.Background(), strings.NewReader(body), types.ModelIdentifier{Model: "test-model"}, nil)
	if err != nil {
		t.Fatalf("expected no error for a normal completion, got %v", err)
	}
	if len(result.Response.Content) != 1 {
		t.Fatalf("expected one content block, got %d", len(result.Response.Content))
	}
	text, ok := result.Response.Content[0].(types.TextContent)
	if !ok || text.Text != "hello" {
		t.Fatalf("expected text content %q, got %+v", "hello", result.Response.Content[0])
	}
}

func TestParseOpenAISSEStreamEmptyButFinishedIsNotInterrupted(t *testing.T) {
	// An explicit finish_reason with no content is a legitimate (if terse)
	// completion, not a silent failure - must not be misclassified.
	body := `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" + "data: [DONE]\n\n"
	_, err := parseOpenAISSEStream(context.Background(), strings.NewReader(body), types.ModelIdentifier{Model: "test-model"}, nil)
	if err != nil {
		t.Fatalf("expected no error for an explicitly finished empty stream, got %v", err)
	}
}

func newTestAnthropicResponse(body string) *http.Response {
	return &http.Response{Body: io.NopCloser(strings.NewReader(body))}
}

func TestStreamAnthropicResponseSilentEOFWithoutMessageStopEmitsError(t *testing.T) {
	c := &Client{}
	body := `data: {"type":"content_block_start","content_block":{"type":"text","text":""}}` + "\n\n" +
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}` + "\n\n"
	// Connection drops here - no message_stop, no [DONE].
	resp := newTestAnthropicResponse(body)
	defer resp.Body.Close()
	chunkChan := make(chan types.APIResponseChunk, 10)
	c.streamAnthropicResponse(context.Background(), resp, chunkChan)

	var lastErr error
	for chunk := range chunkChan {
		if chunk.Type == types.APIChunkTypeError {
			lastErr = chunk.Error
		}
	}
	if lastErr == nil {
		t.Fatal("expected a terminal error chunk for a stream that never saw message_stop")
	}
	if !errors.Is(lastErr, errStreamInterrupted) {
		t.Fatalf("expected errStreamInterrupted, got %v", lastErr)
	}
}

func TestStreamAnthropicResponseNormalCompletionNoError(t *testing.T) {
	c := &Client{}
	body := `data: {"type":"content_block_start","content_block":{"type":"text","text":""}}` + "\n\n" +
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		`data: {"type":"content_block_stop"}` + "\n\n" +
		`data: {"type":"message_stop","stop_reason":"end_turn"}` + "\n\n"
	resp := newTestAnthropicResponse(body)
	defer resp.Body.Close()
	chunkChan := make(chan types.APIResponseChunk, 10)
	c.streamAnthropicResponse(context.Background(), resp, chunkChan)

	for chunk := range chunkChan {
		if chunk.Type == types.APIChunkTypeError {
			t.Fatalf("expected no error chunk for a normal completion, got %v", chunk.Error)
		}
	}
}
