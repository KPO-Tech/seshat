package vision

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/model"
	"github.com/KPO-Tech/seshat/internal/types"
)

type fakeCaller struct {
	respFor func(req types.APIRequest) (string, error)
	lastReq types.APIRequest
}

func (f *fakeCaller) CreateMessage(_ context.Context, req types.APIRequest) (*types.APIResponse, error) {
	f.lastReq = req
	text, err := f.respFor(req)
	if err != nil {
		return nil, err
	}
	return &types.APIResponse{Content: []types.ContentBlock{types.TextContent{Text: text}}}, nil
}

func visionRegistry() *model.Registry {
	r := model.NewRegistry()
	r.Register(model.Metadata{
		ID:           "vision-model",
		Provider:     "test-provider",
		Capabilities: model.Capabilities{Vision: true},
	})
	r.Register(model.Metadata{
		ID:           "text-only-model",
		Provider:     "test-provider",
		Capabilities: model.Capabilities{Vision: false},
	})
	return r
}

func TestTranscriber_IsAvailable_TrueForVisionCapableModel(t *testing.T) {
	tr := New(&fakeCaller{}, Config{
		Model:    types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"},
		Registry: visionRegistry(),
	})
	if !tr.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to be true for a registered vision-capable model")
	}
}

func TestTranscriber_IsAvailable_FalseForTextOnlyModel(t *testing.T) {
	tr := New(&fakeCaller{}, Config{
		Model:    types.ModelIdentifier{Provider: "test-provider", Model: "text-only-model"},
		Registry: visionRegistry(),
	})
	if tr.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to be false for a text-only model")
	}
}

func TestTranscriber_IsAvailable_FalseForUnknownModel(t *testing.T) {
	tr := New(&fakeCaller{}, Config{
		Model:    types.ModelIdentifier{Provider: "test-provider", Model: "does-not-exist"},
		Registry: visionRegistry(),
	})
	if tr.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to be false for a model absent from the registry")
	}
}

func TestTranscriber_IsAvailable_FalseWithNilCaller(t *testing.T) {
	tr := New(nil, Config{
		Model:    types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"},
		Registry: visionRegistry(),
	})
	if tr.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to be false with a nil caller")
	}
}

func TestTranscriber_TranscribePage_SendsImageAndReturnsText(t *testing.T) {
	caller := &fakeCaller{respFor: func(types.APIRequest) (string, error) {
		return "# Transcribed heading\n\nSome body text.", nil
	}}
	tr := New(caller, Config{Model: types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"}, Registry: visionRegistry()})

	got, err := tr.TranscribePage(context.Background(), []byte("fake-png-bytes"))
	if err != nil {
		t.Fatalf("TranscribePage: %v", err)
	}
	if got != "# Transcribed heading\n\nSome body text." {
		t.Fatalf("unexpected transcription: %q", got)
	}

	// The image must have actually been attached to the outgoing request.
	var sawImage bool
	for _, msg := range caller.lastReq.Messages {
		for _, block := range msg.Content {
			if img, ok := block.(types.ImageContent); ok {
				sawImage = true
				if img.Source.MediaType != "image/png" {
					t.Errorf("expected image/png media type, got %q", img.Source.MediaType)
				}
				if img.Source.Data == "" {
					t.Error("expected non-empty base64 image data")
				}
			}
		}
	}
	if !sawImage {
		t.Fatal("expected an ImageContent block in the outgoing request")
	}
}

func TestTranscriber_TranscribePage_BlankPageSentinelReturnsEmptyString(t *testing.T) {
	caller := &fakeCaller{respFor: func(types.APIRequest) (string, error) {
		return "(no text)", nil
	}}
	tr := New(caller, Config{Model: types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"}, Registry: visionRegistry()})

	got, err := tr.TranscribePage(context.Background(), []byte("fake-png-bytes"))
	if err != nil {
		t.Fatalf("TranscribePage: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for the blank-page sentinel, got %q", got)
	}
}

func TestTranscriber_TranscribePage_PropagatesCallerError(t *testing.T) {
	caller := &fakeCaller{respFor: func(types.APIRequest) (string, error) {
		return "", errors.New("simulated LLM failure")
	}}
	tr := New(caller, Config{Model: types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"}, Registry: visionRegistry()})

	if _, err := tr.TranscribePage(context.Background(), []byte("fake-png-bytes")); err == nil {
		t.Fatal("expected TranscribePage to propagate the caller's error")
	}
}

func TestTranscriber_TranscribePage_RejectsEmptyImage(t *testing.T) {
	tr := New(&fakeCaller{}, Config{Model: types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"}, Registry: visionRegistry()})
	if _, err := tr.TranscribePage(context.Background(), nil); err == nil {
		t.Fatal("expected an error for an empty page image")
	}
}

func TestTranscriber_TranscribePage_TrimsWhitespace(t *testing.T) {
	caller := &fakeCaller{respFor: func(types.APIRequest) (string, error) {
		return "  \n  transcribed text  \n  ", nil
	}}
	tr := New(caller, Config{Model: types.ModelIdentifier{Provider: "test-provider", Model: "vision-model"}, Registry: visionRegistry()})

	got, err := tr.TranscribePage(context.Background(), []byte("fake-png-bytes"))
	if err != nil {
		t.Fatalf("TranscribePage: %v", err)
	}
	if got != "transcribed text" {
		t.Fatalf("expected trimmed text, got %q", got)
	}
	if strings.TrimSpace(got) != got {
		t.Fatalf("result was not fully trimmed: %q", got)
	}
}
