package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	EditToolName    = "notebook_edit"
	EditDisplayName = "Edit Notebook"
	EditDescription = `Edit cells in a Jupyter notebook — single operation or batch.

Single mode: provide cell_id + edit_mode + new_source at the top level.
Batch mode: provide an ops array of operations (executed in order).

Each operation:
- edit_mode: "replace" | "insert" | "delete" (default: replace)
- cell_id:   cell ID or "cell-N" positional reference
- new_source: new content (required for replace/insert)
- cell_type: "code" | "markdown" (required for insert, default: code)

Parameters:
- notebook_path: Absolute path to the .ipynb file (required)
- ops:           Array of batch operations (mutually exclusive with cell_id)
- cell_id:       Single-op: target cell ID or positional ref (e.g. "cell-0")
- edit_mode:     Single-op: replace | insert | delete (default: replace)
- new_source:    Single-op: new cell content
- cell_type:     Single-op: code | markdown (required for insert)`
)

type EditMode string

const (
	EditModeReplace EditMode = "replace"
	EditModeInsert  EditMode = "insert"
	EditModeDelete  EditMode = "delete"
)

type EditOp struct {
	EditMode  EditMode `json:"edit_mode"`
	CellID    string   `json:"cell_id,omitempty"`
	NewSource string   `json:"new_source,omitempty"`
	CellType  string   `json:"cell_type,omitempty"`
}

type editInput struct {
	NotebookPath string   `json:"notebook_path"`
	Ops          []EditOp `json:"ops,omitempty"`
	// Single-op fields (alias for ops[0])
	CellID    string   `json:"cell_id,omitempty"`
	EditMode  EditMode `json:"edit_mode,omitempty"`
	NewSource string   `json:"new_source,omitempty"`
	CellType  string   `json:"cell_type,omitempty"`
}

func (i *editInput) resolveOps() ([]EditOp, error) {
	if len(i.Ops) > 0 {
		for idx, op := range i.Ops {
			if err := validateOp(op, idx); err != nil {
				return nil, err
			}
		}
		return i.Ops, nil
	}
	// single-op mode
	op := EditOp{
		EditMode:  i.EditMode,
		CellID:    i.CellID,
		NewSource: i.NewSource,
		CellType:  i.CellType,
	}
	if op.EditMode == "" {
		op.EditMode = EditModeReplace
	}
	if err := validateOp(op, 0); err != nil {
		return nil, err
	}
	return []EditOp{op}, nil
}

func validateOp(op EditOp, idx int) error {
	if op.EditMode != "" && op.EditMode != EditModeReplace && op.EditMode != EditModeInsert && op.EditMode != EditModeDelete {
		return &validationError{msg: fmt.Sprintf("ops[%d].edit_mode must be replace, insert, or delete", idx)}
	}
	mode := op.EditMode
	if mode == "" {
		mode = EditModeReplace
	}
	if (mode == EditModeReplace || mode == EditModeInsert) && op.NewSource == "" {
		return &validationError{msg: fmt.Sprintf("ops[%d].new_source is required for %s", idx, mode)}
	}
	if mode == EditModeInsert && op.CellType == "" {
		return &validationError{msg: fmt.Sprintf("ops[%d].cell_type is required for insert", idx)}
	}
	return nil
}

type opResult struct {
	OpIndex  int    `json:"op_index"`
	EditMode string `json:"edit_mode"`
	CellID   string `json:"cell_id,omitempty"`
	CellType string `json:"cell_type,omitempty"`
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
}

type editOutput struct {
	NotebookPath string     `json:"notebook_path"`
	OpsApplied   int        `json:"ops_applied"`
	OpsTotal     int        `json:"ops_total"`
	Results      []opResult `json:"results"`
	Error        string     `json:"error,omitempty"`
}

// EditTool implements notebook_edit.
type EditTool struct{}

func NewEditTool() *EditTool { return &EditTool{} }

func (t *EditTool) Definition() tool.Definition {
	opSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"edit_mode":  map[string]any{"type": "string", "enum": []string{"replace", "insert", "delete"}},
			"cell_id":    map[string]any{"type": "string"},
			"new_source": map[string]any{"type": "string"},
			"cell_type":  map[string]any{"type": "string", "enum": []string{"code", "markdown"}},
		},
	}
	return tool.Definition{
		Name:        EditToolName,
		DisplayName: EditDisplayName,
		Description: EditDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{"type": "string", "description": "Absolute path to the .ipynb file."},
				"ops":           map[string]any{"type": "array", "items": opSchema, "description": "Batch of edit operations (executed in order)."},
				"cell_id":       map[string]any{"type": "string", "description": "Single-op: target cell."},
				"edit_mode":     map[string]any{"type": "string", "enum": []string{"replace", "insert", "delete"}, "description": "Single-op: operation type."},
				"new_source":    map[string]any{"type": "string", "description": "Single-op: new cell content."},
				"cell_type":     map[string]any{"type": "string", "enum": []string{"code", "markdown"}, "description": "Single-op: cell type (required for insert)."},
			},
			"required": []string{"notebook_path"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      true,
		RequiresPermission: true,
	}
}

func (t *EditTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseEditInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if parsed.NotebookPath == "" {
		return tool.NewErrorResult(&validationError{msg: "notebook_path is required"}), nil
	}
	ops, err := parsed.resolveOps()
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	fullPath, err := absNotebookPath(parsed.NotebookPath)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: EditToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", orDefault(res.Reason, "notebook_edit requires approval"))), nil
		}
	}
	out := runEdit(fullPath, ops)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else {
		result.Content = formatEditOutput(out)
	}
	return result, nil
}

