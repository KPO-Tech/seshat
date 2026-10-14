package audioproviders

import "strings"

// Profile describes one provider that serves speech over the OpenAI-compatible
// audio API (/audio/transcriptions, /audio/speech). It is the audio counterpart
// of the chat catalog in internal/providers: the SDK looks a provider up here
// instead of hard-coding "openai", and a provider that is not listed still works
// when the host gives its base URL.
type Profile struct {
	ID          string
	DisplayName string
	BaseURL     string
	// EnvVars are the environment variables that can carry the API key,
	// read when the host passes none.
	EnvVars []string
	// STT and TTS are nil when the provider does not offer that direction.
	STT *Capability
	TTS *Capability
	// STTResponseFormat is the response_format to send for transcription:
	// "" chooses from the model, ResponseFormatOmit sends none.
	STTResponseFormat string
}

// Capability lists the models of one direction. The first model is the default.
type Capability struct {
	Models []string
}

// DefaultModel returns the model used when the host does not name one.
func (c *Capability) DefaultModel() string {
	if c == nil || len(c.Models) == 0 {
		return ""
	}
	return c.Models[0]
}

// profiles is the audio catalog, checked against each provider's own
// documentation (2026-10). Add a provider here when it speaks this API; one
// that does not (Deepgram, ElevenLabs...) needs its own client file instead.
var profiles = []Profile{
	{
		ID: "openai", DisplayName: "OpenAI", BaseURL: "https://api.openai.com/v1",
		EnvVars: []string{"OPENAI_API_KEY"},
		// whisper-1 stays first: it is the one that returns segments and
		// timings. gpt-transcribe is OpenAI's recommended general model but
		// returns text only.
		STT: &Capability{Models: []string{"whisper-1", "gpt-transcribe", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"}},
		TTS: &Capability{Models: []string{"gpt-4o-mini-tts", "tts-1", "tts-1-hd"}},
	},
	{
		ID: "groq", DisplayName: "Groq", BaseURL: "https://api.groq.com/openai/v1",
		EnvVars: []string{"GROQ_API_KEY"},
		STT:     &Capability{Models: []string{"whisper-large-v3-turbo", "whisper-large-v3"}},
	},
	{
		ID: "mistral", DisplayName: "Mistral AI", BaseURL: "https://api.mistral.ai/v1",
		EnvVars:           []string{"MISTRAL_API_KEY"},
		STT:               &Capability{Models: []string{"voxtral-mini-latest"}},
		STTResponseFormat: ResponseFormatOmit,
	},
}

// Profiles returns the audio providers this build knows.
func Profiles() []Profile {
	out := make([]Profile, len(profiles))
	copy(out, profiles)
	return out
}

// ProfileFor returns the profile of a provider ID, case-insensitively.
func ProfileFor(id string) (Profile, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}
