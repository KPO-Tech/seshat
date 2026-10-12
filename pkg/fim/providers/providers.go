// Package fimproviders exposes seshat's fim.Completer implementations for
// direct use by a host application, independent of the agent/tool loop.
package fimproviders

import (
	"net/http"

	internalfim "github.com/KPO-Tech/seshat-runtime/internal/fim"
	internalproviders "github.com/KPO-Tech/seshat-runtime/internal/fim/providers"
)

type (
	// MistralOption configures a Mistral FIM client.
	MistralOption = internalproviders.MistralOption
	// DeepSeekOption configures a DeepSeek FIM client.
	DeepSeekOption = internalproviders.DeepSeekOption
)

// NewMistral creates a Mistral Codestral FIM completion client.
func NewMistral(opts ...MistralOption) internalfim.Completer {
	return internalproviders.NewMistral(opts...)
}

// WithMistralAPIKey sets the API key.
func WithMistralAPIKey(key string) MistralOption { return internalproviders.WithMistralAPIKey(key) }

// WithMistralModel sets the completion model.
func WithMistralModel(model string) MistralOption { return internalproviders.WithMistralModel(model) }

// WithMistralMaxTokens caps the number of generated tokens.
func WithMistralMaxTokens(n int64) MistralOption { return internalproviders.WithMistralMaxTokens(n) }

// WithMistralTemperature sets sampling temperature.
func WithMistralTemperature(t float64) MistralOption {
	return internalproviders.WithMistralTemperature(t)
}

// WithMistralBaseURL overrides the default endpoint (self-hosted server, or a proxy).
func WithMistralBaseURL(url string) MistralOption { return internalproviders.WithMistralBaseURL(url) }

// WithMistralHTTPClient injects a custom HTTP client.
func WithMistralHTTPClient(hc *http.Client) MistralOption {
	return internalproviders.WithMistralHTTPClient(hc)
}

// NewDeepSeek creates a DeepSeek FIM completion client.
func NewDeepSeek(opts ...DeepSeekOption) internalfim.Completer {
	return internalproviders.NewDeepSeek(opts...)
}

// WithDeepSeekAPIKey sets the API key.
func WithDeepSeekAPIKey(key string) DeepSeekOption { return internalproviders.WithDeepSeekAPIKey(key) }

// WithDeepSeekModel sets the completion model.
func WithDeepSeekModel(model string) DeepSeekOption {
	return internalproviders.WithDeepSeekModel(model)
}

// WithDeepSeekMaxTokens caps the number of generated tokens.
func WithDeepSeekMaxTokens(n int64) DeepSeekOption {
	return internalproviders.WithDeepSeekMaxTokens(n)
}

// WithDeepSeekTemperature sets sampling temperature.
func WithDeepSeekTemperature(t float64) DeepSeekOption {
	return internalproviders.WithDeepSeekTemperature(t)
}

// WithDeepSeekBaseURL overrides the default endpoint (self-hosted server, or a proxy).
func WithDeepSeekBaseURL(url string) DeepSeekOption {
	return internalproviders.WithDeepSeekBaseURL(url)
}

// WithDeepSeekHTTPClient injects a custom HTTP client.
func WithDeepSeekHTTPClient(hc *http.Client) DeepSeekOption {
	return internalproviders.WithDeepSeekHTTPClient(hc)
}
