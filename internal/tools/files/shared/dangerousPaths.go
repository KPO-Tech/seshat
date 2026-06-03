package shared

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// DangerousFiles are files that should never be auto-edited.
// Aligned with OpenClaude's DANGEROUS_FILES (filesystem.ts:516-525).
var DangerousFiles = []string{
	// Shell config
	".bashrc",
	".bash_profile",
	".zshrc",
	".zprofile",
	".profile",
	".bash_logout",
	// Git config
	".gitconfig",
	".gitmodules",
	// SSH — any modification could lock out the user or grant access
	"authorized_keys",
	"known_hosts",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	// Secret/credential files
	".env",
	".env.local",
	".env.production",
	".env.staging",
	".envrc",
	".netrc",
	// Package registry tokens
	".npmrc",
	".yarnrc",
	".pypirc",
	// Cloud credentials
	"credentials", // .aws/credentials, etc.
	"config",      // .aws/config (matched by dir check below)
	// AI tooling
	".mcp.json",
	".claude.json",
	".ripgreprc",
	// CI tokens
	".travis.yml",
}

// DangerousDirectories are directories that should never be auto-edited.
// Aligned with OpenClaude's DANGEROUS_DIRECTORIES (filesystem.ts:527-534).
var DangerousDirectories = []string{
	".git",
	".vscode",
	".idea",
	".claude",
	".openclaude",
	// SSH directory — all files inside are sensitive
	".ssh",
	// Cloud credential directories
	".aws",
	".gcp",
	".azure",
	// GPG keys
	".gnupg",
}

// IsDangerousFile checks if a file path points to a dangerous file.
func IsDangerousFile(path string) bool {
	baseName := strings.ToLower(filepath.Base(path))
	for _, dangerous := range DangerousFiles {
		if strings.EqualFold(baseName, dangerous) {
			return true
		}
	}
	return false
}

// IsDangerousDirectory checks if a path is in or points to a dangerous directory.
func IsDangerousDirectory(path string) bool {
	normalizedPath := strings.ToLower(filepath.Clean(path))
	pathParts := strings.Split(normalizedPath, string(filepath.Separator))

	for _, dangerousDir := range DangerousDirectories {
		dangerousParts := strings.Split(strings.ToLower(dangerousDir), string(filepath.Separator))
		if isPathInDirectory(pathParts, dangerousParts) {
			return true
		}
	}
	return false
}

