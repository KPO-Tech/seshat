// Package audioproviders exposes seshat's stt.SpeechToText and
// tts.Generation implementations for direct use by a host application,
// independent of the agent/tool loop.
package audioproviders

import (
	"net/http"

	internalproviders "github.com/KPO-Tech/seshat-runtime/internal/audio/providers"
)

type (
	// OpenAISTT transcribes audio using OpenAI Whisper - or, via
	// WithSTTBaseURL, any OpenAI-API-compatible transcription server, such
	// as a locally running whisper.cpp server (no API key required in that
	// case, since local servers don't check the Authorization header).
	OpenAISTT = internalproviders.OpenAISTT
	// OpenAISTTOption configures an OpenAISTT client.
	OpenAISTTOption = internalproviders.OpenAISTTOption

	// OpenAITTS synthesises speech using OpenAI's TTS models.
	OpenAITTS = internalproviders.OpenAITTS
	// OpenAITTSOption configures an OpenAITTS client.
	OpenAITTSOption = internalproviders.OpenAITTSOption
)

// NewOpenAISTT creates an OpenAI Whisper STT client. audioData passed to
// Transcribe may be MP3, MP4, MPEG, MPGA, M4A, WAV, or WebM.
func NewOpenAISTT(apiKey string, opts ...OpenAISTTOption) *OpenAISTT {
	return internalproviders.NewOpenAISTT(apiKey, opts...)
}

// WithSTTModel sets the transcription model (default "whisper-1").
func WithSTTModel(model string) OpenAISTTOption { return internalproviders.WithSTTModel(model) }

// WithSTTLanguage sets the IETF language tag; empty means auto-detect.
func WithSTTLanguage(lang string) OpenAISTTOption { return internalproviders.WithSTTLanguage(lang) }

// WithSTTBaseURL overrides the default endpoint - point this at a
// self-hosted, OpenAI-API-compatible transcription server (e.g. a local
// whisper.cpp server) instead of the real OpenAI cloud.
func WithSTTBaseURL(url string) OpenAISTTOption { return internalproviders.WithSTTBaseURL(url) }

// WithSTTHTTPClient injects a custom HTTP client.
func WithSTTHTTPClient(hc *http.Client) OpenAISTTOption {
	return internalproviders.WithSTTHTTPClient(hc)
}

// NewOpenAITTS creates an OpenAI text-to-speech client.
func NewOpenAITTS(apiKey string, opts ...OpenAITTSOption) *OpenAITTS {
	return internalproviders.NewOpenAITTS(apiKey, opts...)
}

// WithTTSModel sets the TTS model.
func WithTTSModel(model string) OpenAITTSOption { return internalproviders.WithTTSModel(model) }

// WithTTSVoice sets the voice (e.g. "alloy", "nova").
func WithTTSVoice(voice string) OpenAITTSOption { return internalproviders.WithTTSVoice(voice) }

// WithTTSFormat sets the output audio format (e.g. "mp3", "wav").
func WithTTSFormat(format string) OpenAITTSOption { return internalproviders.WithTTSFormat(format) }

// WithTTSBaseURL overrides the default endpoint - point this at a
// self-hosted, OpenAI-API-compatible TTS server instead of the real OpenAI cloud.
func WithTTSBaseURL(url string) OpenAITTSOption { return internalproviders.WithTTSBaseURL(url) }

// WithTTSHTTPClient injects a custom HTTP client.
func WithTTSHTTPClient(hc *http.Client) OpenAITTSOption {
	return internalproviders.WithTTSHTTPClient(hc)
}
