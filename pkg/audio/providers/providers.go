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

type (
	// Profile describes a provider that serves speech over the OpenAI-compatible
	// audio API: its endpoint, key variables and STT/TTS models.
	Profile = internalproviders.Profile
	// Capability lists the models a provider offers for one direction.
	Capability = internalproviders.Capability
)

// ResponseFormatOmit makes an STT client send no response_format field.
const ResponseFormatOmit = internalproviders.ResponseFormatOmit

// Profiles returns the audio providers this build knows (OpenAI, Groq, Mistral).
func Profiles() []Profile { return internalproviders.Profiles() }

// ProfileFor returns the audio profile of a provider ID.
func ProfileFor(id string) (Profile, bool) { return internalproviders.ProfileFor(id) }

// WithSTTProviderName sets the name an STT client reports from Provider().
func WithSTTProviderName(name string) OpenAISTTOption {
	return internalproviders.WithSTTProviderName(name)
}

// WithSTTResponseFormat forces the response_format an STT client sends; empty
// chooses from the model, ResponseFormatOmit sends none.
func WithSTTResponseFormat(format string) OpenAISTTOption {
	return internalproviders.WithSTTResponseFormat(format)
}

// WithTTSProviderName sets the name a TTS client reports from Provider().
func WithTTSProviderName(name string) OpenAITTSOption {
	return internalproviders.WithTTSProviderName(name)
}

type (
	// ElevenLabsTTS synthesises speech with ElevenLabs (its own API, not
	// OpenAI-compatible). A voice_id is optional: without one the first voice
	// of the account is used.
	ElevenLabsTTS = internalproviders.ElevenLabsTTS
	// ElevenLabsTTSOption configures an ElevenLabsTTS client.
	ElevenLabsTTSOption = internalproviders.ElevenLabsTTSOption
	// ElevenLabsSTT transcribes audio with ElevenLabs Scribe.
	ElevenLabsSTT = internalproviders.ElevenLabsSTT
	// ElevenLabsSTTOption configures an ElevenLabsSTT client.
	ElevenLabsSTTOption = internalproviders.ElevenLabsSTTOption
)

// APIElevenLabs is the Profile.API value of ElevenLabs.
const APIElevenLabs = internalproviders.APIElevenLabs

// NewElevenLabsTTS creates an ElevenLabs text-to-speech client.
func NewElevenLabsTTS(apiKey string, opts ...ElevenLabsTTSOption) *ElevenLabsTTS {
	return internalproviders.NewElevenLabsTTS(apiKey, opts...)
}

// NewElevenLabsSTT creates an ElevenLabs Scribe client.
func NewElevenLabsSTT(apiKey string, opts ...ElevenLabsSTTOption) *ElevenLabsSTT {
	return internalproviders.NewElevenLabsSTT(apiKey, opts...)
}

// WithElevenLabsTTSModel sets the model (default eleven_multilingual_v2).
func WithElevenLabsTTSModel(model string) ElevenLabsTTSOption {
	return internalproviders.WithElevenLabsTTSModel(model)
}

// WithElevenLabsTTSVoice sets the voice_id.
func WithElevenLabsTTSVoice(voice string) ElevenLabsTTSOption {
	return internalproviders.WithElevenLabsTTSVoice(voice)
}

// WithElevenLabsTTSFormat sets mp3, wav, opus, pcm or a full output_format.
func WithElevenLabsTTSFormat(format string) ElevenLabsTTSOption {
	return internalproviders.WithElevenLabsTTSFormat(format)
}

// WithElevenLabsTTSBaseURL overrides the API root (for example a regional host).
func WithElevenLabsTTSBaseURL(baseURL string) ElevenLabsTTSOption {
	return internalproviders.WithElevenLabsTTSBaseURL(baseURL)
}

// WithElevenLabsSTTModel sets the Scribe model (default scribe_v2).
func WithElevenLabsSTTModel(model string) ElevenLabsSTTOption {
	return internalproviders.WithElevenLabsSTTModel(model)
}

// WithElevenLabsSTTLanguage sets an ISO-639 language code; empty auto-detects.
func WithElevenLabsSTTLanguage(language string) ElevenLabsSTTOption {
	return internalproviders.WithElevenLabsSTTLanguage(language)
}

// WithElevenLabsSTTBaseURL overrides the API root.
func WithElevenLabsSTTBaseURL(baseURL string) ElevenLabsSTTOption {
	return internalproviders.WithElevenLabsSTTBaseURL(baseURL)
}
