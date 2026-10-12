package searchsession

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bashTool "github.com/KPO-Tech/seshat-runtime/internal/tools/bash"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// requireRipgrep skips the test when rg isn't on PATH - the streaming
// search tests spawn a real rg subprocess (that's the point: proving the
// spawn -> output-file -> job_output chain actually works, not just that
// search_start's own response is formatted correctly), but not every CI
// environment has ripgrep installed.
func requireRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep (rg) not found on PATH, skipping")
	}
}

// newTestTaskManager wires a fresh, isolated BackgroundTaskManager as the
// package-level global search_start relies on (via bashTool.GlobalTaskManager()),
// the same way internal/tools/bash's own tests do.
func newTestTaskManager(t *testing.T) {
	t.Helper()
	// bashTool.NewTool with a config sets the package global as a side
	// effect (see bash.go's NewTool doc) - simplest way to get a real,
	// initialized manager without reaching into bash-package internals.
	_ = bashTool.NewTool(bashTool.DefaultToolConfig())
}

func defaultToolCtx() tool.ToolUseContext {
	return tool.NewToolUseContext("test-session", "test-turn", "test-use", types.PermissionModeOnRequest)
}

func TestSearchOfficeFiles_FindsMatchInXLSXAndDOCX(t *testing.T) {
	dir := t.TempDir()

	// Real fixtures from the officetext package's own testdata (already
	// validated there) rather than hand-rolled ones, so this test exercises
	// the actual native extractor end to end.
	fixtureDir := filepath.Join("..", "..", "..", "..", "internal", "officetext", "testdata")
	copyFixture(t, filepath.Join(fixtureDir, "sample.docx"), filepath.Join(dir, "report.docx"))
	copyFixture(t, filepath.Join(fixtureDir, "sample.xlsx"), filepath.Join(dir, "data.xlsx"))

	matches, err := searchOfficeFiles(dir, "Alice", "", false)
	if err != nil {
		t.Fatalf("searchOfficeFiles: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected matches in both the docx and xlsx fixture, got %d: %v", len(matches), matches)
	}
	foundDocx, foundXlsx := false, false
	for _, m := range matches {
		if strings.Contains(m, "report.docx") {
			foundDocx = true
		}
		if strings.Contains(m, "data.xlsx") {
			foundXlsx = true
		}
	}
	if !foundDocx || !foundXlsx {
		t.Errorf("expected matches from both report.docx and data.xlsx, got: %v", matches)
	}
}

func TestSearchOfficeFiles_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	fixtureDir := filepath.Join("..", "..", "..", "..", "internal", "officetext", "testdata")
	copyFixture(t, filepath.Join(fixtureDir, "sample.docx"), filepath.Join(dir, "report.docx"))

	if matches, _ := searchOfficeFiles(dir, "alice", "", false); len(matches) != 0 {
		t.Errorf("expected no case-sensitive match for lowercase 'alice', got %v", matches)
	}
	matches, err := searchOfficeFiles(dir, "alice", "", true)
	if err != nil {
		t.Fatalf("searchOfficeFiles: %v", err)
	}
	if len(matches) == 0 {
		t.Error("expected a case-insensitive match for 'alice'")
	}
}

func TestSearchOfficeFiles_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	fixtureDir := filepath.Join("..", "..", "..", "..", "internal", "officetext", "testdata")
	copyFixture(t, filepath.Join(fixtureDir, "sample.docx"), filepath.Join(dir, "report.docx"))
	copyFixture(t, filepath.Join(fixtureDir, "sample.xlsx"), filepath.Join(dir, "data.xlsx"))

	matches, err := searchOfficeFiles(dir, "Alice", "*.xlsx", false)
	if err != nil {
		t.Fatalf("searchOfficeFiles: %v", err)
	}
	for _, m := range matches {
		if strings.Contains(m, "report.docx") {
			t.Errorf("glob '*.xlsx' should have excluded report.docx, got match: %s", m)
		}
	}
	if len(matches) == 0 {
		t.Error("expected the xlsx fixture to still match")
	}
}

func TestTool_Call_StartsBackgroundSearchAndOfficeMatchesReturnImmediately(t *testing.T) {
	requireRipgrep(t)
	newTestTaskManager(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\n\nfunc Alice() {}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fixtureDir := filepath.Join("..", "..", "..", "..", "internal", "officetext", "testdata")
	copyFixture(t, filepath.Join(fixtureDir, "sample.xlsx"), filepath.Join(dir, "data.xlsx"))

	tl := NewTool(dir)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"file_path": dir, // ignored; ensures unknown keys don't break anything
			"pattern":   "Alice",
			"path":      dir,
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if !strings.Contains(result.Content, "job_id:") && !strings.Contains(result.Content, "job_id") {
		t.Errorf("expected a job_id reference in the response, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "data.xlsx") {
		t.Errorf("expected the immediate Office-content match for data.xlsx, got:\n%s", result.Content)
	}

	jobID := extractJobID(t, result.Content)

	// Poll job_output until ripgrep (a real subprocess) has actually
	// finished and produced the code.go match - this is what proves the
	// background job is real and job_output correctly reads its output,
	// not just that search_start returned some text.
	joTool := bashTool.NewJobOutputTool()
	deadline := time.Now().Add(10 * time.Second)
	var out string
	for time.Now().Before(deadline) {
		r, callErr := joTool.Call(context.Background(), tool.CallInput{
			Parsed: map[string]any{"job_id": jobID},
		}, nil)
		if callErr != nil {
			t.Fatalf("job_output Call: %v", callErr)
		}
		out = r.Content
		if strings.Contains(out, "code.go") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(out, "code.go") {
		t.Fatalf("expected the background ripgrep job to eventually report code.go, got:\n%s", out)
	}
}

func copyFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write fixture copy %s: %v", dst, err)
	}
}

func extractJobID(t *testing.T, content string) string {
	t.Helper()
	idx := strings.Index(content, "job_id: ")
	if idx == -1 {
		t.Fatalf("no job_id found in:\n%s", content)
	}
	rest := content[idx+len("job_id: "):]
	end := strings.IndexAny(rest, ")\n")
	if end == -1 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func TestTool_ValidateInput(t *testing.T) {
	tl := NewTool("/tmp")
	ctx := context.Background()
	if _, err := tl.ValidateInput(ctx, map[string]any{}); err == nil {
		t.Error("expected error for missing pattern")
	}
	if _, err := tl.ValidateInput(ctx, map[string]any{"pattern": "x"}); err != nil {
		t.Errorf("expected valid input to pass: %v", err)
	}
}

func TestTool_CheckPermissions_UNCBlocked(t *testing.T) {
	// "//" (forward slashes) rather than "\\" (backslashes): IsUNCPath checks
	// for either prefix, but a backslash-prefixed path is only ever treated
	// as absolute by filepath.IsAbs on Windows - on Linux (where CI runs)
	// resolvePath would instead join it onto the working directory as a
	// relative path, mangling it into something that no longer starts with
	// "\\" or "//" by the time IsUNCPath sees it. "//" is recognized as
	// absolute (and thus passed through unresolved) on every platform.
	tl := NewTool("/tmp")
	got := tl.CheckPermissions(context.Background(), map[string]any{
		"pattern": "x",
		"path":    "//evil/share",
	}, defaultToolCtx())
	if got.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny for a UNC path, got %v", got.Behavior)
	}
}
