package pdfwrite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/KPO-Tech/seshat/internal/sandbox"
	fileReadTool "github.com/KPO-Tech/seshat/internal/tools/files/read"
	"github.com/KPO-Tech/seshat/internal/tools/files/shared"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
)

const (
	// ToolName is the name of the PDF write tool.
	ToolName = "write_pdf"

	// SearchHint is a hint for tool search functionality.
	SearchHint = "create a PDF from text, append pages, or delete pages"

	// ToolDescription is the description of the write_pdf tool.
	ToolDescription = "Create a new PDF from plain/markdown-ish text, append text as new page(s) to an existing PDF, or delete pages from an existing PDF.\n\n" +
		"## When to use\n\n" +
		"- Creating a new PDF — provide content; text wraps and paginates automatically, lines starting with \"#\".. \"######\" become headings\n" +
		"- Appending content to an existing PDF — provide content for an existing file; it's rendered and added as new page(s) at the end\n" +
		"- Deleting pages from an existing PDF — provide delete_pages (e.g. \"2\", \"2-4\", \"2,4-6\")\n\n" +
		"## When NOT to use\n\n" +
		"- Reading PDF content — use FileRead instead\n" +
		"- Editing existing text in place, images, or complex layout — this only appends/deletes whole pages, it cannot find/replace text inside a PDF (unlike docx_edit) because PDF text isn't addressable the same way\n\n" +
		"## Rules\n\n" +
		"- content and delete_pages are mutually exclusive in a single call.\n" +
		"- Modifying an existing file (content-append or delete_pages) requires it to have been read with FileRead first, same as other write tools.\n"
)

// Tool implements the write_pdf tool.
type Tool struct {
	workingDir       string
	filesystemPolicy *sandbox.FilesystemPolicy
}

// NewTool creates a new PDF write tool.
func NewTool(workingDir string) *Tool {
	return &Tool{
		workingDir:       workingDir,
		filesystemPolicy: sandbox.NewDefaultFilesystemPolicy(),
	}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Write PDF",
		SearchHint:  SearchHint,
		Description: ToolDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the .pdf file to create, append to, or delete pages from",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to render. When the file doesn't exist, becomes the whole document. When it exists, is appended as new page(s). Lines starting with \"#\" through \"######\" become headings.",
				},
				"delete_pages": map[string]any{
					"type":        "string",
					"description": "Pages to delete from an existing PDF, e.g. \"2\", \"2-4\", or \"2,4-6\". Mutually exclusive with content.",
				},
			},
			"required": []string{"file_path"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
	}
}

