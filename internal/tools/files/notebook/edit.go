package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	read "github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	EditToolName    = "notebook_edit"
	EditDisplayName = "Edit Notebook"
	EditSearchHint  = "edit Jupyter notebook cells (.ipynb)"
	EditDescription = `Replace the contents of a specific cell in a Jupyter notebook.

Completely replaces the contents of a specific cell in a Jupyter notebook (.ipynb file) with new source.
The notebook_path parameter must be an absolute path.

Usage Examples:

  Replace a cell:
    notebook_path: /home/user/analysis.ipynb
    edit_mode: replace
    cell_id: cell-0
    new_source: "import pandas as pd\ndf = pd.read_csv('data.csv')"

  Insert a new cell:
    notebook_path: /home/user/analysis.ipynb
    edit_mode: insert
    cell_id: cell-2
    cell_type: code
    new_source: "print('New cell inserted here')"

  Delete a cell:
    notebook_path: /home/user/analysis.ipynb
    edit_mode: delete
    cell_id: cell-5`
)

// EditMode defines the type of edit operation.
type EditMode string

const (
	EditModeReplace EditMode = "replace"
	EditModeInsert  EditMode = "insert"
	EditModeDelete  EditMode = "delete"
)

// editInput defines the parameters for notebook_edit.
type editInput struct {
	NotebookPath string   `json:"notebook_path"`
	CellID       string   `json:"cell_id,omitempty"`
	NewSource    string   `json:"new_source"`
	CellType     string   `json:"cell_type,omitempty"`
	EditMode     EditMode `json:"edit_mode,omitempty"`
}

func (i *editInput) validate() error {
	if i.NotebookPath == "" {
		return &validationError{msg: "notebook_path is required"}
	}
	if i.EditMode == "" {
		i.EditMode = EditModeReplace
	}
	if (i.EditMode == EditModeReplace || i.EditMode == EditModeInsert) && i.NewSource == "" {
		return &validationError{msg: "new_source is required"}
	}
	if i.EditMode != EditModeReplace && i.EditMode != EditModeInsert && i.EditMode != EditModeDelete {
		return &validationError{msg: "edit_mode must be replace, insert, or delete"}
	}
	if i.EditMode == EditModeInsert && i.CellType == "" {
		return &validationError{msg: "cell_type is required when using edit_mode=insert"}
	}
	return nil
}

// editOutput is the result of a notebook_edit operation.
type editOutput struct {
	NewSource    string `json:"new_source"`
	CellID       string `json:"cell_id,omitempty"`
	CellType     string `json:"cell_type"`
	Language     string `json:"language"`
	EditMode     string `json:"edit_mode"`
	Error        string `json:"error,omitempty"`
	NotebookPath string `json:"notebook_path"`
	OriginalFile string `json:"original_file,omitempty"`
	UpdatedFile  string `json:"updated_file,omitempty"`
}

// EditTool implements the notebook_edit tool.
type EditTool struct{}

func NewEditTool() *EditTool { return &EditTool{} }

func (t *EditTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        EditToolName,
		DisplayName: EditDisplayName,
		SearchHint:  EditSearchHint,
		Description: EditDescription,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the .ipynb file",
				},
				"cell_id": map[string]any{
					"type":        "string",
					"description": "ID of the cell to edit. For insert, new cell is placed after this cell.",
				},
				"new_source": map[string]any{
					"type":        "string",
					"description": "New source content for the cell",
				},
				"cell_type": map[string]any{
					"type":        "string",
					"description": "Cell type: code or markdown (required for insert)",
					"enum":        []string{"code", "markdown"},
				},
				"edit_mode": map[string]any{
					"type":        "string",
					"description": "Operation: replace, insert, or delete (default: replace)",
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

func (t *EditTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseEditInput(input.Parsed)
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
		return tool.NewErrorResult(fmt.Errorf("file must be a Jupyter notebook (.ipynb)")), nil
	}

	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: EditToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			reason := res.Reason
			if reason == "" {
				reason = "notebook_edit requires approval"
			}
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
		}
	}

	out, err := editNotebook(fullPath, parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	result := tool.NewJSONResult(out)
	result.Content = formatEditOutput(out)
	result.Metadata = &tool.ResultMetadata{Additional: editResultMetadata(out)}
	return result, nil
}

