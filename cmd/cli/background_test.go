package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

func TestBackgroundSessionResolveByIDPrefixAndName(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	now := time.Now().UTC().Format(time.RFC3339)
	session := backgroundSession{
		ID:        "bg-abcdef123456",
		Name:      "audit",
		PID:       999999,
		Cwd:       t.TempDir(),
		Status:    backgroundStatusFailed,
		StartedAt: now,
		UpdatedAt: now,
		StdoutLog: filepath.Join(t.TempDir(), "out.log"),
		StderrLog: filepath.Join(t.TempDir(), "err.log"),
	}
	if err := saveBackgroundSession(session); err != nil {
		t.Fatalf("save background session: %v", err)
	}

	byPrefix, err := resolveBackgroundSession("bg-abc")
	if err != nil {
		t.Fatalf("resolve by prefix: %v", err)
	}
	if byPrefix.ID != session.ID {
		t.Fatalf("expected %s, got %s", session.ID, byPrefix.ID)
	}
	byName, err := resolveBackgroundSession("audit")
	if err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if byName.ID != session.ID {
		t.Fatalf("expected %s, got %s", session.ID, byName.ID)
	}
}

func TestBackgroundNameAvailableOnlyBlocksLiveSessions(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	now := time.Now().UTC().Format(time.RFC3339)
	if err := saveBackgroundSession(backgroundSession{
		ID:        "bg-live",
		Name:      "shared",
		PID:       os.Getpid(),
		Status:    backgroundStatusRunning,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save live session: %v", err)
	}
	if err := ensureBackgroundNameAvailable("shared"); err == nil {
		t.Fatal("expected live name to be unavailable")
	}
	if _, err := markBackgroundSessionStatus("bg-live", backgroundStatusKilled); err != nil {
		t.Fatalf("mark killed: %v", err)
	}
	if err := ensureBackgroundNameAvailable("shared"); err != nil {
		t.Fatalf("expected terminal name to be reusable: %v", err)
	}
}

func TestRunBackgroundPSPrintsTrackedSessions(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	now := time.Now().UTC().Format(time.RFC3339)
	if err := saveBackgroundSession(backgroundSession{
		ID:        "bg-print",
		Name:      "printable",
		PID:       999999,
		Status:    backgroundStatusFailed,
		Provider:  "openai",
		Model:     "gpt-test",
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	var out bytes.Buffer
	if err := runBackgroundPS(nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("ps: %v", err)
	}
	got := out.String()
	for _, want := range []string{"bg-print", "printable", "failed", "openai:gpt-test"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected ps output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestClaimBackgroundNameRejectsConcurrentClaimant(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	if err := claimBackgroundName("race", "bg-first"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := claimBackgroundName("race", "bg-second"); err == nil {
		t.Fatal("expected second concurrent claim to fail")
	}
}

func TestClaimBackgroundNameReclaimsStaleClaim(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	now := time.Now().UTC().Format(time.RFC3339)
	if err := saveBackgroundSession(backgroundSession{
		ID:        "bg-dead",
		Name:      "reusable",
		PID:       999999,
		Status:    backgroundStatusKilled,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save terminal session: %v", err)
	}
	if err := claimBackgroundName("reusable", "bg-dead"); err != nil {
		t.Fatalf("claim before any reservation exists: %v", err)
	}
	if err := claimBackgroundName("reusable", "bg-new"); err != nil {
		t.Fatalf("expected stale claim from a terminal session to be reclaimable: %v", err)
	}
}

func TestClaimBackgroundNameDoesNotStealInFlightClaim(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	// No session file has been saved for "bg-pending" yet, simulating the
	// window between claimBackgroundName and saveBackgroundSession in
	// runBackgroundSession. A second claimant must not be able to steal it.
	if err := claimBackgroundName("pending", "bg-pending"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := claimBackgroundName("pending", "bg-other"); err == nil {
		t.Fatal("expected in-flight claim to block a second claimant")
	}
}
