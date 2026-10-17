package workspace

import "testing"

func TestKimiDefaultProviderBaseURLUsesInternationalEndpoint(t *testing.T) {
	if got := defaultProviderBaseURL("kimi"); got != "https://api.moonshot.ai/v1" {
		t.Fatalf("defaultProviderBaseURL(kimi) = %q, want https://api.moonshot.ai/v1", got)
	}
}
