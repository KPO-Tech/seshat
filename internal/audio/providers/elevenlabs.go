package audioproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/KPO-Tech/seshat-runtime/internal/audio/stt"
	"github.com/KPO-Tech/seshat-runtime/internal/audio/tts"
)

// ElevenLabs has its own API (xi-api-key header, voice in the URL, Scribe for
// transcription), so it has its own clients instead of the OpenAI-compatible
// ones. Endpoints and fields checked against elevenlabs.io/docs (2026-10).

const (
	elevenLabsBaseURL          = "https://api.elevenlabs.io"
	elevenLabsDefaultTTSModel  = "eleven_multilingual_v2" // the API default
	elevenLabsDefaultSTTModel  = "scribe_v2"
	elevenLabsDefaultTTSFormat = "mp3_44100_128"
)

// trimElevenLabsBase turns whatever root a host gave into one without a
// trailing slash or /v1, since the clients add the versioned paths themselves.
func trimElevenLabsBase(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.TrimSuffix(baseURL, "/v1")
}

// ---- text to speech

// ElevenLabsTTS synthesises speech with ElevenLabs.
type ElevenLabsTTS struct {
	name       string
	apiKey     string
	baseURL    string
	model      string
	voice      string // voice_id; empty = the first voice of the account
	format     string // an OpenAI-style name (mp3, wav, opus, pcm) or an ElevenLabs output_format
	httpClient *http.Client

	mu            sync.Mutex
	defaultVoiceI string
}

// ElevenLabsTTSOption configures an ElevenLabsTTS client.
type ElevenLabsTTSOption func(*ElevenLabsTTS)

func WithElevenLabsTTSModel(model string) ElevenLabsTTSOption {
	return func(c *ElevenLabsTTS) { c.model = model }
}

// WithElevenLabsTTSVoice sets the voice_id (see ListVoices).
func WithElevenLabsTTSVoice(voice string) ElevenLabsTTSOption {
	return func(c *ElevenLabsTTS) { c.voice = voice }
}

// WithElevenLabsTTSFormat sets the output format: mp3, wav, opus or pcm, or a
// full ElevenLabs output_format such as mp3_44100_192.
func WithElevenLabsTTSFormat(format string) ElevenLabsTTSOption {
	return func(c *ElevenLabsTTS) { c.format = format }
}

func WithElevenLabsTTSBaseURL(baseURL string) ElevenLabsTTSOption {
	return func(c *ElevenLabsTTS) { c.baseURL = trimElevenLabsBase(baseURL) }
}

func WithElevenLabsTTSHTTPClient(hc *http.Client) ElevenLabsTTSOption {
	return func(c *ElevenLabsTTS) { c.httpClient = hc }
}

// NewElevenLabsTTS creates an ElevenLabs text-to-speech client.
func NewElevenLabsTTS(apiKey string, opts ...ElevenLabsTTSOption) *ElevenLabsTTS {
	c := &ElevenLabsTTS{
		name:       "elevenlabs",
		apiKey:     apiKey,
		baseURL:    elevenLabsBaseURL,
		model:      elevenLabsDefaultTTSModel,
		format:     "mp3",
		httpClient: &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *ElevenLabsTTS) Provider() string { return c.name }

// elevenLabsOutput maps the OpenAI-style format names the tool uses to an
// ElevenLabs output_format and the MIME type of the result.
func elevenLabsOutput(format string) (outputFormat, contentType string) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "mp3":
		return elevenLabsDefaultTTSFormat, "audio/mpeg"
	case "wav":
		return "wav_44100", "audio/wav"
	case "opus":
		return "opus_48000_128", "audio/ogg"
	case "pcm":
		return "pcm_24000", "audio/pcm"
	}
	switch {
	case strings.HasPrefix(format, "mp3_"):
		return format, "audio/mpeg"
	case strings.HasPrefix(format, "wav_"):
		return format, "audio/wav"
	case strings.HasPrefix(format, "opus_"):
		return format, "audio/ogg"
	case strings.HasPrefix(format, "pcm_"):
		return format, "audio/pcm"
	case strings.HasPrefix(format, "ulaw_"), strings.HasPrefix(format, "alaw_"):
		return format, "audio/basic"
	}
	// An unknown name is sent as is: ElevenLabs answers with a clear error.
	return format, "application/octet-stream"
}

func (c *ElevenLabsTTS) GenerateAudio(ctx context.Context, text string) (*tts.Response, error) {
	voice, err := c.voiceID(ctx)
	if err != nil {
		return nil, err
	}
	outputFormat, contentType := elevenLabsOutput(c.format)
	body, err := json.Marshal(map[string]any{"text": text, "model_id": c.model})
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: marshal request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s?output_format=%s", c.baseURL, url.PathEscape(voice), url.QueryEscape(outputFormat))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: http: %w", err)
	}
	defer resp.Body.Close()
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs tts: API error %d: %s", resp.StatusCode, string(audio))
	}
	return &tts.Response{
		AudioData:      audio,
		ContentType:    contentType,
		Model:          c.model,
		CharactersUsed: len([]rune(text)),
	}, nil
}

