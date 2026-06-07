// Package appdir is the single source of truth for the CLI's filesystem layout.
//
// All persistent data lives under Root() (~/.config/nexus-cli/ by default, or
// the value of NEXUS_RUNTIME_ROOT). Session-scoped data is further isolated
// under sessions/{session_id}/ so that deleting a session is a single
// os.RemoveAll call.
//
// Internal packages should NOT import this package — they accept explicit paths
// instead. Only cmd/cli wires the paths together.
package appdir

import (
	"os"
	"path/filepath"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// Root returns the application root directory, resolved via NEXUS_RUNTIME_ROOT
// or the platform default (~/.config/nexus-cli/ on Linux/macOS).
func Root() string { return runtimepath.ResolveRoot("") }

// ─── Global directories ───────────────────────────────────────────────────────

func LogsDir() string     { return runtimepath.LogsDir("") }
func SessionsDir() string { return runtimepath.SessionsDir("") }

// ─── Per-session directories ──────────────────────────────────────────────────

func SessionDir(sessionID string) string      { return runtimepath.SessionDir("", sessionID) }
func SessionImagesDir(sessionID string) string { return runtimepath.SessionImagesDir("", sessionID) }
func SessionPlansDir(sessionID string) string  { return runtimepath.SessionPlansDir("", sessionID) }
func SessionToolsDir(sessionID string) string  { return runtimepath.SessionToolsDir("", sessionID) }
func SessionLogPath(sessionID string) string   { return runtimepath.SessionLogPath("", sessionID) }

// ─── Lifecycle helpers ────────────────────────────────────────────────────────

// EnsureAppDirs creates the top-level directories required at startup.
// Safe to call multiple times (uses os.MkdirAll).
func EnsureAppDirs() error {
	dirs := []string{
		LogsDir(),
		SessionsDir(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSessionDir creates sessions/{id}/ and its standard subdirectories
// (images, plans, tools). Call this when a session starts, before any tools run.
// Safe to call multiple times.
func EnsureSessionDir(sessionID string) error {
	dirs := []string{
		SessionImagesDir(sessionID),
		SessionPlansDir(sessionID),
		SessionToolsDir(sessionID),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSessionDir removes sessions/{id}/ and all its contents in one call.
// This covers images, plans, tools, and any other files created in the session.
// Errors are intentionally ignored — the session was already removed from the
// database; filesystem cleanup is best-effort.
func DeleteSessionDir(sessionID string) {
	_ = os.RemoveAll(filepath.Join(SessionsDir(), sessionID))
}