// isPathInDirectory checks if pathParts contains the directory parts.
func isPathInDirectory(pathParts, dirParts []string) bool {
	if len(dirParts) == 0 {
		return false
	}

	for i := 0; i <= len(pathParts)-len(dirParts); i++ {
		matches := true
		for j := 0; j < len(dirParts); j++ {
			if !strings.EqualFold(pathParts[i+j], dirParts[j]) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// CheckDangerousRemovalPath checks if a removal path is dangerous.
// Aligned with OpenClaude's checkDangerousRemovalPaths (pathValidation.ts:331-367).
func CheckDangerousRemovalPath(path string, cwd string) error {
	// Expand tilde first
	expandedPath := expandTilde(path)
	cleanPath := filepath.Clean(expandedPath)
	if filepath.IsAbs(cleanPath) {
		return checkAbsoluteRemovalPath(cleanPath)
	}
	// For relative paths, resolve against cwd
	absPath := filepath.Join(cwd, cleanPath)
	return checkAbsoluteRemovalPath(absPath)
}

// checkAbsoluteRemovalPath checks if an absolute removal path is dangerous.
func checkAbsoluteRemovalPath(absPath string) error {
	// Check for wildcard patterns
	if strings.Contains(absPath, "*") || strings.HasSuffix(absPath, "/*") || strings.HasSuffix(absPath, "\\*") {
		return fmt.Errorf("dangerous removal path with wildcard: %s", absPath)
	}

	// Check for root directory
	if absPath == "/" || absPath == "\\" || absPath == "." {
		return fmt.Errorf("dangerous removal path: root directory")
	}

	// Check for home directory
	homeDir := filepath.Clean(homeDir())
	if absPath == homeDir || absPath == homeDir+string(filepath.Separator) {
		return fmt.Errorf("dangerous removal path: home directory")
	}

	// On Unix, check for direct children of root
	if runtime.GOOS != "windows" {
		if strings.HasPrefix(absPath, "/") {
			// Get the path after root
			afterRoot := strings.TrimPrefix(absPath, "/")
			parts := strings.Split(afterRoot, string(filepath.Separator))
			if len(parts) == 1 && parts[0] != "" {
				// Direct child of root (e.g., /usr, /tmp, /etc)
				return fmt.Errorf("dangerous removal path: direct child of root directory")
			}
		}
	}

	// On Windows, check for drive root
	if runtime.GOOS == "windows" {
		if len(absPath) == 3 && strings.HasSuffix(absPath, ":\\") {
			return fmt.Errorf("dangerous removal path: drive root")
		}
		if len(absPath) > 3 {
			drivePart := absPath[:3]
			if drivePart[1] == ':' && (drivePart[2] == '\\' || drivePart[2] == '/') {
				// Check if it's a direct child of drive root
				afterDrive := absPath[3:]
				parts := strings.Split(strings.Trim(afterDrive, "\\/"), string(filepath.Separator))
				if len(parts) == 1 {
					return fmt.Errorf("dangerous removal path: direct child of drive root")
				}
			}
		}
	}

	return nil
}

// homeDir returns the user's home directory.
func homeDir() string {
	if homeDirFunc != nil {
		if h := homeDirFunc(); h != "" {
			return h
		}
	}
	// Fallback to common home directories
	home := filepath.Join("/", "home") // Unix
	if runtime.GOOS == "windows" {
		home = filepath.Join("C:", "Users") // Windows
	}
	return home
}

// homeDirFunc is a function that returns the home directory.
// Can be overridden for testing.
var homeDirFunc func() string

// expandTilde expands ~ and ~/ to the user's home directory.
// Aligned with OpenClaude's expandTilde (pathValidation.ts:80-89).
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(path, "~\\")) {
		return homeDir() + path[1:]
	}
	return path
}

// IsInWorkingDirectory checks if a path is within the working directory
// after resolving all symlinks to prevent escape attacks.
func IsInWorkingDirectory(path, workingDir string) bool {
	realPath, err := GetAbsolutePath(path)
	if err != nil {
		return false
	}
	realWD, err := GetAbsolutePath(workingDir)
	if err != nil {
		return false
	}
	sep := string(filepath.Separator)
	if realWD != "/" {
		realWD = realWD + sep
	}
	return strings.HasPrefix(realPath+sep, realWD)
}

// HasSuspiciousPattern checks if a path has suspicious patterns that could bypass safety checks.
// Aligned with OpenClaude's Windows path patterns (pathValidation.ts:546-611).
func HasSuspiciousPattern(path string) bool {
	baseName := filepath.Base(path)
	lowerPath := strings.ToLower(path)
	lowerBase := strings.ToLower(baseName)

	// Check for NTFS Alternate Data Streams (Windows)
	if runtime.GOOS == "windows" {
		if strings.Contains(lowerPath, ":$data") ||
			strings.Contains(lowerPath, ":$") ||
			strings.Contains(lowerBase, ":") {
			return true
		}
	}

	// Check for 8.3 short names (Windows) - patterns like GIT~1, CLAUDE~1
	if runtime.GOOS == "windows" {
		if matchesPattern(lowerBase, `~\d+$`) {
			return true
		}
	}

	// Check for long path prefixes (Windows)
	if strings.HasPrefix(lowerPath, `\\?\`) ||
		strings.HasPrefix(lowerPath, `//?/`) ||
		strings.HasPrefix(lowerPath, `\\.\`) {
		return true
	}

	// Check for trailing dots and spaces (Windows case-insensitive)
	if strings.HasSuffix(lowerBase, ".") ||
		strings.HasSuffix(lowerBase, " ") {
		return true
	}

	// Check for DOS device names (Windows)
	if runtime.GOOS == "windows" {
		dosDevices := []string{".con", ".prn", ".aux", ".nul",
			".com1", ".com2", ".com3", ".com4", ".com5", ".com6", ".com7", ".com8", ".com9",
			".lpt1", ".lpt2", ".lpt3", ".lpt4", ".lpt5", ".lpt6", ".lpt7", ".lpt8", ".lpt9"}
		for _, device := range dosDevices {
			if strings.HasSuffix(lowerBase, device) {
				return true
			}
		}
	}

	// Check for three or more consecutive dots in path components
	if strings.Contains(path, "...") {
		parts := strings.Split(lowerPath, string(filepath.Separator))
		for _, part := range parts {
			if matchesPattern(part, `\.{3,}`) {
				return true
			}
		}
	}

	// Check for UNC paths (\\server\share or //server/share)
	if strings.HasPrefix(lowerPath, `\\`) || strings.HasPrefix(lowerPath, `//`) {
		// Split into parts and check if it's a valid UNC path
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(lowerPath, `\\`), `//`), string(filepath.Separator))
		if len(parts) >= 2 {
			// This looks like a UNC path: \\server\share\path
			return true
		}
	}

	return false
}

// matchesPattern checks if a string matches a simple pattern.
func matchesPattern(s, pattern string) bool {
	// Simple pattern matching - just check if pattern exists in string
	// For a full implementation, use regex
	return strings.Contains(s, pattern)
}
