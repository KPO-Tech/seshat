// Package appdir is the single source of truth for the CLI's filesystem layout.
//
// All persistent data lives under Root() (~/.config/nexus-tui/ by default, or
// the value of NEXUS_RUNTIME_ROOT). Session-scoped data is isolated under
// sessions/{session_id}/ so deleting a session is a single os.RemoveAll call.
//
// Directory layout:
//
//	~/.config/nexus-tui/
//	├── logs/
//	├── documents/          ← user-uploaded PDFs and docs (global, persistent)
//	├── rag/                ← RAG-indexed documents (global, persistent)
//	└── sessions/
//	    └── {session_id}/
//	        ├── artifacts/
//	        │   ├── screenshots/    ← browser screenshots
//	        │   ├── images/         ← AI-generated images
//	        │   ├── web/            ← web-scraped content
//	        │   └── audio/          ← TTS / STT audio
//	        ├── pastes/
//	        │   ├── text/
//	        │   ├── images/
//	        │   └── other/
//	        ├── plans/              ← plan-mode markdown files
//	        └── tools/              ← browser downloads
package appdir

import (
	"os"
	"path/filepath"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// Root returns the application root directory, resolved via NEXUS_RUNTIME_ROOT
// or the platform default (~/.config/nexus-tui/ on Linux/macOS).
func Root() string { return runtimepath.ResolveRoot("") }

// ─── Global directories ───────────────────────────────────────────────────────

func LogsDir() string     { return runtimepath.LogsDir("") }
func SessionsDir() string { return runtimepath.SessionsDir("") }

// ─── Per-session directories ──────────────────────────────────────────────────

func SessionDir(sessionID string) string { return runtimepath.SessionDir("", sessionID) }
func SessionScreenshotsDir(sessionID string) string {
	return runtimepath.SessionScreenshotsDir("", sessionID)
}
func SessionPlansDir(sessionID string) string  { return runtimepath.SessionPlansDir("", sessionID) }
func SessionPastesDir(sessionID string) string { return runtimepath.SessionPastesDir("", sessionID) }
func SessionToolsDir(sessionID string) string  { return runtimepath.SessionToolsDir("", sessionID) }
func SessionLogPath(sessionID string) string   { return runtimepath.SessionLogPath("", sessionID) }

// Artifact subdirectories — agent-produced content, session-scoped.
func SessionArtifactsDir(sessionID string) string {
	return runtimepath.SessionArtifactsDir("", sessionID)
}
func SessionArtifactsWebDir(sessionID string) string {
	return runtimepath.SessionArtifactsWebDir("", sessionID)
}
func SessionArtifactsImagesDir(sessionID string) string {
	return runtimepath.SessionArtifactsImagesDir("", sessionID)
}
func SessionPastesTextDir(sessionID string) string {
	return runtimepath.SessionPastesTextDir("", sessionID)
}
func SessionPastesImagesDir(sessionID string) string {
	return runtimepath.SessionPastesImagesDir("", sessionID)
}
func SessionPastesOtherDir(sessionID string) string {
	return runtimepath.SessionPastesOtherDir("", sessionID)
}
func SessionArtifactsAudioDir(sessionID string) string {
	return runtimepath.SessionArtifactsAudioDir("", sessionID)
}

// ─── Lifecycle helpers ────────────────────────────────────────────────────────

// GlobalConfigPath returns the path to the global TUI config file (nexus.json).
func GlobalConfigPath() string { return filepath.Join(Root(), "nexus.json") }

// EnsureAppDirs creates the top-level directories required at startup and
// seeds nexus.json with an empty object if the file does not yet exist.
// Safe to call multiple times (os.MkdirAll and the existence check are both idempotent).
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
	cfgPath := GlobalConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSessionDir creates sessions/{id}/ and all standard subdirectories.
// Call this when a session starts, before any tools run. Safe to call multiple times.
func EnsureSessionDir(sessionID string) error {
	dirs := []string{
		SessionScreenshotsDir(sessionID),
		SessionArtifactsImagesDir(sessionID),
		SessionArtifactsWebDir(sessionID),
		SessionArtifactsAudioDir(sessionID),
		SessionPastesTextDir(sessionID),
		SessionPastesImagesDir(sessionID),
		SessionPastesOtherDir(sessionID),
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
// Covers screenshots, plans, tools, artifacts, logs — everything.
// Errors are intentionally ignored; DB cleanup is the authoritative deletion.
func DeleteSessionDir(sessionID string) {
	_ = os.RemoveAll(filepath.Join(SessionsDir(), sessionID))
}
