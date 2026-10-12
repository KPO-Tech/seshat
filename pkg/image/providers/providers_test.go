package imageproviders

import (
	"testing"

	publicimage "github.com/KPO-Tech/seshat-runtime/pkg/image"
)

func TestNewOpenAI_PublicWrapper(t *testing.T) {
	c := NewOpenAI("test-key", WithOpenAIModel("dall-e-2"), WithOpenAISize("512x512"))
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
	var _ publicimage.Generation = c // compile-time interface satisfaction check
	if c.Provider() != "openai" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "openai")
	}
}

func TestNewGemini_PublicWrapper(t *testing.T) {
	c := NewGemini("test-key", WithGeminiModel("imagen-3.0-fast-generate-001"), WithGeminiCount(2))
	if c == nil {
		t.Fatal("expected a non-nil client")
	}
	var _ publicimage.Generation = c
	if c.Provider() != "gemini" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "gemini")
	}
}

func TestWithOpenAIBaseURL_PublicWrapper_AllowsSelfHosted(t *testing.T) {
	c := NewOpenAI("", WithOpenAIBaseURL("http://localhost:8000/v1"))
	if c == nil {
		t.Fatal("expected a non-nil client even with an empty API key, given a self-hosted BaseURL")
	}
}
