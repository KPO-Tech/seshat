package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tools/notebook/kernel"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	RunToolName    = "notebook_run"
	RunDisplayName = "Run Code in Kernel"
	RunDescription = `Run arbitrary code in a Jupyter kernel and return the output.

Does not require a notebook file. Use this for quick code execution,
exploration, or testing snippets without creating a notebook.

Parameters:
- code:        Code to execute (required)
- kernel_id:   Reuse an existing kernel by ID (optional)
- kernel_name: Kernel to start if kernel_id is omitted (default: python3)
- timeout_sec: Execution timeout in seconds (default: 30)
- server_url:  Jupyter server URL (default: $JUPYTER_SERVER_URL or http://localhost:8888)
- token:       Jupyter token (default: $JUPYTER_TOKEN)`
)

type runInput struct {
	Code       string `json:"code"`
	KernelID   string `json:"kernel_id,omitempty"`
	KernelName string `json:"kernel_name,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
	ServerURL  string `json:"server_url,omitempty"`
	Token      string `json:"token,omitempty"`
}

func (i *runInput) validate() error {
	if i.Code == "" {
		return &validationError{msg: "code is required"}
	}
	return nil
}

func (i *runInput) timeout() time.Duration {
	if i.TimeoutSec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(i.TimeoutSec) * time.Second
}

type runOutput struct {
	KernelID   string `json:"kernel_id"`
	KernelName string `json:"kernel_name"`
	Code       string `json:"code"`
	Output     string `json:"output"`
	HasError   bool   `json:"has_error"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// RunTool implements notebook_run.
type RunTool struct{}

func NewRunTool() *RunTool { return &RunTool{} }

func (t *RunTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        RunToolName,
		DisplayName: RunDisplayName,
		Description: RunDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":        map[string]any{"type": "string", "description": "Code to execute."},
				"kernel_id":   map[string]any{"type": "string", "description": "Reuse existing kernel by ID."},
				"kernel_name": map[string]any{"type": "string", "description": "Kernel to start (default: python3)."},
				"timeout_sec": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)."},
				"server_url":  map[string]any{"type": "string", "description": "Jupyter server URL."},
				"token":       map[string]any{"type": "string", "description": "Jupyter server token."},
			},
			"required": []string{"code"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

func (t *RunTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseRunInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsed.validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}
	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: RunToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", orDefault(res.Reason, "notebook_run requires approval"))), nil
		}
	}
	out := runCode(ctx, parsed)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else if out.HasError {
		result.Content = fmt.Sprintf("[kernel %s] execution error:\n%s", out.KernelID, out.Output)
	} else {
		result.Content = out.Output
		if out.Output == "" {
			result.Content = "[No output]"
		}
	}
	return result, nil
}

func runCode(ctx context.Context, input *runInput) runOutput {
	cfg := kernel.MergeConfig(kernel.DefaultConfig(), input.ServerURL, input.Token)
	client := kernel.New(cfg)

	kernelID := input.KernelID
	kernelName := orDefault(input.KernelName, defaultKernel)

	if kernelID == "" {
		info, err := client.StartKernel(ctx, kernelName)
		if err != nil {
			return runOutput{Error: fmt.Sprintf("start kernel: %v", err)}
		}
		kernelID = info.ID
		kernelName = info.Name
	}

	ch, err := client.OpenChannel(ctx, kernelID)
	if err != nil {
		return runOutput{Error: fmt.Sprintf("open channel: %v", err), KernelID: kernelID, KernelName: kernelName}
	}
	defer ch.Close()

	start := time.Now()
	outputs, err := ch.Execute(ctx, input.Code, input.timeout())
	dur := time.Since(start).Milliseconds()

	if err != nil {
		return runOutput{
			KernelID:   kernelID,
			KernelName: kernelName,
			Code:       input.Code,
			Output:     fmt.Sprintf("[execution error: %v]", err),
			HasError:   true,
			DurationMs: dur,
		}
	}

	hasError := false
	for _, o := range outputs {
		if o.Type == "error" {
			hasError = true
		}
	}

	return runOutput{
		KernelID:   kernelID,
		KernelName: kernelName,
		Code:       input.Code,
		Output:     kernel.FormatOutputs(outputs),
		HasError:   hasError,
		DurationMs: dur,
	}
}

func parseRunInput(raw map[string]any) (*runInput, error) {
	in := &runInput{}
	if v, ok := raw["code"].(string); ok {
		in.Code = v
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
	return in, nil
}

func (t *RunTool) Description(_ context.Context) (string, error) { return RunDescription, nil }
func (t *RunTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseRunInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *RunTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *RunTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *RunTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *RunTool) IsEnabled() bool                         { return true }
func (t *RunTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *RunTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