func (t *Tool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	filePath, ok := input.Parsed["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return tool.NewErrorResult(fmt.Errorf("file_path is required and must be a string")), nil
	}
	if err := shared.ValidateFilePath(filePath, "writing"); err != nil {
		return tool.NewErrorResult(err), nil
	}
	if !strings.EqualFold(filepath.Ext(filePath), ".pdf") {
		return tool.NewErrorResult(fmt.Errorf("write_pdf only writes .pdf files, got: %s", filePath)), nil
	}

	content, hasContent := input.Parsed["content"].(string)
	deletePages, hasDeletePages := input.Parsed["delete_pages"].(string)
	hasContent = hasContent && content != ""
	hasDeletePages = hasDeletePages && strings.TrimSpace(deletePages) != ""
	if hasContent && hasDeletePages {
		return tool.NewErrorResult(fmt.Errorf("content and delete_pages are mutually exclusive - provide one or the other")), nil
	}
	if !hasContent && !hasDeletePages {
		return tool.NewErrorResult(fmt.Errorf("provide either content (to write/append) or delete_pages (to remove pages)")), nil
	}

	var selectedPages []string
	if hasDeletePages {
		var err error
		selectedPages, err = parseSelectedPages(deletePages)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
	}

	toolCtx := input.ToolContextValue()
	absolutePath, err := t.resolvePath(filePath, toolCtx)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := t.validateWritePath(toolCtx, absolutePath); err != nil {
		return tool.NewErrorResult(fmt.Errorf("path validation failed: %w", err)), nil
	}
	if err := shared.ValidateFilePath(absolutePath, "writing"); err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := shared.ValidateUNCPathSecurity(absolutePath); err != nil {
		return tool.NewErrorResult(err), nil
	}

	fileExists := false
	if info, statErr := os.Stat(absolutePath); statErr == nil {
		if info.IsDir() {
			return tool.NewErrorResult(fmt.Errorf("path is a directory, not a file: %s", filePath)), nil
		}
		fileExists = true
	}

	if hasDeletePages && !fileExists {
		return tool.NewErrorResult(fmt.Errorf("file not found: %s", filePath)), nil
	}
	if fileExists {
		if _, wasRead := fileReadTool.GetLastReadState(absolutePath); !wasRead {
			return tool.NewErrorResult(fmt.Errorf("file has not been read yet. Read it first before modifying it: %s", filePath)), nil
		}
	}

	if permissionCheck != nil {
		req := sandbox.PermissionRequest{
			ToolName:      ToolName,
			Environment:   sandbox.EnvironmentLocal,
			Access:        sandbox.AccessWrite,
			Paths:         []string{absolutePath},
			Justification: "Write PDF document contents",
			Scope:         sandbox.ApprovalScopeToolCall,
		}
		permResult, err := sandbox.ResolveToolPermission(ctx, permissionCheck, req, sandbox.ToolPermissionOptions{
			ToolInput: map[string]any{
				"file_path": absolutePath,
			},
			ToolUseID:              toolCtx.ToolUseID,
			SessionID:              toolCtx.SessionID,
			TurnID:                 toolCtx.TurnID,
			PermissionMode:         toolCtx.PermissionMode,
			WorkingDirectory:       t.effectiveWorkingDir(toolCtx),
			IsToolRunningInSandbox: toolCtx.EnableSandbox,
		})
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		if err := sandbox.ErrorForPermissionResult(permResult, "PDF write requires approval"); err != nil {
			return tool.NewErrorResult(err), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(absolutePath), 0755); err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to create parent directories: %w", err)), nil
	}

	var operationType string
	var pageCount int

	switch {
	case hasDeletePages:
		if err := deletePagesFromFile(ctx, absolutePath, selectedPages); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to delete pages from %s: %w", filePath, err)), nil
		}
		operationType = "delete_pages"
	case !fileExists:
		docBytes, err := renderTextToPDF(content)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to build PDF: %w", err)), nil
		}
		if err := os.WriteFile(absolutePath, docBytes, 0644); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to write %s: %w", filePath, err)), nil
		}
		operationType = "create"
	default:
		if err := appendContentToFile(ctx, absolutePath, content); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to append to %s: %w", filePath, err)), nil
		}
		operationType = "append"
	}

	if info, statErr := os.Stat(absolutePath); statErr == nil {
		fileReadTool.RecordExternalRead(absolutePath, info.ModTime(), "", true)
	}
	if n, err := api.PageCountFile(ctx, absolutePath); err == nil {
		pageCount = n
	}

	output := map[string]any{
		"file_path":  filePath,
		"success":    true,
		"type":       operationType,
		"page_count": pageCount,
	}
	return tool.NewJSONResult(output), nil
}

// parseSelectedPages splits a comma-separated page-range spec ("2", "2-4",
// "2,4-6") into pdfcpu's expected []string form, validating each part is a
// well-formed page number or range.
func parseSelectedPages(spec string) ([]string, error) {
	parts := strings.Split(spec, ",")
	selected := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			bounds := strings.SplitN(p, "-", 2)
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid page range: %q", p)
			}
			for _, b := range bounds {
				if _, err := strconv.Atoi(strings.TrimSpace(b)); err != nil {
					return nil, fmt.Errorf("invalid page range: %q", p)
				}
			}
		} else if _, err := strconv.Atoi(p); err != nil {
			return nil, fmt.Errorf("invalid page number: %q", p)
		}
		selected = append(selected, p)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("delete_pages must contain at least one page number or range")
	}
	return selected, nil
}

