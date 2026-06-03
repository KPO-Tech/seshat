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

func SessionStoreDir(root string) string { return Join(root, "data", "sessions") }

func PlansDir(root string) string { return Join(root, "plans") }

func TasksDir(root string) string { return Join(root, "tmp", "tasks") }

func BashTasksDir(root string) string { return Join(root, "tmp", "bash-tasks") }

func ElectronUserDataDir(root string) string { return Join(root, "electron", "user-data") }

func ElectronSessionDataDir(root string) string { return Join(root, "electron", "session-data") }

func ElectronLogsDir(root string) string { return Join(root, "electron", "logs") }

func ElectronCrashDumpsDir(root string) string { return Join(root, "electron", "crash-dumps") }
