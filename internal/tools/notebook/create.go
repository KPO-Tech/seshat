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
	CreateDescription = `Create a new Jupyter notebook (.ipynb) file.

Fails if the file already exists — use notebook_write to create or overwrite.
Parent directories are created automatically.

Parameters:
- notebook_path: Absolute path to the new .ipynb file (required)
- kernel:        Jupyter kernel name, e.g. python3, ir (default: python3)
- language:      Programming language, e.g. python, r (default: python)
- cells:         Optional initial cells: [{cell_type: "code"|"markdown", source: "..."}]`
)

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
			return &validationError{msg: fmt.Sprintf("cells[%d].cell_type must be 'code' or 'markdown'", idx)}
		}
	}
	return nil
}

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
		Description: CreateDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{"type": "string", "description": "Absolute path to the new .ipynb file. Fails if already exists."},
				"kernel":        map[string]any{"type": "string", "description": "Jupyter kernel name (e.g. python3, ir). Default: python3."},
				"language":      map[string]any{"type": "string", "description": "Programming language (e.g. python, r). Default: python."},
				"cells": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"cell_type": map[string]any{"type": "string", "enum": []string{"code", "markdown"}}, "source": map[string]any{"type": "string"}},
						"required":   []string{"cell_type", "source"},
					},
					"description": "Optional initial cells.",
				},
			},
			"required": []string{"notebook_path"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
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
	fullPath, err := absNotebookPath(parsed.NotebookPath)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: CreateToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", orDefault(res.Reason, "notebook_create requires approval"))), nil
		}
	}
	out := runCreate(fullPath, parsed)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else {
		result.Content = fmt.Sprintf("Created %s (%s · %s · %d cells)", out.NotebookPath, out.Kernel, out.Language, out.CellCount)
	}
	return result, nil
}

func runCreate(fullPath string, input *createInput) createOutput {
	if _, err := os.Stat(fullPath); err == nil {
		return createOutput{Error: "file already exists — use notebook_write to overwrite", NotebookPath: fullPath}
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return createOutput{Error: fmt.Sprintf("create directory: %v", err), NotebookPath: fullPath}
	}
	kernel := orDefault(input.Kernel, defaultKernel)
	language := orDefault(input.Language, defaultLanguage)
	nb := buildNotebook(kernel, language, input.Cells)
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return createOutput{Error: fmt.Sprintf("serialize: %v", err), NotebookPath: fullPath}
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return createOutput{Error: fmt.Sprintf("write: %v", err), NotebookPath: fullPath}
	}
	return createOutput{NotebookPath: fullPath, Kernel: kernel, Language: language, CellCount: len(input.Cells)}
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
func (t *CreateTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *CreateTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── shared path/util helpers used by all tools in this package ───────────────

func absNotebookPath(p string) (string, error) {
	if p == "" {
		return "", &validationError{msg: "notebook_path is required"}
	}
	if !filepath.IsAbs(p) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		p = filepath.Join(cwd, p)
	}
	if !strings.HasSuffix(strings.ToLower(p), ".ipynb") {
		return "", &validationError{msg: "notebook_path must end with .ipynb"}
	}
	return p, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
