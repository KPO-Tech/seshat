// Package fimproviders provides concrete FIM provider implementations.
package fimproviders

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/fim"
)

const mistralDefaultBaseURL = "https://api.mistral.ai/v1/fim/completions"
const mistralDefaultModel = "codestral-latest"

// MistralOption configures the Mistral FIM client.
type MistralOption func(*mistralClient)

// WithMistralAPIKey sets the API key.
func WithMistralAPIKey(key string) MistralOption {
	return func(c *mistralClient) { c.apiKey = key }
}

// WithMistralModel sets the model (default: codestral-latest).
func WithMistralModel(model string) MistralOption {
	return func(c *mistralClient) { c.model = model }
}

// WithMistralMaxTokens sets the default max tokens to generate.
func WithMistralMaxTokens(n int64) MistralOption {
	return func(c *mistralClient) { c.maxTokens = &n }
}

// WithMistralTemperature sets the default sampling temperature.
func WithMistralTemperature(t float64) MistralOption {
	return func(c *mistralClient) { c.temperature = &t }
}

// WithMistralBaseURL overrides the API endpoint (useful for testing).
func WithMistralBaseURL(url string) MistralOption {
	return func(c *mistralClient) { c.baseURL = url }
}

// WithMistralHTTPClient sets a custom HTTP client.
func WithMistralHTTPClient(hc *http.Client) MistralOption {
	return func(c *mistralClient) { c.httpClient = hc }
}

type mistralClient struct {
	apiKey      string
	model       string
	baseURL     string
	maxTokens   *int64
	temperature *float64
	httpClient  *http.Client
}

// NewMistral creates a Mistral Codestral FIM client.
func NewMistral(opts ...MistralOption) fim.Completer {
	c := &mistralClient{
		model:      mistralDefaultModel,
		baseURL:    mistralDefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *mistralClient) Provider() string { return "mistral" }
func (c *mistralClient) Model() string    { return c.model }

// ── wire types ─────────────────────────────────────────────────────────────

type mistralRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Suffix      string   `json:"suffix,omitempty"`
	MaxTokens   *int64   `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	RandomSeed  *int64   `json:"random_seed,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	Stream      bool     `json:"stream"`
}

type mistralChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type mistralUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type mistralResponse struct {
	Model   string          `json:"model"`
	Choices []mistralChoice `json:"choices"`
	Usage   mistralUsage    `json:"usage"`
}

type mistralStreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type mistralStreamResponse struct {
	Model   string                `json:"model"`
	Choices []mistralStreamChoice `json:"choices"`
	Usage   *mistralUsage         `json:"usage,omitempty"`
}

// ── Complete ────────────────────────────────────────────────────────────────

func (c *mistralClient) Complete(ctx context.Context, req fim.Request) (*fim.Response, error) {
	body, err := json.Marshal(c.buildReq(req, false))
	if err != nil {
		return nil, fmt.Errorf("fim/mistral: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fim/mistral: new request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fim/mistral: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fim/mistral: status %d: %s", resp.StatusCode, b)
	}

	var out mistralResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("fim/mistral: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("fim/mistral: no choices")
	}
	return &fim.Response{
		Content:      out.Choices[0].Message.Content,
		FinishReason: mapMistralFinish(out.Choices[0].FinishReason),
		Usage:        fim.Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens},
		Model:        out.Model,
		Provider:     "mistral",
	}, nil
}

// ── CompleteStream ──────────────────────────────────────────────────────────

func (c *mistralClient) CompleteStream(ctx context.Context, req fim.Request) <-chan fim.Event {
	ch := make(chan fim.Event)
	go func() {
		defer close(ch)
		body, err := json.Marshal(c.buildReq(req, true))
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/mistral: marshal: %w", err)}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/mistral: new request: %w", err)}
			return
		}
		c.setHeaders(httpReq)
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/mistral: http: %w", err)}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/mistral: status %d: %s", resp.StatusCode, b)}
			return
		}

		var buf strings.Builder
		var usage fim.Usage
		var finish fim.FinishReason
		var modelID string
		scanner := bufio.NewReader(resp.Body)

		for {
			line, err := scanner.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					ch <- fim.Event{Type: fim.EventComplete, Response: &fim.Response{Content: buf.String(), Usage: usage, FinishReason: finish, Model: modelID, Provider: "mistral"}}
				} else {
					ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/mistral: stream: %w", err)}
				}
				return
			}
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte("data: ")) {
				continue
			}
			data := bytes.TrimPrefix(line, []byte("data: "))
			if bytes.Equal(data, []byte("[DONE]")) {
				ch <- fim.Event{Type: fim.EventComplete, Response: &fim.Response{Content: buf.String(), Usage: usage, FinishReason: finish, Model: modelID, Provider: "mistral"}}
				return
			}
			var sr mistralStreamResponse
			if json.Unmarshal(data, &sr) != nil {
				continue
			}
			if modelID == "" {
				modelID = sr.Model
			}
			for _, c2 := range sr.Choices {
				if c2.Delta.Content != "" {
					buf.WriteString(c2.Delta.Content)
					ch <- fim.Event{Type: fim.EventContentDelta, Content: c2.Delta.Content}
				}
				if c2.FinishReason != nil {
					finish = mapMistralFinish(*c2.FinishReason)
				}
			}
			if sr.Usage != nil {
				usage = fim.Usage{InputTokens: sr.Usage.PromptTokens, OutputTokens: sr.Usage.CompletionTokens}
			}
		}
	}()
	return ch
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (c *mistralClient) buildReq(req fim.Request, stream bool) mistralRequest {
	out := mistralRequest{Model: c.model, Prompt: req.Prompt, Suffix: req.Suffix, Stream: stream}
	if req.MaxTokens != nil {
		out.MaxTokens = req.MaxTokens
	} else {
		out.MaxTokens = c.maxTokens
	}
	if req.Temperature != nil {
		out.Temperature = req.Temperature
	} else {
		out.Temperature = c.temperature
	}
	if req.TopP != nil {
		out.TopP = req.TopP
	}
	out.RandomSeed = req.RandomSeed
	out.Stop = req.Stop
	return out
}

func (c *mistralClient) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func mapMistralFinish(s string) fim.FinishReason {
	switch s {
	case "stop":
		return fim.FinishReasonStop
	case "length":
		return fim.FinishReasonLength
	default:
		return fim.FinishReasonUnknown
	}
}
