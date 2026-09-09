package types

import "testing"

func TestGetContextWindowIsPublic(t *testing.T) {
	window := GetContextWindow(ModelIdentifier{
		Provider: APIProviderZAi,
		Model:    "glm-4.5",
	})
	if window.MaxTokens <= 0 {
		t.Fatalf("MaxTokens = %d, want positive", window.MaxTokens)
	}
	if window.MaxOutputTokens <= 0 {
		t.Fatalf("MaxOutputTokens = %d, want positive", window.MaxOutputTokens)
	}
}
