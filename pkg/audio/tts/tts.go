// Package tts is the public, provider-agnostic text-to-speech interface -
// a standalone client library separate from the text_to_speech agent tool,
// for a host application synthesising speech directly without an LLM
// tool-calling loop in the way.
package tts

import internaltts "github.com/KPO-Tech/seshat-runtime/internal/audio/tts"

type (
	// Generation is the core interface for text-to-speech providers.
	Generation = internaltts.Generation

	// Response holds the synthesised audio from a single GenerateAudio call.
	Response = internaltts.Response

	// Voice describes a speaker voice offered by a provider.
	Voice = internaltts.Voice
)
