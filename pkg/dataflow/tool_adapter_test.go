package dataflow

import (
	"context"
	"strings"
	"testing"
)

func TestToolOnlyNodeNeverRunsEagerly(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	stub := &stubAgentCaller{response: "done"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "agent1", Type: "agent", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"echo"}},
		{ID: "echo", Type: "echo"},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	for _, id := range result.Order {
		if id == "echo" {
			t.Fatalf("tool-only node must not run eagerly, got Order=%v", result.Order)
		}
	}
	if len(stub.gotGraphTools) != 1 || stub.gotGraphTools[0].Name != "echo" {
		t.Fatalf("expected agent to receive echo as a graphTool, got %#v", stub.gotGraphTools)
	}
}

func TestToolAlsoWiredIntoConnectionsStillRunsNormally(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	reg.Register("start", passthrough("start"))
	rt := &Runtime{Agent: &stubAgentCaller{response: "done"}}

	def := Definition{Nodes: []Node{
		{ID: "start", Type: "start", Connections: map[string][]string{"main": {"echo"}}},
		{ID: "echo", Type: "echo"},
		{ID: "agent1", Type: "agent", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"echo"}},
	}}
	result, err := Run(context.Background(), def, reg, rt, []Item{{"x": 1}}, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	found := false
	for _, id := range result.Order {
		if id == "echo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dual-use node must still run normally, got Order=%v", result.Order)
	}
}

func TestValidateRejectsToolsOnNonAgentNode(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "wait", Tools: []string{"b"}},
		{ID: "b", Type: "wait"},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for tools set on a non-agent node")
	}
}

func TestValidateRejectsUnknownToolsTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Tools: []string{"missing"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for a tools target that doesn't exist")
	}
}

func TestValidateRejectsAgentAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Tools: []string{"b"}},
		{ID: "b", Type: "agent"},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for an agent node as a tools target")
	}
}

func TestValidateRejectsSelfAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Tools: []string{"a"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for a node listing itself as a tool")
	}
}

func TestRunRejectsTriggerAsToolTarget(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("trig", funcExecutor{
		desc: NodeDescription{Type: "trig", IsTrigger: true},
		execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
			return Main(input), nil
		},
	})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"t"}},
		{ID: "t", Type: "trig"},
	}}
	if _, err := Run(context.Background(), def, reg, &Runtime{Agent: &stubAgentCaller{}}, nil, Options{}); err == nil {
		t.Fatal("expected error for a trigger node as a tools target")
	}
}

func TestRunRejectsLogicAsToolTarget(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("branch", funcExecutor{
		desc: NodeDescription{Type: "branch", Category: "Logic"},
		execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
			return Main(input), nil
		},
	})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"b"}},
		{ID: "b", Type: "branch"},
	}}
	if _, err := Run(context.Background(), def, reg, &Runtime{Agent: &stubAgentCaller{}}, nil, Options{}); err == nil {
		t.Fatal("expected error for a logic node as a tools target")
	}
}

func TestBuildAgentToolsSchemaMatchesNodeProperties(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("widget", funcExecutor{
		desc: NodeDescription{Type: "widget", Description: "does widget things", Properties: []NodeProperty{
			{Name: "query", Type: PropString, Required: true, Description: "the query"},
			{Name: "limit", Type: PropNumber},
			{Name: "flag", Type: PropBoolean},
			{Name: "mode", Type: PropOptions, Options: []NodePropertyOption{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}},
			{Name: "raw", Type: PropJSON},
			{Name: "secret", Type: PropSecretRef},
		}},
		execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) { return Main(nil), nil },
	})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Tools: []string{"w"}},
		{ID: "w", Type: "widget"},
	}}
	specs, err := BuildAgentTools(def, reg, &Runtime{}, "a")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "w" || spec.Description != "does widget things" {
		t.Fatalf("unexpected spec: %#v", spec)
	}
	props, _ := spec.Schema["properties"].(map[string]any)
	if _, ok := props["secret"]; ok {
		t.Fatal("PropSecretRef must not appear in the exposed schema")
	}
	if len(props) != 5 {
		t.Fatalf("expected 5 exposed properties, got %d: %#v", len(props), props)
	}
	required, _ := spec.Schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("expected only 'query' required, got %#v", required)
	}
	mode, _ := props["mode"].(map[string]any)
	enum, _ := mode["enum"].([]string)
	if len(enum) != 2 || enum[0] != "a" || enum[1] != "b" {
		t.Fatalf("unexpected enum for options property: %#v", enum)
	}
}

func TestBuildAgentToolsEmptyForNoTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := Definition{Nodes: []Node{{ID: "a", Type: "agent"}}}
	specs, err := BuildAgentTools(def, reg, &Runtime{}, "a")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if specs != nil {
		t.Fatalf("expected nil specs for an agent with no Tools, got %#v", specs)
	}
}

func TestToolSpecInvokeMergesArgsOverParams(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", funcExecutor{
		desc: NodeDescription{Type: "echo"},
		execute: func(_ context.Context, _ *Runtime, _ []Item, params map[string]any) (Output, error) {
			return Main([]Item{{"got": params["msg"]}}), nil
		},
	})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "agent", Tools: []string{"e"}},
		{ID: "e", Type: "echo", Parameters: map[string]any{"msg": "default"}},
	}}
	specs, err := BuildAgentTools(def, reg, &Runtime{}, "a")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	result, err := specs[0].Invoke(context.Background(), map[string]any{"msg": "override"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(result, "override") {
		t.Fatalf("expected result to reflect the overridden arg, got %q", result)
	}
}
