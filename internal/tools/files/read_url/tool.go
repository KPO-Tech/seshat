package readurl

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/docling"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	ToolName    = "read_document_url"
	DisplayName = "Read Document URL"
	SearchHint  = "fetch and convert a remote document or web page to markdown"
	Description = "Fetch a document at a URL and convert it to readable markdown. " +
		"Supports PDF, DOCX, PPTX, XLSX, HTML pages, and arXiv papers. " +
		"Requires docling-serve to be configured. " +
		"Optionally saves the extracted markdown to a workspace path."
)

// Tool fetches a remote document and converts it to markdown via docling-serve.
type Tool struct {
	doclingClient *docling.Client
}

// Config holds the configuration for the read_document_url tool.
type Config struct {
	// DoclingURL is the base URL of a running docling-serve instance.
	// When empty the tool registers but always returns a "not configured" message.
	DoclingURL string
}

func NewTool(cfg Config) *Tool {
	t := &Tool{}
	if cfg.DoclingURL != "" {
		t.doclingClient = docling.NewClient(cfg.DoclingURL)
	}
	return t
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: DisplayName,
		SearchHint:  SearchHint,
		Description: Description,
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

func (t *Tool) Call(
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

	return tool.NewTextResult(format(rawURL, conversion, savedAt)), nil
}

func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	rawURL, ok := input["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("url is required")
	}
	if err := validateURL(strings.TrimSpace(rawURL)); err != nil {
		return nil, err
	}
	return input, nil
}

func (t *Tool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}

func (t *Tool) IsEnabled() bool                                                      { return true }
func (t *Tool) IsReadOnly(_ map[string]any) bool                                     { return true }
func (t *Tool) IsConcurrencySafe(_ map[string]any) bool                              { return true }
func (t *Tool) BackfillInput(_ context.Context, input map[string]any) map[string]any { return input }
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
func (t *Tool) Description(_ context.Context) (string, error) { return Description, nil }

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

func format(sourceURL string, r *docling.ConversionResult, savedAt string) string {
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
