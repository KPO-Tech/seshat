package audioproviders

import "testing"

// A speech provider that stops answering must not hold the call forever.
func TestSpeechClientsHaveATimeout(t *testing.T) {
	t.Parallel()
	if c := NewOpenAISTT("key"); c.httpClient.Timeout != requestTimeout {
		t.Errorf("STT client timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
	if c := NewOpenAITTS("key"); c.httpClient.Timeout != requestTimeout {
		t.Errorf("TTS client timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
}
