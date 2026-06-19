package notebook

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/notebook/kernel"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	KernelToolName    = "notebook_kernel"
	KernelDisplayName = "Manage Kernels"
	KernelDescription = `Manage Jupyter kernels: list, start, restart, interrupt, or stop.

Actions:
- list:      List all running kernels
- start:     Start a new kernel (kernel_name optional, default: python3)
- restart:   Restart an existing kernel by ID
- interrupt: Interrupt (Ctrl+C) a running kernel
- stop:      Shut down a kernel by ID

Parameters:
- action:      list | start | restart | interrupt | stop (required)
- kernel_id:   Required for restart, interrupt, stop
- kernel_name: For start: kernel type (default: python3)
- server_url:  Jupyter server URL (default: $JUPYTER_SERVER_URL or http://localhost:8888)
- token:       Jupyter token (default: $JUPYTER_TOKEN)`
)

type kernelInput struct {
	Action     string `json:"action"`
	KernelID   string `json:"kernel_id,omitempty"`
	KernelName string `json:"kernel_name,omitempty"`
	ServerURL  string `json:"server_url,omitempty"`
	Token      string `json:"token,omitempty"`
}

func (i *kernelInput) validate() error {
	switch i.Action {
	case "list", "start":
		// kernel_id not required
	case "restart", "interrupt", "stop":
		if i.KernelID == "" {
			return &validationError{msg: fmt.Sprintf("kernel_id is required for action=%s", i.Action)}
		}
	default:
		return &validationError{msg: "action must be one of: list, start, restart, interrupt, stop"}
	}
	return nil
}

type kernelListItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ExecutionState string `json:"execution_state"`
	LastActivity   string `json:"last_activity"`
	Connections    int    `json:"connections"`
}

type kernelOutput struct {
	Action  string           `json:"action"`
	Kernels []kernelListItem `json:"kernels,omitempty"`
	Kernel  *kernelListItem  `json:"kernel,omitempty"`
	Message string           `json:"message,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// KernelTool implements notebook_kernel.
type KernelTool struct{}

func NewKernelTool() *KernelTool { return &KernelTool{} }

func (t *KernelTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        KernelToolName,
		DisplayName: KernelDisplayName,
		Description: KernelDescription,
		Category:    "notebook",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":      map[string]any{"type": "string", "enum": []string{"list", "start", "restart", "interrupt", "stop"}, "description": "Kernel management action."},
				"kernel_id":   map[string]any{"type": "string", "description": "Kernel ID (required for restart/interrupt/stop)."},
				"kernel_name": map[string]any{"type": "string", "description": "Kernel type for start (default: python3)."},
				"server_url":  map[string]any{"type": "string", "description": "Jupyter server URL."},
				"token":       map[string]any{"type": "string", "description": "Jupyter server token."},
			},
			"required": []string{"action"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

func (t *KernelTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed, err := parseKernelInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsed.validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}
	if permissionCheck != nil && parsed.Action != "list" {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: KernelToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: %s", orDefault(res.Reason, "notebook_kernel requires approval"))), nil
		}
	}
	out := runKernelAction(ctx, parsed)
	result := tool.NewJSONResult(out)
	if out.Error != "" {
		result.Content = "Error: " + out.Error
	} else {
		result.Content = formatKernelOutput(out)
	}
	return result, nil
}

func runKernelAction(ctx context.Context, input *kernelInput) kernelOutput {
	cfg := kernel.MergeConfig(kernel.DefaultConfig(), input.ServerURL, input.Token)
	client := kernel.New(cfg)

	switch input.Action {
	case "list":
		infos, err := client.ListKernels(ctx)
		if err != nil {
			return kernelOutput{Action: input.Action, Error: fmt.Sprintf("list kernels: %v", err)}
		}
		items := make([]kernelListItem, 0, len(infos))
		for _, ki := range infos {
			items = append(items, toListItem(ki))
		}
		return kernelOutput{Action: "list", Kernels: items}

	case "start":
		name := orDefault(input.KernelName, defaultKernel)
		info, err := client.StartKernel(ctx, name)
		if err != nil {
			return kernelOutput{Action: "start", Error: fmt.Sprintf("start kernel: %v", err)}
		}
		item := toListItem(*info)
		return kernelOutput{Action: "start", Kernel: &item, Message: fmt.Sprintf("Started kernel %s (%s)", info.ID, info.Name)}

	case "restart":
		if err := client.RestartKernel(ctx, input.KernelID); err != nil {
			return kernelOutput{Action: "restart", Error: fmt.Sprintf("restart kernel: %v", err)}
		}
		return kernelOutput{Action: "restart", Message: fmt.Sprintf("Restarted kernel %s", input.KernelID)}

	case "interrupt":
		if err := client.InterruptKernel(ctx, input.KernelID); err != nil {
			return kernelOutput{Action: "interrupt", Error: fmt.Sprintf("interrupt kernel: %v", err)}
		}
		return kernelOutput{Action: "interrupt", Message: fmt.Sprintf("Interrupted kernel %s", input.KernelID)}

	case "stop":
		if err := client.StopKernel(ctx, input.KernelID); err != nil {
			return kernelOutput{Action: "stop", Error: fmt.Sprintf("stop kernel: %v", err)}
		}
		return kernelOutput{Action: "stop", Message: fmt.Sprintf("Stopped kernel %s", input.KernelID)}

	default:
		return kernelOutput{Action: input.Action, Error: "unknown action"}
	}
}

func toListItem(ki kernel.KernelInfo) kernelListItem {
	return kernelListItem{
		ID:             ki.ID,
		Name:           ki.Name,
		ExecutionState: ki.ExecutionState,
		LastActivity:   ki.LastActivity,
		Connections:    ki.Connections,
	}
}

func formatKernelOutput(out kernelOutput) string {
	if out.Error != "" {
		return "Error: " + out.Error
	}
	if out.Message != "" && out.Kernels == nil {
		return out.Message
	}
	if out.Action == "list" {
		if len(out.Kernels) == 0 {
			return "No kernels running."
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d kernel(s) running:\n", len(out.Kernels)))
		for _, k := range out.Kernels {
			sb.WriteString(fmt.Sprintf("  %s  %s  [%s]  connections=%d\n",
				k.ID, k.Name, k.ExecutionState, k.Connections))
		}
		return strings.TrimRight(sb.String(), "\n")
	}
	return out.Message
}

func parseKernelInput(raw map[string]any) (*kernelInput, error) {
	in := &kernelInput{}
	if v, ok := raw["action"].(string); ok {
		in.Action = v
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
	return in, nil
}

func (t *KernelTool) Description(_ context.Context) (string, error) { return KernelDescription, nil }
func (t *KernelTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	p, err := parseKernelInput(input)
	if err != nil {
		return nil, err
	}
	return input, p.validate()
}
func (t *KernelTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *KernelTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *KernelTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *KernelTool) IsEnabled() bool                         { return true }
func (t *KernelTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *KernelTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
