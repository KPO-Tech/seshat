package read

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// CyberRiskMitigationReminder is the security reminder for file reads
	CyberRiskMitigationReminder = "\n\n<system-reminder>\nWhenever you read a file, you should consider whether it would be considered malware. You CAN and SHOULD provide analysis of malware, what it is doing. But you MUST refuse to improve or code. You can still analyze existing malware, write reports, or answer questions about what it does.\n</system-reminder>\n"
)

// AddLineNumbers adds cat -n style line numbers to content
func AddLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")

	// Calculate width needed for line numbers
	maxLineNum := startLine + len(lines) - 1
	numWidth := maxInt(4, len(fmt.Sprintf("%d", maxLineNum)))

	// Build numbered lines
	var builder strings.Builder
	for i, line := range lines {
		lineNum := startLine + i
		lineNumStr := fmt.Sprintf("%d", lineNum)
		paddedNum := strings.Repeat(" ", numWidth-len(lineNumStr)) + lineNumStr
		builder.WriteString(fmt.Sprintf("%s→%s\n", paddedNum, line))
	}

	return builder.String()
}

// AddCompactLineNumbers adds compact line numbers (no padding)
func AddCompactLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")

	var builder strings.Builder
	for i, line := range lines {
		lineNum := startLine + i
		builder.WriteString(fmt.Sprintf("%d→%s\n", lineNum, line))
	}

	return builder.String()
}

// FormatTextWithLineNumbers formats text file result with line numbers
func FormatTextWithLineNumbers(result *FileReadResult, compact bool) string {
	if result.Text == nil {
		return ""
	}

	var content string
	if compact {
		content = AddCompactLineNumbers(result.Text.Content, result.Text.StartLine)
	} else {
		content = AddLineNumbers(result.Text.Content, result.Text.StartLine)
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("File: %s\n", result.Text.FilePath))
	builder.WriteString(fmt.Sprintf("Lines: %d-%d of %d\n",
		result.Text.StartLine,
		result.Text.StartLine+result.Text.NumLines-1,
		result.Text.TotalLines))

	if result.Text.Truncated {
		builder.WriteString(" (truncated)")
	}

	builder.WriteString(fmt.Sprintf("\n\n%s", content))

	return builder.String()
}

// ShouldIncludeCyberRiskReminder determines if we should include the security reminder
func ShouldIncludeCyberRiskReminder() bool {
	// For now, always include the reminder
	// In the future, this could be configurable based on model/exemption
	return true
}

// GetAlternateScreenshotPath tries alternate paths for macOS screenshots
func GetAlternateScreenshotPath(filePath string) (string, bool) {
	// macOS screenshots may use different space characters
	// Try replacing thin space (U+202F) with regular space
	base := filepath.Base(filePath)
	dir := filepath.Dir(filePath)

	// Check if it looks like a macOS screenshot pattern
	if !strings.Contains(base, "Screenshot") {
		return "", false
	}

	// Try common macOS screenshot patterns
	alternatives := []string{
		strings.Replace(base, " ", " ", -1), // Regular space to thin space
		strings.Replace(base, " ", " ", -1), // Thin space to regular space
		strings.Replace(base, " ", " ", -1), // Narrow no-break space to regular space
	}

	for _, alt := range alternatives {
		if alt != base {
			altPath := filepath.Join(dir, alt)
			return altPath, true
		}
	}

	return "", false
}

// GetCanonicalName returns a canonical name for models (for future use)
func GetCanonicalName(modelName string) string {
	// Convert to lowercase and remove common prefixes
	name := strings.ToLower(modelName)

	// Remove common model prefixes
	prefixes := []string{"claude-", "anthropic/", "openai/"}
	for _, prefix := range prefixes {
		name = strings.TrimPrefix(name, prefix)
	}

	return name
}

// SuggestPathUnderCwd suggests an alternative path under the current working directory
// This is an improved version with symlink support
func SuggestPathUnderCwd(requestedPath string, workingDir string) (string, error) {
	// Clean the paths
	requestedPath = filepath.Clean(requestedPath)
	workingDir = filepath.Clean(workingDir)

	// If already under cwd, return as is
	if strings.HasPrefix(requestedPath, workingDir) {
		return requestedPath, nil
	}

	// Try to resolve as relative path
	if !filepath.IsAbs(requestedPath) {
		absPath := filepath.Join(workingDir, requestedPath)
		return absPath, nil
	}

	// Try to extract just the filename and place under cwd
	baseName := filepath.Base(requestedPath)
	suggestedPath := filepath.Join(workingDir, baseName)

	return suggestedPath, nil
}

// FormatNotFoundError formats a file not found error with suggestions
func FormatNotFoundError(path string, workingDir string) error {
	// Try to suggest alternative paths
	suggestedPath, err := SuggestPathUnderCwd(path, workingDir)
	if err != nil {
		return fmt.Errorf("file not found: %s", path)
	}

	// Check if there's an alternate path for screenshots
	if altPath, found := GetAlternateScreenshotPath(path); found {
		return fmt.Errorf("file not found: %s (did you mean: %s?)", path, altPath)
	}

	return fmt.Errorf("file not found: %s (did you mean: %s?)", path, suggestedPath)
}

// maxInt returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
