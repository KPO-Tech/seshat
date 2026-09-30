package multimedia

import (
	"context"
	"errors"
	"testing"

	"github.com/KPO-Tech/seshat/internal/image"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

type fakeGenerator struct {
	resp     *image.GenerationResponse
	err      error
	provider string
}

func (f *fakeGenerator) GenerateImage(_ context.Context, _ string) (*image.GenerationResponse, error) {
	return f.resp, f.err
}

func (f *fakeGenerator) Provider() string { return f.provider }

func TestImageGenTool_IsEnabled(t *testing.T) {
	if (&ImageGenTool{}).IsEnabled() {
		t.Fatal("expected IsEnabled() to be false with no generator configured")
	}
	tl := NewImageGenTool(&fakeGenerator{})
	if !tl.IsEnabled() {
		t.Fatal("expected IsEnabled() to be true once a generator is configured")
	}
}

func TestImageGenTool_Call_NoGeneratorConfigured(t *testing.T) {
	tl := NewImageGenTool(nil)
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{"prompt": "a cat"}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error result when no generator is configured")
	}
}

func TestImageGenTool_Call_MissingPrompt(t *testing.T) {
	tl := NewImageGenTool(&fakeGenerator{})
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error result for a missing prompt")
	}
}

func TestImageGenTool_Call_GeneratorError(t *testing.T) {
	tl := NewImageGenTool(&fakeGenerator{err: errors.New("provider down")})
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{"prompt": "a cat"}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error result when the provider call fails")
	}
}

func TestImageGenTool_Call_NoImagesReturned(t *testing.T) {
	tl := NewImageGenTool(&fakeGenerator{resp: &image.GenerationResponse{Images: nil}})
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{"prompt": "a cat"}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error result when the provider returns zero images")
	}
}

func TestImageGenTool_Call_Success(t *testing.T) {
	gen := &fakeGenerator{
		provider: "openai",
		resp: &image.GenerationResponse{
			Model: "dall-e-3",
			Images: []image.GenerationResult{{
				ImageBase64:   "aGVsbG8=",
				MIMEType:      "image/png",
				RevisedPrompt: "a fluffy cat, detailed, studio lighting",
			}},
		},
	}
	tl := NewImageGenTool(gen)
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{"prompt": "a cat"}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if result.ContentType != tool.ContentTypeJSON {
		t.Fatalf("expected JSON content type, got %s", result.ContentType)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected result.Data to be a map, got %T", result.Data)
	}
	if data["provider"] != "openai" {
		t.Fatalf("expected provider openai, got %v", data["provider"])
	}
	if data["image_base64"] != "aGVsbG8=" {
		t.Fatalf("expected the base64 image to be passed through, got %v", data["image_base64"])
	}
	if data["revised_prompt"] != "a fluffy cat, detailed, studio lighting" {
		t.Fatalf("expected the revised prompt to be passed through, got %v", data["revised_prompt"])
	}
}

func TestImageGenTool_FormatResult(t *testing.T) {
	tl := &ImageGenTool{}

	base64Result := tl.FormatResult(map[string]any{
		"provider":     "openai",
		"model":        "dall-e-3",
		"image_base64": "aGVsbG8=",
	})
	if base64Result == "" {
		t.Fatal("expected a non-empty summary for a base64 result")
	}

	urlResult := tl.FormatResult(map[string]any{
		"provider":  "gemini",
		"model":     "imagen-3",
		"image_url": "https://example.com/cat.png",
	})
	if urlResult == "" {
		t.Fatal("expected a non-empty summary for a URL result")
	}
}
