package audioproviders

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sttFormField runs one Transcribe against a stub server and returns the
// response_format form field it received, and whether it was present at all.
func sttFormField(t *testing.T, opts ...OpenAISTTOption) (string, bool) {
	t.Helper()
	var got string
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("content type: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			if part.FormName() == "response_format" {
				b, _ := io.ReadAll(part)
				got, present = string(b), true
			}
		}
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	}))
	defer srv.Close()
	c := NewOpenAISTT("k", append([]OpenAISTTOption{WithSTTBaseURL(srv.URL)}, opts...)...)
	if _, err := c.Transcribe(context.Background(), []byte("audio")); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	return got, present
}

func TestSTTResponseFormatFollowsTheModel(t *testing.T) {
	if got, _ := sttFormField(t); got != "verbose_json" {
		t.Errorf("default whisper-1: response_format = %q, want verbose_json", got)
	}
	if got, _ := sttFormField(t, WithSTTModel("gpt-transcribe")); got != "json" {
		t.Errorf("gpt-transcribe: response_format = %q, want json (it rejects verbose_json)", got)
	}
	if got, _ := sttFormField(t, WithSTTModel("whisper-large-v3-turbo")); got != "verbose_json" {
		t.Errorf("groq whisper: response_format = %q, want verbose_json", got)
	}
	if _, present := sttFormField(t, WithSTTModel("voxtral-mini-latest"), WithSTTResponseFormat(ResponseFormatOmit)); present {
		t.Error("ResponseFormatOmit must send no response_format field")
	}
}

func TestProviderNameDefaultsToOpenAIAndCanBeSet(t *testing.T) {
	if got := NewOpenAISTT("k").Provider(); got != "openai" {
		t.Errorf("STT default name = %q", got)
	}
	if got := NewOpenAITTS("k", WithTTSProviderName("acme")).Provider(); got != "acme" {
		t.Errorf("TTS name = %q", got)
	}
}

func TestProfileFor(t *testing.T) {
	p, ok := ProfileFor(" Groq ")
	if !ok || p.STT.DefaultModel() != "whisper-large-v3-turbo" || p.TTS != nil {
		t.Fatalf("groq profile wrong: %+v ok=%v", p, ok)
	}
	if _, ok := ProfileFor("nope"); ok {
		t.Fatal("unknown provider must not resolve")
	}
	if got := (*Capability)(nil).DefaultModel(); got != "" {
		t.Fatalf("nil capability default = %q", got)
	}
}
