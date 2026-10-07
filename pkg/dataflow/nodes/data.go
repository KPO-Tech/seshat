package nodes

import (
	"context"
	"strings"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/dataflow/expr"
)

// Set is the "set" node type: adds/overwrites fields on every item.
// Parameters: fields (map[string]any) — a string value starting with "="
// (n8n's convention for "this is an expression, not a literal") is
// evaluated per item with $json bound to the item being modified;
// everything else is copied through as a literal.
type Set struct{ pool *expr.Pool }

// NewSet creates a Set node that evaluates its "="-prefixed field values with pool.
func NewSet(pool *expr.Pool) *Set { return &Set{pool: pool} }

// Description returns the catalog entry of the set node: it adds or overwrites fields on every item, a string value that starts with "=" being evaluated as a JS expression.
func (n *Set) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "set", Name: "Set", Category: "Data",
		Description: "Adds or overwrites fields on every item, keeping existing fields not mentioned. " +
			"Parameters: fields (object, required) — field name -> value; a string value starting with \"=\" is evaluated as a JS expression (current item bound as $json, e.g. \"=$json.n * 2\"), anything else (including a plain string) is used as a literal."}
}

// ValidateParameters accepts any parameters: without a fields object the items go through unchanged.
func (n *Set) ValidateParameters(map[string]any) error { return nil }

// Execute returns the items with the fields added or overwritten and the other fields kept. A field value that is a string starting with "=" is evaluated as a JS expression with the item bound as $json; any other value is used as it is.
func (n *Set) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	fields, _ := params["fields"].(map[string]any)
	out := make([]dataflow.Item, 0, len(input))
	for i, item := range input {
		next := make(dataflow.Item, len(item)+len(fields))
		for k, v := range item {
			next[k] = v
		}
		for k, v := range fields {
			value, err := n.resolve(ctx, rt, v, item, i)
			if err != nil {
				return dataflow.Output{}, err
			}
			next[k] = value
		}
		out = append(out, next)
	}
	return dataflow.Main(out), nil
}

// resolve is dataflow.ResolveValue when the caller has wired Runtime.Expr,
// falling back to this node's own pool (exactly the pre-Tier-2.1 behavior:
// $json-bound only) when it hasn't - a caller that constructs its own
// Runtime without setting Expr (e.g. a service that hasn't adopted the new
// field yet) must not see its existing "=" expressions in "set" silently
// stop evaluating.
func (n *Set) resolve(ctx context.Context, rt *dataflow.Runtime, raw any, item dataflow.Item, itemIndex int) (any, error) {
	s, ok := raw.(string)
	if !ok || !strings.HasPrefix(s, "=") {
		return raw, nil
	}
	if rt != nil && rt.Expr != nil {
		return dataflow.ResolveValue(ctx, rt, raw, item, itemIndex)
	}
	return n.pool.Eval(ctx, strings.TrimPrefix(s, "="), map[string]any{"$json": map[string]any(item)})
}

// Merge is the "merge" node type. The engine already concatenates items
// from every upstream connection into one input slice (see engine.go), so
// by default this is an identity pass-through — its value is as an explicit,
// labeled join point in a graph rather than as a transformation. Fancier
// merge strategies (pair-by-index, join-by-key) are deliberately not built
// yet; add them here if/when a workflow actually needs one.
type Merge struct{}

// NewMerge creates a Merge node.
func NewMerge() Merge { return Merge{} }

// Description returns the catalog entry of the merge node: it joins the items of several upstream branches into one list.
func (Merge) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "merge", Name: "Merge", Category: "Data",
		Description: "Joins items from multiple upstream branches into one list — connect two or more nodes' outputs to this node's id to combine them; it needs no parameters of its own, the engine already concatenates every input a node receives. Use it as an explicit join point when a graph branches then needs to reconverge. " +
			"Parameters: none."}
}

// ValidateParameters accepts any parameters: the node has none.
func (Merge) ValidateParameters(map[string]any) error { return nil }

// Execute passes its input on unchanged, as the engine has already concatenated the items of the branches connected to the node.
func (Merge) Execute(_ context.Context, _ *dataflow.Runtime, input []dataflow.Item, _ map[string]any) (dataflow.Output, error) {
	return dataflow.Main(input), nil
}
