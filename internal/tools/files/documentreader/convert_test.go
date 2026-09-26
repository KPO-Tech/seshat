package documentreader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/documentreader"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/types"
)

func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func TestConvertTool_Call_ReportsNotConfiguredWithoutDocumentReader(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "scan.pdf")
	writeFixture(t, filePath, "not a real pdf, just needs to exist")

	tl := NewConvertTool(Config{}, dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"path": filePath},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if !strings.Contains(result.Content, "requires a configured document reader") {
		t.Errorf("expected a 'requires a configured document reader' message, got:\n%s", result.Content)
	}
}

func TestConvertTool_Call_RejectsMissingPath(t *testing.T) {
	tl := NewConvertTool(Config{}, "/tmp")
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for missing path")
	}
}

func TestConvertTool_Call_RejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	tl := NewConvertTool(Config{DocumentReaderURL: "http://127.0.0.1:1"}, dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"path": filepath.Join(dir, "does-not-exist.pdf")},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for a nonexistent file")
	}
}

func TestConvertTool_ValidateInput(t *testing.T) {
	tl := NewConvertTool(Config{}, "/tmp")
	ctx := context.Background()
	if _, err := tl.ValidateInput(ctx, map[string]any{}); err == nil {
		t.Error("expected error for missing path")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"path": "x.pdf"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
}

func TestConvertTool_CheckPermissions_UNCBlocked(t *testing.T) {
	tl := NewConvertTool(Config{}, "/tmp")
	got := tl.CheckPermissions(context.Background(), map[string]any{
		"path": "//evil/share/x.pdf",
	}, defaultToolCtx())
	if got.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny for a UNC path, got %v", got.Behavior)
	}
}

func TestFormatConvertResult(t *testing.T) {
	out := formatConvertResult("scan.pdf", &documentreader.ConversionResult{
		Markdown:  "# Scanned Report\n\nOCR'd content here.",
		PageCount: 3,
		Images: []documentreader.ExtractedImage{
			{Filename: "image_0001.png", MimeType: "image/png", Base64: "iVBORw0KGgo"},
		},
	}, "")

	if !strings.Contains(out, "File: scan.pdf") {
		t.Errorf("expected source file in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Pages: 3") {
		t.Errorf("expected page count in output, got:\n%s", out)
	}
	if !strings.Contains(out, "# Scanned Report") {
		t.Errorf("expected markdown content in output, got:\n%s", out)
	}
	if !strings.Contains(out, "image_0001.png") {
		t.Errorf("expected image metadata in output, got:\n%s", out)
	}
}

func TestFormatConvertResult_IncludesSavedAt(t *testing.T) {
	out := formatConvertResult("scan.pdf", &documentreader.ConversionResult{Markdown: "content"}, "/tmp/out.md")
	if !strings.Contains(out, "Saved to: /tmp/out.md") {
		t.Errorf("expected saved-to line in output, got:\n%s", out)
	}
}
