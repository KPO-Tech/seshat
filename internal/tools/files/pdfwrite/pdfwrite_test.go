package pdfwrite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"github.com/KPO-Tech/seshat/internal/pdftext"
	fileReadTool "github.com/KPO-Tech/seshat/internal/tools/files/read"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/types"
)

func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

func extractText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	res, err := pdftext.Extract(data)
	if err != nil {
		t.Fatalf("pdftext.Extract: %v", err)
	}
	return res.Text
}

func TestTool_Call_CreatesNewPDFWithHeadingAndParagraph(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"content":   "# Report Title\n\nThis is the first paragraph of the report, describing Alice's findings.\n",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	text := extractText(t, path)
	if !strings.Contains(text, "Report Title") {
		t.Errorf("expected heading text in extracted PDF text, got:\n%s", text)
	}
	if !strings.Contains(text, "Alice") {
		t.Errorf("expected paragraph text in extracted PDF text, got:\n%s", text)
	}
}

func TestTool_Call_RequiresReadFirstForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.pdf")

	// Build the fixture directly via renderTextToPDF, bypassing the tool's
	// own write path, so no read-state is recorded for it - same pattern
	// excel_edit/docx_edit use to prove the read-before-write gate actually
	// blocks.
	docBytes, err := renderTextToPDF("Original content.")
	if err != nil {
		t.Fatalf("renderTextToPDF: %v", err)
	}
	if err := os.WriteFile(path, docBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"content":   "Appended content.",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error result for editing an unread file")
	}
	if !strings.Contains(result.Error.Error(), "has not been read yet") {
		t.Errorf("expected a read-first error, got: %v", result.Error)
	}
}

func TestTool_Call_AppendsToExistingPDFAfterRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.pdf")

	docBytes, err := renderTextToPDF("Page one content about Bob.")
	if err != nil {
		t.Fatalf("renderTextToPDF: %v", err)
	}
	if err := os.WriteFile(path, docBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	fileReadTool.RecordExternalRead(path, info.ModTime(), "", true)

	countBefore, err := api.PageCountFile(context.Background(), path)
	if err != nil {
		t.Fatalf("PageCountFile before: %v", err)
	}

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"content":   "# New Section\n\nSecond page content about Carol.",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	countAfter, err := api.PageCountFile(context.Background(), path)
	if err != nil {
		t.Fatalf("PageCountFile after: %v", err)
	}
	if countAfter <= countBefore {
		t.Errorf("expected page count to grow after append, before=%d after=%d", countBefore, countAfter)
	}

	text := extractText(t, path)
	if !strings.Contains(text, "Bob") || !strings.Contains(text, "Carol") {
		t.Errorf("expected both original and appended content present, got:\n%s", text)
	}
}

func TestTool_Call_DeletesPagesFromExistingPDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.pdf")

	// Force multiple pages via explicit form-feed-like content: a long run
	// of blank-line-separated paragraphs is unreliable to size exactly, so
	// build a 3-page PDF the same way the tool itself would produce one via
	// two appends, which is deterministic (one render call == "however many
	// pages the content naturally takes", which for a single short line is
	// always 1).
	first, err := renderTextToPDF("Page one.")
	if err != nil {
		t.Fatalf("renderTextToPDF: %v", err)
	}
	if err := os.WriteFile(path, first, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := appendContentToFile(context.Background(), path, "Page two."); err != nil {
		t.Fatalf("appendContentToFile (seed page 2): %v", err)
	}
	if err := appendContentToFile(context.Background(), path, "Page three."); err != nil {
		t.Fatalf("appendContentToFile (seed page 3): %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	fileReadTool.RecordExternalRead(path, info.ModTime(), "", true)

	countBefore, err := api.PageCountFile(context.Background(), path)
	if err != nil {
		t.Fatalf("PageCountFile before: %v", err)
	}
	if countBefore != 3 {
		t.Fatalf("expected 3-page fixture, got %d", countBefore)
	}

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":    path,
			"delete_pages": "2",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	countAfter, err := api.PageCountFile(context.Background(), path)
	if err != nil {
		t.Fatalf("PageCountFile after: %v", err)
	}
	if countAfter != 2 {
		t.Errorf("expected 2 pages after deleting page 2, got %d", countAfter)
	}

	text := extractText(t, path)
	if strings.Contains(text, "Page two.") {
		t.Errorf("expected 'Page two.' to be gone after deletion, got:\n%s", text)
	}
	if !strings.Contains(text, "Page one.") || !strings.Contains(text, "Page three.") {
		t.Errorf("expected pages one and three to survive deletion, got:\n%s", text)
	}
}

func TestTool_Call_RejectsBothContentAndDeletePages(t *testing.T) {
	dir := t.TempDir()
	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":    filepath.Join(dir, "x.pdf"),
			"content":      "hello",
			"delete_pages": "1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for mutually exclusive content+delete_pages")
	}
}

func TestTool_Call_RejectsNonPDFExtension(t *testing.T) {
	dir := t.TempDir()
	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": filepath.Join(dir, "x.txt"),
			"content":   "hello",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for a non-.pdf file_path")
	}
}

func TestParseSelectedPages(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"2", []string{"2"}, false},
		{"2-4", []string{"2-4"}, false},
		{"2,4-6", []string{"2", "4-6"}, false},
		{" 2 , 4-6 ", []string{"2", "4-6"}, false},
		{"", nil, true},
		{"x", nil, true},
		{"2-", nil, true},
	}
	for _, c := range cases {
		got, err := parseSelectedPages(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSelectedPages(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSelectedPages(%q): unexpected error: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseSelectedPages(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTool_ValidateInput(t *testing.T) {
	tl := NewTool("/tmp")
	ctx := context.Background()
	if _, err := tl.ValidateInput(ctx, map[string]any{}); err == nil {
		t.Error("expected error for missing file_path")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "x.pdf"}); err == nil {
		t.Error("expected error for neither content nor delete_pages")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "x.pdf", "content": "hi"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "x.pdf", "delete_pages": "1"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "x.pdf", "content": "hi", "delete_pages": "1"}); err == nil {
		t.Error("expected error for both content and delete_pages")
	}
}

func TestTool_CheckPermissions_UNCBlocked(t *testing.T) {
	tl := NewTool("/tmp")
	got := tl.CheckPermissions(context.Background(), map[string]any{
		"file_path": "//evil/share/x.pdf",
		"content":   "hi",
	}, defaultToolCtx())
	if got.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny for a UNC path, got %v", got.Behavior)
	}
}
