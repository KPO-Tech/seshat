package runtimepath

import (
	"os"
	"path/filepath"
	"strings"
)

const EnvRuntimeRoot = "SESHAT_RUNTIME_ROOT"

// "electron/" under the runtime root is reserved: Electron-based consumers
// (e.g. seshat-ai's desktop app) put their userData/sessionData/logs/
// crashDumps there by convention. No Go code in this SDK reads or writes
// under it, so there's no helper for it here - don't repurpose that name.

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
		return filepath.Join(home, ".config", "seshat")
	}

	if home = strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".config", "seshat")
	}

	return filepath.Join(os.TempDir(), "seshat")
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

func BackendDBPath(root string) string { return Join(root, "seshat.db") }

func HNSWDataDir(root string) string { return Join(root, "data", "hnsw") }

// DeepDocModelsDir holds the ONNX models (det.ort, rec.ort, layout.ort,
// tsr.ort, ocr.res) that back pkg/nativedoc's native OCR/layout/table
// pipeline - fetched by scripts/install-deepdoc-models.sh from
// https://huggingface.co/InfiniFlow/deepdoc (Apache-2.0), mirroring
// docling-serve's own .venv provisioning convention.
func DeepDocModelsDir(root string) string { return Join(root, "models", "deepdoc") }

// RAGSQLiteDBPath is the fallback vector store used when the embedded HNSW
// backend isn't available on the current platform (Windows - see
// internal/vector/hnsw_store_windows.go).
func RAGSQLiteDBPath(root string) string { return Join(root, "data", "rag.sqlite3") }

func SessionStoreDir(root string) string { return Join(root, "data", "sessions") }

func PlansDir(root string) string { return Join(root, "plans") }

func CompanionPath(root string) string { return Join(root, "companion.json") }

func TasksDir(root string) string { return Join(root, "tmp", "tasks") }

func BashTasksDir(root string) string { return Join(root, "tmp", "bash-tasks") }

// ─── Session-scoped directories ───────────────────────────────────────────────
//
// All per-session physical data lives under sessions/{session_id}/. Deleting a
// session requires only os.RemoveAll(SessionDir(root, id)) for filesystem data
// and store.DeleteSession(id) for the database — nothing else.
//
// Layout:
//   sessions/{id}/
//   ├── artifacts/
//   │   ├── screenshots/    ← browser screenshots
//   │   └── images/         ← AI-generated images (DALL-E, Stable Diffusion, …)
//   ├── pastes/
//   │   ├── text/           ← persisted pasted text attachments
//   │   ├── images/         ← persisted pasted image attachments
//   │   └── other/          ← persisted pasted binary attachments
//   ├── plans/              ← plan-mode markdown files
//   ├── tools/              ← browser downloads and tool outputs
//   └── session.log

func SessionsDir(root string) string { return Join(root, "sessions") }

func SessionDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID)
}

// SessionScreenshotsDir holds browser screenshots under artifacts/.
func SessionScreenshotsDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "artifacts", "screenshots")
}

// SessionPastesDir holds persisted pasted attachments for the session.
func SessionPastesDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), "sessions", sessionID, "pastes")
}

func SessionPastesTextDir(root, sessionID string) string {
	return filepath.Join(SessionPastesDir(root, sessionID), "text")
}

func SessionPastesImagesDir(root, sessionID string) string {
	return filepath.Join(SessionPastesDir(root, sessionID), "images")
}

func SessionPastesOtherDir(root, sessionID string) string {
	return filepath.Join(SessionPastesDir(root, sessionID), "other")
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
