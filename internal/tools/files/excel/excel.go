// Package excel provides a structured cell-range editor for XLSX
// spreadsheets, the write-side counterpart to internal/officetext's native
// XLSX read support. Text-oriented tools (write_file/edit_file) don't apply
// here - a spreadsheet is a grid of typed cells and formulas, not a string
// to append/replace - so this is a separate tool with its own input shape,
// modeled on DesktopCommanderMCP's ExcelFileHandler.editRange (write a 2D
// block of values starting at a given cell, formulas included) rather than
// bolted onto an existing text-editing tool.
package excel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/KPO-Tech/seshat-runtime/internal/sandbox"
	fileReadTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/read"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/files/shared"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const (
	// ToolName is the name of the Excel edit tool.
	ToolName = "excel_edit"

	// SearchHint is a hint for tool search functionality.
	SearchHint = "write or edit cells in an Excel (.xlsx) spreadsheet"

	// ToolDescription is the description of the excel_edit tool.
	ToolDescription = "Write values (and formulas) into an Excel (.xlsx) spreadsheet, creating the file or sheet if they don't exist yet.\n\n" +
		"## When to use\n\n" +
		"- Creating a new .xlsx file with data\n" +
		"- Updating specific cells or a block of cells in an existing spreadsheet\n" +
		"- Writing formulas (any value starting with \"=\" is set as a formula, e.g. \"=SUM(A1:A10)\")\n\n" +
		"## When NOT to use\n\n" +
		"- Reading spreadsheet content — use FileRead instead (it extracts .xlsx natively, no need to write anything first)\n" +
		"- Plain CSV/text data — use FileWrite\n\n" +
		"## Rules\n\n" +
		"- values is a 2D array (rows of cells): [[\"Name\", \"Score\"], [\"Alice\", 90], [\"Bob\", 85]]\n" +
		"- range is the top-left cell to start writing at (e.g. \"B2\"); values fills out from there row by row, column by column. Omit it to start at A1.\n" +
		"- sheet defaults to the workbook's first sheet; naming a sheet that doesn't exist yet creates it.\n" +
		"- If the file already exists, you must read it with FileRead first (same rule as FileWrite) — this tool errors otherwise, since a spreadsheet write can silently clobber other cells' formulas/formatting if made blind.\n"
)

// Tool implements the excel_edit tool.
type Tool struct {
	workingDir       string
	filesystemPolicy *sandbox.FilesystemPolicy
}

// NewTool creates a new Excel edit tool.
func NewTool(workingDir string) *Tool {
	return &Tool{
		workingDir:       workingDir,
		filesystemPolicy: sandbox.NewDefaultFilesystemPolicy(),
	}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Edit Excel Spreadsheet",
		SearchHint:  SearchHint,
		Description: ToolDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the .xlsx file to write or edit",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Sheet name. Defaults to the workbook's first sheet (or \"Sheet1\" for a new file). Created automatically if it doesn't exist.",
				},
				"range": map[string]any{
					"type":        "string",
					"description": "Top-left cell to start writing at, e.g. \"B2\". Omit to start at A1.",
				},
				"values": map[string]any{
					"type":        "array",
					"description": "2D array of rows: each row is an array of cell values (string, number, boolean, or a formula string starting with \"=\").",
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{},
					},
				},
			},
			"required": []string{"file_path", "values"},
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
	if !strings.EqualFold(filepath.Ext(filePath), ".xlsx") {
		return tool.NewErrorResult(fmt.Errorf("excel_edit only writes .xlsx files, got: %s", filePath)), nil
	}

	rows, err := parseValues(input.Parsed["values"])
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if len(rows) == 0 {
		return tool.NewErrorResult(fmt.Errorf("values must contain at least one row")), nil
	}

	sheet, _ := input.Parsed["sheet"].(string)
	rangeStart, _ := input.Parsed["range"].(string)
	startCol, startRow, err := parseRangeStart(rangeStart)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	absolutePath, err := t.resolvePath(filePath, input.ToolContextValue())
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := t.validateWritePath(input.ToolContextValue(), absolutePath); err != nil {
		return tool.NewErrorResult(fmt.Errorf("path validation failed: %w", err)), nil
	}
	if err := shared.ValidateFilePath(absolutePath, "writing"); err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := shared.ValidateUNCPathSecurity(absolutePath); err != nil {
		return tool.NewErrorResult(err), nil
	}

	if info, statErr := os.Stat(absolutePath); statErr == nil {
		if info.IsDir() {
			return tool.NewErrorResult(fmt.Errorf("path is a directory, not a file: %s", filePath)), nil
		}
		// Same read-before-write discipline as write_file/edit_file: for an
		// existing file, require it to have been read this session first.
		// There's no cheap content-staleness diff for a binary spreadsheet
		// (unlike text files), so this read-gate is the safety net instead.
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
			Justification: "Write Excel spreadsheet cells",
			Scope:         sandbox.ApprovalScopeToolCall,
			Metadata: map[string]any{
				"sheet": sheet,
				"range": rangeStart,
				"rows":  len(rows),
				"cells": countCells(rows),
			},
		}
		toolCtx := input.ToolContextValue()
		permResult, err := sandbox.ResolveToolPermission(ctx, permissionCheck, req, sandbox.ToolPermissionOptions{
			ToolInput: map[string]any{
				"file_path": absolutePath,
				"sheet":     sheet,
				"range":     rangeStart,
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
		if err := sandbox.ErrorForPermissionResult(permResult, "spreadsheet edit requires approval"); err != nil {
			return tool.NewErrorResult(err), nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(absolutePath), 0755); err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to create parent directories: %w", err)), nil
	}

	f, fileExists, err := openOrCreate(absolutePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to open %s: %w", filePath, err)), nil
	}
	defer f.Close()

	sheetName, err := ensureSheet(f, sheet)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	cellsWritten, err := writeRows(f, sheetName, startCol, startRow, rows)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to write cells: %w", err)), nil
	}

	if err := f.SaveAs(absolutePath); err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to save %s: %w", filePath, err)), nil
	}

	// Mark as read post-write (same convention as write_file), so a
	// follow-up edit in the same session doesn't need a fresh FileRead call.
	if info, statErr := os.Stat(absolutePath); statErr == nil {
		fileReadTool.RecordExternalRead(absolutePath, info.ModTime(), "", true)
	}

	operationType := "update"
	if !fileExists {
		operationType = "create"
	}

	output := map[string]any{
		"file_path":     filePath,
		"sheet":         sheetName,
		"success":       true,
		"type":          operationType,
		"rows_written":  len(rows),
		"cells_written": cellsWritten,
	}
	return tool.NewJSONResult(output), nil
}

