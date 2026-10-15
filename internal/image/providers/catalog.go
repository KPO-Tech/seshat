package imageproviders

import "strings"

// Wire protocols an image provider can speak.
const (
	APIOpenAI     = "openai"     // POST {base}/images/generations (DALL-E, GPT Image and compatible servers)
	APIGemini     = "gemini"     // Gemini API: generateContent for Gemini image models, predict for Imagen
	APIOpenRouter = "openrouter" // POST {base}/images, the OpenRouter image API
)

// Profile describes an image provider: protocol, endpoint, key variable and
// models. It is the image counterpart of the chat catalog in internal/providers:
// the SDK looks a provider up here instead of accepting only "openai" and
// "gemini", and a provider that is not listed works when it speaks the OpenAI
// image API and the host gives its base URL.
type Profile struct {
	ID          string
	DisplayName string
	API         string
	BaseURL     string
	EnvVars     []string // environment variables that can carry the API key
	// Models lists the models of the provider; the first is the default.
	Models []string
}

// DefaultModel returns the model used when the host does not name one.
func (p Profile) DefaultModel() string {
	if len(p.Models) == 0 {
		return ""
	}
	return p.Models[0]
}

// profiles is the image catalog, checked against each provider's own
// documentation and OpenRouter's image model list (2026-10).
var profiles = []Profile{
	{
		ID: "openai", DisplayName: "OpenAI", API: APIOpenAI, BaseURL: "https://api.openai.com/v1",
		EnvVars: []string{"OPENAI_API_KEY"},
		// flare is the fast everyday model, sunburst the one for precise edits.
		Models: []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2", "gpt-image-1", "gpt-image-1-mini"},
	},
	{
		ID: "gemini", DisplayName: "Google Gemini", API: APIGemini, BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		EnvVars: []string{"GOOGLE_API_KEY", "GOOGLE_GEMINI_API_KEY"},
		Models:  []string{"gemini-nano-banana-2.1", "gemini-3.1-flash-image", "gemini-3-pro-image", "gemini-3.1-flash-lite-image"},
	},
	{
		ID: "openrouter", DisplayName: "OpenRouter", API: APIOpenRouter, BaseURL: "https://openrouter.ai/api/v1",
		EnvVars: []string{"OPENROUTER_API_KEY"},
		Models: []string{
			"google/gemini-nano-banana-2.1",
			"openai/gpt-image-2.5-flare",
			"openai/gpt-image-2.5-sunburst",
			"black-forest-labs/flux-3-image",
			"bytedance-seed/seedream-5-0-pro",
			"recraft/recraft-v4.1",
			"x-ai/grok-imagine-image-2.0",
			"qwen/qwen-image-3",
		},
	},
}

// Profiles returns the image providers this build knows.
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
