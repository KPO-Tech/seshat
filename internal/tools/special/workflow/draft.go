package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/KPO-Tech/seshat-runtime/pkg/workflow"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const (
	ToolName = "workflow_draft"

	ToolDescription = "Validate and render a static workflow DAG draft as YAML or JSON. " +
		"Use this when a user asks to design a reusable workflow, split a complex task into parallel agents, or prepare a workflow file for `seshat workflow run`."
)

type Tool struct{}

type input struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Format      string      `json:"format"`
	Nodes       []core.Node `json:"nodes"`
}

func NewTool() *Tool {
	return &Tool{}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Workflow Draft",
		SearchHint:  "draft validate render static workflow DAG yaml json parallel agents",
		Description: ToolDescription,
		Prompt: "Build a compact DAG before calling this tool. Each node should do one bounded task. " +
			"Use `needs` only when a node requires prior outputs; otherwise leave nodes independent so they can run in parallel. " +
			"The final node should synthesize dependency outputs when the workflow needs a single answer.",
		Category: "context",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Workflow name.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Optional workflow description.",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Rendered output format.",
					"enum":        []string{"yaml", "json"},
					"default":     "yaml",
				},
				"nodes": map[string]any{
					"type":        "array",
					"description": "DAG nodes. Each node should contain a single task.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "Stable node identifier used by dependencies.",
							},
							"kind": map[string]any{
								"type":        "string",
								"description": "Optional node role. agent runs the main task, verifier checks correctness, critic finds weaknesses, router chooses allowed downstream routes.",
								"enum":        []string{"agent", "verifier", "critic", "router"},
								"default":     "agent",
							},
							"agent": map[string]any{
								"type":        "string",
								"description": "Optional label for the agent/persona expected to execute the node.",
							},
							"prompt": map[string]any{
								"type":        "string",
								"description": "Task prompt for this node.",
							},
							"needs": map[string]any{
								"type":        "array",
								"description": "Node ids that must finish before this node can run.",
								"items":       map[string]any{"type": "string"},
							},
							"routes": map[string]any{
								"type":        "array",
								"description": "For router nodes, allowed route node ids the router may select. V1 records the routing decision; execution remains a static DAG.",
								"items":       map[string]any{"type": "string"},
							},
							"max_turns": map[string]any{
								"type":        "integer",
								"description": "Optional maximum agent turns for this node.",
								"minimum":     1,
							},
							"output_format": map[string]any{
								"type":        "string",
								"description": "Optional expected output shape for this node.",
							},
						},
						"required": []string{"id", "prompt"},
					},
					"minItems": 1,
				},
			},
			"required": []string{"name", "nodes"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *Tool) Call(_ context.Context, callInput tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	params, err := parseInput(callInput)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	draft, err := core.Draft(core.DraftOptions{
		Format: params.Format,
		Definition: core.Definition{
			Name:        params.Name,
			Description: params.Description,
			Nodes:       params.Nodes,
		},
	})
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("invalid workflow draft: %w", err)), nil
	}

	content := formatDraft(draft)
	result := tool.NewJSONResult(map[string]any{
		"definition":  draft.Definition,
		"format":      draft.Format,
		"content":     draft.Content,
		"node_count":  len(draft.Definition.Nodes),
		"diagnostics": draft.Diagnostics,
	})
	result.Content = content
	return result, nil
}

func (t *Tool) Description(_ context.Context) (string, error) { return ToolDescription, nil }

func (t *Tool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	params, err := decodeMap(in)
	if err != nil {
		return nil, err
	}
	if _, err := core.Draft(core.DraftOptions{
		Format: params.Format,
		Definition: core.Definition{
			Name:        params.Name,
			Description: params.Description,
			Nodes:       params.Nodes,
		},
	}); err != nil {
		return nil, err
	}
	return in, nil
}

func (t *Tool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.AllowWithInput("", in)
}

func (t *Tool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *Tool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *Tool) IsEnabled() bool                         { return true }
func (t *Tool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *Tool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}

func parseInput(callInput tool.CallInput) (input, error) {
	if callInput.Raw != "" {
		var params input
		if err := json.Unmarshal([]byte(callInput.Raw), &params); err != nil {
			return input{}, fmt.Errorf("invalid workflow_draft input: %w", err)
		}
		return params, nil
	}
	return decodeMap(callInput.Parsed)
}

func decodeMap(in map[string]any) (input, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return input{}, err
	}
	var params input
	if err := json.Unmarshal(data, &params); err != nil {
		return input{}, err
	}
	return params, nil
}

func formatDraft(draft core.DraftResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workflow draft is valid (%s, %d node", draft.Format, len(draft.Definition.Nodes))
	if len(draft.Definition.Nodes) != 1 {
		b.WriteString("s")
	}
	b.WriteString(").\n\n")
	b.WriteString("```")
	b.WriteString(draft.Format)
	b.WriteString("\n")
	b.WriteString(draft.Content)
	b.WriteString("```")
	return b.String()
}
