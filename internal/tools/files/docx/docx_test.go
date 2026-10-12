package docx

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileReadTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/read"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	officetext "github.com/KPO-Tech/seshat-runtime/pkg/officetext"
)

func TestPrettyPrintCompactRoundTrip(t *testing.T) {
	original := `<?xml version="1.0"?><w:document><w:body><w:p><w:r><w:t>Hello</w:t></w:r></w:p></w:body></w:document>`
	pretty := prettyPrintXML(original)
	if !strings.Contains(pretty, "\n") {
		t.Fatalf("expected pretty-printed XML to span multiple lines, got:\n%s", pretty)
	}
	compacted := compactXML(pretty)
	if compacted != original {
		t.Errorf("round-trip mismatch:\ngot:  %q\nwant: %q", compacted, original)
	}
}

func TestPrettyPrintCompactRoundTrip_PreservesInternalWhitespace(t *testing.T) {
	// A text node with leading/trailing spaces must survive round-tripping -
	// compactXML must never trim whitespace that's part of the text itself.
	original := `<w:p><w:r><w:t xml:space="preserve">  two spaces before, one after </w:t></w:r></w:p>`
	pretty := prettyPrintXML(original)
	compacted := compactXML(pretty)
	if compacted != original {
		t.Errorf("expected text-node whitespace preserved:\ngot:  %q\nwant: %q", compacted, original)
	}
}

func TestTool_Call_CreatesNewDocxWithHeadingsAndParagraphs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "# Report Title\n\nA plain paragraph.\n\n## Section One\n\nAnother paragraph here.",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	// Round-trip through the real native reader, same discipline as the
	// excel_edit tests: prove write+read agree, not just that our own zip
	// construction "looks right".
	markdown, ok, _, err := officetext.Extract("report.docx", data)
	if err != nil || !ok {
		t.Fatalf("failed to read back the written docx: ok=%v err=%v", ok, err)
	}
	for _, want := range []string{"# Report Title", "A plain paragraph.", "## Section One", "Another paragraph here."} {
		if !strings.Contains(markdown, want) {
			t.Errorf("expected %q in round-tripped content, got:\n%s", want, markdown)
		}
	}
}

func TestTool_Call_EditsExistingDocxAfterRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")

	tl := NewTool(dir)
	if _, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "Original paragraph text.",
		},
	}, nil); err != nil {
		t.Fatalf("initial create Call: %v", err)
	}

	// The create call already recorded read-state (post-write, same
	// convention as write_file/excel_edit), so this edit is allowed.
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"old_string": "Original paragraph text.",
			"new_string": "Edited paragraph text.",
		},
	}, nil)
	if err != nil {
		t.Fatalf("edit Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("edit Call returned an error result: %v", result.Error)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	markdown, ok, _, err := officetext.Extract("report.docx", data)
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	if strings.Contains(markdown, "Original paragraph text.") {
		t.Errorf("expected the original text to be replaced, still present in:\n%s", markdown)
	}
	if !strings.Contains(markdown, "Edited paragraph text.") {
		t.Errorf("expected the new text, got:\n%s", markdown)
	}
}

func TestTool_Call_RequiresReadFirstForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.docx")

	// Built directly, bypassing this tool's own write path (which records
	// read-state on success) - the only way to get a file with genuinely no
	// read-state recorded, since that cache is a process-wide singleton.
	docBytes, err := createMinimalDocx("preexisting content")
	if err != nil {
		t.Fatalf("createMinimalDocx: %v", err)
	}
	if err := os.WriteFile(path, docBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"old_string": "preexisting content",
			"new_string": "replaced",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error requiring a read before editing an existing file")
	}
}

func TestTool_Call_FuzzyMatchSuggestionOnNearMiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")

	tl := NewTool(dir)
	if _, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "The quick brown fox jumps over the lazy dog.",
		},
	}, nil); err != nil {
		t.Fatalf("create Call: %v", err)
	}

	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"old_string": "The quick brown fox jumps over the lazy dog!", // trailing ! instead of .
			"new_string": "replacement",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for a near-miss old_string")
	}
	if !strings.Contains(result.Error.Error(), "Closest match") {
		t.Errorf("expected a fuzzy-match suggestion in the error, got: %v", result.Error)
	}
}

func TestTool_Call_CreateFailsIfFileAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.docx")
	docBytes, err := createMinimalDocx("x")
	if err != nil {
		t.Fatalf("createMinimalDocx: %v", err)
	}
	if err := os.WriteFile(path, docBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "new content",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error creating over an existing file without old_string")
	}
}

func TestTool_Call_PreservesUnrelatedZipParts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.docx")

	tl := NewTool(dir)
	if _, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "Body text.",
		},
	}, nil); err != nil {
		t.Fatalf("create Call: %v", err)
	}
	if _, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"old_string": "Body text.",
			"new_string": "Edited body text.",
		},
	}, nil); err != nil {
		t.Fatalf("edit Call: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open written docx as zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"[Content_Types].xml", "_rels/.rels", "word/styles.xml", "word/document.xml"} {
		if !names[want] {
			t.Errorf("expected %s to survive the edit's repack, zip contains: %v", want, names)
		}
	}
}

func TestTool_ValidateInput(t *testing.T) {
	tl := NewTool("/tmp")
	ctx := context.Background()
	if _, err := tl.ValidateInput(ctx, map[string]any{"new_string": "x"}); err == nil {
		t.Error("expected error for missing file_path")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "a.docx"}); err == nil {
		t.Error("expected error for missing new_string")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "a.docx", "new_string": ""}); err != nil {
		t.Errorf("expected empty (but present) new_string to pass ValidateInput: %v", err)
	}
}

func TestTool_Call_RejectsNonDocxExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"new_string": "x",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for a non-.docx extension")
	}
}

// Sanity check that fileReadTool.RecordExternalRead really is what gates
// the read-before-write check here (same shared mechanism excel_edit uses).
func TestTool_Call_ExplicitRecordExternalReadSatisfiesGate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.docx")
	docBytes, err := createMinimalDocx("hello there")
	if err != nil {
		t.Fatalf("createMinimalDocx: %v", err)
	}
	if err := os.WriteFile(path, docBytes, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	fileReadTool.RecordExternalRead(path, info.ModTime(), "", true)

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path":  path,
			"old_string": "hello there",
			"new_string": "goodbye now",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected the edit to succeed after RecordExternalRead, got error: %v", result.Error)
	}
}
