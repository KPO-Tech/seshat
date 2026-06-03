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

const deepseekDefaultBaseURL = "https://api.deepseek.com/beta/completions"
const deepseekDefaultModel = "deepseek-coder"

// DeepSeekOption configures the DeepSeek FIM client.
type DeepSeekOption func(*deepseekClient)

// WithDeepSeekAPIKey sets the API key.
func WithDeepSeekAPIKey(key string) DeepSeekOption {
	return func(c *deepseekClient) { c.apiKey = key }
}

// WithDeepSeekModel sets the model (default: deepseek-coder).
func WithDeepSeekModel(model string) DeepSeekOption {
	return func(c *deepseekClient) { c.model = model }
}

// WithDeepSeekMaxTokens sets the default max tokens.
func WithDeepSeekMaxTokens(n int64) DeepSeekOption {
	return func(c *deepseekClient) { c.maxTokens = &n }
}

// WithDeepSeekTemperature sets the default sampling temperature.
func WithDeepSeekTemperature(t float64) DeepSeekOption {
	return func(c *deepseekClient) { c.temperature = &t }
}

// WithDeepSeekBaseURL overrides the API endpoint.
func WithDeepSeekBaseURL(url string) DeepSeekOption {
	return func(c *deepseekClient) { c.baseURL = url }
}

// WithDeepSeekHTTPClient sets a custom HTTP client.
func WithDeepSeekHTTPClient(hc *http.Client) DeepSeekOption {
	return func(c *deepseekClient) { c.httpClient = hc }
}

type deepseekClient struct {
	apiKey      string
	model       string
	baseURL     string
	maxTokens   *int64
	temperature *float64
	httpClient  *http.Client
}

// NewDeepSeek creates a DeepSeek FIM client.
func NewDeepSeek(opts ...DeepSeekOption) fim.Completer {
	c := &deepseekClient{
		model:      deepseekDefaultModel,
		baseURL:    deepseekDefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *deepseekClient) Provider() string { return "deepseek" }
func (c *deepseekClient) Model() string    { return c.model }

// ── wire types ─────────────────────────────────────────────────────────────

type deepseekRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Suffix      string   `json:"suffix,omitempty"`
	MaxTokens   *int64   `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	Stream      bool     `json:"stream"`
}

type deepseekChoice struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

type deepseekUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type deepseekResponse struct {
	Model   string           `json:"model"`
	Choices []deepseekChoice `json:"choices"`
	Usage   deepseekUsage    `json:"usage"`
}

type deepseekStreamChoice struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type deepseekStreamResponse struct {
	Model   string                 `json:"model"`
	Choices []deepseekStreamChoice `json:"choices"`
	Usage   *deepseekUsage         `json:"usage,omitempty"`
}

// ── Complete ────────────────────────────────────────────────────────────────

func (c *deepseekClient) Complete(ctx context.Context, req fim.Request) (*fim.Response, error) {
	body, err := json.Marshal(c.buildReq(req, false))
	if err != nil {
		return nil, fmt.Errorf("fim/deepseek: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fim/deepseek: new request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fim/deepseek: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fim/deepseek: status %d: %s", resp.StatusCode, b)
	}

	var out deepseekResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("fim/deepseek: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("fim/deepseek: no choices")
	}
	return &fim.Response{
		Content:      out.Choices[0].Text,
		FinishReason: mapDeepSeekFinish(out.Choices[0].FinishReason),
		Usage:        fim.Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens},
		Model:        out.Model,
		Provider:     "deepseek",
	}, nil
}

// ── CompleteStream ──────────────────────────────────────────────────────────

func (c *deepseekClient) CompleteStream(ctx context.Context, req fim.Request) <-chan fim.Event {
	ch := make(chan fim.Event)
	go func() {
		defer close(ch)
		body, err := json.Marshal(c.buildReq(req, true))
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/deepseek: marshal: %w", err)}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/deepseek: new request: %w", err)}
			return
		}
		c.setHeaders(httpReq)
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/deepseek: http: %w", err)}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/deepseek: status %d: %s", resp.StatusCode, b)}
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
					ch <- fim.Event{Type: fim.EventComplete, Response: &fim.Response{Content: buf.String(), Usage: usage, FinishReason: finish, Model: modelID, Provider: "deepseek"}}
				} else {
					ch <- fim.Event{Type: fim.EventError, Error: fmt.Errorf("fim/deepseek: stream: %w", err)}
				}
				return
			}
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte("data: ")) {
				continue
			}
			data := bytes.TrimPrefix(line, []byte("data: "))
			if bytes.Equal(data, []byte("[DONE]")) {
				ch <- fim.Event{Type: fim.EventComplete, Response: &fim.Response{Content: buf.String(), Usage: usage, FinishReason: finish, Model: modelID, Provider: "deepseek"}}
				return
			}
			var sr deepseekStreamResponse
			if json.Unmarshal(data, &sr) != nil {
				continue
			}
			if modelID == "" {
				modelID = sr.Model
			}
			for _, c2 := range sr.Choices {
				if c2.Text != "" {
					buf.WriteString(c2.Text)
					ch <- fim.Event{Type: fim.EventContentDelta, Content: c2.Text}
				}
				if c2.FinishReason != "" {
					finish = mapDeepSeekFinish(c2.FinishReason)
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

func (c *deepseekClient) buildReq(req fim.Request, stream bool) deepseekRequest {
	out := deepseekRequest{Model: c.model, Prompt: req.Prompt, Suffix: req.Suffix, Stream: stream}
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
	out.Stop = req.Stop
	return out
}

func (c *deepseekClient) setHeaders(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func mapDeepSeekFinish(s string) fim.FinishReason {
	switch s {
	case "stop":
		return fim.FinishReasonStop
	case "length":
		return fim.FinishReasonLength
	default:
		return fim.FinishReasonUnknown
	}
}
