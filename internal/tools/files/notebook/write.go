package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	WriteToolName    = "notebook_write"
	WriteDisplayName = "Write Notebook"
	WriteSearchHint  = "write or overwrite an entire Jupyter notebook (.ipynb)"
	WriteDescription = `Write an entire Jupyter notebook in one shot — creates or fully replaces the file.

Use this to write a complete notebook at once rather than editing cell by cell.
Parent directories are created automatically.

Parameters:
- notebook_path: Absolute path to the .ipynb file (required)
- cells:         Array of cells to write (required): [{cell_type: "code"|"markdown", source: "..."}]
- kernel:        Jupyter kernel name (e.g. python3, ir). Default: python3
- language:      Programming language (e.g. python, r). Default: python

Example:

  notebook_path: /home/user/analysis.ipynb
  kernel: python3
  language: python
  cells:
    - cell_type: markdown
      source: "# Data Analysis"
    - cell_type: code
      source: "import pandas as pd\ndf = pd.read_csv('data.csv')\ndf.head()"
    - cell_type: code
      source: "df.describe()"`
)

// writeInput holds the parameters for notebook_write.
type writeInput struct {
	NotebookPath string     `json:"notebook_path"`
	Cells        []CellSpec `json:"cells"`
	Kernel       string     `json:"kernel,omitempty"`
	Language     string     `json:"language,omitempty"`
}

func (i *writeInput) validate() error {
	if i.NotebookPath == "" {
		return &validationError{msg: "notebook_path is required"}
	}
	if len(i.Cells) == 0 {
		return &validationError{msg: "cells must not be empty"}
	}
	for idx, c := range i.Cells {
		if c.CellType != "code" && c.CellType != "markdown" {
			return &validationError{
				msg: fmt.Sprintf("cells[%d].cell_type must be 'code' or 'markdown', got %q", idx, c.CellType),
			}
		}
	}
	return nil
}

// writeOutput is the result returned to the model.
type writeOutput struct {
	NotebookPath  string `json:"notebook_path"`
	Kernel        string `json:"kernel"`
	Language      string `json:"language"`
	CellCount     int    `json:"cell_count"`
	CodeCells     int    `json:"code_cells"`
	MarkdownCells int    `json:"markdown_cells"`
	Overwritten   bool   `json:"overwritten"`
	Error         string `json:"error,omitempty"`
}

// WriteTool implements notebook_write.
type WriteTool struct{}

func NewWriteTool() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        WriteToolName,
		DisplayName: WriteDisplayName,
		SearchHint:  WriteSearchHint,
		Description: WriteDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the .ipynb file. Created or overwritten.",
				},
				"cells": map[string]any{
					"type":        "array",
					"description": "Cells to write.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"cell_type": map[string]any{"type": "string", "enum": []string{"code", "markdown"}},
							"source":    map[string]any{"type": "string"},
						},
						"required": []string{"cell_type", "source"},
					},
				},
				"kernel": map[string]any{
					"type":        "string",
					"description": "Jupyter kernel name (e.g. python3, ir). Default: python3.",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language (e.g. python, r). Default: python.",
				},
			},
			"required": []string{"notebook_path", "cells"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *WriteTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseWriteInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsed.validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}

	fullPath := parsed.NotebookPath
	if !filepath.IsAbs(fullPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("resolve working directory: %w", err)), nil
		}
		fullPath = filepath.Join(cwd, fullPath)
	}
	if !strings.HasSuffix(strings.ToLower(fullPath), ".ipynb") {
		return tool.NewErrorResult(fmt.Errorf("notebook_path must end with .ipynb")), nil
	}

	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: WriteToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			reason := res.Reason
			if reason == "" {
				reason = "notebook_write requires approval"
			}
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
		}
	}

	out := writeNotebook(fullPath, parsed)
	result := tool.NewJSONResult(out)
	result.Content = formatWriteOutput(out)
	result.Metadata = &tool.ResultMetadata{Additional: writeResultMetadata(out)}
	return result, nil
}

func writeNotebook(fullPath string, input *writeInput) writeOutput {
	_, overwritten := os.Stat(fullPath)
	alreadyExists := overwritten == nil

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return writeOutput{Error: fmt.Sprintf("create directory: %v", err), NotebookPath: fullPath}
	}

	kernel := input.Kernel
	if kernel == "" {
		kernel = defaultKernel
	}
	language := input.Language
	if language == "" {
		language = defaultLanguage
	}

	nb := buildNotebook(kernel, language, input.Cells)
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return writeOutput{Error: fmt.Sprintf("serialize notebook: %v", err), NotebookPath: fullPath}
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return writeOutput{Error: fmt.Sprintf("write file: %v", err), NotebookPath: fullPath}
	}

	code, md := 0, 0
	for _, c := range input.Cells {
		switch c.CellType {
		case "code":
			code++
		case "markdown":
			md++
		}
	}

	return writeOutput{
		NotebookPath:  fullPath,
		Kernel:        kernel,
		Language:      language,
		CellCount:     len(input.Cells),
		CodeCells:     code,
		MarkdownCells: md,
		Overwritten:   alreadyExists,
	}
}

func formatWriteOutput(out writeOutput) string {
	if out.Error != "" {
		return "Error: " + out.Error
	}
	action := "Created"
	if out.Overwritten {
		action = "Overwrote"
	}
	return fmt.Sprintf("%s %s (%s · %d cells: %d code, %d markdown)",
		action, out.NotebookPath, out.Kernel,
		out.CellCount, out.CodeCells, out.MarkdownCells)
}

func writeResultMetadata(out writeOutput) map[string]any {
	meta := map[string]any{
		"notebook_path":  out.NotebookPath,
		"kernel":         out.Kernel,
		"language":       out.Language,
		"cell_count":     out.CellCount,
		"code_cells":     out.CodeCells,
		"markdown_cells": out.MarkdownCells,
		"overwritten":    out.Overwritten,
	}
	if out.Error != "" {
		meta["error"] = out.Error
	}
	return meta
}

func parseWriteInput(raw map[string]any) (*writeInput, error) {
	in := &writeInput{}
	if v, ok := raw["notebook_path"].(string); ok {
		in.NotebookPath = v
	}
	if v, ok := raw["kernel"].(string); ok {
		in.Kernel = v
	}
	if v, ok := raw["language"].(string); ok {
		in.Language = v
	}
	if rawCells, ok := raw["cells"].([]any); ok {
		in.Cells = parseCellSpecs(rawCells)
	}
	return in, nil
}

func (t *WriteTool) Description(_ context.Context) (string, error) { return WriteDescription, nil }
func (t *WriteTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseWriteInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *WriteTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *WriteTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *WriteTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *WriteTool) IsEnabled() bool                         { return true }
func (t *WriteTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
func (t *WriteTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
