// Package fim provides a unified interface for Fill-in-the-Middle code completion.
//
// FIM (Fill-in-the-Middle) lets a model complete code by receiving a prompt
// (code before the cursor) and an optional suffix (code after the cursor), then
// generating the fragment that belongs between them. It is particularly suited
// to IDE integrations and in-editor ghost text.
//
// This package defines the [Completer] interface and the data types that flow
// through it. Concrete provider implementations live in internal/fim/providers.
//
// Example:
//
//	import (
//	    "github.com/EngineerProjects/nexus-engine/internal/fim"
//	    fimproviders "github.com/EngineerProjects/nexus-engine/internal/fim/providers"
//	)
//
//	client := fimproviders.NewMistral(
//	    fimproviders.WithMistralAPIKey("your-api-key"),
//	)
//	resp, err := client.Complete(ctx, fim.Request{
//	    Prompt: "func add(a, b int) int {\n    ",
//	    Suffix: "\n}",
//	})
package fim

import "context"

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	FinishReasonStop    FinishReason = "stop"
	FinishReasonLength  FinishReason = "length"
	FinishReasonUnknown FinishReason = "unknown"
)

// Usage tracks token consumption for a FIM completion.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Request holds the parameters for a FIM completion call.
type Request struct {
	// Prompt is the code/text before the cursor (required).
	Prompt string
	// Suffix is the code/text after the cursor (optional).
	Suffix string
	// MaxTokens caps the number of generated tokens.
	MaxTokens *int64
	// Temperature controls sampling randomness (0.0 = deterministic).
	Temperature *float64
	// TopP controls nucleus sampling.
	TopP *float64
	// Stop lists sequences that halt generation when encountered.
	Stop []string
	// RandomSeed enables reproducible generation when set.
	RandomSeed *int64
}

// Response holds the result of a completed FIM call.
type Response struct {
	// Content is the generated fragment that fills between prompt and suffix.
	Content string
	// Usage reports token consumption.
	Usage Usage
	// FinishReason indicates why generation stopped.
	FinishReason FinishReason
	// Model is the provider-reported model identifier.
	Model string
	// Provider is the provider name ("mistral" | "deepseek").
	Provider string
}

// EventType identifies the type of a streaming FIM event.
type EventType string

const (
	EventContentDelta EventType = "content_delta"
	EventComplete     EventType = "complete"
	EventError        EventType = "error"
)

// Event is a single event in a streaming FIM response.
type Event struct {
	Type     EventType
	Content  string
	Response *Response
	Error    error
}

// Completer is the interface for FIM providers.
type Completer interface {
	// Complete sends a FIM request and returns the full response.
	Complete(ctx context.Context, req Request) (*Response, error)
	// CompleteStream sends a FIM request and returns a channel of events.
	// The channel is closed after EventComplete or EventError.
	CompleteStream(ctx context.Context, req Request) <-chan Event
	// Provider returns the provider identifier string.
	Provider() string
	// Model returns the model identifier being used.
	Model() string
}