// parseValues converts the raw "values" parameter (JSON-decoded as
// []any of []any) into a typed 2D grid.
func parseValues(raw any) ([][]any, error) {
	outer, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("values is required and must be a 2D array")
	}
	rows := make([][]any, 0, len(outer))
	for i, r := range outer {
		row, ok := r.([]any)
		if !ok {
			return nil, fmt.Errorf("values[%d] must be an array of cell values", i)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func countCells(rows [][]any) int {
	n := 0
	for _, r := range rows {
		n += len(r)
	}
	return n
}

// parseRangeStart resolves the top-left cell of range to 1-indexed
// (col, row) coordinates. An empty range defaults to A1. Only the part
// before ":" is used when a full range (e.g. "A1:C3") is given - values
// determines the actual extent, the end of an explicit range is not a hard
// bound.
func parseRangeStart(rangeStr string) (col, row int, err error) {
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return 1, 1, nil
	}
	start := rangeStr
	if idx := strings.Index(rangeStr, ":"); idx >= 0 {
		start = rangeStr[:idx]
	}
	col, row, err = excelize.CellNameToCoordinates(start)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %w", rangeStr, err)
	}
	return col, row, nil
}

// openOrCreate opens an existing workbook or starts a new in-memory one.
func openOrCreate(path string) (f *excelize.File, existed bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		f, err = excelize.OpenFile(path)
		return f, true, err
	}
	return excelize.NewFile(), false, nil
}

// ensureSheet returns the sheet to write to, creating it (and setting it
// active) if it doesn't exist yet. An empty name means "use the workbook's
// current first sheet" (its default for both a fresh and an opened file).
func ensureSheet(f *excelize.File, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		list := f.GetSheetList()
		if len(list) == 0 {
			return "", fmt.Errorf("workbook has no sheets")
		}
		return list[0], nil
	}
	if idx, err := f.GetSheetIndex(name); err == nil && idx != -1 {
		return name, nil
	}
	if _, err := f.NewSheet(name); err != nil {
		return "", fmt.Errorf("failed to create sheet %q: %w", name, err)
	}
	return name, nil
}

// writeRows writes a 2D grid of values starting at (startCol, startRow),
// treating any string value beginning with "=" as a formula.
func writeRows(f *excelize.File, sheet string, startCol, startRow int, rows [][]any) (int, error) {
	written := 0
	for i, row := range rows {
		for j, value := range row {
			cell, err := excelize.CoordinatesToCellName(startCol+j, startRow+i)
			if err != nil {
				return written, err
			}
			if s, ok := value.(string); ok && strings.HasPrefix(s, "=") {
				if err := f.SetCellFormula(sheet, cell, s); err != nil {
					return written, fmt.Errorf("set formula at %s: %w", cell, err)
				}
			} else {
				if err := f.SetCellValue(sheet, cell, value); err != nil {
					return written, fmt.Errorf("set value at %s: %w", cell, err)
				}
			}
			written++
		}
	}
	return written, nil
}

// ── Tool interface plumbing (mirrors write_file's conventions) ────────────────

func (t *Tool) Description(_ context.Context) (string, error) {
	return ToolDescription, nil
}

func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file_path is required and must be a string")
	}
	if _, ok := input["values"].([]any); !ok {
		return nil, fmt.Errorf("values is required and must be an array")
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
