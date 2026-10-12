package audioproviders

import (
	"testing"

	publicstt "github.com/KPO-Tech/seshat-runtime/pkg/audio/stt"
	publictts "github.com/KPO-Tech/seshat-runtime/pkg/audio/tts"
)

func TestNewOpenAISTT_PublicWrapper(t *testing.T) {
	c := NewOpenAISTT("test-key", WithSTTModel("whisper-1"), WithSTTLanguage("en"))
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
	var _ publicstt.SpeechToText = c // compile-time interface satisfaction check
	if c.Provider() != "openai" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "openai")
	}
}

func TestWithSTTBaseURL_PublicWrapper_AllowsSelfHostedWhisperCpp(t *testing.T) {
	// No API key at all - a local whisper.cpp server doesn't check auth.
	c := NewOpenAISTT("", WithSTTBaseURL("http://127.0.0.1:8080"))
	if c == nil {
		t.Fatal("expected a non-nil client for a self-hosted whisper.cpp-style server")
	}
}

func TestNewOpenAITTS_PublicWrapper(t *testing.T) {
	c := NewOpenAITTS("test-key", WithTTSVoice("alloy"), WithTTSFormat("mp3"))
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
	var _ publictts.Generation = c // compile-time interface satisfaction check
	if c.Provider() != "openai" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "openai")
	}
}
