package notebook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	"github.com/EngineerProjects/nexus-engine/internal/tools/notebook/kernel"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	ExecuteToolName    = "notebook_execute"
	ExecuteDisplayName = "Execute Notebook"
	ExecuteDescription = `Execute all (or selected) cells in a Jupyter notebook using a live kernel.

Connects to a Jupyter server, starts or reuses a kernel, executes cells in order,
and writes execution outputs back into the .ipynb file.

Parameters:
- notebook_path:  Absolute path to the .ipynb file (required)
- cells:          0-based indices of cells to execute (default: all code cells)
- kernel_id:      Reuse an existing kernel by ID (optional; starts a new one if omitted)
- kernel_name:    Kernel to start if kernel_id is omitted (default: python3)
- timeout_sec:    Per-cell execution timeout in seconds (default: 60)
- server_url:     Jupyter server URL (default: $JUPYTER_SERVER_URL or http://localhost:8888)
- token:          Jupyter token (default: $JUPYTER_TOKEN)`
)

type executeInput struct {
	NotebookPath string `json:"notebook_path"`
	Cells        []int  `json:"cells,omitempty"`
	KernelID     string `json:"kernel_id,omitempty"`
	KernelName   string `json:"kernel_name,omitempty"`
	TimeoutSec   int    `json:"timeout_sec,omitempty"`
	ServerURL    string `json:"server_url,omitempty"`
	Token        string `json:"token,omitempty"`
}

func (i *executeInput) validate() error {
	if i.NotebookPath == "" {
		return &validationError{msg: "notebook_path is required"}
	}
	return nil
}

func (i *executeInput) cellTimeout() time.Duration {
	if i.TimeoutSec <= 0 {
		return 60 * time.Second
	}
	return time.Duration(i.TimeoutSec) * time.Second
}

type cellExecution struct {
	CellIndex  int    `json:"cell_index"`
	CellID     string `json:"cell_id,omitempty"`
	Source     string `json:"source"`
	Output     string `json:"output"`
	HasError   bool   `json:"has_error"`
	DurationMs int64  `json:"duration_ms"`
}

type executeOutput struct {
	NotebookPath string          `json:"notebook_path"`
	KernelID     string          `json:"kernel_id"`
	KernelName   string          `json:"kernel_name"`
	CellsRun     int             `json:"cells_run"`
	CellsTotal   int             `json:"cells_total"`
	HasErrors    bool            `json:"has_errors"`
	Results      []cellExecution `json:"results"`
	Error        string          `json:"error,omitempty"`
}

// ExecuteTool implements notebook_execute.
type ExecuteTool struct{}

func NewExecuteTool() *ExecuteTool { return &ExecuteTool{} }

func (t *ExecuteTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ExecuteToolName,
		DisplayName: ExecuteDisplayName,
		Description: ExecuteDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"notebook_path": map[string]any{"type": "string", "description": "Absolute path to the .ipynb file."},
				"cells":         map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "0-based indices of cells to execute (default: all code cells)."},
				"kernel_id":     map[string]any{"type": "string", "description": "Reuse existing kernel by ID."},
				"kernel_name":   map[string]any{"type": "string", "description": "Kernel to start (default: python3)."},
				"timeout_sec":   map[string]any{"type": "integer", "description": "Per-cell timeout in seconds (default: 60)."},
				"server_url":    map[string]any{"type": "string", "description": "Jupyter server URL."},
				"token":         map[string]any{"type": "string", "description": "Jupyter server token."},
			},
			"required": []string{"notebook_path"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

func (t *ExecuteTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseExecuteInput(input.Parsed)
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
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: ExecuteToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", orDefault(res.Reason, "notebook_execute requires approval"))), nil
		}
	}
	out := runExecute(ctx, fullPath, parsed)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else {
		errNote := ""
		if out.HasErrors {
			errNote = " (with errors)"
		}
		result.Content = fmt.Sprintf("Executed %d/%d cells on kernel %s%s\n\nNotebook: %s",
			out.CellsRun, out.CellsTotal, out.KernelID, errNote, out.NotebookPath)
	}
	return result, nil
}

