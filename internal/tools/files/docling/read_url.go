package docling

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat/internal/docling"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
)

const (
	// ReadURLToolName is the name of the read_document_url tool.
	ReadURLToolName = "read_document_url"

	// ReadURLDisplayName is the display name of the read_document_url tool.
	ReadURLDisplayName = "Read Document URL"

	// ReadURLSearchHint is a hint for tool search functionality.
	ReadURLSearchHint = "fetch and convert a remote document or web page to markdown"

	// ReadURLDescription is the description of the read_document_url tool.
	ReadURLDescription = "Fetch a document at a URL and convert it to readable markdown. " +
		"Supports PDF, DOCX, PPTX, XLSX, HTML pages, and arXiv papers. " +
		"Requires docling-serve to be configured. " +
		"Optionally saves the extracted markdown to a workspace path."
)

// ReadURLTool fetches a remote document and converts it to markdown via a
// document-conversion backend.
type ReadURLTool struct {
	doclingClient docling.DocumentConverterBackend
}

// NewReadURLTool creates a new read_document_url tool.
func NewReadURLTool(cfg Config) *ReadURLTool {
	return &ReadURLTool{doclingClient: cfg.converter()}
}

func (t *ReadURLTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ReadURLToolName,
		DisplayName: ReadURLDisplayName,
		SearchHint:  ReadURLSearchHint,
		Description: ReadURLDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL of the document to fetch and convert (e.g. an arXiv PDF, a DOCX on a server, or any HTML page)",
				},
				"save_path": map[string]any{
					"type":        "string",
					"description": "Optional path (relative to workspace) to write the extracted markdown. Example: \"docs/paper.md\"",
				},
			},
			"required": []string{"url"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

func (t *ReadURLTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	rawURL, ok := input.Parsed["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return tool.NewErrorResult(fmt.Errorf("url is required")), nil
	}
	rawURL = strings.TrimSpace(rawURL)

	if err := validateURL(rawURL); err != nil {
		return tool.NewErrorResult(err), nil
	}

	savePath, _ := input.Parsed["save_path"].(string)
	savePath = strings.TrimSpace(savePath)

	if t.doclingClient == nil || !t.doclingClient.IsAvailable(ctx) {
		return tool.NewTextResult(fmt.Sprintf(
			"URL: %s\n\nread_document_url requires docling-serve. "+
				"Configure the DOCLING_URL setting to enable remote document conversion.",
			rawURL,
		)), nil
	}

	conversion, err := t.doclingClient.ConvertURL(ctx, rawURL)
	if err != nil {
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("read_document_url cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("docling conversion failed for %s: %w", rawURL, err)), nil
	}

	// Optionally write markdown to workspace path.
	savedAt := ""
	if savePath != "" {
		absPath := resolveWorkspacePath(savePath, input.ToolContextValue())
		if absPath != "" {
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err == nil {
				if err := os.WriteFile(absPath, []byte(conversion.Markdown), 0o644); err == nil {
					savedAt = absPath
				}
			}
		}
	}

	return tool.NewTextResult(formatURLResult(rawURL, conversion, savedAt)), nil
}

func (t *ReadURLTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	rawURL, ok := input["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("url is required")
	}
	if err := validateURL(strings.TrimSpace(rawURL)); err != nil {
		return nil, err
	}
	return input, nil
}

func (t *ReadURLTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}

func (t *ReadURLTool) IsEnabled() bool                         { return true }
func (t *ReadURLTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *ReadURLTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *ReadURLTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
func (t *ReadURLTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
func (t *ReadURLTool) Description(_ context.Context) (string, error) { return ReadURLDescription, nil }

// ── helpers ──────────────────────────────────────────────────────────────────

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return nil
	case "":
		return fmt.Errorf("URL must include a scheme (http:// or https://)")
	default:
		return fmt.Errorf("unsupported URL scheme %q: only http and https are allowed", u.Scheme)
	}
}

func resolveWorkspacePath(path string, toolCtx tool.ToolUseContext) string {
	if toolCtx.Workspace != nil {
		resolved, err := toolCtx.Workspace.Resolve(path)
		if err != nil {
			return ""
		}
		return resolved
	}
	if filepath.IsAbs(path) {
		return path
	}
	workingDir := strings.TrimSpace(toolCtx.WorkingDirectory)
	if workingDir == "" {
		return ""
	}
	return filepath.Join(workingDir, path)
}

func formatURLResult(sourceURL string, r *docling.ConversionResult, savedAt string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Source: %s\n", sourceURL))
	if r.PageCount > 0 {
		b.WriteString(fmt.Sprintf("Pages: %d\n", r.PageCount))
	}
	if len(r.Images) > 0 {
		b.WriteString(fmt.Sprintf("Extracted images: %d\n", len(r.Images)))
		for _, img := range r.Images {
			b.WriteString(fmt.Sprintf("  - %s (%s) [data:%s;base64,%s]\n",
				img.Filename, img.MimeType, img.MimeType, img.Base64))
		}
	}
	if savedAt != "" {
		b.WriteString(fmt.Sprintf("Saved to: %s\n", savedAt))
	}
	b.WriteString("\n")
	b.WriteString(r.Markdown)
	return b.String()
}
