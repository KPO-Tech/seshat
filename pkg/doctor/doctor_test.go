package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/config"
)

func TestRunReportsCoreChecks(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	dbPath := filepath.Join(root, "sessions.sqlite3")

	report := Run(context.Background(), Options{
		Version: "test",
		Config: config.Config{
			RuntimeRoot:   root,
			Cwd:           workspace,
			SessionDBPath: dbPath,
			Model:         "ollama:qwen2.5-coder:7b",
		},
	})

	if report.Version != "test" {
		t.Fatalf("expected version to be preserved, got %q", report.Version)
	}
	assertCheck(t, report, "runtime root", StatusOK)
	assertCheck(t, report, "working directory", StatusOK)
	assertCheck(t, report, "session database", StatusOK)
	assertCheck(t, report, "model", StatusOK)
	assertCheck(t, report, "api key", StatusSkipped)
	if report.HasFailures() {
		t.Fatalf("expected no failing checks, got %#v", report.Checks)
	}
}

func assertCheck(t *testing.T, report Report, name string, want Status) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			if check.Status != want {
				t.Fatalf("expected %s to be %s, got %s (%s)", name, want, check.Status, check.Detail)
			}
			return
		}
	}
	t.Fatalf("missing check %q in %#v", name, report.Checks)
}
