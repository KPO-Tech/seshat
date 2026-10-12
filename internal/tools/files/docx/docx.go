// Package docx provides find/replace editing for DOCX documents: the
// write-side counterpart to internal/officetext's native DOCX read support,
// the same relationship internal/tools/files/excel has to XLSX. A DOCX body
// is just WordprocessingML XML in a zip - not something write_file/edit_file
// can touch (they'd corrupt the zip treating it as UTF-8 text) - so this
// pretty-prints word/document.xml into readable, addressable text, reuses
// edit_file's own exact-match/fuzzy-match/diff engine
// (internal/tools/files/edit) against that pretty-printed text, then
// compacts and repacks the result. Modeled on DesktopCommanderMCP's
// docx.ts, which uses the same pretty-print/compact round-trip.
package docx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/sandbox"
	editTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/edit"
	fileReadTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/read"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/files/shared"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const (
	// ToolName is the name of the DOCX edit tool.
	ToolName = "docx_edit"

	// SearchHint is a hint for tool search functionality.
	SearchHint = "create or find/replace edit a Word (.docx) document"

	// ToolDescription is the description of the docx_edit tool.
	ToolDescription = "Create a new Word (.docx) document, or find/replace edit an existing one's text.\n\n" +
		"## When to use\n\n" +
		"- Creating a new .docx from plain text — omit old_string; new_string becomes the document body (lines starting with \"#\".. \"######\" become headings, blank lines become paragraph breaks)\n" +
		"- Editing text in an existing .docx — provide old_string (the exact text to find) and new_string (its replacement), same as FileEdit\n\n" +
		"## When NOT to use\n\n" +
		"- Reading .docx content — use FileRead instead (extracts it natively, no need to write anything first)\n" +
		"- Complex formatting, images, or tables — this only handles plain paragraph/heading text\n\n" +
		"## Rules\n\n" +
		"- Same read-before-edit rule as FileEdit: an existing file must have been read with FileRead first.\n" +
		"- old_string must match a run of actual document text exactly (or closely — a fuzzy fallback with a diff kicks in if the exact match fails, same as FileEdit).\n" +
		"- Set replace_all if old_string appears more than once and you want every occurrence replaced.\n"
)

// Tool implements the docx_edit tool.
type Tool struct {
	workingDir       string
	filesystemPolicy *sandbox.FilesystemPolicy
}