func runEdit(fullPath string, ops []EditOp) editOutput {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return editOutput{Error: fmt.Sprintf("read: %v", err), NotebookPath: fullPath}
		}
		// Create from empty if first op is insert
		if len(ops) > 0 && ops[0].EditMode == EditModeInsert {
			if mkErr := os.MkdirAll(filepath.Dir(fullPath), 0o755); mkErr != nil {
				return editOutput{Error: fmt.Sprintf("mkdir: %v", mkErr), NotebookPath: fullPath}
			}
			raw = emptyNotebookJSON()
		} else {
			return editOutput{Error: "notebook file not found", NotebookPath: fullPath}
		}
	}

	var nb read.Notebook
	if err := json.Unmarshal(raw, &nb); err != nil {
		return editOutput{Error: "not valid notebook JSON", NotebookPath: fullPath}
	}

	applied := 0
	results := make([]opResult, 0, len(ops))
	nbFmt := nb.NBFormat
	nbFmtMinor := nb.NBFormatMinor
	hasIDs := nbFmt > 4 || (nbFmt == 4 && nbFmtMinor >= 5)

	for i, op := range ops {
		mode := op.EditMode
		if mode == "" {
			mode = EditModeReplace
		}
		cellType := op.CellType
		if cellType == "" {
			cellType = "code"
		}

		cellIndex := -1
		if op.CellID != "" {
			cellIndex = findCellByID(&nb, op.CellID)
			if cellIndex == -1 {
				if idx := parseCellIndex(op.CellID); idx != nil && *idx >= 0 && *idx < len(nb.Cells) {
					cellIndex = *idx
				} else if mode != EditModeInsert {
					results = append(results, opResult{OpIndex: i, EditMode: string(mode), Error: fmt.Sprintf("cell %q not found", op.CellID)})
					continue
				}
			}
		}

		var resultID string
		switch mode {
		case EditModeDelete:
			if cellIndex < 0 || cellIndex >= len(nb.Cells) {
				results = append(results, opResult{OpIndex: i, EditMode: string(mode), Error: "cell not found for delete"})
				continue
			}
			resultID = nb.Cells[cellIndex].ID
			copy(nb.Cells[cellIndex:], nb.Cells[cellIndex+1:])
			nb.Cells = nb.Cells[:len(nb.Cells)-1]

		case EditModeInsert:
			newCell := buildCell(cellType, op.NewSource)
			if hasIDs {
				resultID = newCell.ID
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
					newCell := buildCell(cellType, op.NewSource)
					if hasIDs {
						resultID = newCell.ID
					}
					nb.Cells = append(nb.Cells, newCell)
				} else {
					cellIndex = 0
				}
			}
			if cellIndex >= 0 && cellIndex < len(nb.Cells) {
				target := &nb.Cells[cellIndex]
				target.Source = []string{op.NewSource}
				if cellType != "" && cellType != target.CellType {
					target.CellType = cellType
				}
				if target.CellType == "code" {
					target.ExecutionCount = nil
					target.Outputs = []read.NotebookOutput{}
				}
				if hasIDs {
					resultID = target.ID
				}
			} else if cellIndex >= len(nb.Cells) {
				newCell := buildCell(cellType, op.NewSource)
				if hasIDs {
					resultID = newCell.ID
				}
				nb.Cells = append(nb.Cells, newCell)
			}
		}

		applied++
		results = append(results, opResult{
			OpIndex:  i,
			EditMode: string(mode),
			CellID:   resultID,
			CellType: cellType,
			Message:  fmt.Sprintf("%s cell %s", mode, resultID),
		})
	}

	updated, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return editOutput{Error: fmt.Sprintf("serialize: %v", err), NotebookPath: fullPath}
	}
	if err := os.WriteFile(fullPath, updated, 0o644); err != nil {
		return editOutput{Error: fmt.Sprintf("write: %v", err), NotebookPath: fullPath}
	}

	return editOutput{
		NotebookPath: fullPath,
		OpsApplied:   applied,
		OpsTotal:     len(ops),
		Results:      results,
	}
}

func findCellByID(nb *read.Notebook, id string) int {
	for i, c := range nb.Cells {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func parseCellIndex(cellID string) *int {
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
	if out.Error != "" {
		return "Error: " + out.Error
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Applied %d/%d ops on %s\n", out.OpsApplied, out.OpsTotal, out.NotebookPath))
	for _, r := range out.Results {
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("  [%d] ERROR: %s\n", r.OpIndex, r.Error))
		} else {
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", r.OpIndex, r.Message))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func parseEditInput(raw map[string]any) (*editInput, error) {
	in := &editInput{}
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
	if rawOps, ok := raw["ops"].([]any); ok {
		for _, ro := range rawOps {
			if m, ok := ro.(map[string]any); ok {
				op := EditOp{}
				if v, ok := m["edit_mode"].(string); ok {
					op.EditMode = EditMode(v)
				}
				if v, ok := m["cell_id"].(string); ok {
					op.CellID = v
				}
				if v, ok := m["new_source"].(string); ok {
					op.NewSource = v
				}
				if v, ok := m["cell_type"].(string); ok {
					op.CellType = v
				}
				in.Ops = append(in.Ops, op)
			}
		}
	}
	return in, nil
}

func (t *EditTool) Description(_ context.Context) (string, error) { return EditDescription, nil }
func (t *EditTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseEditInput(input)
	if err != nil {
		return nil, err
	}
	_, err = p.resolveOps()
	return input, err
}
func (t *EditTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *EditTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *EditTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *EditTool) IsEnabled() bool                         { return true }
func (t *EditTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *EditTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
