package documentreader

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/types"
)

type fakePageRenderer struct {
	gotPage   int
	oversized bool
}

func (f *fakePageRenderer) RenderPage(_ context.Context, _ []byte, pageNum int) ([]byte, error) {
	f.gotPage = pageNum
	if f.oversized {
		return make([]byte, maxInlinedImageBytes+1), nil
	}
	return []byte("png-bytes"), nil
}

func TestRenderPageTool_Call_AttachesRealImageContentBlock(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(filePath, []byte("%PDF test"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	renderer := &fakePageRenderer{}
	tl := NewRenderPageTool(Config{DocumentPageRenderer: renderer}, dir)

	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{
		"path": filePath,
		"page": float64(2),
	}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if renderer.gotPage != 2 {
		t.Fatalf("expected page 2, got %d", renderer.gotPage)
	}

	// The tool result itself must stay short text - no base64 blob, so it
	// never gets mangled by MicroCompactor's character-based trim threshold
	// and never wastes tokens on an opaque blob the model can't interpret.
	if strings.Contains(result.Content, "base64") {
		t.Fatalf("tool result Content must not embed base64 image data, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "Page 2") && !strings.Contains(result.Content, "page 2") {
		t.Fatalf("expected page number in summary, got:\n%s", result.Content)
	}

	// The actual image must be delivered as a real ImageContent block in a
	// follow-up message - this is the only way any provider adapter renders
	// it as a viewable image rather than literal text (see types.ToolResultContent's
	// Content field, which is a plain string).
	if len(result.NewMessages) != 1 {
		t.Fatalf("expected exactly one follow-up message carrying the image, got %d", len(result.NewMessages))
	}
	msg := result.NewMessages[0]
	if msg.Role != types.RoleUser {
		t.Fatalf("expected follow-up message to be user role, got %s", msg.Role)
	}
	var foundImage *types.ImageContent
	for _, block := range msg.Content {
		if img, ok := block.(types.ImageContent); ok {
			imgCopy := img
			foundImage = &imgCopy
		}
	}
	if foundImage == nil {
		t.Fatalf("expected an ImageContent block in the follow-up message, content: %#v", msg.Content)
	}
	if foundImage.Source.MediaType != "image/png" {
		t.Fatalf("expected image/png media type, got %s", foundImage.Source.MediaType)
	}
	wantData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	if foundImage.Source.Data != wantData {
		t.Fatalf("expected image data %q, got %q", wantData, foundImage.Source.Data)
	}
}

func TestRenderPageTool_Call_RejectsOversizedRender(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(filePath, []byte("%PDF test"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	renderer := &fakePageRenderer{oversized: true}
	tl := NewRenderPageTool(Config{DocumentPageRenderer: renderer}, dir)

	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{
		"path": filePath,
		"page": float64(1),
	}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if len(result.NewMessages) != 0 {
		t.Fatalf("expected no follow-up image message for an oversized render, got %d", len(result.NewMessages))
	}
	if !strings.Contains(result.Content, "exceeds") {
		t.Fatalf("expected a size-limit explanation, got:\n%s", result.Content)
	}
}

func TestRenderPageTool_Call_ReportsNotConfiguredWithoutRenderer(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(filePath, []byte("%PDF test"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	tl := NewRenderPageTool(Config{}, dir)

	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: map[string]any{
		"path": filePath,
		"page": float64(1),
	}}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if !strings.Contains(result.Content, "requires a configured native document page renderer") {
		t.Fatalf("expected not configured message, got:\n%s", result.Content)
	}
}
