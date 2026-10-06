package imageproviders

import "testing"

// An image provider that stops answering must not hold the call forever.
func TestImageClientsHaveATimeout(t *testing.T) {
	t.Parallel()
	if c := NewOpenAI("key"); c.httpClient.Timeout != requestTimeout {
		t.Errorf("OpenAI client timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
	if c := NewGemini("key"); c.httpClient.Timeout != requestTimeout {
		t.Errorf("Gemini client timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
}
