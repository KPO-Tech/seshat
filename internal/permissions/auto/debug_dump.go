// Package auto - Debug utilities for classifier diagnostics.
//
// This module provides debug utilities for dumping classifier requests,
// responses, and transcripts for troubleshooting.
// Aligned with OpenClaude's yoloClassifier.ts debug functions.
package auto

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// AutoModeDumpDir is the directory for classifier debug dumps.
	AutoModeDumpDir = "auto-mode"
)

// GetAutoModeClassifierErrorDumpPath returns the path for dumping classifier errors.
// Aligned with OpenClaude's getAutoModeClassifierErrorDumpPath (yoloClassifier.ts:189).
func GetAutoModeClassifierErrorDumpPath() string {
	tempDir := getClaudeTempDir()
	return filepath.Join(tempDir, AutoModeDumpDir, "classifier-errors")
}

// GetAutoModeDumpDir returns the base directory for auto mode dumps.
// Aligned with OpenClaude's getAutoModeDumpDir (yoloClassifier.ts:147).
func GetAutoModeDumpDir() string {
	tempDir := getClaudeTempDir()
	return filepath.Join(tempDir, AutoModeDumpDir)
}

// EnsureAutoModeDumpDir ensures the auto mode dump directory exists.
func EnsureAutoModeDumpDir() error {
	dir := GetAutoModeDumpDir()
	return os.MkdirAll(dir, 0755)
}

// DumpClassifierRequest dumps a classifier request for debugging.
func DumpClassifierRequest(reqID string, content string) error {
	dir := GetAutoModeDumpDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dump dir: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.req.json", reqID))
	return os.WriteFile(path, []byte(content), 0644)
}

// DumpClassifierResponse dumps a classifier response for debugging.
func DumpClassifierResponse(reqID string, content string) error {
	dir := GetAutoModeDumpDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dump dir: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.res.json", reqID))
	return os.WriteFile(path, []byte(content), 0644)
}

// getClaudeTempDir returns the Claude temp directory.
// This is a placeholder - in production, use the actual temp dir.
func getClaudeTempDir() string {
	// Use /tmp/.claude or equivalent
	return filepath.Join(os.TempDir(), ".claude")
}
