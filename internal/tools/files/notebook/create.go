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
	CreateToolName    = "notebook_create"
	CreateDisplayName = "Create Notebook"
	CreateSearchHint  = "create a new Jupyter notebook (.ipynb)"
	CreateDescription = `Create a new Jupyter notebook (.ipynb) file.

Fails if the file already exists — use notebook_write to create or overwrite.
Parent directories are created automatically.

Parameters:
- notebook_path: Absolute path to the new .ipynb file (required)
- kernel:        Jupyter kernel name, e.g. python3, ir (default: python3)
- language:      Programming language, e.g. python, r (default: python)
- cells:         Optional initial cells: [{cell_type: "code"|"markdown", source: "..."}]

Examples:

  Empty Python notebook:
    notebook_path: /home/user/analysis.ipynb

  With initial cells:
    notebook_path: /home/user/analysis.ipynb
    cells:
      - cell_type: markdown
        source: "# My Analysis"
      - cell_type: code
        source: "import pandas as pd"`
)

// createInput holds the parameters for notebook_create.
type createInput struct {
	NotebookPath string     `json:"notebook_path"`
	Kernel       string     `json:"kernel,omitempty"`
	Language     string     `json:"language,omitempty"`
	Cells        []CellSpec `json:"cells,omitempty"`
}

func (i *createInput) validate() error {
	if i.NotebookPath == "" {
		return &validationError{msg: "notebook_path is required"}
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

// createOutput is the result returned to the model.
type createOutput struct {
	NotebookPath string `json:"notebook_path"`
	Kernel       string `json:"kernel"`
	Language     string `json:"language"`
	CellCount    int    `json:"cell_count"`
	Error        string `json:"error,omitempty"`
}

// CreateTool implements notebook_create.
type CreateTool struct{}

func NewCreateTool() *CreateTool { return &CreateTool{} }

func (t *CreateTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        CreateToolName,
		DisplayName: CreateDisplayName,
		SearchHint:  CreateSearchHint,
		Description: CreateDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the new .ipynb file. Fails if the file already exists.",
				},
				"kernel": map[string]any{
					"type":        "string",
					"description": "Jupyter kernel name (e.g. python3, ir). Default: python3.",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Programming language (e.g. python, r, julia). Default: python.",
				},
				"cells": map[string]any{
					"type":        "array",
					"description": "Optional initial cells.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"cell_type": map[string]any{"type": "string", "enum": []string{"code", "markdown"}},
							"source":    map[string]any{"type": "string"},
						},
						"required": []string{"cell_type", "source"},
					},
				},
			},
			"required": []string{"notebook_path"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *CreateTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseCreateInput(input.Parsed)
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
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: CreateToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			reason := res.Reason
			if reason == "" {
				reason = "notebook_create requires approval"
			}
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
		}
	}

	out := createNotebook(fullPath, parsed)
	result := tool.NewJSONResult(out)
	result.Content = formatCreateOutput(out)
	result.Metadata = &tool.ResultMetadata{Additional: createResultMetadata(out)}
	return result, nil
}

func createNotebook(fullPath string, input *createInput) createOutput {
	if _, err := os.Stat(fullPath); err == nil {
		return createOutput{
			Error:        "file already exists — use notebook_write to overwrite",
			NotebookPath: fullPath,
		}
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return createOutput{Error: fmt.Sprintf("create directory: %v", err), NotebookPath: fullPath}
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
		return createOutput{Error: fmt.Sprintf("serialize notebook: %v", err), NotebookPath: fullPath}
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return createOutput{Error: fmt.Sprintf("write file: %v", err), NotebookPath: fullPath}
	}

	return createOutput{
		NotebookPath: fullPath,
		Kernel:       kernel,
		Language:     language,
		CellCount:    len(input.Cells),
	}
}

func formatCreateOutput(out createOutput) string {
	if out.Error != "" {
		return "Error: " + out.Error
	}
	return fmt.Sprintf("Created %s (%s · %s · %d cells)",
		out.NotebookPath, out.Kernel, out.Language, out.CellCount)
}

func createResultMetadata(out createOutput) map[string]any {
	meta := map[string]any{
		"notebook_path": out.NotebookPath,
		"kernel":        out.Kernel,
		"language":      out.Language,
		"cell_count":    out.CellCount,
	}
	if out.Error != "" {
		meta["error"] = out.Error
	}
	return meta
}

func parseCreateInput(raw map[string]any) (*createInput, error) {
	in := &createInput{}
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

func (t *CreateTool) Description(_ context.Context) (string, error) { return CreateDescription, nil }
func (t *CreateTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseCreateInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *CreateTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *CreateTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *CreateTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *CreateTool) IsEnabled() bool                         { return true }
func (t *CreateTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
func (t *CreateTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