// voiceID returns the configured voice, or the first voice of the account when
// none was configured (ElevenLabs has no built-in default voice name). The
// answer is kept for the next call.
func (c *ElevenLabsTTS) voiceID(ctx context.Context) (string, error) {
	if c.voice != "" {
		return c.voice, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.defaultVoiceI != "" {
		return c.defaultVoiceI, nil
	}
	voices, err := c.ListVoices(ctx)
	if err != nil {
		return "", err
	}
	if len(voices) == 0 {
		return "", fmt.Errorf("elevenlabs tts: no voice configured and the account has none; set a voice_id")
	}
	c.defaultVoiceI = voices[0].ID
	return c.defaultVoiceI, nil
}

// ListVoices returns the voices of the account (first page, up to 100).
func (c *ElevenLabsTTS) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v2/voices?page_size=100&include_total_count=false", nil)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: create request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs tts: API error %d: %s", resp.StatusCode, string(raw))
	}
	var apiResp struct {
		Voices []struct {
			VoiceID     string            `json:"voice_id"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Labels      map[string]string `json:"labels"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("elevenlabs tts: decode voices: %w", err)
	}
	voices := make([]tts.Voice, 0, len(apiResp.Voices))
	for _, v := range apiResp.Voices {
		desc := v.Description
		if desc == "" {
			desc = v.Labels["descriptive"]
		}
		voices = append(voices, tts.Voice{ID: v.VoiceID, Name: v.Name, Language: v.Labels["language"], Description: desc})
	}
	return voices, nil
}

var _ tts.Generation = (*ElevenLabsTTS)(nil)

// ---- speech to text

// ElevenLabsSTT transcribes audio with ElevenLabs Scribe.
type ElevenLabsSTT struct {
	name       string
	apiKey     string
	baseURL    string
	model      string
	language   string // ISO-639 code, empty = auto-detect
	httpClient *http.Client
}

// ElevenLabsSTTOption configures an ElevenLabsSTT client.
type ElevenLabsSTTOption func(*ElevenLabsSTT)

func WithElevenLabsSTTModel(model string) ElevenLabsSTTOption {
	return func(c *ElevenLabsSTT) { c.model = model }
}

func WithElevenLabsSTTLanguage(language string) ElevenLabsSTTOption {
	return func(c *ElevenLabsSTT) { c.language = language }
}

func WithElevenLabsSTTBaseURL(baseURL string) ElevenLabsSTTOption {
	return func(c *ElevenLabsSTT) { c.baseURL = trimElevenLabsBase(baseURL) }
}

func WithElevenLabsSTTHTTPClient(hc *http.Client) ElevenLabsSTTOption {
	return func(c *ElevenLabsSTT) { c.httpClient = hc }
}

// NewElevenLabsSTT creates an ElevenLabs Scribe client.
func NewElevenLabsSTT(apiKey string, opts ...ElevenLabsSTTOption) *ElevenLabsSTT {
	c := &ElevenLabsSTT{
		name:       "elevenlabs",
		apiKey:     apiKey,
		baseURL:    elevenLabsBaseURL,
		model:      elevenLabsDefaultSTTModel,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *ElevenLabsSTT) Provider() string { return c.name }

type elevenLabsTranscript struct {
	Text         string `json:"text"`
	LanguageCode string `json:"language_code"`
	Words        []struct {
		Text  string   `json:"text"`
		Type  string   `json:"type"`
		Start *float64 `json:"start"`
		End   *float64 `json:"end"`
	} `json:"words"`
}

func (c *ElevenLabsSTT) Transcribe(ctx context.Context, audioData []byte) (*stt.Response, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: create form file: %w", err)
	}
	if _, err := fw.Write(audioData); err != nil {
		return nil, fmt.Errorf("elevenlabs stt: write audio: %w", err)
	}
	_ = mw.WriteField("model_id", c.model)
	_ = mw.WriteField("timestamps_granularity", "word")
	if c.language != "" {
		_ = mw.WriteField("language_code", c.language)
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/speech-to-text", &buf)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs stt: API error %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp struct {
		elevenLabsTranscript
		AudioDurationSecs float64                `json:"audio_duration_secs"`
		Transcripts       []elevenLabsTranscript `json:"transcripts"` // multichannel
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("elevenlabs stt: decode response: %w", err)
	}
	tr := apiResp.elevenLabsTranscript
	if tr.Text == "" && len(apiResp.Transcripts) > 0 {
		parts := make([]string, 0, len(apiResp.Transcripts))
		for _, t := range apiResp.Transcripts {
			parts = append(parts, t.Text)
		}
		tr = apiResp.Transcripts[0]
		tr.Text = strings.Join(parts, "\n")
	}

	var words []stt.Word
	duration := apiResp.AudioDurationSecs
	for _, w := range tr.Words {
		if w.Type != "word" || w.Start == nil || w.End == nil {
			continue // spacing and audio events carry no word timing
		}
		words = append(words, stt.Word{Word: w.Text, Start: *w.Start, End: *w.End})
		if apiResp.AudioDurationSecs == 0 && *w.End > duration {
			duration = *w.End // the API did not say: the last word ends the audio
		}
	}
	return &stt.Response{
		Text:     tr.Text,
		Language: tr.LanguageCode,
		Duration: duration,
		Words:    words,
		Model:    c.model,
	}, nil
}

var _ stt.SpeechToText = (*ElevenLabsSTT)(nil)
