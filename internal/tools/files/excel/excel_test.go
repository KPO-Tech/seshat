package excel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	fileReadTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/read"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
	officetext "github.com/KPO-Tech/seshat-runtime/pkg/officetext"
)

func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

func TestParseRangeStart(t *testing.T) {
	cases := []struct {
		in           string
		wantCol      int
		wantRow      int
		expectErrror bool
	}{
		{"", 1, 1, false},
		{"A1", 1, 1, false},
		{"B2", 2, 2, false},
		{"B2:D5", 2, 2, false},
		{"not-a-cell", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			col, row, err := parseRangeStart(tc.in)
			if tc.expectErrror {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if col != tc.wantCol || row != tc.wantRow {
				t.Errorf("parseRangeStart(%q) = (%d,%d), want (%d,%d)", tc.in, col, row, tc.wantCol, tc.wantRow)
			}
		})
	}
}

func TestTool_Call_CreatesNewXLSXWithHeaderAndFormula(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.xlsx")

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"sheet":     "Data",
			"values": []any{
				[]any{"Name", "Score"},
				[]any{"Alice", float64(90)},
				[]any{"Bob", float64(85)},
				[]any{"Total", "=SUM(B2:B3)"},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}

	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected file to exist at %s: %v", path, statErr)
	}

	// Round-trip: read it back with the native XLSX extractor (the read-side
	// counterpart) rather than re-deriving assertions from excelize
	// directly, so this test actually proves write+read agree with each
	// other, not just that excelize's own API was called correctly.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	markdown, ok, _, err := officetext.Extract("sample.xlsx", data)
	if err != nil || !ok {
		t.Fatalf("failed to read back the written xlsx: ok=%v err=%v", ok, err)
	}
	for _, want := range []string{"Data", "Name", "Score", "Alice", "90", "Bob", "85", "Total"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("expected %q in the round-tripped content, got:\n%s", want, markdown)
		}
	}
}

func TestTool_Call_RequiresReadFirstForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.xlsx")

	// Created directly with excelize, deliberately bypassing the tool's own
	// write path (which records read-state on success) - this is the only
	// way to get a file on disk with genuinely no read-state recorded for
	// it, since that cache is a process-wide singleton keyed by path, not
	// scoped to a *Tool instance.
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "preexisting"); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	f.Close()

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"values":    []any{[]any{"b"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error requiring a read before editing an existing file")
	}
}

func TestTool_Call_EditsExistingFileAfterRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.xlsx")

	tl := NewTool(dir)
	if _, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"values":    []any{[]any{"original"}},
		},
	}, nil); err != nil {
		t.Fatalf("initial Call: %v", err)
	}

	// Simulate having read the file (same tool instance already recorded
	// this via the post-write RecordExternalRead, matching write_file's
	// convention - a real agent session would have this from either the
	// initial write or an explicit FileRead call).
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	fileReadTool.RecordExternalRead(path, info.ModTime(), "", true)

	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"range":     "B1",
			"values":    []any{[]any{"added"}},
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
		t.Fatalf("read: %v", err)
	}
	markdown, ok, _, err := officetext.Extract("existing.xlsx", data)
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(markdown, "original") || !strings.Contains(markdown, "added") {
		t.Errorf("expected both the original and the newly-added cell to survive the edit, got:\n%s", markdown)
	}
}

func TestTool_Call_RejectsNonXLSXExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": path,
			"values":    []any{[]any{"a"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for a non-.xlsx extension")
	}
}

func TestTool_ValidateInput(t *testing.T) {
	tl := NewTool("/tmp")
	ctx := context.Background()

	if _, err := tl.ValidateInput(ctx, map[string]any{"values": []any{}}); err == nil {
		t.Error("expected error for missing file_path")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "a.xlsx"}); err == nil {
		t.Error("expected error for missing values")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"file_path": "a.xlsx", "values": []any{}}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
}
