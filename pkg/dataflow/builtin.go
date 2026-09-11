package dataflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/pkg/workflow"
)

// RegisterBuiltins registers the "agent" and "subworkflow" node types on
// registry — the two node kinds that bridge into the LLM side (a single
// scoped agent turn, or a full pkg/workflow multi-agent chain) rather than
// running deterministic code. Callers still need to register whatever
// deterministic node types (http_request, filter, ...) their graph uses.
func RegisterBuiltins(registry *Registry) {
	registry.Register("agent", agentNode{})
	registry.Register("subworkflow", subworkflowNode{})
}

// agentNode runs a single prompt through Runtime.Agent — the deterministic
// graph's equivalent of one `internal/automation.Job.Task` turn, usable as
// one step among others instead of the whole job.
type agentNode struct{}

func (agentNode) Description() NodeDescription {
	return NodeDescription{Type: "agent", Name: "Agent", Category: "AI",
		Description: "Runs a single prompt through a scoped agent turn and returns its text output as one item (field \"text\"). " +
			"Parameters: prompt (string, required) — the instruction to run; upstream input items (if any) are appended as JSON context automatically. " +
			"agent (string, optional) — agent slug/persona to run as; empty uses the workflow's default agent. " +
			"tools (string, optional) — comma-separated tool names to scope this turn to; empty uses the resolved agent's own configured tools. " +
			"Setting either agent or tools to something other than the workflow's default runs this node as an isolated turn (no shared conversation with other nodes) instead of continuing the job's own session.",
		// "agent" (persona slug) and "tools" (tool names) can't be real
		// dropdowns here - the set of valid slugs/tool names is only known at
		// runtime/per-tenant, and Description() is a stateless method with no
		// access to that. Both stay plain string fields; the authoring UI
		// can't offer a picker for either yet (tools is comma-separated,
		// parsed via StringListParam - same convention as Automation's own
		// job-level Tags field).
		Properties: []NodeProperty{
			{Name: "prompt", DisplayName: "Prompt", Type: PropText, Required: true,
				Description: "Upstream input items (if any) are appended as JSON context automatically."},
			{Name: "agent", DisplayName: "Agent", Type: PropString,
				Description: "Agent slug/persona to run as; empty uses the workflow's default agent."},
			{Name: "tools", DisplayName: "Tools", Type: PropString,
				Description: "Comma-separated tool names to scope this turn to; empty uses the resolved agent's own configured tools."},
		}}
}

func (agentNode) ValidateParameters(params map[string]any) error {
	if StringParam(params, "prompt", "") == "" {
		return errors.New("prompt is required")
	}
	return nil
}

func (agentNode) Execute(ctx context.Context, rt *Runtime, input []Item, params map[string]any) (Output, error) {
	if rt == nil || rt.Agent == nil {
		return Output{}, errors.New("dataflow: no AgentCaller configured on Runtime")
	}
	prompt := StringParam(params, "prompt", "")
	agentSlug := StringParam(params, "agent", "")
	tools := StringListParam(params, "tools")
	var graphTools []ToolSpec
	if gc, ok := graphContextFrom(ctx); ok {
		var err error
		graphTools, err = BuildAgentTools(gc.def, gc.registry, rt, gc.nodeID)
		if err != nil {
			return Output{}, fmt.Errorf("agent node: %w", err)
		}
	}
	output, err := rt.Agent.Ask(ctx, agentSlug, buildAgentPrompt(prompt, input), tools, graphTools)
	if err != nil {
		return Output{}, fmt.Errorf("agent node: %w", err)
	}
	return Main([]Item{{"text": output}}), nil
}

// buildAgentPrompt appends upstream items to prompt as JSON context, so an
// "agent" node fed by a deterministic node (http_request, filter, a
// SecretRef-backed database query, ...) can actually act on that data
// instead of only ever seeing its own static prompt parameter.
func buildAgentPrompt(prompt string, input []Item) string {
	if len(input) == 0 {
		return prompt
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\nInput data:\n")
	b.Write(encoded)
	return b.String()
}

// subworkflowNode delegates to pkg/workflow.Run for a step that needs
// multi-agent orchestration (triage -> draft -> verify) rather than one
// prompt — pkg/workflow keeps its own execution model unchanged; this node
// just embeds it as one step in a larger deterministic graph.
type subworkflowNode struct{}

func (subworkflowNode) Description() NodeDescription {
	return NodeDescription{Type: "subworkflow", Name: "Sub-workflow", Category: "AI",
		Description: "Runs a pkg/workflow multi-agent chain (nodes with dependencies, routing) as one step, for a sub-task that itself needs several agent turns (e.g. triage -> draft -> verify). Output is one item per sub-workflow node, in execution order (fields \"node_id\", \"agent\", \"output\"). " +
			"Parameters: definition (workflow.Definition, required) — the sub-chain: {name, nodes: [{id, kind (agent|verifier|critic|router), prompt, needs: [id...], routes: [id...]}]}. See the SDK's workflow_draft tool for the exact shape."}
}

func (subworkflowNode) ValidateParameters(params map[string]any) error {
	def, ok := params["definition"].(workflow.Definition)
	if !ok {
		return errors.New("definition (workflow.Definition) is required")
	}
	return workflow.Validate(def)
}

func (subworkflowNode) Execute(ctx context.Context, rt *Runtime, input []Item, params map[string]any) (Output, error) {
	if rt == nil || rt.Subworkflow == nil {
		return Output{}, errors.New("dataflow: no SubworkflowRunner configured on Runtime")
	}
	def, _ := params["definition"].(workflow.Definition)
	result, err := rt.Subworkflow.Run(ctx, def)
	if err != nil {
		return Output{}, fmt.Errorf("subworkflow node: %w", err)
	}
	if !result.Success {
		return Output{}, fmt.Errorf("subworkflow node: %s failed", def.Name)
	}
	items := make([]Item, 0, len(result.Order))
	for _, id := range result.Order {
		nr := result.Results[id]
		items = append(items, Item{"node_id": id, "agent": nr.Agent, "output": nr.Output})
	}
	return Main(items), nil
}
