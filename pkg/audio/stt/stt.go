// Package stt is the public, provider-agnostic speech-to-text interface -
// a standalone client library separate from the speech_to_text agent tool,
// for a host application transcribing audio directly (e.g. a UI "record and
// transcribe" feature) without an LLM tool-calling loop in the way.
package stt

import internalstt "github.com/KPO-Tech/seshat-runtime/internal/audio/stt"

type (
	// SpeechToText is the core interface for transcription providers.
	SpeechToText = internalstt.SpeechToText

	// Response is the canonical transcription result.
	Response = internalstt.Response

	// Segment is a sentence-level transcript chunk.
	Segment = internalstt.Segment

	// Word holds per-word timing and confidence data (when available).
	Word = internalstt.Word
)
