package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribeAudioReturnsTranscript(t *testing.T) {
	var gotAuth, gotModel string
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		gotModel = r.FormValue("model")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":     "hello world",
			"language": "en",
			"duration": 1.5,
		})
	}))
	defer fakeServer.Close()

	result, err := TranscribeAudio(context.Background(), "sk-test", []byte("fake-audio-bytes"), WithTranscribeBaseURL(fakeServer.URL))
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if result.Text != "hello world" {
		t.Fatalf("expected transcript text, got %q", result.Text)
	}
	if result.Language != "en" {
		t.Fatalf("expected detected language, got %q", result.Language)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("expected the API key as bearer auth, got %q", gotAuth)
	}
	if gotModel != "whisper-1" {
		t.Fatalf("expected the default whisper-1 model, got %q", gotModel)
	}
}

func TestTranscribeAudioHonorsModelOption(t *testing.T) {
	var gotModel string
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		gotModel = r.FormValue("model")
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "ok"})
	}))
	defer fakeServer.Close()

	if _, err := TranscribeAudio(
		context.Background(), "sk-test", []byte("audio"),
		WithTranscribeBaseURL(fakeServer.URL), WithTranscribeModel("whisper-2"),
	); err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if gotModel != "whisper-2" {
		t.Fatalf("expected the overridden model, got %q", gotModel)
	}
}

func TestTranscribeAudioPropagatesAPIError(t *testing.T) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer fakeServer.Close()

	if _, err := TranscribeAudio(context.Background(), "bad-key", []byte("audio"), WithTranscribeBaseURL(fakeServer.URL)); err == nil {
		t.Fatal("expected an error for an invalid API key")
	}
}
