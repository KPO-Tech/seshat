// Package dataflow is a deterministic node-graph execution engine: HTTP
// calls, filters, data transforms, database queries, and similar steps that
// don't need an LLM turn to run. It complements pkg/workflow (a multi-agent
// DAG where every node is a Prompt) rather than replacing it — a dataflow
// graph can invoke a pkg/workflow.Definition as a single "subworkflow" node
// (see builtin.go) when a step genuinely needs judgment, without either
// engine reimplementing the other's execution model.
package dataflow

import "time"

// Definition is a node graph: which node types run, their parameters, and
// how each node's output routes to downstream nodes.
type Definition struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Nodes       []Node `json:"nodes" yaml:"nodes"`
	// PinnedData freezes a node's output to a fixed set of items instead of
	// actually running it - Run checks this (keyed by node ID) before
	// invoking a node's own Execute, mirroring n8n's IPinData. Kept
	// separate from each Node's own Parameters, since "frozen test output"
	// is a different concept from real node configuration - a pinned
	// node's ValidateParameters/Execute (and even its registry lookup) are
	// never called at all, so a graph can be tested with a node whose real
	// config is incomplete, or whose type isn't implemented yet.
	PinnedData map[string][]Item `json:"pinned_data,omitempty" yaml:"pinned_data,omitempty"`
}

