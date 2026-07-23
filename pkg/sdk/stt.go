package sdk

import (
	"context"

	audioproviders "github.com/KPO-Tech/seshat/internal/audio/providers"
	"github.com/KPO-Tech/seshat/internal/audio/stt"
)

// TranscriptionResult mirrors internal/audio/stt.Response — re-exported so
// hosts outside this module (which cannot import internal/ packages) can
// name the return type of TranscribeAudio.
type TranscriptionResult = stt.Response

// transcribeConfig holds the optional settings TranscribeOption can adjust.
type transcribeConfig struct {
	model    string
	language string
	baseURL  string
}

// TranscribeOption configures an optional TranscribeAudio setting.
type TranscribeOption func(*transcribeConfig)

// WithTranscribeModel overrides the Whisper model (default "whisper-1").
func WithTranscribeModel(model string) TranscribeOption {
	return func(c *transcribeConfig) { c.model = model }
}

// WithTranscribeLanguage hints the spoken language as an IETF tag (e.g. "en"); omit to auto-detect.
func WithTranscribeLanguage(language string) TranscribeOption {
	return func(c *transcribeConfig) { c.language = language }
}

// WithTranscribeBaseURL overrides the API base URL (default OpenAI's), mainly for tests.
func WithTranscribeBaseURL(baseURL string) TranscribeOption {
	return func(c *transcribeConfig) { c.baseURL = baseURL }
}

// TranscribeAudio performs a one-off speech-to-text transcription using
// OpenAI's Whisper API, independent of any chat session. Use this when a
// host needs raw audio-to-text directly (e.g. a voice-input control in its
// own UI) rather than the agent-facing speech_to_text tool, which only runs
// when the model itself decides to call it mid-turn.
//
// audioData may be MP3, MP4, MPEG, MPGA, M4A, WAV, or WebM.
func TranscribeAudio(ctx context.Context, apiKey string, audioData []byte, opts ...TranscribeOption) (*TranscriptionResult, error) {
	cfg := &transcribeConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	sttOpts := make([]audioproviders.OpenAISTTOption, 0, 3)
	if cfg.model != "" {
		sttOpts = append(sttOpts, audioproviders.WithSTTModel(cfg.model))
	}
	if cfg.language != "" {
		sttOpts = append(sttOpts, audioproviders.WithSTTLanguage(cfg.language))
	}
	if cfg.baseURL != "" {
		sttOpts = append(sttOpts, audioproviders.WithSTTBaseURL(cfg.baseURL))
	}

	transcriber := audioproviders.NewOpenAISTT(apiKey, sttOpts...)
	return transcriber.Transcribe(ctx, audioData)
}
