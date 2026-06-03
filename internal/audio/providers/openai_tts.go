// Package audioproviders implements audio.tts.Generation and audio.stt.SpeechToText
// for supported providers. Add a new file per provider (elevenlabs_tts.go,
// deepgram_stt.go, …) — no other package needs updating.
package audioproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/EngineerProjects/nexus-engine/internal/audio/tts"
)

const (
	openAITTSBaseURL      = "https://api.openai.com/v1"
	openAITTSDefaultModel = "tts-1"
	openAITTSDefaultVoice = "alloy"
	openAITTSDefaultFmt   = "mp3"
)

// openAIVoices is the static voice catalog for OpenAI TTS.
var openAIVoices = []tts.Voice{
	{ID: "alloy", Name: "Alloy", Language: "en-US", Description: "Versatile, balanced voice"},
	{ID: "echo", Name: "Echo", Language: "en-US", Description: "Neutral, clear voice"},
	{ID: "fable", Name: "Fable", Language: "en-US", Description: "Warm, narrative voice"},
	{ID: "onyx", Name: "Onyx", Language: "en-US", Description: "Deep, authoritative voice"},
	{ID: "nova", Name: "Nova", Language: "en-US", Description: "Energetic, bright voice"},
	{ID: "shimmer", Name: "Shimmer", Language: "en-US", Description: "Soft, expressive voice"},
}

// OpenAITTS generates speech using the OpenAI TTS API.
type OpenAITTS struct {
	apiKey     string
	baseURL    string
	model      string
	voice      string
	format     string
	httpClient *http.Client
}

// OpenAITTSOption configures an OpenAITTS client.
type OpenAITTSOption func(*OpenAITTS)

func WithTTSModel(model string) OpenAITTSOption { return func(c *OpenAITTS) { c.model = model } }
func WithTTSVoice(voice string) OpenAITTSOption { return func(c *OpenAITTS) { c.voice = voice } }
func WithTTSFormat(fmt string) OpenAITTSOption  { return func(c *OpenAITTS) { c.format = fmt } }
func WithTTSBaseURL(url string) OpenAITTSOption { return func(c *OpenAITTS) { c.baseURL = url } }
func WithTTSHTTPClient(hc *http.Client) OpenAITTSOption {
	return func(c *OpenAITTS) { c.httpClient = hc }
}

// NewOpenAITTS creates an OpenAI TTS client.
// Supported models: tts-1, tts-1-hd. Voices: alloy, echo, fable, onyx, nova, shimmer.
func NewOpenAITTS(apiKey string, opts ...OpenAITTSOption) *OpenAITTS {
	c := &OpenAITTS{
		apiKey:     apiKey,
		baseURL:    openAITTSBaseURL,
		model:      openAITTSDefaultModel,
		voice:      openAITTSDefaultVoice,
		format:     openAITTSDefaultFmt,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *OpenAITTS) Provider() string { return "openai" }

func (c *OpenAITTS) GenerateAudio(ctx context.Context, text string) (*tts.Response, error) {
	body, err := json.Marshal(map[string]any{
		"model":           c.model,
		"input":           text,
		"voice":           c.voice,
		"response_format": c.format,
	})
	if err != nil {
		return nil, fmt.Errorf("openai tts: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai tts: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai tts: http: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai tts: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai tts: API error %d: %s", resp.StatusCode, string(audio))
	}

	contentType := "audio/" + c.format
	return &tts.Response{
		AudioData:      audio,
		ContentType:    contentType,
		Model:          c.model,
		CharactersUsed: len([]rune(text)),
	}, nil
}

func (c *OpenAITTS) ListVoices(_ context.Context) ([]tts.Voice, error) {
	return openAIVoices, nil
}

var _ tts.Generation = (*OpenAITTS)(nil)
