package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/workflow"
)

func TestClientRunWorkflowUsesInjectedExecutor(t *testing.T) {
	client, err := NewClient(&ClientConfig{PersistSessions: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	result, err := client.RunWorkflow(context.Background(), WorkflowDefinition{
		Name: "sdk-workflow",
		Nodes: []WorkflowNode{
			{ID: "research", Prompt: "collect"},
			{ID: "final", Prompt: "finish", Needs: []string{"research"}},
		},
	}, WorkflowOptions{
		Executor: workflow.ExecutorFunc(func(_ context.Context, node workflow.Node, inputs map[string]workflow.NodeResult) (string, error) {
			prompt := BuildWorkflowNodePrompt(node, inputs)
			if node.ID == "final" && !strings.Contains(prompt, "research output") {
				t.Fatalf("expected final node prompt to include dependency output, got:\n%s", prompt)
			}
			return node.ID + " output", nil
		}),
	})
	if err != nil {
		t.Fatalf("RunWorkflow failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected workflow success")
	}
	if result.Results["final"].Output != "final output" {
		t.Fatalf("unexpected final output: %#v", result.Results["final"])
	}
}
