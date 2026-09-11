package dataflow

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/workflow"
)

type stubAgentCaller struct {
	gotSlug       string
	gotPrompt     string
	gotTools      []string
	gotGraphTools []ToolSpec
	response      string
}

func (s *stubAgentCaller) Ask(_ context.Context, agentSlug, prompt string, tools []string, graphTools []ToolSpec) (string, error) {
	s.gotSlug, s.gotPrompt, s.gotTools, s.gotGraphTools = agentSlug, prompt, tools, graphTools
	return s.response, nil
}

type stubSubworkflowRunner struct {
	gotDef workflow.Definition
	result workflow.Result
}

func (s *stubSubworkflowRunner) Run(_ context.Context, def workflow.Definition) (workflow.Result, error) {
	s.gotDef = def
	return s.result, nil
}

func TestQueryNodeCallsRuntimeAgentViaConnectedAgentNode(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	stub := &stubAgentCaller{response: "hello"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "draft a reply"}},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if stub.gotSlug != "inbox" || stub.gotPrompt != "draft a reply" {
		t.Fatalf("unexpected call: slug=%q prompt=%q", stub.gotSlug, stub.gotPrompt)
	}
	out := result.Results["q"].Output
	if len(out) != 1 || out[0]["text"] != "hello" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestQueryNodeForwardsToolsOverride(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	stub := &stubAgentCaller{response: "ok"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "check the weather", "tools": " weather_lookup , calendar_read ,,"}},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	want := []string{"weather_lookup", "calendar_read"}
	if len(stub.gotTools) != len(want) || stub.gotTools[0] != want[0] || stub.gotTools[1] != want[1] {
		t.Fatalf("expected trimmed tool names %v, got %v", want, stub.gotTools)
	}
}

func TestQueryNodeOmitsToolsWhenNotSet(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	stub := &stubAgentCaller{response: "ok"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "draft a reply"}},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if stub.gotTools != nil {
		t.Fatalf("expected nil tools when the parameter is unset, got %v", stub.gotTools)
	}
}

func TestQueryNodeIncludesUpstreamInputInPrompt(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	stub := &stubAgentCaller{response: "done"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "source", Type: "source", Connections: map[string][]string{"main": {"query"}}},
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "query", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "handle this"}},
	}}
	reg.Register("source", funcExecutor{desc: NodeDescription{Type: "source"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		return Main([]Item{{"subject": "invoice overdue"}}), nil
	}})

	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if !strings.Contains(stub.gotPrompt, "handle this") || !strings.Contains(stub.gotPrompt, "invoice overdue") {
		t.Fatalf("expected prompt to include both the static prompt and upstream data, got %q", stub.gotPrompt)
	}
}

func TestQueryNodeRequiresRuntimeAgent(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}},
	}}
	result, err := Run(context.Background(), def, reg, &Runtime{}, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure without a configured AgentCaller")
	}
}

func TestAgentNodeIsAPureNoOp(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
	}}
	result, err := Run(context.Background(), def, reg, &Runtime{}, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if out := result.Results["id"].Output; len(out) != 0 {
		t.Fatalf("expected an agent node to produce no output, got %#v", out)
	}
}

func TestAgentNodeAllowsEmptyAgentParameter(t *testing.T) {
	// A fresh/mid-edit agent node (or a built-in template shipping as a
	// generic starting point - see automation-app-pages.md §9) has no
	// persona picked yet; that's valid, not an error - see agentNode's own
	// doc comment for why.
	if err := (agentNode{}).ValidateParameters(nil); err != nil {
		t.Fatalf("expected no error for an agent node with no agent parameter, got %v", err)
	}
}

func TestQueryNodeUsesJobDefaultWhenAgentIdentityIsUnconfigured(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	stub := &stubAgentCaller{response: "ok"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent"}, // no "agent" parameter set
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if stub.gotSlug != "" {
		t.Fatalf("expected an empty slug (job default) when the agent identity has none set, got %q", stub.gotSlug)
	}
}

func TestValidateRejectsQueryNodeWithNoAgent(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "q", Type: "query", Parameters: map[string]any{"prompt": "x"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected an error for a query node with no Agent reference")
	}
}

func TestValidateRejectsQueryAgentReferencingUnknownNode(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "q", Type: "query", Agent: "missing", Parameters: map[string]any{"prompt": "x"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected an error for a query node referencing a non-existent agent")
	}
}

func TestValidateRejectsQueryAgentReferencingNonAgentNode(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "w", Type: "wait"},
		{ID: "q", Type: "query", Agent: "w", Parameters: map[string]any{"prompt": "x"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected an error for a query node referencing a non-agent-typed node")
	}
}

func TestValidateRejectsAgentSetOnNonQueryNode(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "w", Type: "wait", Agent: "id"},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected an error for a non-query node setting Agent")
	}
}

func TestSubworkflowNodeDelegatesToPkgWorkflow(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := workflow.Definition{Name: "chain", Nodes: []workflow.Node{{ID: "n1", Prompt: "do it"}}}
	stub := &stubSubworkflowRunner{result: workflow.Result{
		Success: true,
		Results: map[string]workflow.NodeResult{"n1": {ID: "n1", Success: true, Output: "done"}},
		Order:   []string{"n1"},
	}}
	rt := &Runtime{Subworkflow: stub}

	graph := Definition{Nodes: []Node{
		{ID: "sub", Type: "subworkflow", Parameters: map[string]any{"definition": def}},
	}}
	result, err := Run(context.Background(), graph, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if stub.gotDef.Name != "chain" {
		t.Fatalf("expected subworkflow to receive the definition, got %#v", stub.gotDef)
	}
	out := result.Results["sub"].Output
	if len(out) != 1 || out[0]["node_id"] != "n1" {
		t.Fatalf("unexpected output: %#v", out)
	}
}
