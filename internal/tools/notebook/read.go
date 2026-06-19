package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	ReadToolName    = "notebook_read"
	ReadDisplayName = "Read Notebook"
	ReadDescription = `Read and display the contents of a Jupyter notebook (.ipynb) file.

Returns all cells with their types, sources, and execution outputs.
Optionally filter to specific cell indices with the cells parameter.

Parameters:
- notebook_path: Absolute path to the .ipynb file (required)
- cells:         Optional list of 0-based cell indices to include (default: all cells)
- include_outputs: Whether to include cell execution outputs (default: true)`
)

type readInput struct {
	NotebookPath   string `json:"notebook_path"`
	Cells          []int  `json:"cells,omitempty"`
	IncludeOutputs *bool  `json:"include_outputs,omitempty"`
}

func (i *readInput) validate() error {
	if i.NotebookPath == "" {
		return &validationError{msg: "notebook_path is required"}
	}
	return nil
}

func (i *readInput) includeOutputs() bool {
	if i.IncludeOutputs == nil {
		return true
	}
	return *i.IncludeOutputs
}

type readCellResult struct {
	Index          int            `json:"index"`
	ID             string         `json:"id,omitempty"`
	CellType       string         `json:"cell_type"`
	Source         string         `json:"source"`
	ExecutionCount *int           `json:"execution_count,omitempty"`
	Outputs        []outputResult `json:"outputs,omitempty"`
}

type outputResult struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type readOutput struct {
	NotebookPath  string           `json:"notebook_path"`
	Kernel        string           `json:"kernel"`
	Language      string           `json:"language"`
	NBFormat      int              `json:"nbformat"`
	TotalCells    int              `json:"total_cells"`
	FilteredCells int              `json:"filtered_cells"`
	Cells         []readCellResult `json:"cells"`
	Error         string           `json:"error,omitempty"`
}

// ReadTool implements notebook_read.
type ReadTool struct{}

func NewReadTool() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ReadToolName,
		DisplayName: ReadDisplayName,
		Description: ReadDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{"type": "string", "description": "Absolute path to the .ipynb file."},
				"cells": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer"},
					"description": "0-based indices of cells to return (default: all).",
				},
				"include_outputs": map[string]any{"type": "boolean", "description": "Include execution outputs (default: true)."},
			},
			"required": []string{"notebook_path"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *ReadTool) Call(_ context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseReadInput(input.Parsed)
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
	out := runRead(fullPath, parsed)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else {
		result.Content = formatReadOutput(out)
	}
	return result, nil
}

func runRead(fullPath string, input *readInput) readOutput {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return readOutput{Error: fmt.Sprintf("read file: %v", err), NotebookPath: fullPath}
	}
	var nb read.Notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return readOutput{Error: "not a valid notebook JSON", NotebookPath: fullPath}
	}

	kernel := defaultKernel
	language := defaultLanguage
	if nb.Metadata.Kernelspec != nil {
		if nb.Metadata.Kernelspec.Name != "" {
			kernel = nb.Metadata.Kernelspec.Name
		}
		if nb.Metadata.Kernelspec.Language != "" {
			language = nb.Metadata.Kernelspec.Language
		}
	}
	if nb.Metadata.LanguageInfo != nil && nb.Metadata.LanguageInfo.Name != "" {
		language = nb.Metadata.LanguageInfo.Name
	}

	wantIdx := buildIndexSet(input.Cells, len(nb.Cells))
	withOutputs := input.includeOutputs()

	var cells []readCellResult
	for i, c := range nb.Cells {
		if !wantIdx[i] {
			continue
		}
		cr := readCellResult{
			Index:          i,
			ID:             c.ID,
			CellType:       c.CellType,
			Source:         cellSource(c),
			ExecutionCount: c.ExecutionCount,
		}
		if withOutputs && c.CellType == "code" {
			for _, o := range c.Outputs {
				cr.Outputs = append(cr.Outputs, convertOutput(o))
			}
		}
		cells = append(cells, cr)
	}

	return readOutput{
		NotebookPath:  fullPath,
		Kernel:        kernel,
		Language:      language,
		NBFormat:      nb.NBFormat,
		TotalCells:    len(nb.Cells),
		FilteredCells: len(cells),
		Cells:         cells,
	}
}

func buildIndexSet(filter []int, total int) map[int]bool {
	if len(filter) == 0 {
		m := make(map[int]bool, total)
		for i := range total {
			m[i] = true
		}
		return m
	}
	m := make(map[int]bool, len(filter))
	for _, idx := range filter {
		if idx >= 0 && idx < total {
			m[idx] = true
		}
	}
	return m
}

func convertOutput(o read.NotebookOutput) outputResult {
	or_ := outputResult{Type: o.OutputType}
	if len(o.Text) > 0 {
		or_.Text = strings.Join(o.Text, "")
	} else if o.Data != nil {
		if plain, ok := o.Data["text/plain"].(string); ok {
			or_.Text = plain
		}
	}
	return or_
}

func formatReadOutput(out readOutput) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Notebook: %s\nKernel: %s · Language: %s · nbformat %d\n%d cells",
		out.NotebookPath, out.Kernel, out.Language, out.NBFormat, out.TotalCells))
	if out.FilteredCells != out.TotalCells {
		sb.WriteString(fmt.Sprintf(" (showing %d)", out.FilteredCells))
	}
	sb.WriteString("\n\n")
	for _, c := range out.Cells {
		sb.WriteString(fmt.Sprintf("[%d] %s", c.Index, c.CellType))
		if c.ID != "" {
			sb.WriteString(" id=" + c.ID)
		}
		sb.WriteString("\n")
		sb.WriteString(c.Source)
		sb.WriteString("\n")
		if len(c.Outputs) > 0 {
			sb.WriteString("--- outputs ---\n")
			for _, o := range c.Outputs {
				if o.Name != "" {
					sb.WriteString("[" + o.Name + "] ")
				}
				sb.WriteString(o.Text)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func parseReadInput(raw map[string]any) (*readInput, error) {
	in := &readInput{}
	if v, ok := raw["notebook_path"].(string); ok {
		in.NotebookPath = v
	}
	if v, ok := raw["include_outputs"].(bool); ok {
		in.IncludeOutputs = &v
	}
	if rawCells, ok := raw["cells"].([]any); ok {
		for _, rc := range rawCells {
			switch v := rc.(type) {
			case float64:
				in.Cells = append(in.Cells, int(v))
			case int:
				in.Cells = append(in.Cells, v)
			}
		}
	}
	return in, nil
}

func (t *ReadTool) Description(_ context.Context) (string, error) { return ReadDescription, nil }
func (t *ReadTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseReadInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *ReadTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *ReadTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *ReadTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *ReadTool) IsEnabled() bool                         { return true }
func (t *ReadTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *ReadTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
