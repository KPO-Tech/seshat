package dataflow

import (
	"context"
	"encoding/json"
	"fmt"
)

// graphContextKey/graphContext thread the running Definition/Registry/
// current-node-ID through ctx rather than widening NodeExecutor.Execute's
// signature (which every node type would need to accept, most never using
// it) - Run wraps ctx with this once per node right before calling
// executeNode (see engine.go), so it's naturally goroutine-safe: each
// concurrently-executing node in a level gets its own derived ctx value,
// never a shared mutable field on Runtime. Only agentNode.Execute reads it
// today, to resolve its own Tools into real ToolSpecs.
type graphContextKey struct{}

type graphContext struct {
	def      Definition
	registry *Registry
	nodeID   string
}

func withGraphContext(ctx context.Context, def Definition, registry *Registry, nodeID string) context.Context {
	return context.WithValue(ctx, graphContextKey{}, graphContext{def: def, registry: registry, nodeID: nodeID})
}

// graphContextFrom returns false for any Execute call Run itself didn't
// make (a node type's own unit test calling Execute directly, for
// instance) - agentNode.Execute treats that as "no graph tools available,
// behave exactly as before Tools existed" rather than an error.
func graphContextFrom(ctx context.Context) (graphContext, bool) {
	gc, ok := ctx.Value(graphContextKey{}).(graphContext)
	return gc, ok
}

// ToolSpec is one node exposed as an LLM-callable tool - the plain-data
// bridge between a graph's Tools reference and a real registrable tool
// (e.g. pkg/sdk.Tool). Deliberately has no dependency on pkg/sdk or
// internal/tools - dataflow stays a deterministic engine package; turning a
// ToolSpec into a real sdk.Tool (internal/tools/registry.NewBuilder(...))
// happens at the AgentCaller implementation's own call site, mirroring how
// internal/tools/system/mcp/wrapper.go wraps an external MCP tool schema.
type ToolSpec struct {
	// Name is the target node's own ID - stable, unique within the graph,
	// and already meaningful to whoever built it (unlike a synthesized
	// name), so it doubles as the tool's name the LLM sees.
	Name        string
	Description string
	// Schema is a JSON-schema object (the same shape sdk.Tool's
	// InputSchema/schema.FromMap expects: {"type":"object","properties":
	// {...},"required":[...]}), built from the target node's own
	// NodeDescription.Properties.
	Schema map[string]any
	// Invoke runs the target node for real with args overriding its own
	// static Parameters (LLM-supplied keys win), and returns its output
	// serialized as JSON text - the same "records, not a single scalar"
	// shape every other dataflow node produces.
	Invoke func(ctx context.Context, args map[string]any) (string, error)
}

// BuildAgentTools resolves agentNodeID's own Tools list into real,
// invocable ToolSpecs - called by agentNode.Execute (see builtin.go) before
// asking rt.Agent. Returns nil (not an error) for an agent node with no
// Tools set, the common case today.
func BuildAgentTools(def Definition, registry *Registry, rt *Runtime, agentNodeID string) ([]ToolSpec, error) {
	var agentNode *Node
	for i := range def.Nodes {
		if def.Nodes[i].ID == agentNodeID {
			agentNode = &def.Nodes[i]
			break
		}
	}
	if agentNode == nil {
		return nil, fmt.Errorf("dataflow: no node %q in this graph", agentNodeID)
	}
	if len(agentNode.Tools) == 0 {
		return nil, nil
	}
	byID := make(map[string]Node, len(def.Nodes))
	for _, node := range def.Nodes {
		byID[node.ID] = node
	}

	specs := make([]ToolSpec, 0, len(agentNode.Tools))
	for _, targetID := range agentNode.Tools {
		target, ok := byID[targetID]
		if !ok {
			return nil, fmt.Errorf("dataflow: node %q tools references unknown node %q", agentNodeID, targetID)
		}
		executor, err := registry.Get(target.Type)
		if err != nil {
			return nil, fmt.Errorf("dataflow: node %q tools references %q: %w", agentNodeID, targetID, err)
		}
		desc := executor.Description()
		spec := ToolSpec{
			Name:        target.ID,
			Description: desc.Description,
			Schema:      nodePropertiesToJSONSchema(desc.Properties),
			Invoke:      invokeNodeAsTool(executor, target, rt),
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// invokeNodeAsTool closes over the resolved executor/node/Runtime so
// BuildAgentTools itself stays a pure lookup+conversion function - the
// actual on-demand execution this returns happens later, only if/when the
// LLM calls it. rt is the same Runtime the rest of this Run uses (secrets,
// expression eval, ...) - a tool-invoked node needs it exactly as much as a
// normally-scheduled one would.
func invokeNodeAsTool(executor NodeExecutor, target Node, rt *Runtime) func(ctx context.Context, args map[string]any) (string, error) {
	return func(ctx context.Context, args map[string]any) (string, error) {
		params := make(map[string]any, len(target.Parameters)+len(args))
		for k, v := range target.Parameters {
			params[k] = v
		}
		for k, v := range args {
			params[k] = v
		}
		if err := executor.ValidateParameters(params); err != nil {
			return "", fmt.Errorf("invalid arguments for %q: %w", target.ID, err)
		}
		// A tool call has no upstream data feed of its own - only its own
		// arguments (already merged into params above) - so input is empty,
		// matching a node run in isolation rather than mid-pipeline.
		output, err := executor.Execute(ctx, rt, nil, params)
		if err != nil {
			return "", err
		}
		items := output.Ports[mainPort]
		if items == nil {
			for _, port := range output.Ports {
				items = append(items, port...)
			}
		}
		encoded, err := json.Marshal(items)
		if err != nil {
			return "", fmt.Errorf("encode %q result: %w", target.ID, err)
		}
		return string(encoded), nil
	}
}

// nodePropertiesToJSONSchema converts a node type's authoring-UI Properties
// into the JSON-schema object an LLM tool call needs - the same field set
// ParameterForm.tsx already renders as a form, viewed from the other side
// (what values a caller may supply) instead of a human filling them in.
// PropSecretRef is deliberately excluded: it's resolved server-side via
// SecretResolver, never something an LLM should see or supply.
func nodePropertiesToJSONSchema(props []NodeProperty) map[string]any {
	properties := make(map[string]any, len(props))
	required := make([]string, 0, len(props))
	for _, p := range props {
		if p.Type == PropSecretRef {
			continue
		}
		field := map[string]any{}
		if p.Description != "" {
			field["description"] = p.Description
		}
		switch p.Type {
		case PropNumber:
			field["type"] = "number"
		case PropBoolean:
			field["type"] = "boolean"
		case PropJSON:
			field["type"] = "object"
		case PropOptions:
			field["type"] = "string"
			if len(p.Options) > 0 {
				enum := make([]string, len(p.Options))
				for i, opt := range p.Options {
					enum[i] = opt.Value
				}
				field["enum"] = enum
			}
		default: // PropString, PropText
			field["type"] = "string"
		}
		properties[p.Name] = field
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
