package repomap

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/KPO-Tech/seshat-runtime/pkg/repomap"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const (
	ToolName = "repo_map"

	ToolDescription = "Build a compact structural map of the current repository. " +
		"Use this when you need a ranked overview of important Go files, packages, and function/type signatures before reading implementation bodies."
)

type Tool struct {
	workingDir string
}

type input struct {
	Tokens       int      `json:"tokens"`
	FocusFiles   []string `json:"focus_files"`
	FocusSymbols []string `json:"focus_symbols"`
}

func NewTool(workingDir string) *Tool {
	return &Tool{workingDir: workingDir}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Repo Map",
		SearchHint:  "summarize repository structure and Go symbols",
		Description: ToolDescription,
		Category:    "context",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tokens": map[string]any{
					"type":        "integer",
					"description": "Approximate token budget for the rendered repo map.",
					"default":     core.DefaultTokenBudget,
					"minimum":     128,
					"maximum":     12000,
				},
				"focus_files": map[string]any{
					"type":        "array",
					"description": "Path fragments to boost in the ranking.",
					"items":       map[string]any{"type": "string"},
				},
				"focus_symbols": map[string]any{
					"type":        "array",
					"description": "Symbol names whose defining files should be boosted.",
					"items":       map[string]any{"type": "string"},
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *Tool) Call(ctx context.Context, callInput tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	var params input
	if callInput.Raw != "" {
		if err := json.Unmarshal([]byte(callInput.Raw), &params); err != nil {
			return tool.CallResult{}, fmt.Errorf("invalid repo_map input: %w", err)
		}
	}
	root := callInput.ToolContextValue().WorkingDirectory
	if root == "" {
		root = t.workingDir
	}
	m, err := core.Build(ctx, core.Options{
		Root:         root,
		TokenBudget:  params.Tokens,
		FocusFiles:   params.FocusFiles,
		FocusSymbols: params.FocusSymbols,
	})
	if err != nil {
		return tool.CallResult{}, err
	}
	rendered := core.Render(m, params.Tokens)
	result := tool.NewTextResult(rendered)
	result.Data = map[string]any{
		"root":           m.Root,
		"files_scanned":  m.FilesScanned,
		"files_included": m.FilesIncluded,
		"content":        rendered,
	}
	return result, nil
}

func (t *Tool) Description(_ context.Context) (string, error) { return ToolDescription, nil }
func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *Tool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *Tool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *Tool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *Tool) IsEnabled() bool                         { return true }
func (t *Tool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *Tool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
