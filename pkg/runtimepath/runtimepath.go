package runtimepath

import (
	"os"
	"path/filepath"
	"strings"
)

const EnvRuntimeRoot = "NEXUS_RUNTIME_ROOT"

// ExpandTilde replaces a leading "~" with the current user's home directory.
// Go's filepath package does not do this automatically.
func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if home = os.Getenv("HOME"); home == "" {
			home = os.Getenv("USERPROFILE")
		}
	}
	if home == "" {
		return path
	}
	return filepath.Join(home, path[1:])
}

func ResolveRoot(explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return filepath.Clean(ExpandTilde(trimmed))
	}

	if fromEnv := strings.TrimSpace(os.Getenv(EnvRuntimeRoot)); fromEnv != "" {
		return filepath.Clean(ExpandTilde(fromEnv))
	}

	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config", "nexus")
	}

	if home = strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".config", "nexus")
	}

	return filepath.Join(os.TempDir(), "nexus")
}

func Join(root string, parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	all = append(all, ResolveRoot(root))
	all = append(all, parts...)
	return filepath.Join(all...)
}

func DataDir(root string) string { return Join(root, "data") }

func SkillsDir(root string) string { return Join(root, "skills") }

func CacheDir(root string) string { return Join(root, "cache") }

func LogsDir(root string) string { return Join(root, "logs") }

func StorageDir(root string) string { return Join(root, "storage") }

func TmpDir(root string) string { return Join(root, "tmp") }

func BackendDBPath(root string) string { return Join(root, "data", "nexus.db") }

func HNSWDataDir(root string) string { return Join(root, "data", "hnsw") }

func SessionStoreDir(root string) string { return Join(root, "data", "sessions") }

func PlansDir(root string) string { return Join(root, "plans") }

func TasksDir(root string) string { return Join(root, "tmp", "tasks") }

func BashTasksDir(root string) string { return Join(root, "tmp", "bash-tasks") }

func ElectronUserDataDir(root string) string { return Join(root, "electron", "user-data") }

func ElectronSessionDataDir(root string) string { return Join(root, "electron", "session-data") }

func ElectronLogsDir(root string) string { return Join(root, "electron", "logs") }

func ElectronCrashDumpsDir(root string) string { return Join(root, "electron", "crash-dumps") }

// ─── Session-scoped directories ───────────────────────────────────────────────
//
// All per-session physical data lives under sessions/{session_id}/. Deleting a
// session requires only os.RemoveAll(SessionDir(root, id)) for filesystem data
// and store.DeleteSession(id) for the database — nothing else.
//
// Layout:
//   sessions/{id}/
//   ├── screenshots/        ← browser screenshots
//   ├── plans/              ← plan-mode markdown files
//   ├── tools/              ← browser downloads
//   └── artifacts/
//       ├── web/            ← web-scraped content
//       ├── images/         ← AI-generated images (DALL-E, Stable Diffusion, …)
//       └── audio/          ← TTS/STT audio files

func SessionsDir(root string) string { return Join(root, "sessions") }

func SessionDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID)
}

// SessionScreenshotsDir holds browser screenshots.
func SessionScreenshotsDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "screenshots")
}

// SessionPlansDir holds plan-mode markdown files for the session.
func SessionPlansDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "plans")
}

// SessionToolsDir holds browser downloads and tool-produced output files.
func SessionToolsDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "tools")
}

// SessionLogPath is the per-session log file for errors and diagnostics.
func SessionLogPath(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "session.log")
}

// SessionArtifactsDir is the parent for all agent-produced artifacts.
func SessionArtifactsDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "artifacts")
}

// SessionArtifactsWebDir holds web-scraped/fetched content.
func SessionArtifactsWebDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "artifacts", "web")
}

// SessionArtifactsImagesDir holds AI-generated images (not browser screenshots).
func SessionArtifactsImagesDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "artifacts", "images")
}

// SessionArtifactsAudioDir holds TTS output and STT input audio files.
func SessionArtifactsAudioDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "artifacts", "audio")
}
