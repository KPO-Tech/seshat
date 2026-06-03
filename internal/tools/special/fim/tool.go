// Package fimtool exposes Fill-in-the-Middle code completion as a Nexus tool.
package fimtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/fim"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	ToolName        = "code_complete"
	ToolDescription = `Fill in code at the cursor position using a FIM (fill-in-the-middle) model.
Provide the code before the cursor as 'prompt' and optionally the code after the cursor as 'suffix'.
The model generates the fragment that belongs between them.

Best used for: completing function bodies, filling missing logic, generating boilerplate in context.

Returns JSON with:
- "completion": the generated code fragment
- "provider": which FIM provider was used (e.g. "mistral", "deepseek")
- "model": the specific model used
- "finish_reason": "stop" | "length" | "unknown"
- "tokens_used": total tokens consumed`
)

// Tool implements the code_complete built-in tool.
type Tool struct {
	completer fim.Completer
}

// New creates a code_complete Tool. When completer is nil, IsEnabled returns
// false and the tool is not surfaced to the LLM.
func New(completer fim.Completer) *Tool {
	return &Tool{completer: completer}
}

// Definition implements tool.Tool.
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Code Complete (FIM)",
		Description: ToolDescription,
		Category:    "code",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Code before the cursor — everything up to the insertion point (required).",
				},
				"suffix": map[string]any{
					"type":        "string",
					"description": "Code after the cursor — what comes after the insertion point (optional).",
				},
				"max_tokens": map[string]any{
					"type":        "integer",
					"description": "Maximum tokens to generate (optional, provider default if omitted).",
				},
			},
			"required": []string{"prompt"},
		}),
		Metadata: map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call implements tool.Tool.
func (t *Tool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	if t.completer == nil {
		return tool.CallResult{Error: fmt.Errorf("code_complete: no FIM provider configured")}, nil
	}

	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	prompt, _ := parsed["prompt"].(string)
	if prompt == "" {
		return tool.CallResult{Error: fmt.Errorf("code_complete: prompt is required")}, nil
	}

	req := fim.Request{
		Prompt: prompt,
		Suffix: stringField(parsed, "suffix"),
	}
	if n, ok := parsed["max_tokens"].(float64); ok && n > 0 {
		tokens := int64(n)
		req.MaxTokens = &tokens
	}

	resp, err := t.completer.Complete(ctx, req)
	if err != nil {
		return tool.CallResult{Error: fmt.Errorf("code_complete: %w", err)}, nil
	}

	out, _ := json.Marshal(map[string]any{
		"completion":    resp.Content,
		"provider":      resp.Provider,
		"model":         resp.Model,
		"finish_reason": string(resp.FinishReason),
		"tokens_used":   resp.Usage.InputTokens + resp.Usage.OutputTokens,
	})
	return tool.CallResult{
		ContentType: tool.ContentTypeText,
		Content:     string(out),
	}, nil
}

// Description implements tool.Tool.
func (t *Tool) Description(_ context.Context) (string, error) { return ToolDescription, nil }

// ValidateInput implements tool.Tool.
func (t *Tool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions implements tool.Tool — FIM calls are read-only, always allowed.
func (t *Tool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe implements tool.Tool — FIM calls are stateless HTTP requests.
func (t *Tool) IsConcurrencySafe(_ map[string]any) bool { return true }

// IsReadOnly implements tool.Tool — FIM only reads code, it never writes files.
func (t *Tool) IsReadOnly(_ map[string]any) bool { return true }

// IsEnabled implements tool.Tool.
func (t *Tool) IsEnabled() bool { return t.completer != nil }

// FormatResult implements tool.Tool.
func (t *Tool) FormatResult(data any) string {
	if m, ok := data.(map[string]any); ok {
		if comp, ok := m["completion"].(string); ok && comp != "" {
			provider, _ := m["provider"].(string)
			model, _ := m["model"].(string)
			return fmt.Sprintf("FIM completion from %s/%s (%d chars)", provider, model, len(comp))
		}
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput implements tool.Tool.
func (t *Tool) BackfillInput(_ context.Context, input map[string]any) map[string]any { return input }

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
