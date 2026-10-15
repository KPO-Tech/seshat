package imageproviders

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serve starts a stub answering response and records the request it receives.
func serve(t *testing.T, response string) (*httptest.Server, *request) {
	t.Helper()
	got := &request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.path, got.query, got.header = r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		_ = json.Unmarshal(b, &got.body)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

type request struct {
	path, query string
	header      http.Header
	body        map[string]any
}

func TestOpenAIGPTImageRequestHasNoDallEFields(t *testing.T) {
	srv, got := serve(t, `{"data":[{"b64_json":"QUJD"}]}`)
	c := NewOpenAI("k", WithOpenAIBaseURL(srv.URL))
	res, err := c.GenerateImage(context.Background(), "a cat")
	if err != nil || len(res.Images) != 1 || res.Images[0].ImageBase64 != "QUJD" {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if got.body["model"] != "gpt-image-2.5-flare" || got.body["quality"] != "auto" {
		t.Errorf("body = %v", got.body)
	}
	if _, has := got.body["response_format"]; has {
		t.Error("GPT Image models reject response_format")
	}
}

func TestOpenAIDallERequestKeepsItsFields(t *testing.T) {
	srv, got := serve(t, `{"data":[{"b64_json":"QUJD"}]}`)
	c := NewOpenAI("k", WithOpenAIBaseURL(srv.URL), WithOpenAIModel("dall-e-3"))
	if _, err := c.GenerateImage(context.Background(), "a cat"); err != nil {
		t.Fatal(err)
	}
	if got.body["response_format"] != "b64_json" || got.body["quality"] != "standard" {
		t.Errorf("dall-e body = %v", got.body)
	}
	// A DALL-E quality given for a GPT Image model must not be sent as is.
	srv2, got2 := serve(t, `{"data":[{"b64_json":"QUJD"}]}`)
	c2 := NewOpenAI("k", WithOpenAIBaseURL(srv2.URL), WithOpenAIQuality("hd"))
	if _, err := c2.GenerateImage(context.Background(), "a cat"); err != nil {
		t.Fatal(err)
	}
	if got2.body["quality"] != "auto" {
		t.Errorf("hd on a GPT Image model must become auto, got %v", got2.body["quality"])
	}
}

func TestGeminiImageModelUsesGenerateContent(t *testing.T) {
	srv, got := serve(t, `{"candidates":[{"content":{"parts":[{"text":"here"},{"inlineData":{"mimeType":"image/jpeg","data":"QUJD"}}]}}]}`)
	c := NewGemini("secret", WithGeminiBaseURL(srv.URL))
	res, err := c.GenerateImage(context.Background(), "a cat")
	if err != nil || len(res.Images) != 1 || res.Images[0].MIMEType != "image/jpeg" || res.Images[0].ImageBase64 != "QUJD" {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if got.path != "/models/gemini-nano-banana-2.1:generateContent" || got.query != "" || got.header.Get("x-goog-api-key") != "secret" {
		t.Errorf("request = %s ?%s key=%q (the key must travel in the header)", got.path, got.query, got.header.Get("x-goog-api-key"))
	}
}

func TestGeminiImagenModelKeepsPredict(t *testing.T) {
	srv, got := serve(t, `{"predictions":[{"bytesBase64Encoded":"QUJD","mimeType":"image/png"}]}`)
	c := NewGemini("k", WithGeminiBaseURL(srv.URL), WithGeminiModel("imagen-4.0-generate-001"))
	if res, err := c.GenerateImage(context.Background(), "a cat"); err != nil || len(res.Images) != 1 {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if got.path != "/models/imagen-4.0-generate-001:predict" {
		t.Errorf("path = %s", got.path)
	}
}

func TestGeminiBlockedPromptIsAnError(t *testing.T) {
	srv, _ := serve(t, `{"candidates":[{"content":{"parts":[{"text":"sorry"}]}}]}`)
	if _, err := NewGemini("k", WithGeminiBaseURL(srv.URL)).GenerateImage(context.Background(), "x"); err == nil {
		t.Fatal("a response without an image must be an error")
	}
}

func TestOpenRouterImage(t *testing.T) {
	srv, got := serve(t, `{"data":[{"b64_json":"QUJD","media_type":"image/webp"}]}`)
	c := NewOpenRouter("k", WithOpenRouterBaseURL(srv.URL+"/"), WithOpenRouterModel("black-forest-labs/flux-3-image"), WithOpenRouterAspectRatio("16:9"))
	res, err := c.GenerateImage(context.Background(), "a cat")
	if err != nil || len(res.Images) != 1 || res.Images[0].MIMEType != "image/webp" {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if got.path != "/images" || got.header.Get("Authorization") != "Bearer k" || got.body["model"] != "black-forest-labs/flux-3-image" || got.body["aspect_ratio"] != "16:9" {
		t.Errorf("request = %s %v %v", got.path, got.header.Get("Authorization"), got.body)
	}
	if c.Provider() != "openrouter" {
		t.Errorf("Provider() = %q", c.Provider())
	}
}

func TestProfileFor(t *testing.T) {
	p, ok := ProfileFor(" OpenRouter ")
	if !ok || p.API != APIOpenRouter || p.DefaultModel() != "google/gemini-nano-banana-2.1" {
		t.Fatalf("openrouter profile = %+v ok=%v", p, ok)
	}
	if _, ok := ProfileFor("nope"); ok {
		t.Fatal("unknown provider must not resolve")
	}
}
