package documentreader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

type fakePageRenderer struct {
	gotPage int
}

func (f *fakePageRenderer) RenderPage(_ context.Context, _ []byte, pageNum int) ([]byte, error) {
	f.gotPage = pageNum
	return []byte("png-bytes"), nil
}

func TestRenderPageTool_Call_ReturnsPNGDataURI(t *testing.T) {
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
	if !strings.Contains(result.Content, "Mime-Type: image/png") {
		t.Fatalf("expected image/png metadata, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "data:image/png;base64,cG5nLWJ5dGVz") {
		t.Fatalf("expected png data URI, got:\n%s", result.Content)
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
