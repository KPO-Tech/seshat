package kernel

import (
	"fmt"
	"regexp"
	"strings"
)

// Output is a parsed result from a single Jupyter kernel message.
type Output struct {
	Type      string         // "stream", "execute_result", "display_data", "error"
	Name      string         // for stream: "stdout" or "stderr"
	Text      string         // text representation
	ImagePNG  string         // base64-encoded PNG (display_data)
	ImageJPEG string         // base64-encoded JPEG (display_data)
	Data      map[string]any // raw data dict from the message
}

// IsImage reports whether the output contains an image.
func (o Output) IsImage() bool {
	return o.ImagePNG != "" || o.ImageJPEG != ""
}

// ImageMIME returns the MIME type of the image, or empty string.
func (o Output) ImageMIME() string {
	if o.ImagePNG != "" {
		return "image/png"
	}
	if o.ImageJPEG != "" {
		return "image/jpeg"
	}
	return ""
}

// ImageData returns the base64 image data, or empty string.
func (o Output) ImageData() string {
	if o.ImagePNG != "" {
		return o.ImagePNG
	}
	return o.ImageJPEG
}

// FormatOutputs converts a slice of Outputs into a human-readable string
// suitable for returning to the model. Images are represented as a
// placeholder referencing their MIME type.
func FormatOutputs(outputs []Output) string {
	if len(outputs) == 0 {
		return "[No output]"
	}
	var sb strings.Builder
	for i, o := range outputs {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch {
		case o.IsImage():
			sb.WriteString("[" + o.ImageMIME() + " image — base64 data available]")
		case o.Type == "error":
			sb.WriteString("[ERROR] ")
			sb.WriteString(o.Text)
		case o.Type == "stream" && o.Name == "stderr":
			sb.WriteString("[stderr] ")
			sb.WriteString(strings.TrimRight(o.Text, "\n"))
		default:
			sb.WriteString(strings.TrimRight(o.Text, "\n"))
		}
	}
	return sb.String()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// extractText picks the best text representation from a Jupyter data dict,
// preferring text/plain over other MIME types.
func extractText(data map[string]any) string {
	for _, key := range []string{"text/plain", "text/html", "text/latex"} {
		switch v := data[key].(type) {
		case string:
			return v
		case []any:
			var sb strings.Builder
			for _, item := range v {
				sb.WriteString(fmt.Sprint(item))
			}
			return sb.String()
		}
	}
	return ""
}

// ansiEscape strips ANSI escape sequences from strings (traceback lines).
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansiEscape.ReplaceAllString(l, "")
	}
	return out
}