func runExecute(ctx context.Context, fullPath string, input *executeInput) executeOutput {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return executeOutput{Error: fmt.Sprintf("read notebook: %v", err), NotebookPath: fullPath}
	}
	var nb read.Notebook
	if err := json.Unmarshal(raw, &nb); err != nil {
		return executeOutput{Error: "invalid notebook JSON", NotebookPath: fullPath}
	}

	cfg := kernel.MergeConfig(kernel.DefaultConfig(), input.ServerURL, input.Token)
	client := kernel.New(cfg)

	kernelID := input.KernelID
	kernelName := orDefault(input.KernelName, defaultKernel)

	if kernelID == "" {
		info, err := client.StartKernel(ctx, kernelName)
		if err != nil {
			return executeOutput{Error: fmt.Sprintf("start kernel: %v", err), NotebookPath: fullPath}
		}
		kernelID = info.ID
		kernelName = info.Name
	}

	ch, err := client.OpenChannel(ctx, kernelID)
	if err != nil {
		return executeOutput{Error: fmt.Sprintf("open channel: %v", err), NotebookPath: fullPath}
	}
	defer ch.Close()

	wantIdx := buildIndexSet(input.Cells, len(nb.Cells))
	timeout := input.cellTimeout()

	var results []cellExecution
	hasErrors := false
	cellsRun := 0

	for i := range nb.Cells {
		cell := &nb.Cells[i]
		if cell.CellType != "code" {
			continue
		}
		if !wantIdx[i] {
			continue
		}
		src := cellSource(*cell)
		if src == "" {
			continue
		}

		start := time.Now()
		outputs, err := ch.Execute(ctx, src, timeout)
		dur := time.Since(start).Milliseconds()

		outText := ""
		cellErr := false
		if err != nil {
			outText = fmt.Sprintf("[execution error: %v]", err)
			cellErr = true
		} else {
			outText = kernel.FormatOutputs(outputs)
			for _, o := range outputs {
				if o.Type == "error" {
					cellErr = true
				}
			}
		}

		// Write outputs back into notebook
		cell.Outputs = convertKernelOutputs(outputs)
		if cellErr {
			hasErrors = true
		}
		cellsRun++

		results = append(results, cellExecution{
			CellIndex:  i,
			CellID:     cell.ID,
			Source:     src,
			Output:     outText,
			HasError:   cellErr,
			DurationMs: dur,
		})
	}

	// Persist updated outputs
	updated, err := json.MarshalIndent(nb, "", " ")
	if err == nil {
		_ = os.WriteFile(fullPath, updated, 0o644)
	}

	return executeOutput{
		NotebookPath: fullPath,
		KernelID:     kernelID,
		KernelName:   kernelName,
		CellsRun:     cellsRun,
		CellsTotal:   len(nb.Cells),
		HasErrors:    hasErrors,
		Results:      results,
	}
}

// convertKernelOutputs converts kernel.Output slice to read.NotebookOutput slice
// so results can be written back into the notebook file.
func convertKernelOutputs(outputs []kernel.Output) []read.NotebookOutput {
	result := make([]read.NotebookOutput, 0, len(outputs))
	for _, o := range outputs {
		no := read.NotebookOutput{OutputType: o.Type}
		if o.Type == "stream" {
			no.Text = []string{o.Text}
		} else if o.Type == "error" {
			no.Ename = "ExecutionError"
			no.Evalue = o.Text
		} else if o.Data != nil {
			no.Data = o.Data
			if o.Text != "" {
				no.Data["text/plain"] = o.Text
			}
		} else if o.Text != "" {
			no.Text = []string{o.Text}
		}
		result = append(result, no)
	}
	return result
}

func parseExecuteInput(raw map[string]any) (*executeInput, error) {
	in := &executeInput{}
	if v, ok := raw["notebook_path"].(string); ok {
		in.NotebookPath = v
	}
	if v, ok := raw["kernel_id"].(string); ok {
		in.KernelID = v
	}
	if v, ok := raw["kernel_name"].(string); ok {
		in.KernelName = v
	}
	if v, ok := raw["server_url"].(string); ok {
		in.ServerURL = v
	}
	if v, ok := raw["token"].(string); ok {
		in.Token = v
	}
	if v, ok := raw["timeout_sec"].(float64); ok {
		in.TimeoutSec = int(v)
	}
	if rawCells, ok := raw["cells"].([]any); ok {
		for _, rc := range rawCells {
			if v, ok := rc.(float64); ok {
				in.Cells = append(in.Cells, int(v))
			}
		}
	}
	return in, nil
}

func (t *ExecuteTool) Description(_ context.Context) (string, error) { return ExecuteDescription, nil }
func (t *ExecuteTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseExecuteInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *ExecuteTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *ExecuteTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *ExecuteTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *ExecuteTool) IsEnabled() bool                         { return true }
func (t *ExecuteTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *ExecuteTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
