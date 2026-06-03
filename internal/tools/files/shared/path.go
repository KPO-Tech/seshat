package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetAbsolutePath returns the real absolute path with symlinks resolved.
// If the path doesn't exist, the parent is resolved to prevent escape.
func GetAbsolutePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = filepath.Join(cwd, path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		parentReal, err2 := filepath.EvalSymlinks(filepath.Dir(path))
		if err2 != nil {
			return filepath.Clean(path), nil
		}
		return filepath.Join(parentReal, filepath.Base(path)), nil
	}
	return real, nil
}

// IsPathWithin checks if a path is within a base directory (after symlink resolution).
func IsPathWithin(base, path string) (bool, error) {
	absBase, err := GetAbsolutePath(base)
	if err != nil {
		return false, err
	}

	absPath, err := GetAbsolutePath(path)
	if err != nil {
		return false, err
	}

	// Ensure base ends with separator to prevent /foo matching /foobar
	if absBase != "/" {
		absBase = absBase + string(filepath.Separator)
	}
	return strings.HasPrefix(absPath+string(filepath.Separator), absBase), nil
}

// SuggestPathUnderCwd suggests a corrected path if the requested path exists
// under the parent of the current working directory but not under cwd itself.
// This helps users who accidentally type paths like "../project/file.go" when
// they mean "file.go" (assuming the file exists in both locations).
//
// IMPORTANT: This function only suggests if the requested path DOES NOT exist.
// If the requested path exists, no suggestion is made even if a similar file exists under cwd.
func SuggestPathUnderCwd(requestedPath string, workingDir string) (string, error) {
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

// FormatNotFoundError formats a helpful "not found" error with optional suggestion
func FormatNotFoundError(path string, workingDir string) error {
	baseMsg := fmt.Sprintf("path '%s' does not exist", path)

	// Try to suggest a corrected path
	suggestion, err := SuggestPathUnderCwd(path, workingDir)
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

// ValidatePathExists checks if a path exists
func ValidatePathExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("failed to access path: %s: %w", path, err)
	}
	return nil
}

// ValidatePathIsDirectory checks if a path exists and is a directory
func ValidatePathIsDirectory(path string) error {
	stats, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		return fmt.Errorf("failed to access directory: %s: %w", path, err)
	}

	if !stats.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// Plural returns the plural suffix for a count
func Plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
