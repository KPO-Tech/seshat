package fimproviders

import (
	"testing"

	publicfim "github.com/KPO-Tech/seshat-runtime/pkg/fim"
)

func TestNewMistral_PublicWrapper(t *testing.T) {
	c := NewMistral(WithMistralAPIKey("test-key"), WithMistralModel("codestral-latest"))
	if c == nil {
		t.Fatal("expected a non-nil completer")
	}
	var _ publicfim.Completer = c // compile-time interface satisfaction check
	if c.Provider() != "mistral" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "mistral")
	}
	if c.Model() != "codestral-latest" {
		t.Errorf("Model() = %q, want %q", c.Model(), "codestral-latest")
	}
}

func TestNewDeepSeek_PublicWrapper(t *testing.T) {
	c := NewDeepSeek(WithDeepSeekAPIKey("test-key"))
	if c == nil {
		t.Fatal("expected a non-nil completer")
	}
	var _ publicfim.Completer = c
	if c.Provider() != "deepseek" {
		t.Errorf("Provider() = %q, want %q", c.Provider(), "deepseek")
	}
}

func TestWithMistralBaseURL_PublicWrapper_AllowsSelfHosted(t *testing.T) {
	c := NewMistral(WithMistralBaseURL("http://localhost:8000/v1"))
	if c == nil {
		t.Fatal("expected a non-nil completer even with no API key, given a self-hosted BaseURL")
	}
}
