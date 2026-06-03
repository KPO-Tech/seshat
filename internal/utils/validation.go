package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathValidator validates and sanitizes file paths
type PathValidator struct {
	// AllowedDirectories are the only directories that can be accessed
	AllowedDirectories []string
}

// NewPathValidator creates a new path validator
func NewPathValidator(allowedDirs []string) *PathValidator {
	return &PathValidator{
		AllowedDirectories: allowedDirs,
	}
}

// ValidatePath validates that a path is within allowed directories
func (v *PathValidator) ValidatePath(path string) error {
	cleanPath := filepath.Clean(path)

	// If no restrictions, allow any path
	if len(v.AllowedDirectories) == 0 {
		return nil
	}

	// Check if path is within allowed directories
	for _, allowedDir := range v.AllowedDirectories {
		allowedClean := filepath.Clean(allowedDir)
		if strings.HasPrefix(cleanPath, allowedClean) {
			return nil
		}
	}

	return fmt.Errorf("path '%s' is not within allowed directories: %v",
		path, v.AllowedDirectories)
}

// SanitizePathComponent sanitizes a path component for use in filenames
func SanitizePathComponent(path string) string {
	// Remove any directory separators
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "\\", "_")

	// Remove other problematic characters
	problematic := []string{":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range problematic {
		path = strings.ReplaceAll(path, char, "_")
	}

	// Limit length
	if len(path) > 100 {
		path = path[:100]
	}

	return path
}

// IsValidToolName validates a tool name
func IsValidToolName(name string) bool {
	if name == "" {
		return false
	}

	// Tool names should be alphanumeric with underscores
	for _, char := range name {
		if !isValidToolNameChar(char) {
			return false
		}
	}

	return true
}

func isValidToolNameChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '_' || char == '-'
}

// ValidateModelIdentifier validates a model identifier
func ValidateModelIdentifier(model string) error {
	if model == "" {
		return fmt.Errorf("model identifier cannot be empty")
	}

	// Model identifier should be in format: provider:model or provider:model@version
	parts := strings.Split(model, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid model identifier format: %s (expected provider:model)", model)
	}

	provider := parts[0]
	validProviders := []string{"anthropic", "openai", "ollama"}
	isValid := false
	for _, vp := range validProviders {
		if provider == vp {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("unknown provider: %s (valid: %v)", provider, validProviders)
	}

	return nil
}

// ValidateSessionID validates a session ID format
func ValidateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	if len(id) < 10 {
		return fmt.Errorf("session ID too short (min 10 characters)")
	}

	return nil
}

// ValidateTurnID validates a turn ID format
func ValidateTurnID(id string) error {
	if id == "" {
		return fmt.Errorf("turn ID cannot be empty")
	}

	if len(id) < 10 {
		return fmt.Errorf("turn ID too short (min 10 characters)")
	}

	return nil
}

// ValidateMessageID validates a message ID format
func ValidateMessageID(id string) error {
	if id == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	if len(id) < 10 {
		return fmt.Errorf("message ID too short (min 10 characters)")
	}

	return nil
}

// Clamp clamps a value between min and max
func Clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampFloat64 clamps a float64 value between min and max
func ClampFloat64(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
