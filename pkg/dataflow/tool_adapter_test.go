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
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "query1", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"echo"}},
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
		t.Fatalf("expected query to receive echo as a graphTool, got %#v", stub.gotGraphTools)
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
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "query1", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"echo"}},
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

func TestValidateRejectsToolsOnNonQueryOrToolsNode(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "wait", Tools: []string{"b"}},
		{ID: "b", Type: "wait"},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for tools set on a node that isn't query or tools")
	}
}

func TestValidateRejectsUnknownToolsTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}, Tools: []string{"missing"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for a tools target that doesn't exist")
	}
}

func TestValidateRejectsAgentAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}, Tools: []string{"id"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for an agent node as a tools target")
	}
}

func TestValidateRejectsQueryAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q1", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}, Tools: []string{"q2"}},
		{ID: "q2", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "y"}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for a query node as a tools target")
	}
}

func TestValidateAllowsToolsNodeAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}, Tools: []string{"grp"}},
		{ID: "grp", Type: "tools"},
	}}
	if err := Validate(def); err != nil {
		t.Fatalf("expected a tools node to be a valid tools target, got %v", err)
	}
}

func TestValidateRejectsSelfAsToolTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "x"}, Tools: []string{"q"}},
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
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"t"}},
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
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"b"}},
		{ID: "b", Type: "branch"},
	}}
	if _, err := Run(context.Background(), def, reg, &Runtime{Agent: &stubAgentCaller{}}, nil, Options{}); err == nil {
		t.Fatal("expected error for a logic node as a tools target")
	}
}

func TestResolveToolsSchemaMatchesNodeProperties(t *testing.T) {
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
		{ID: "q", Type: "query", Tools: []string{"w"}},
		{ID: "w", Type: "widget"},
	}}
	specs, err := ResolveTools(def, reg, &Runtime{}, "q")
	if err != nil {
		t.Fatalf("resolve: %v", err)
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

func TestResolveToolsEmptyForNoTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := Definition{Nodes: []Node{{ID: "q", Type: "query"}}}
	specs, err := ResolveTools(def, reg, &Runtime{}, "q")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if specs != nil {
		t.Fatalf("expected nil specs for a query with no Tools, got %#v", specs)
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
		{ID: "q", Type: "query", Tools: []string{"e"}},
		{ID: "e", Type: "echo", Parameters: map[string]any{"msg": "default"}},
	}}
	specs, err := ResolveTools(def, reg, &Runtime{}, "q")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	result, err := specs[0].Invoke(context.Background(), map[string]any{"msg": "override"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(result, "override") {
		t.Fatalf("expected result to reflect the overridden arg, got %q", result)
	}
}

// --- Phase 3: "tools" hub node (reuse, disable, cycles) ---

func TestResolveToolsExpandsToolsNodeRecursively(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	def := Definition{Nodes: []Node{
		{ID: "q", Type: "query", Tools: []string{"grp"}},
		{ID: "grp", Type: "tools", Tools: []string{"a", "b"}},
		{ID: "a", Type: "echo"},
		{ID: "b", Type: "echo"},
	}}
	specs, err := ResolveTools(def, reg, &Runtime{}, "q")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
	}
	if !names["a"] || !names["b"] || names["grp"] {
		t.Fatalf("expected {a,b} expanded from the tools node, not the hub itself, got %#v", specs)
	}
}

func TestResolveToolsHonorsDisabledOnToolsNode(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	def := Definition{Nodes: []Node{
		{ID: "q", Type: "query", Tools: []string{"grp"}},
		{ID: "grp", Type: "tools", Tools: []string{"a", "b"}, Parameters: map[string]any{"disabled": "b"}},
		{ID: "a", Type: "echo"},
		{ID: "b", Type: "echo"},
	}}
	specs, err := ResolveTools(def, reg, &Runtime{}, "q")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "a" {
		t.Fatalf("expected only 'a' (b disabled), got %#v", specs)
	}
}

func TestResolveToolsSharedAcrossTwoQueries(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	def := Definition{Nodes: []Node{
		{ID: "q1", Type: "query", Tools: []string{"grp"}},
		{ID: "q2", Type: "query", Tools: []string{"grp"}},
		{ID: "grp", Type: "tools", Tools: []string{"a"}},
		{ID: "a", Type: "echo"},
	}}
	for _, qid := range []string{"q1", "q2"} {
		specs, err := ResolveTools(def, reg, &Runtime{}, qid)
		if err != nil {
			t.Fatalf("resolve for %s: %v", qid, err)
		}
		if len(specs) != 1 || specs[0].Name != "a" {
			t.Fatalf("expected %s to share the same tools group, got %#v", qid, specs)
		}
	}
}

func TestResolveToolsRejectsCycle(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	def := Definition{Nodes: []Node{
		{ID: "q", Type: "query", Tools: []string{"g1"}},
		{ID: "g1", Type: "tools", Tools: []string{"g2"}},
		{ID: "g2", Type: "tools", Tools: []string{"g1"}},
	}}
	if _, err := ResolveTools(def, reg, &Runtime{}, "q"); err == nil {
		t.Fatal("expected an error for a tools -> tools cycle")
	}
}

func TestCapabilityNodeReachableOnlyThroughToolsHubNeverRunsEagerly(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	reg.Register("echo", passthrough("echo"))
	stub := &stubAgentCaller{response: "done"}
	rt := &Runtime{Agent: stub}

	def := Definition{Nodes: []Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "q", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}, Tools: []string{"grp"}},
		{ID: "grp", Type: "tools", Tools: []string{"a"}},
		{ID: "a", Type: "echo"},
	}}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	for _, id := range result.Order {
		if id == "a" || id == "grp" {
			t.Fatalf("neither the hub nor its member should run eagerly, got Order=%v", result.Order)
		}
	}
}