func editNotebook(fullPath string, input *editInput) (editOutput, error) {
	originalContent, err := os.ReadFile(fullPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return editOutput{Error: fmt.Sprintf("Failed to read notebook: %v", err), NotebookPath: fullPath}, nil
		}
		mode := input.EditMode
		if mode == "" {
			mode = EditModeReplace
		}
		if mode != EditModeInsert {
			return editOutput{Error: "Notebook file does not exist.", NotebookPath: fullPath}, nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(fullPath), 0o755); mkErr != nil {
			return editOutput{Error: fmt.Sprintf("Failed to create directory: %v", mkErr), NotebookPath: fullPath}, nil
		}
		originalContent = emptyNotebookJSON()
	}

	var nb read.Notebook
	if err := json.Unmarshal(originalContent, &nb); err != nil {
		return editOutput{Error: "Notebook is not valid JSON.", NotebookPath: fullPath}, nil
	}

	cellIndex := -1
	if input.CellID != "" {
		cellIndex = findCellIndex(&nb, input.CellID)
		if cellIndex == -1 {
			if idx := parseCellID(input.CellID); idx != nil && *idx >= 0 && *idx < len(nb.Cells) {
				cellIndex = *idx
			} else {
				return editOutput{Error: fmt.Sprintf("Cell %q not found", input.CellID), NotebookPath: fullPath}, nil
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
	if nb.Metadata.LanguageInfo != nil {
		language = nb.Metadata.LanguageInfo.Name
	}

	nbFormat := nb.NBFormat
	nbFormatMinor := nb.NBFormatMinor
	newCellID := ""

	switch editMode {
	case EditModeDelete:
		if cellIndex < 0 || cellIndex >= len(nb.Cells) {
			return editOutput{Error: "Cannot delete: cell not found", NotebookPath: fullPath}, nil
		}
		copy(nb.Cells[cellIndex:], nb.Cells[cellIndex+1:])
		nb.Cells = nb.Cells[:len(nb.Cells)-1]

	case EditModeInsert:
		newCell := buildCell(cellType, input.NewSource)
		if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
			newCellID = newCell.ID
		}
		insertAt := cellIndex
		if insertAt < 0 {
			insertAt = 0
		}
		if insertAt > len(nb.Cells) {
			insertAt = len(nb.Cells)
		}
		nb.Cells = append(nb.Cells[:insertAt], append([]read.NotebookCell{newCell}, nb.Cells[insertAt:]...)...)

	default: // replace
		if cellIndex < 0 {
			if len(nb.Cells) == 0 {
				newCell := buildCell(cellType, input.NewSource)
				if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
					newCellID = newCell.ID
				}
				nb.Cells = append(nb.Cells, newCell)
			} else {
				cellIndex = 0
			}
		}
		if cellIndex >= len(nb.Cells) {
			newCell := buildCell(cellType, input.NewSource)
			if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
				newCellID = newCell.ID
			}
			nb.Cells = append(nb.Cells, newCell)
		} else if cellIndex >= 0 {
			target := &nb.Cells[cellIndex]
			target.Source = []string{input.NewSource}
			if cellType != "" && cellType != target.CellType {
				target.CellType = cellType
			}
			if target.CellType == "code" {
				target.ExecutionCount = nil
				target.Outputs = []read.NotebookOutput{}
			}
			if nbFormat > 4 || (nbFormat == 4 && nbFormatMinor >= 5) {
				newCellID = input.CellID
			}
		}
	}

	updated, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return editOutput{Error: fmt.Sprintf("Failed to serialize notebook: %v", err), NotebookPath: fullPath}, nil
	}
	if err := os.WriteFile(fullPath, updated, 0o644); err != nil {
		return editOutput{Error: fmt.Sprintf("Failed to write notebook: %v", err), NotebookPath: fullPath}, nil
	}

	return editOutput{
		NewSource:    input.NewSource,
		CellID:       newCellID,
		CellType:     cellType,
		Language:     language,
		EditMode:     string(editMode),
		NotebookPath: fullPath,
		OriginalFile: string(originalContent),
		UpdatedFile:  string(updated),
	}, nil
}

func findCellIndex(nb *read.Notebook, cellID string) int {
	for i, cell := range nb.Cells {
		if cell.ID == cellID {
			return i
		}
	}
	return -1
}

func parseCellID(cellID string) *int {
	var idx int
	if _, err := fmt.Sscanf(cellID, "cell-%d", &idx); err == nil {
		return &idx
	}
	if _, err := fmt.Sscanf(cellID, "%d", &idx); err == nil {
		return &idx
	}
	return nil
}

func formatEditOutput(out editOutput) string {
	switch out.EditMode {
	case "replace":
		return fmt.Sprintf("Updated cell %s", out.CellID)
	case "insert":
		return fmt.Sprintf("Inserted cell %s (%s)", out.CellID, out.CellType)
	case "delete":
		return fmt.Sprintf("Deleted cell %s", out.CellID)
	default:
		return "Unknown edit mode"
	}
}

func editResultMetadata(out editOutput) map[string]any {
	meta := map[string]any{
		"notebook_path": out.NotebookPath,
		"cell_id":       out.CellID,
		"cell_type":     out.CellType,
		"language":      out.Language,
		"edit_mode":     out.EditMode,
		"new_source":    out.NewSource,
	}
	if out.Error != "" {
		meta["error"] = out.Error
	}
	if out.OriginalFile != "" {
		meta["original_file"] = out.OriginalFile
	}
	if out.UpdatedFile != "" {
		meta["updated_file"] = out.UpdatedFile
	}
	return meta
}

func parseEditInput(raw map[string]any) (*editInput, error) {
	in := &editInput{EditMode: EditModeReplace}
	if v, ok := raw["notebook_path"].(string); ok {
		in.NotebookPath = v
	}
	if v, ok := raw["cell_id"].(string); ok {
		in.CellID = v
	}
	if v, ok := raw["new_source"].(string); ok {
		in.NewSource = v
	}
	if v, ok := raw["cell_type"].(string); ok {
		in.CellType = v
	}
	if v, ok := raw["edit_mode"].(string); ok {
		in.EditMode = EditMode(v)
	}
	return in, nil
}

func (t *EditTool) Description(ctx context.Context) (string, error) { return EditDescription, nil }
func (t *EditTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseEditInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *EditTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *EditTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *EditTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *EditTool) IsEnabled() bool                         { return true }
func (t *EditTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
func (t *EditTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
