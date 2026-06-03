package notebookedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileReadTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const Description = `Replace the contents of a specific cell in a Jupyter notebook.

Completely replaces the contents of a specific cell in a Jupyter notebook (.ipynb file) with new source.
Jupyter notebooks are interactive documents that combine code, text, and visualizations, commonly used for data analysis and scientific computing.

The notebook_path parameter must be an absolute path, not a relative path.
The cell_number is 0-indexed.

Usage Examples:

Example 1 - Replace a cell:
  notebook_path: /home/user/analysis.ipynb
  edit_mode: replace
  cell_number: 0
  source: "import pandas as pd\ndf = pd.read_csv('data.csv')"

Example 2 - Insert a new cell:
  notebook_path: /home/user/analysis.ipynb
  edit_mode: insert
  cell_number: 2
  source: "print('New cell inserted here')"

Example 3 - Delete a cell:
  notebook_path: /home/user/analysis.ipynb
  edit_mode: delete
  cell_number: 5

Parameters:
- notebook_path: Absolute path to the .ipynb file (required)
- edit_mode: Operation mode - "replace", "insert", or "delete" (required)
- cell_number: Index of the cell (0-indexed, required)
- source: New cell content (required for replace/insert)`

// Tool implements the notebook edit tool.
// This tool allows editing Jupyter notebook (.ipynb) cells with support for
// replace, insert, and delete operations.
type Tool struct{}

// NewTool creates a new notebook edit tool instance.
func NewTool() *Tool {
	return &Tool{}
}

// Definition returns the tool definition including name, description, and input schema.
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
				"notebook_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the Jupyter notebook file to edit (must be absolute, not relative)",
				},
				"cell_id": map[string]any{
					"type":        "string",
					"description": "The ID of the cell to edit. When inserting a new cell, the new cell will be inserted after the cell with this ID, or at the beginning if not specified.",
				},
				"new_source": map[string]any{
					"type":        "string",
					"description": "The new source for the cell",
				},
				"cell_type": map[string]any{
					"type":        "string",
					"description": "The type of the cell (code or markdown). If not specified, it defaults to the current cell type. If using edit_mode=insert, this is required.",
					"enum":        []string{"code", "markdown"},
				},
				"edit_mode": map[string]any{
					"type":        "string",
					"description": "The type of edit to make (replace, insert, delete). Defaults to replace.",
					"enum":        []string{"replace", "insert", "delete"},
				},
			},
			"required": []string{"notebook_path", "new_source"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call executes the notebook edit operation.
// It validates input, checks permissions, and performs the edit.
func (t *Tool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsedInput, err := parseCallInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsedInput.Validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}

	fullPath := parsedInput.NotebookPath
	if !filepath.IsAbs(fullPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to get current working directory: %w", err)), nil
		}
		fullPath = filepath.Join(cwd, fullPath)
	}

	if !strings.HasSuffix(strings.ToLower(fullPath), ".ipynb") {
		return tool.NewErrorResult(fmt.Errorf("file must be a Jupyter notebook (.ipynb file). For editing other file types, use the edit_file tool")), nil
	}

	if permissionCheck != nil {
		permResult := permissionCheck(ctx, types.ToolPermissionRequest{
			ToolName:  ToolName,
			ToolInput: input.Parsed,
		})
		if permResult.Behavior != types.PermissionBehaviorAllow {
			reason := permResult.Reason
			if reason == "" {
				reason = "NotebookEdit requires approval"
			}
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
		}
	}

	result, err := t.editNotebook(fullPath, parsedInput)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	output := tool.NewJSONResult(result)
	output.Content = formatOutput(result)
	return output, nil
}