// deletePagesFromFile removes selectedPages from the PDF at path, writing
// through a sibling temp file and renaming over the original so a failure
// partway through never corrupts the existing file.
func deletePagesFromFile(ctx context.Context, path string, selectedPages []string) error {
	tmp := path + ".tmp"
	conf := model.NewDefaultConfiguration()
	if err := api.RemovePagesFile(ctx, path, tmp, selectedPages, conf); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// appendContentToFile renders content into a standalone PDF and merges its
// page(s) onto the end of the existing PDF at path.
func appendContentToFile(ctx context.Context, path, content string) error {
	docBytes, err := renderTextToPDF(content)
	if err != nil {
		return err
	}
	tmpContent, err := os.CreateTemp("", "write_pdf_append_*.pdf")
	if err != nil {
		return err
	}
	tmpContentPath := tmpContent.Name()
	defer os.Remove(tmpContentPath)
	if _, err := tmpContent.Write(docBytes); err != nil {
		tmpContent.Close()
		return err
	}
	if err := tmpContent.Close(); err != nil {
		return err
	}

	conf := model.NewDefaultConfiguration()
	return api.MergeAppendFile(ctx, []string{tmpContentPath}, path, false, conf)
}

// ── Tool interface plumbing ────────────────────────────────────────────────

func (t *Tool) Description(_ context.Context) (string, error) {
	return ToolDescription, nil
}

func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file_path is required and must be a string")
	}
	content, hasContent := input["content"].(string)
	deletePages, hasDeletePages := input["delete_pages"].(string)
	hasContent = hasContent && content != ""
	hasDeletePages = hasDeletePages && strings.TrimSpace(deletePages) != ""
	if !hasContent && !hasDeletePages {
		return nil, fmt.Errorf("provide either content or delete_pages")
	}
	if hasContent && hasDeletePages {
		return nil, fmt.Errorf("content and delete_pages are mutually exclusive")
	}
	return input, nil
}

func (t *Tool) CheckPermissions(_ context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	filePath, _ := input["file_path"].(string)
	if strings.TrimSpace(filePath) == "" {
		return types.Deny("file_path is required and must be a string")
	}
	absolutePath, err := t.resolvePath(filePath, toolCtx)
	if err != nil {
		return types.Deny(err.Error())
	}
	if err := shared.ValidateUNCPathSecurity(absolutePath); err != nil {
		return types.Deny(err.Error())
	}
	if err := t.validateWritePath(toolCtx, absolutePath); err != nil {
		return types.Deny(err.Error())
	}
	return types.Passthrough(input)
}

func (t *Tool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *Tool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *Tool) IsEnabled() bool                         { return true }
func (t *Tool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *Tool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

func (t *Tool) validateWritePath(toolCtx tool.ToolUseContext, path string) error {
	sandboxCtx := sandbox.Context{
		WorkingDirectory: strings.TrimSpace(toolCtx.WorkingDirectory),
		Environment:      sandbox.EnvironmentLocal,
		SandboxEnabled:   toolCtx.EnableSandbox,
	}
	if toolCtx.Workspace != nil {
		sandboxCtx.WorkspaceRoot = strings.TrimSpace(toolCtx.Workspace.Root)
	}
	decision, err := t.filesystemPolicy.EvaluatePath(sandboxCtx, path, sandbox.AccessWrite)
	if err != nil {
		return err
	}
	return sandbox.ErrorForDecision(decision.DecisionResult)
}

func (t *Tool) resolvePath(path string, toolCtx tool.ToolUseContext) (string, error) {
	if toolCtx.Workspace != nil {
		return toolCtx.Workspace.Resolve(path)
	}
	workingDir := t.effectiveWorkingDir(toolCtx)
	if filepath.IsAbs(path) || strings.TrimSpace(workingDir) == "" {
		return path, nil
	}
	return filepath.Join(workingDir, path), nil
}

func (t *Tool) effectiveWorkingDir(toolCtx tool.ToolUseContext) string {
	if strings.TrimSpace(toolCtx.WorkingDirectory) != "" {
		return toolCtx.WorkingDirectory
	}
	if strings.TrimSpace(t.workingDir) != "" {
		return t.workingDir
	}
	return "."
}
