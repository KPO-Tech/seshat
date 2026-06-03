package audioproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/EngineerProjects/nexus-engine/internal/audio/stt"
)

const (
	openAISTTBaseURL      = "https://api.openai.com/v1"
	openAISTTDefaultModel = "whisper-1"
)

// OpenAISTT transcribes audio using OpenAI Whisper.
type OpenAISTT struct {
	apiKey     string
	baseURL    string
	model      string
	language   string // IETF tag, empty = auto-detect
	httpClient *http.Client
}

// OpenAISTTOption configures an OpenAISTT client.
type OpenAISTTOption func(*OpenAISTT)

func WithSTTModel(model string) OpenAISTTOption   { return func(c *OpenAISTT) { c.model = model } }
func WithSTTLanguage(lang string) OpenAISTTOption { return func(c *OpenAISTT) { c.language = lang } }
func WithSTTBaseURL(url string) OpenAISTTOption   { return func(c *OpenAISTT) { c.baseURL = url } }
func WithSTTHTTPClient(hc *http.Client) OpenAISTTOption {
	return func(c *OpenAISTT) { c.httpClient = hc }
}

// NewOpenAISTT creates an OpenAI Whisper STT client.
// audioData passed to Transcribe may be MP3, MP4, MPEG, MPGA, M4A, WAV, or WebM.
func NewOpenAISTT(apiKey string, opts ...OpenAISTTOption) *OpenAISTT {
	c := &OpenAISTT{
		apiKey:     apiKey,
		baseURL:    openAISTTBaseURL,
		model:      openAISTTDefaultModel,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *OpenAISTT) Provider() string { return "openai" }

func (c *OpenAISTT) Transcribe(ctx context.Context, audioData []byte) (*stt.Response, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("openai stt: create form file: %w", err)
	}
	if _, err := fw.Write(audioData); err != nil {
		return nil, fmt.Errorf("openai stt: write audio: %w", err)
	}
	_ = mw.WriteField("model", c.model)
	_ = mw.WriteField("response_format", "verbose_json")
	if c.language != "" {
		_ = mw.WriteField("language", filepath.Base(c.language))
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return nil, fmt.Errorf("openai stt: create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai stt: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai stt: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai stt: API error %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
		Segments []struct {
			ID    int     `json:"id"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
		Words []struct {
			Word  string  `json:"word"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"words"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("openai stt: decode response: %w", err)
	}

	segments := make([]stt.Segment, len(apiResp.Segments))
	for i, s := range apiResp.Segments {
		segments[i] = stt.Segment{ID: s.ID, Start: s.Start, End: s.End, Text: s.Text}
	}
	words := make([]stt.Word, len(apiResp.Words))
	for i, w := range apiResp.Words {
		words[i] = stt.Word{Word: w.Word, Start: w.Start, End: w.End}
	}

	return &stt.Response{
		Text:     apiResp.Text,
		Language: apiResp.Language,
		Duration: apiResp.Duration,
		Segments: segments,
		Words:    words,
		Model:    c.model,
	}, nil
}

var _ stt.SpeechToText = (*OpenAISTT)(nil)
