package webfetch

import (
	"fmt"
	"strings"
)

func (t *Tool) applyPrompt(content, prompt string, isPreapproved bool) string {
	trimmedContent := content
	if len(trimmedContent) > MaxPromptPreviewLength {
		trimmedContent = trimmedContent[:MaxPromptPreviewLength] + "\n\n[Content preview truncated...]"
	}

	prefix := "Processed content"
	if isPreapproved {
		prefix = "Processed preapproved content"
	}
	return fmt.Sprintf("%s for prompt: %s\n\n%s", prefix, prompt, trimmedContent)
}

func (t *Tool) formatOutput(output Output) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Fetched from: %s\n", output.URL))
	sb.WriteString(fmt.Sprintf("Mode: %s\n", output.Mode))
	sb.WriteString(fmt.Sprintf("Status: %d %s\n", output.Code, output.CodeText))
	sb.WriteString(fmt.Sprintf("Size: %d bytes\n", output.Bytes))
	if output.PersistedPath != "" {
		sb.WriteString(fmt.Sprintf("Stored artifact: %s", output.PersistedPath))
		if output.PersistedSize > 0 {
			sb.WriteString(fmt.Sprintf(" (%d bytes)", output.PersistedSize))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("Time: %dms\n\n", output.DurationMs))
	sb.WriteString("Result:\n")
	sb.WriteString(output.Result)
	return sb.String()
}

func getStatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 307:
		return "Temporary Redirect"
	case 308:
		return "Permanent Redirect"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 500:
		return "Internal Server Error"
	default:
		return "Unknown"
	}
}