// editNotebook performs the actual notebook editing operation.
// It handles replace, insert, and delete modes for notebook cells.
func (t *Tool) editNotebook(fullPath string, input *Input) (Output, error) {
	originalContent, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Output{Error: "Notebook file does not exist.", NotebookPath: fullPath}, nil
		}
		return Output{Error: fmt.Sprintf("Failed to read notebook: %v", err), NotebookPath: fullPath}, nil
	}

	var notebook fileReadTool.Notebook
	if err := json.Unmarshal(originalContent, &notebook); err != nil {
		return Output{Error: "Notebook is not valid JSON.", NotebookPath: fullPath}, nil
	}

	cellIndex := -1
	if input.CellID != "" {
		cellIndex = findCellIndex(&notebook, input.CellID)
		if cellIndex == -1 {
			parsedIndex := parseCellId(input.CellID)
			if parsedIndex != nil && *parsedIndex >= 0 && *parsedIndex < len(notebook.Cells) {
				cellIndex = *parsedIndex
			} else {
				return Output{Error: fmt.Sprintf("Cell with ID \"%s\" not found in notebook", input.CellID), NotebookPath: fullPath}, nil
			}
		}
	}

	editMode := input.EditMode
	if editMode == "" {
		editMode = EditModeReplace
	}

	cellType := input.CellType
	if cellType == "" {
		cellType = "code"
	}

	language := "python"
	if notebook.Metadata.LanguageInfo != nil {
		language = notebook.Metadata.LanguageInfo.Name
	}

	nbFormat := notebook.NBFormat
	nbFormatMinor := notebook.NBFormatMinor
	newCellID := ""

	if editMode == EditModeDelete {
		if cellIndex < 0 || cellIndex >= len(notebook.Cells) {
			return Output{Error: "Cannot delete: cell not found", NotebookPath: fullPath}, nil
		}
		copy(notebook.Cells[cellIndex:], notebook.Cells[cellIndex+1:])
		notebook.Cells = notebook.Cells[:len(notebook.Cells)-1]
	} else if editMode == EditModeInsert {
		newCell := createNewCell(cellType, input.NewSource)
		if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
			newCellID = generateCellID()
			newCell.ID = newCellID
		}

		insertIndex := cellIndex
		if insertIndex < 0 {
			insertIndex = 0
		}
		if insertIndex > len(notebook.Cells) {
			insertIndex = len(notebook.Cells)
		}
		notebook.Cells = append(notebook.Cells[:insertIndex], append([]fileReadTool.NotebookCell{newCell}, notebook.Cells[insertIndex:]...)...)
	} else {
		if cellIndex < 0 {
			if len(notebook.Cells) == 0 {
				newCell := createNewCell(cellType, input.NewSource)
				if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
					newCellID = generateCellID()
					newCell.ID = newCellID
				}
				notebook.Cells = append(notebook.Cells, newCell)
			} else {
				cellIndex = 0
			}
		}

		if cellIndex >= len(notebook.Cells) {
			newCell := createNewCell(cellType, input.NewSource)
			if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
				newCellID = generateCellID()
				newCell.ID = newCellID
			}
			notebook.Cells = append(notebook.Cells, newCell)
		} else {
			targetCell := &notebook.Cells[cellIndex]
			targetCell.Source = []string{input.NewSource}
			if cellType != "" && cellType != targetCell.CellType {
				targetCell.CellType = cellType
			}
			if targetCell.CellType == "code" {
				targetCell.ExecutionCount = nil
				targetCell.Outputs = []fileReadTool.NotebookOutput{}
			}
			if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
				newCellID = input.CellID
			}
		}
	}

	updatedContent, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return Output{Error: fmt.Sprintf("Failed to serialize notebook: %v", err), NotebookPath: fullPath}, nil
	}

	if err := os.WriteFile(fullPath, updatedContent, 0644); err != nil {
		return Output{Error: fmt.Sprintf("Failed to write notebook: %v", err), NotebookPath: fullPath}, nil
	}

	return Output{
		NewSource:    input.NewSource,
		CellID:       newCellID,
		CellType:     cellType,
		Language:     language,
		EditMode:     string(editMode),
		NotebookPath: fullPath,
		OriginalFile: string(originalContent),
		UpdatedFile:  string(updatedContent),
	}, nil
}

// createNewCell creates a new notebook cell with the specified type and source.
func createNewCell(cellType, source string) fileReadTool.NotebookCell {
	cell := fileReadTool.NotebookCell{
		CellType: cellType,
		Source:   []string{source},
		Metadata: json.RawMessage{},
	}
	if cellType == "code" {
		cell.ExecutionCount = nil
		cell.Outputs = []fileReadTool.NotebookOutput{}
	}
	return cell
}

// findCellIndex searches for a cell by its ID and returns its index.
func findCellIndex(notebook *fileReadTool.Notebook, cellID string) int {
	for i, cell := range notebook.Cells {
		if cell.ID == cellID {
			return i
		}
	}
	return -1
}

// parseCellId parses a cell ID string (e.g., "cell-0" or "0") into an index.
func parseCellId(cellID string) *int {
	var idx int
	_, err := fmt.Sscanf(cellID, "cell-%d", &idx)
	if err != nil {
		_, err = fmt.Sscanf(cellID, "%d", &idx)
		if err != nil {
			return nil
		}
	}
	return &idx
}

// generateCellID generates a unique cell ID based on current timestamp.
func generateCellID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())[:12]
}

// Description returns the tool description for the model.
func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}

// ValidateInput validates and normalizes tool input before execution.
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	parsedInput, err := parseCallInput(input)
	if err != nil {
		return nil, err
	}
	if err := parsedInput.Validate(); err != nil {
		return nil, err
	}
	return input, nil
}

// CheckPermissions performs tool-specific permission checks.
// Returns passthrough to let the global permission pipeline decide.
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	_ = ctx
	_ = toolCtx
	return types.Passthrough(input)
}

// IsConcurrencySafe reports whether this tool can run concurrently with other tools.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	_ = input
	return false
}

// IsReadOnly reports whether this tool modifies any state.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	_ = input
	return false
}

// IsEnabled reports whether this tool is currently active.
func (t *Tool) IsEnabled() bool {
	return true
}

// FormatResult serializes the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches a shallow clone of the parsed input with derived fields.
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// parseCallInput parses the raw input map into a structured Input object.
func parseCallInput(parsed map[string]any) (*Input, error) {
	input := &Input{
		NotebookPath: "",
		CellID:       "",
		NewSource:    "",
		CellType:     "",
		EditMode:     EditModeReplace,
	}

	if v, ok := parsed["notebook_path"].(string); ok {
		input.NotebookPath = v
	}
	if v, ok := parsed["cell_id"].(string); ok {
		input.CellID = v
	}
	if v, ok := parsed["new_source"].(string); ok {
		input.NewSource = v
	}
	if v, ok := parsed["cell_type"].(string); ok {
		input.CellType = v
	}
	if v, ok := parsed["edit_mode"].(string); ok {
		input.EditMode = EditMode(v)
	}

	return input, nil
}

// formatOutput formats the tool output for display in the tool result.
func formatOutput(output Output) string {
	var builder strings.Builder

	switch output.EditMode {
	case "replace":
		builder.WriteString(fmt.Sprintf("Updated cell %s with %s", output.CellID, output.NewSource))
	case "insert":
		builder.WriteString(fmt.Sprintf("Inserted cell %s with %s", output.CellID, output.NewSource))
	case "delete":
		builder.WriteString(fmt.Sprintf("Deleted cell %s", output.CellID))
	default:
		builder.WriteString("Unknown edit mode")
	}

	return builder.String()
}
