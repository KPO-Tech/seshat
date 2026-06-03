package browser

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func jsonResult(data any, content string) tool.CallResult {
	result := tool.NewJSONResult(data)
	result.Content = content
	return result
}

func validateRequiredString(input map[string]any, key string) (map[string]any, error) {
	value, ok := input[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("%s is required", key)
	}
	return input, nil
}

func validateOptionalString(input map[string]any, key string) (map[string]any, error) {
	if value, ok := input[key]; ok {
		if _, isString := value.(string); !isString {
			return nil, fmt.Errorf("%s must be a string", key)
		}
	}
	return input, nil
}

func readRequiredString(input map[string]any, key string) string {
	return strings.TrimSpace(readOptionalString(input, key))
}

func readOptionalString(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func readOptionalInt(input map[string]any, key string) int {
	if value, ok := input[key].(float64); ok {
		return int(value)
	}
	return 0
}

func readOptionalBool(input map[string]any, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func formatJSONish(data any) string {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(encoded)
}

func formatPages(pages []browsercore.PageInfo) string {
	if len(pages) == 0 {
		return "No browser pages are open."
	}
	var builder strings.Builder
	builder.WriteString("Browser pages:\n")
	for _, page := range pages {
		active := ""
		if page.Active {
			active = " [active]"
		}
		builder.WriteString(fmt.Sprintf("- %s%s %s\n", page.ID, active, page.URL))
	}
	return strings.TrimSpace(builder.String())
}

func formatSnapshot(snapshot browsercore.Snapshot) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Page: %s\n", snapshot.Page.ID))
	builder.WriteString(fmt.Sprintf("URL: %s\n", snapshot.Page.URL))
	if snapshot.Page.Title != "" {
		builder.WriteString(fmt.Sprintf("Title: %s\n", snapshot.Page.Title))
	}
	if len(snapshot.Headings) > 0 {
		builder.WriteString("\nHeadings:\n")
		for _, heading := range snapshot.Headings {
			builder.WriteString(fmt.Sprintf("- h%d %s\n", heading.Level, heading.Text))
		}
	}
	builder.WriteString("\nText:\n")
	builder.WriteString(snapshot.Text)
	if len(snapshot.Elements) > 0 {
		builder.WriteString("\n\nInteractive elements:\n")
		for _, element := range snapshot.Elements {
			builder.WriteString(fmt.Sprintf("- %s [%s] %s\n", element.ID, element.Role, strings.TrimSpace(element.Name)))
		}
	}
	return strings.TrimSpace(builder.String())
}

func formatExtract(snapshot browsercore.Snapshot) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("URL: %s\n", snapshot.Page.URL))
	if snapshot.Page.Title != "" {
		builder.WriteString(fmt.Sprintf("Title: %s\n", snapshot.Page.Title))
	}
	if len(snapshot.Headings) > 0 {
		builder.WriteString("\nHeadings:\n")
		for _, heading := range snapshot.Headings {
			builder.WriteString(fmt.Sprintf("- h%d %s\n", heading.Level, heading.Text))
		}
	}
	builder.WriteString("\nContent:\n")
	builder.WriteString(snapshot.Text)
	return strings.TrimSpace(builder.String())
}

func formatScreenshot(screenshot browsercore.Screenshot) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Page: %s\n", screenshot.Page.ID))
	builder.WriteString(fmt.Sprintf("URL: %s\n", screenshot.Page.URL))
	builder.WriteString(fmt.Sprintf("Image: %s (%d bytes)\n", screenshot.MimeType, screenshot.Bytes))
	if screenshot.FullPage {
		builder.WriteString("Capture: full page\n")
	} else {
		builder.WriteString("Capture: viewport\n")
	}
	if screenshot.PersistedPath != "" {
		builder.WriteString(fmt.Sprintf("Path: %s\n", screenshot.PersistedPath))
	}
	builder.WriteString(fmt.Sprintf("Data: %d base64 chars", len(screenshot.DataBase64)))
	return builder.String()
}

func formatNetwork(entries []browsercore.NetworkEntry) string {
	if len(entries) == 0 {
		return "No recent browser network activity."
	}
	var builder strings.Builder
	builder.WriteString("Recent browser network activity:\n")
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("- #%d [%s] %s", entry.Seq, entry.Stage, entry.URL))
		if entry.PageID != "" {
			builder.WriteString(fmt.Sprintf(" (page %s)", entry.PageID))
		}
		if entry.Method != "" {
			builder.WriteString(fmt.Sprintf(" %s", entry.Method))
		}
		if entry.StatusCode > 0 {
			builder.WriteString(fmt.Sprintf(" -> %d", entry.StatusCode))
		}
		if entry.ErrorText != "" {
			builder.WriteString(fmt.Sprintf(" error=%s", entry.ErrorText))
		}
		builder.WriteByte('\n')
	}
	return strings.TrimSpace(builder.String())
}

func formatDownloads(entries []browsercore.DownloadEntry) string {
	if len(entries) == 0 {
		return "No browser downloads recorded."
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Browser downloads: %d\n\n", len(entries)))
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("- %s [%s]\n", entry.GUID, entry.State))
		if entry.PageID != "" {
			builder.WriteString(fmt.Sprintf("  page: %s\n", entry.PageID))
		}
		if entry.SuggestedFilename != "" {
			builder.WriteString(fmt.Sprintf("  file: %s\n", entry.SuggestedFilename))
		}
		if entry.URL != "" {
			builder.WriteString(fmt.Sprintf("  url: %s\n", entry.URL))
		}
		if entry.TotalBytes > 0 || entry.BytesReceived > 0 {
			builder.WriteString(fmt.Sprintf("  bytes: %d/%d\n", entry.BytesReceived, entry.TotalBytes))
		}
		if entry.PersistedPath != "" {
			builder.WriteString(fmt.Sprintf("  stored: %s (%d bytes)\n", entry.PersistedPath, entry.PersistedSize))
		}
		if entry.ErrorText != "" {
			builder.WriteString(fmt.Sprintf("  error: %s\n", entry.ErrorText))
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
