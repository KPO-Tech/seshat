package dataflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/pkg/workflow"
)

// RegisterBuiltins registers the "agent", "query", "tools" and "subworkflow"
// node types on registry — the node kinds that bridge into the LLM side
// (a single scoped agent turn, or a full pkg/workflow multi-agent chain)
// rather than running deterministic code. Callers still need to register
// whatever deterministic node types (http_request, filter, ...) their graph
// uses.
//
// agent/query/tools together replace what used to be one catch-all "agent"
// node (automation-app-pages.md §37.9): agent is pure identity (which local
// persona), query is the task that actually runs against a connected
// agent's identity, and tools is a reusable, individually-toggleable group
// of capability nodes a query may call. This is a breaking change to the
// old single-node shape - its "prompt"/"tools" parameters moved to query.
func RegisterBuiltins(registry *Registry) {
	registry.Register("agent", agentNode{})
	registry.Register("query", queryNode{})
	registry.Register("tools", toolsNode{})
	registry.Register("subworkflow", subworkflowNode{})
}

// agentNode declares a local workflow persona's identity - nothing more. It
// never runs an LLM turn itself (Execute is a no-op); a connected "query"
// node reads its "agent" parameter by direct ID lookup (see
// resolveAgentSlug), not through Connections/scheduling, since this is a
// structural reference, not data flow.
type agentNode struct{}

func (agentNode) Description() NodeDescription {
	return NodeDescription{Type: "agent", Name: "Agent", Category: "AI",
		Description: "Declares a local workflow persona's identity - which agent a connected \"query\" node runs its prompt as. Does nothing on its own; a \"query\" node's Agent field references this node's ID. " +
			"Parameters: agent (string, required) — the persona's slug (created/picked via the authoring UI's agent picker).",
		Properties: []NodeProperty{
			{Name: "agent", DisplayName: "Agent", Type: PropString, Required: true,
				Description: "The persona's slug, created/picked via the authoring UI's agent picker."},
		}}
}

func (agentNode) ValidateParameters(params map[string]any) error {
	if StringParam(params, "agent", "") == "" {
		return errors.New("agent is required")
	}
	return nil
}

func (agentNode) Execute(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
	return Main(nil), nil
}

// queryNode runs a single prompt through Runtime.Agent, as the agent
// identity its own Agent field references — the deterministic graph's
// equivalent of one `internal/automation.Job.Task` turn, usable as one step
// among others instead of the whole job.
type queryNode struct{}

func (queryNode) Description() NodeDescription {
	return NodeDescription{Type: "query", Name: "Query", Category: "AI",
		Description: "Runs a single prompt against a connected \"agent\" node's identity and returns its text output as one item (field \"text\"). " +
			"Parameters: prompt (string, required) — the instruction to run; upstream input items (if any) are appended as JSON context automatically. " +
			"tools (string, optional) — comma-separated tool names to scope this turn to; empty uses the resolved agent's own configured tools. " +
			"Requires an Agent field (the connected \"agent\" node's ID) — a query with no agent identity to run as is invalid.",
		// "tools" (tool names, not to be confused with the Tools field
		// referencing other nodes as callable tools) can't be a real dropdown
		// here - the set of valid names is only known at runtime/per-tenant,
		// and Description() is a stateless method with no access to that.
		Properties: []NodeProperty{
			{Name: "prompt", DisplayName: "Prompt", Type: PropText, Required: true,
				Description: "Upstream input items (if any) are appended as JSON context automatically."},
			{Name: "tools", DisplayName: "Tools", Type: PropString,
				Description: "Comma-separated tool names to scope this turn to; empty uses the resolved agent's own configured tools."},
		}}
}

func (queryNode) ValidateParameters(params map[string]any) error {
	if StringParam(params, "prompt", "") == "" {
		return errors.New("prompt is required")
	}
	return nil
}

func (queryNode) Execute(ctx context.Context, rt *Runtime, input []Item, params map[string]any) (Output, error) {
	if rt == nil || rt.Agent == nil {
		return Output{}, errors.New("dataflow: no AgentCaller configured on Runtime")
	}
	gc, ok := graphContextFrom(ctx)
	if !ok {
		return Output{}, errors.New("dataflow: query node requires the graph context Run provides - it cannot be executed standalone")
	}
	agentSlug, err := resolveAgentSlug(gc.def, gc.nodeID)
	if err != nil {
		return Output{}, fmt.Errorf("query node: %w", err)
	}
	prompt := StringParam(params, "prompt", "")
	tools := StringListParam(params, "tools")
	graphTools, err := ResolveTools(gc.def, gc.registry, rt, gc.nodeID)
	if err != nil {
		return Output{}, fmt.Errorf("query node: %w", err)
	}
	output, err := rt.Agent.Ask(ctx, agentSlug, buildAgentPrompt(prompt, input), tools, graphTools)
	if err != nil {
		return Output{}, fmt.Errorf("query node: %w", err)
	}
	return Main([]Item{{"text": output}}), nil
}

// resolveAgentSlug looks up queryNodeID's own Agent reference and returns
// the referenced "agent" node's own "agent" parameter (its persona slug) -
// a direct Definition lookup, not Connections/scheduling, mirroring
// ResolveTools' own lookup of tool targets. Validate already guarantees
// Agent is set and points at a real "agent"-typed node for any query node
// that made it into a Run, so the error paths here are defensive, not the
// primary way this gets caught.
func resolveAgentSlug(def Definition, queryNodeID string) (string, error) {
	var query *Node
	for i := range def.Nodes {
		if def.Nodes[i].ID == queryNodeID {
			query = &def.Nodes[i]
			break
		}
	}
	if query == nil || query.Agent == "" {
		return "", fmt.Errorf("no Agent reference set on node %q", queryNodeID)
	}
	for _, node := range def.Nodes {
		if node.ID == query.Agent {
			return StringParam(node.Parameters, "agent", ""), nil
		}
	}
	return "", fmt.Errorf("agent reference %q not found in this graph", query.Agent)
}

// toolsNode declares a reusable, individually-toggleable group of
// capability nodes - its own Tools field lists the member node IDs, and its
// "disabled" parameter names which of those are temporarily off without
// removing the wiring. Several "query" nodes can reference the same
// "tools" node's ID to share one capability set instead of each repeating
// the same list (see ResolveTools). Never runs an LLM turn or any real
// work itself - Execute is a no-op, same reasoning as agentNode.
type toolsNode struct{}

func (toolsNode) Description() NodeDescription {
	return NodeDescription{Type: "tools", Name: "Tools", Category: "AI",
		Description: "A reusable, individually-toggleable group of capability nodes a \"query\" node may call as tools - its own Tools field (not a parameter - the same graph-level field a query node itself can set) lists the member node IDs. " +
			"Parameters: disabled (string, optional) — comma-separated node IDs from this node's own Tools list to temporarily exclude without removing the wiring.",
		Properties: []NodeProperty{
			{Name: "disabled", DisplayName: "Disabled", Type: PropString,
				Description: "Comma-separated node IDs from this node's own Tools list to temporarily exclude without removing the wiring."},
		}}
}

func (toolsNode) ValidateParameters(map[string]any) error { return nil }

func (toolsNode) Execute(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
	return Main(nil), nil
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