// Node is one step in the graph. Type selects the NodeExecutor (registered
// by name in a Registry, e.g. "http_request", "if", "agent"). Connections
// maps an output port name to the IDs of nodes that receive items emitted on
// that port — most node types only ever emit on "main"; conditional nodes
// (if/switch) emit on named ports like "true"/"false" instead.
type Node struct {
	ID          string              `json:"id" yaml:"id"`
	Type        string              `json:"type" yaml:"type"`
	Parameters  map[string]any      `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Connections map[string][]string `json:"connections,omitempty" yaml:"connections,omitempty"`
	// RetryOnFail/MaxTries/WaitBetweenTriesMS/OnError (Tier 3.1) are real
	// production behavior - not test/dev overrides like PinnedData - so
	// they're siblings of Parameters/Connections here rather than a
	// separate top-level map, mirroring n8n's own INode shape exactly.
	// RetryOnFail off (the default, and every graph saved before this field
	// existed) means Execute is called exactly once, today's existing
	// behavior. MaxTries is meaningful only when RetryOnFail is true, and
	// clamped 2-5 by the engine; WaitBetweenTriesMS is clamped 0-5000.
	RetryOnFail        bool `json:"retry_on_fail,omitempty" yaml:"retry_on_fail,omitempty"`
	MaxTries           int  `json:"max_tries,omitempty" yaml:"max_tries,omitempty"`
	WaitBetweenTriesMS int  `json:"wait_between_tries_ms,omitempty" yaml:"wait_between_tries_ms,omitempty"`
	// OnError decides what happens once every retry attempt (or the single
	// attempt, if RetryOnFail is off) has failed: "" (unset, the default -
	// today's existing behavior: NodeResult.Success=false, downstream
	// dependents skipped), "continueRegularOutput" (proceed with empty
	// output as if nothing happened), or "continueErrorOutput" (route a
	// single synthesized {"error": message} item onto a new "error" port,
	// letting a downstream branch handle the failure explicitly). Retry and
	// OnError are independent: a node can continue-on-error without ever
	// retrying, or retry a few times and still stop the run.
	OnError string `json:"on_error,omitempty" yaml:"on_error,omitempty"`
	// Tools lists the IDs of other nodes this node (which must be type
	// "query" or "tools" - enforced by Validate) may invoke as LLM-callable
	// tools during its turn, in addition to (never instead of) whatever it's
	// wired to via Connections. A node named here that has no real
	// Connections of its own stays dormant - the engine never schedules it
	// eagerly (see topologicalLevels) - and only runs when the query's LLM
	// actually calls it (see ResolveTools). A node that's both wired into
	// Connections *and* named in some query's Tools keeps running in its
	// normal sequential position too - dual-use, purely additive
	// (automation-app-pages.md §37.9). A target of type "tools" is expanded
	// recursively rather than called directly - see ResolveTools - letting
	// several "query" nodes share the same reusable, individually-toggleable
	// tool group instead of each repeating the same list of IDs.
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	// Agent is the ID of the "agent"-type node providing this "query" node's
	// persona - required on every "query" node (enforced by Validate), never
	// set on any other type. A plain ID reference rather than a Connections
	// edge, same reasoning as Tools above: it's a structural relationship
	// ("which identity do I run as"), not data flow, so the referenced agent
	// node is looked up directly rather than needing to run/be scheduled at
	// all.
	Agent string `json:"agent,omitempty" yaml:"agent,omitempty"`
}

// Item is one record flowing through the graph — the dataflow analog of a
// single JSON object. A node's input/output is always a slice of Item, never
// a single bare value, so merge/fan-out behave uniformly across node types.
type Item map[string]any

// Output is what a node execution produces, split by output port. Use
// Main(items) for the common single-port case.
type Output struct {
	Ports map[string][]Item
}

// Main wraps items as a single-port ("main") Output — the shape almost every
// node type other than a conditional (if/switch) needs.
func Main(items []Item) Output {
	return Output{Ports: map[string][]Item{"main": items}}
}

// ItemSource identifies where one received input item came from - which
// upstream node produced it and on which output port. A zero value
// (Node == "") means the item came from this run's own seed input (Run's
// `input` parameter), not from another node.
type ItemSource struct {
	Node string `json:"node,omitempty"`
	Port string `json:"port,omitempty"`
}

// NodeResult is the recorded outcome of one node's execution within a Run.
type NodeResult struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Skipped bool   `json:"skipped,omitempty"`
	// Pinned is true when this result came from Definition.PinnedData
	// rather than a real Execute call - lets a trace/UI consumer tell
	// "this node actually ran" apart from "this used frozen test data".
	Pinned bool `json:"pinned,omitempty"`
	// Continued is true when this node failed (Success=false) but its own
	// OnError setting (Tier 3.1) let the run proceed anyway rather than
	// stopping - kept distinct from Success so a trace/UI consumer can
	// still tell "this genuinely failed" apart from "this failed but the
	// run kept going", the same distinction n8n's own execution view makes.
	// Run's own downstream-skip/routing logic treats Continued the same as
	// a real success (see engine.go).
	Continued bool `json:"continued,omitempty"`
	// Attempts is how many times Execute was actually called for this node
	// - 1 unless RetryOnFail was set and at least one attempt failed.
	Attempts int `json:"attempts,omitempty"`
	// Input/InputSource are exactly what this node received and, index for
	// index, where each item came from - populated by Run's own scheduling
	// loop (see engine.go), never by the node's own Execute, so every node
	// type gets this for free with no interface change.
	Input       []Item       `json:"input,omitempty"`
	InputSource []ItemSource `json:"input_source,omitempty"`
	// Output is the same single-port view every existing consumer already
	// reads (unchanged) - OutputByPort is the real per-port breakdown Output
	// collapses away (see executeNode).
	Output       []Item            `json:"output,omitempty"`
	OutputByPort map[string][]Item `json:"output_by_port,omitempty"`
	Error        string            `json:"error,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	EndedAt      time.Time         `json:"ended_at"`
	Duration     time.Duration     `json:"duration"`
}

// Result is the outcome of a full graph Run.
type Result struct {
	Name      string                `json:"name"`
	Success   bool                  `json:"success"`
	StartedAt time.Time             `json:"started_at"`
	EndedAt   time.Time             `json:"ended_at"`
	Duration  time.Duration         `json:"duration"`
	Results   map[string]NodeResult `json:"results"`
	// Order is execution order, in the same level-batched order Run used —
	// mirrors pkg/workflow.Result.Order.
	Order []string `json:"order"`
}
