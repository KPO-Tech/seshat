// Package tts provides a provider-agnostic interface for text-to-speech synthesis.
package tts

import "context"

// Voice describes a speaker voice offered by a provider.
type Voice struct {
	// ID is the provider-internal voice identifier (e.g. "alloy", "nova").
	ID string
	// Name is a human-readable label.
	Name string
	// Language is an IETF language tag (e.g. "en-US").
	Language string
	// Description is an optional short description of the voice character.
	Description string
}

// Response holds the synthesised audio from a single GenerateAudio call.
type Response struct {
	// AudioData is the raw audio bytes in the format specified by ContentType.
	AudioData []byte
	// ContentType is the MIME type (e.g. "audio/mpeg", "audio/wav").
	ContentType string
	// Model is the model/voice-engine that produced the audio.
	Model string
	// CharactersUsed is the number of input characters consumed (for billing).
	CharactersUsed int
}

// Generation is the core interface for text-to-speech providers.
// Implementations must be safe for concurrent use.
type Generation interface {
	// GenerateAudio synthesises speech from the provided text.
	GenerateAudio(ctx context.Context, text string) (*Response, error)
	// ListVoices returns voices available from this provider.
	ListVoices(ctx context.Context) ([]Voice, error)
	// Provider returns a stable provider name (e.g. "openai", "elevenlabs").
	Provider() string
}
