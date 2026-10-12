package tools_test

// This file deliberately imports only pkg/* paths - never internal/... -
// to prove a real external module (e.g. seshat-ai) can implement and
// register a custom tool using nothing but this package. If this file
// compiles, so does the equivalent code in a separate module.

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
	"github.com/KPO-Tech/seshat-runtime/pkg/tools"
)

// echoTool is a minimal external-style custom tool: echoes its "message"
// input back. Exercises every method Tool requires.
type echoTool struct{}

func (echoTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "echo",
		Description: "Echoes the message input back.",
		InputSchema: tools.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"message"},
		}),
		IsReadOnly:        true,
		IsConcurrencySafe: true,
	}
}

func (echoTool) Call(_ context.Context, input tools.CallInput, _ tools.CanUseToolFn) (tools.CallResult, error) {
	msg, _ := input.Parsed["message"].(string)
	return tools.NewTextResult(msg), nil
}

func (echoTool) Description(_ context.Context) (string, error) {
	return "Echoes the message input back.", nil
}

func (echoTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (echoTool) CheckPermissions(_ context.Context, input map[string]any, _ tools.ToolUseContext) tools.PermissionResult {
	return tools.Passthrough(input)
}

func (echoTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (echoTool) IsReadOnly(_ map[string]any) bool        { return true }
func (echoTool) IsEnabled() bool                         { return true }

func (echoTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return ""
}

func (echoTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// Compile-time proof: a tools.Tool built entirely from public aliases is
// also an sdk.Tool - no adapter needed to pass it to Client.RegisterTool.
var (
	_ tools.Tool = echoTool{}
	_ sdk.Tool   = echoTool{}
)

func TestEchoTool_CallReturnsInputMessage(t *testing.T) {
	tool := echoTool{}
	result, err := tool.Call(context.Background(), tools.CallInput{
		Parsed: map[string]any{"message": "hello from outside the module"},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Content != "hello from outside the module" {
		t.Fatalf("expected echoed message, got %q", result.Content)
	}
}

func TestEchoTool_RegistersOnARealClient(t *testing.T) {
	client, err := sdk.NewClient(sdk.DefaultClientConfig())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if err := client.RegisterTool(echoTool{}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
}
