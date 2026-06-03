package glob

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// suggestPathUnderCwd suggests a corrected path if the requested path exists
// under the parent of the current working directory but not under cwd itself.
// This helps users who accidentally type paths like "../project/file.go" when
// they mean "file.go" (assuming the file exists in both locations).
//
// IMPORTANT: This function only suggests if the requested path DOES NOT exist.
// If the requested path exists, no suggestion is made even if a similar file exists under cwd.
func suggestPathUnderCwd(requestedPath string, workingDir string) (string, error) {
	// Clean the requested path
	cleanPath := filepath.Clean(requestedPath)

	// Resolve to absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", nil
	}

	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return "", nil
	}

	absCwdParent := filepath.Dir(absWorkingDir)

	// Check if the requested path is under cwd's parent but not under cwd itself
	cwdParentPrefix := absCwdParent
	if absCwdParent != "/" {
		cwdParentPrefix = absCwdParent + string(filepath.Separator)
	}

	// If path doesn't start with cwd parent prefix or is already under cwd, no suggestion
	if !strings.HasPrefix(absPath, cwdParentPrefix) {
		return "", nil
	}

	if strings.HasPrefix(absPath, absWorkingDir+string(filepath.Separator)) || absPath == absWorkingDir {
		return "", nil
	}

	// Get the basename of the requested file
	requestedBase := filepath.Base(absPath)

	// Check if a file with the same name exists under cwd
	correctedPath := filepath.Join(absWorkingDir, requestedBase)

	// Check if the corrected path exists
	if _, err := os.Stat(correctedPath); err == nil {
		return correctedPath, nil
	}

	return "", nil
}

// formatNotFoundError formats a helpful "not found" error with optional suggestion
func formatNotFoundError(path string, workingDir string) error {
	baseMsg := fmt.Sprintf("path '%s' does not exist", path)

	// Try to suggest a corrected path
	suggestion, err := suggestPathUnderCwd(path, workingDir)
	if err != nil {
		return fmt.Errorf("%s: %w", baseMsg, err)
	}

	// Add suggestion if available
	if suggestion != "" {
		// Make the suggestion relative to cwd for readability
		relSuggestion, err := filepath.Rel(workingDir, suggestion)
		if err != nil {
			relSuggestion = suggestion
		}
		return fmt.Errorf("%s (current directory: %s). Did you mean '%s'?",
			baseMsg, workingDir, relSuggestion)
	}

	return fmt.Errorf("%s (current directory: %s)", baseMsg, workingDir)
}

// isUNCPath checks if a path is a UNC path (Windows network path)
// UNC paths start with \\ (e.g., \\server\share\file.txt)
// These can be security risks as they may leak NTLM credentials
func isUNCPath(path string) bool {
	// Check for \\ or // prefix (Windows UNC or similar)
	return strings.HasPrefix(path, "\\\\") || strings.HasPrefix(path, "//")
}

// validatePathForSecurity performs security checks on a path
func validatePathForSecurity(path string) error {
	// Check for UNC paths which can leak NTLM credentials
	if isUNCPath(path) {
		return fmt.Errorf("UNC/network paths are not allowed for security reasons: %s", path)
	}

	// Check for path traversal attempts by counting ../ segments
	// A path like ../../../../../etc/passwd is suspicious
	dotDotSlashCount := strings.Count(path, "../")
	if dotDotSlashCount >= 3 {
		// More than 2 "../" is likely malicious
		return fmt.Errorf("excessive path traversal detected: %s", path)
	}

	// Check if the cleaned path is different and might still be suspicious
	cleanPath := filepath.Clean(path)
	if cleanPath != path && strings.Contains(path, "../") {
		// Check if the cleaned path still goes up directories
		// by resolving to absolute and comparing
		absPath, err := filepath.Abs(path)
		if err == nil {
			// Check if it tries to escape to system directories
			suspiciousPaths := []string{"/etc/", "/sys/", "/proc/", "/root/"}
			for _, suspicious := range suspiciousPaths {
				if strings.HasPrefix(absPath, suspicious) {
					return fmt.Errorf("path traversal to system directory detected: %s", path)
				}
			}
		}
	}

	return nil
}

// formatGlobResult formats the result message in a user-friendly way
func formatGlobResult(filenames []string, numFiles int, durationMs int64, truncated bool) string {
	var parts []string

	// Main result message
	if numFiles == 0 {
		parts = append(parts, "No files found")
	} else {
		parts = append(parts, fmt.Sprintf("Found %d file%s", numFiles, plural(numFiles)))
	}

	// Duration info
	if durationMs > 0 {
		parts = append(parts, fmt.Sprintf("in %dms", durationMs))
	}

	// Truncation warning
	if truncated {
		parts = append(parts, fmt.Sprintf("(results limited to %d files)", MaxResults))
	}

	return strings.Join(parts, " ")
}

// plural returns the plural suffix for a count
func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
