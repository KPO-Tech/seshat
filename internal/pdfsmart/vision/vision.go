// Package vision implements pdfsmart's optional vision-LLM fallback stage:
// transcribing a rendered PDF page image via a vision-capable LLM when
// neither native text extraction nor docling could produce usable text for
// it. See pdfsmart.VisionTranscriber for the interface this implements and
// pdfsmart.Convert's own doc comment for where this slots into the
// page-processing pipeline.
package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/internal/model"
	"github.com/KPO-Tech/seshat/internal/types"
)

// LLMCaller is the minimal interface for calling the provider API.
// *providers.Client satisfies this - defined here as an interface (not
// imported from internal/providers) so Transcriber can be tested without a
// live network call, and so internal/pdfsmart never has to depend on
// internal/providers. Mirrors internal/rag/enricher's and
// internal/memory/longterm's identically-shaped LLMCaller, for the same
// reason.
type LLMCaller interface {
	CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error)
}

// Config controls Transcriber behavior. Zero-value fields are replaced with
// defaults by New, except Model, which must be set to a real, vision-capable
// model identifier for IsAvailable to ever report true.
type Config struct {
	// Model is the vision-capable model to send page images to.
	Model types.ModelIdentifier

	// Registry is consulted by IsAvailable to check whether Model actually
	// supports image content blocks (see model.Registry.VisionCapable) -
	// this is what gates the whole fallback behind "don't attempt this
	// against a text-only model". Defaults to model.Global (populated at
	// startup by internal/providers) when nil; tests can inject their own
	// via model.NewRegistry(), per that package's own doc comment.
	Registry *model.Registry

	// Timeout is the per-page call deadline. Default: 60s.
	Timeout time.Duration
}

func (cfg Config) withDefaults() Config {
	if cfg.Registry == nil {
		cfg.Registry = model.Global
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return cfg
}

// Transcriber sends a rendered PDF page image to a vision-capable LLM and
// returns its transcription - see pdfsmart.VisionTranscriber for the
// interface this implements (duck-typed, no import needed - see that
// interface's own doc comment for why).
type Transcriber struct {
	caller LLMCaller
	config Config
}

// New creates a Transcriber. caller is normally the same *providers.Client
// already used elsewhere for chat completions - see LLMCaller's doc
// comment for why that's an interface here, not the concrete type.
func New(caller LLMCaller, cfg Config) *Transcriber {
	return &Transcriber{caller: caller, config: cfg.withDefaults()}
}

// IsAvailable reports whether Model is actually configured as vision-capable
// - see Config.Registry. A nil caller or an empty Model also reports false.
func (t *Transcriber) IsAvailable(context.Context) bool {
	if t == nil || t.caller == nil || strings.TrimSpace(t.config.Model.Model) == "" {
		return false
	}
	return t.config.Registry.VisionCapable(string(t.config.Model.Provider), t.config.Model.Model)
}

const transcriptionPrompt = `Transcribe the visible text in this page image exactly as it appears, as clean markdown - preserve headings, lists, and table structure where visually apparent. Do not summarize, paraphrase, or add commentary. If the page is blank or contains no legible text, respond with exactly: (no text)`

func (t *Transcriber) TranscribePage(ctx context.Context, pngImage []byte) (string, error) {
	if t == nil || t.caller == nil {
		return "", fmt.Errorf("vision: no LLM caller configured")
	}
	if len(pngImage) == 0 {
		return "", fmt.Errorf("vision: empty page image")
	}
	callCtx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	img := types.ImageContent{}
	img.Source.Type = "base64"
	img.Source.MediaType = "image/png"
	img.Source.Data = base64.StdEncoding.EncodeToString(pngImage)

	req := types.APIRequest{
		Model: t.config.Model,
		Messages: []types.Message{
			types.UserMessageWithImage("vision-transcribe-req", transcriptionPrompt, img),
		},
		MaxTokens: 2048,
	}
	resp, err := t.caller.CreateMessage(callCtx, req)
	if err != nil {
		return "", fmt.Errorf("LLM call: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("nil response from LLM")
	}

	text := strings.TrimSpace(flattenTextContent(resp.Content))
	if text == "(no text)" {
		return "", nil
	}
	return text, nil
}

func flattenTextContent(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if tc, ok := b.(types.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
