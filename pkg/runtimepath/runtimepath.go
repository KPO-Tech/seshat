package runtimepath

import (
	"fmt"
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

// TitleModelsDir stores tiny local models dedicated to cheap background tasks
// such as automatic session-title generation.
func TitleModelsDir(root string) string { return Join(root, "models", "title") }

// RAGSQLiteDBPath is the fallback vector store used when the embedded HNSW
// backend can't be opened, and the store of the installs that ran when HNSW
// did not build on Windows (before 1.2.73).
func RAGSQLiteDBPath(root string) string { return Join(root, "data", "rag.sqlite3") }

func SessionStoreDir(root string) string { return Join(root, "data", "sessions") }

func PlansDir(root string) string { return Join(root, "plans") }

func CompanionPath(root string) string { return Join(root, "companion.json") }

func TasksDir(root string) string { return Join(root, "tmp", "tasks") }

func BashTasksDir(root string) string { return Join(root, "tmp", "bash-tasks") }

// ─── Session-scoped directories ───────────────────────────────────────────────
//
// A session has one directory, workspaces/{session_id}/, and everything that belongs to the session lives
// in it: the files the user attached, the files the agent writes, its plans, screenshots, pasted
// attachments and tool output. It is also the directory hosts such as seshat-backend use as the
// session's workspace, so there is no second per-session directory to keep in step with it. Deleting a
// session is RemoveSessionData(root, id) for the filesystem and store.DeleteSession(id) for the database.
//
// Layout:
//   workspaces/{id}/
//   ├── uploads/            ← files the user attached (seshat-backend)
//   ├── artifacts/
//   │   ├── screenshots/    ← browser screenshots
//   │   ├── images/         ← AI-generated images (DALL-E, Stable Diffusion, …)
//   │   ├── audio/          ← TTS output and STT input
//   │   └── web/            ← web-scraped/fetched content
//   ├── pastes/
//   │   ├── text/           ← persisted pasted text attachments
//   │   ├── images/         ← persisted pasted image attachments
//   │   └── other/          ← persisted pasted binary attachments
//   ├── plans/              ← plan-mode markdown files
//   ├── tools/              ← browser downloads and tool outputs
//   └── session.log
//
// What the agent must not be able to edit stays out of this directory, because the agent can write inside
// it. The permissions a session has been granted are in data/permissions/{id}.json (SessionPermissionsPath).
//
// Before v1.2.59 these were under sessions/{id}/; MigrateLegacySessionDirs moves them.

const (
	workspacesDirName = "workspaces"
	legacyDirName     = "sessions"
)

// WorkspacesDir holds the per-session directories.
func WorkspacesDir(root string) string { return Join(root, workspacesDirName) }

// SessionsDir is the directory that holds the per-session directories. The name is kept; the directory
// is WorkspacesDir.
func SessionsDir(root string) string { return WorkspacesDir(root) }

// LegacySessionsDir is where per-session data lived before v1.2.59.
func LegacySessionsDir(root string) string { return Join(root, legacyDirName) }

func SessionDir(root, sessionID string) string {
	return filepath.Join(ResolveRoot(root), workspacesDirName, sessionID)
}

// SessionPermissionsPath is the file of the permissions granted to a session ("always allow this tool").
// It is outside the session directory on purpose: the agent can write there, and a grant it could edit
// would be a grant it could give itself.
func SessionPermissionsPath(root, sessionID string) string {
	return Join(root, "data", "permissions", sessionID+".json")
}

// SessionScreenshotsDir holds browser screenshots under artifacts/.
func SessionScreenshotsDir(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "artifacts", "screenshots")
}

// SessionPastesDir holds persisted pasted attachments for the session.
func SessionPastesDir(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "pastes")
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
	return filepath.Join(SessionDir(root, sessionID), "plans")
}

// SessionToolsDir holds browser downloads and tool-produced output files.
func SessionToolsDir(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "tools")
}

// SessionLogPath is the per-session log file for errors and diagnostics.
func SessionLogPath(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "session.log")
}

// SessionArtifactsDir is the parent for all agent-produced artifacts.
func SessionArtifactsDir(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "artifacts")
}

// SessionArtifactsWebDir holds web-scraped/fetched content.
func SessionArtifactsWebDir(root, sessionID string) string {
	return filepath.Join(SessionArtifactsDir(root, sessionID), "web")
}

// SessionArtifactsImagesDir holds AI-generated images (not browser screenshots).
func SessionArtifactsImagesDir(root, sessionID string) string {
	return filepath.Join(SessionArtifactsDir(root, sessionID), "images")
}

// SessionArtifactsAudioDir holds TTS output and STT input audio files.
func SessionArtifactsAudioDir(root, sessionID string) string {
	return filepath.Join(SessionArtifactsDir(root, sessionID), "audio")
}

// validSessionID reports whether id can be used as one path element: a session id is a name, never a path.
func validSessionID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`) && filepath.Base(id) == id
}

// RemoveSessionData deletes the directory of a session and its permissions file. It is what deleting a
// session leaves to the filesystem. An id that is not a plain name is refused rather than followed.
func RemoveSessionData(root, sessionID string) error {
	if !validSessionID(sessionID) {
		return fmt.Errorf("runtimepath: %q is not a session id", sessionID)
	}
	err := os.RemoveAll(SessionDir(root, sessionID))
	if removeErr := os.Remove(SessionPermissionsPath(root, sessionID)); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
		err = removeErr
	}
	return err
}

// MigrateLegacySessionDirs moves what older versions wrote under sessions/{id}/ to workspaces/{id}/, and a
// permissions.json to data/permissions/{id}.json. It never overwrites: a file that already exists at the
// destination stays, and what could not be moved stays where it was. It is safe to run on every start.
// It returns how many session directories it moved something out of.
func MigrateLegacySessionDirs(root string) (int, error) {
	legacy := LegacySessionsDir(root)
	entries, err := os.ReadDir(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	moved := 0
	var firstErr error
	for _, entry := range entries {
		if !entry.IsDir() || !validSessionID(entry.Name()) {
			continue
		}
		id := entry.Name()
		src := filepath.Join(legacy, id)

		if grants := filepath.Join(src, "permissions.json"); fileExists(grants) {
			dst := SessionPermissionsPath(root, id)
			if !fileExists(dst) {
				if err := os.MkdirAll(filepath.Dir(dst), 0o700); err == nil {
					if err := os.Rename(grants, dst); err != nil && firstErr == nil {
						firstErr = err
					}
				}
			}
		}

		dst := SessionDir(root, id)
		if err := moveInto(src, dst); err != nil && firstErr == nil {
			firstErr = err
		}
		moved++
		_ = os.Remove(src) // only if it is empty now
	}
	_ = os.Remove(legacy) // likewise
	return moved, firstErr
}

// moveInto moves src to dst, or merges it when dst exists, without overwriting anything.
func moveInto(src, dst string) error {
	if _, err := os.Lstat(dst); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		return os.Rename(src, dst)
	}
	children, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	var firstErr error
	for _, child := range children {
		from, to := filepath.Join(src, child.Name()), filepath.Join(dst, child.Name())
		if _, err := os.Lstat(to); err == nil {
			if child.IsDir() {
				if err := moveInto(from, to); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			continue // a file that is already there stays
		}
		if err := os.Rename(from, to); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = os.Remove(src)
	return firstErr
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