// NewTool creates a new DOCX edit tool.
func NewTool(workingDir string) *Tool {
	return &Tool{
		workingDir:       workingDir,
		filesystemPolicy: sandbox.NewDefaultFilesystemPolicy(),
	}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Edit Word Document",
		SearchHint:  SearchHint,
		Description: ToolDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the .docx file to create or edit",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "Exact text to find and replace. Omit (or leave empty) to create a new document from new_string.",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "Replacement text (when editing), or the full document body (when creating a new file). Lines starting with \"#\" through \"######\" become headings.",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences of old_string instead of requiring it to be unique (default false)",
				},
			},
			"required": []string{"file_path", "new_string"},
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
	if !strings.EqualFold(filepath.Ext(filePath), ".docx") {
		return tool.NewErrorResult(fmt.Errorf("docx_edit only writes .docx files, got: %s", filePath)), nil
	}

	newString, ok := input.Parsed["new_string"].(string)
	if !ok {
		return tool.NewErrorResult(fmt.Errorf("new_string is required and must be a string")), nil
	}
	oldString, _ := input.Parsed["old_string"].(string)
	replaceAll, _ := input.Parsed["replace_all"].(bool)

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

	if oldString == "" && fileExists {
		return tool.NewErrorResult(fmt.Errorf("cannot create new file - file already exists: %s (provide old_string to edit it instead)", filePath)), nil
	}
	if oldString != "" && !fileExists {
		return tool.NewErrorResult(fmt.Errorf("file not found: %s", filePath)), nil
	}
	if oldString != "" {
		if _, wasRead := fileReadTool.GetLastReadState(absolutePath); !wasRead {
			return tool.NewErrorResult(fmt.Errorf("file has not been read yet. Read it first before editing it: %s", filePath)), nil
		}
	}

	if permissionCheck != nil {
		req := sandbox.PermissionRequest{
			ToolName:      ToolName,
			Environment:   sandbox.EnvironmentLocal,
			Access:        sandbox.AccessWrite,
			Paths:         []string{absolutePath},
			Justification: "Write Word document contents",
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
		if err := sandbox.ErrorForPermissionResult(permResult, "document edit requires approval"); err != nil {
			return tool.NewErrorResult(err), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(absolutePath), 0755); err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to create parent directories: %w", err)), nil
	}

	var operationType string
	var replacements int

	if oldString == "" {
		docBytes, err := createMinimalDocx(newString)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to build document: %w", err)), nil
		}
		if err := os.WriteFile(absolutePath, docBytes, 0644); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to write %s: %w", filePath, err)), nil
		}
		operationType = "create"
		replacements = 1
	} else {
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to read %s: %w", filePath, err)), nil
		}
		documentXML, err := readDocumentXML(data)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		pretty := prettyPrintXML(documentXML)

		actualOld := editTool.FindActualString(pretty, oldString)
		if actualOld == "" {
			// docx's own findFuzzyMatch (not edit_file's) scores similarity on
			// tag-stripped text: prettyPrintXML keeps an inline element like
			// <w:t>some text</w:t> on one line, so comparing the raw line
			// against a bare-text old_string would dilute the score with
			// ~20 characters of tag markup around a handful of real
			// differences. The diff shown is on stripped text too, for the
			// same reason - readable, not noisy with tag insertions.
			if match, ok := findFuzzyMatch(pretty, oldString); ok {
				return tool.NewErrorResult(fmt.Errorf(
					"old_string not found in document: %s\n\nClosest match in the document text (%.0f%% similar):\n%s\n\nDiff vs your old_string ({-in your old_string, not in the document-} / {+in the document, not in your old_string+}):\n%s\n\nNote: to target this for a replacement, old_string must match the underlying XML span (including its tags) - re-read the document (offset > 0 shows raw XML) to copy it exactly, or provide more surrounding context.",
					filePath, match.Similarity*100, strings.TrimSpace(stripTags(match.Text)), editTool.CharDiff(oldString, stripTags(match.Text)),
				)), nil
			}
			return tool.NewErrorResult(fmt.Errorf("old_string not found in document: %s", filePath)), nil
		}

		occurrences := strings.Count(pretty, actualOld)
		if occurrences > 1 && !replaceAll {
			return tool.NewErrorResult(fmt.Errorf("found %d matches of old_string in the document. Set replace_all=true or provide more context to make old_string unique: %s", occurrences, filePath)), nil
		}

		var editedPretty string
		if replaceAll {
			editedPretty = strings.ReplaceAll(pretty, actualOld, newString)
			replacements = occurrences
		} else {
			idx := strings.Index(pretty, actualOld)
			editedPretty = pretty[:idx] + newString + pretty[idx+len(actualOld):]
			replacements = 1
		}

		newDocumentXML := compactXML(editedPretty)
		newData, err := replaceDocumentXML(data, newDocumentXML)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to repack document: %w", err)), nil
		}
		if err := os.WriteFile(absolutePath, newData, 0644); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to write %s: %w", filePath, err)), nil
		}
		operationType = "update"
	}

	if info, statErr := os.Stat(absolutePath); statErr == nil {
		fileReadTool.RecordExternalRead(absolutePath, info.ModTime(), "", true)
	}

	output := map[string]any{
		"file_path":    filePath,
		"success":      true,
		"type":         operationType,
		"replacements": replacements,
	}
	return tool.NewJSONResult(output), nil
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
	if _, ok := input["new_string"].(string); !ok {
		return nil, fmt.Errorf("new_string is required and must be a string")
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
