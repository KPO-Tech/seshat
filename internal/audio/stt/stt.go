// Package stt provides a provider-agnostic interface for speech-to-text transcription.
package stt

import "context"

// Word holds per-word timing and confidence data (when the provider returns it).
type Word struct {
	Word       string  `json:"word"`
	Start      float64 `json:"start"` // seconds from audio start
	End        float64 `json:"end"`   // seconds from audio start
	Confidence float64 `json:"confidence"`
}

// Segment is a sentence-level transcript chunk.
type Segment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Response is the canonical transcription result.
type Response struct {
	// Text is the full transcript.
	Text string
	// Language is the detected or requested IETF language tag.
	Language string
	// Duration is the total audio length in seconds.
	Duration float64
	// Segments contains sentence-level chunks (when available).
	Segments []Segment
	// Words contains word-level timing (when available).
	Words []Word
	// Model identifies which model produced the transcript.
	Model string
}

// SpeechToText is the core interface for transcription providers.
// Implementations must be safe for concurrent use.
type SpeechToText interface {
	// Transcribe converts audio bytes to text.
	// audioData must be in a format the provider accepts (MP3, WAV, WebM, …).
	Transcribe(ctx context.Context, audioData []byte) (*Response, error)
	// Provider returns a stable provider name (e.g. "openai", "deepgram").
	Provider() string
}
