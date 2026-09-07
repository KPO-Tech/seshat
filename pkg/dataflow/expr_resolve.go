package dataflow

import (
	"context"
	"strings"
	"time"
)

// ExpressionEvaluator is satisfied by *pkg/dataflow/expr.Pool as-is (no
// changes needed there) - an interface here rather than a hard dependency,
// the same "compose through an interface" split Runtime already uses for
// Secrets/Agent/Subworkflow, so core dataflow never needs to import the
// expr package.
type ExpressionEvaluator interface {
	Eval(ctx context.Context, source string, bindings map[string]any) (any, error)
}

// ResolveValue is dataflow's equivalent of n8n's getNodeParameter: a node's
// own Execute calls this (as many times as it wants, for whichever items it
// wants - Execute's own signature never changes) instead of reading a raw
// parameter directly, to get expression support for free. raw is returned
// unresolved when it isn't a string, doesn't start with "=" (mirrors n8n's
// own isExpression() convention, and this SDK's pre-existing "set" node's
// own hand-rolled version of the same idea), or when rt/rt.Expr is nil
// (expressions not wired up by the caller - not an error, just "off").
//
// Bindings available inside the expression: $json (item), $itemIndex,
// $now/$today (native JS Date), $node(name) - a callable returning
// {json, all} for a node that has already executed (see
// Runtime.NodeOutput) - and $vars (the caller's organization-scoped named
// values, Tier 3.2 - see Runtime.Vars/VarsMap). Deliberately no $prevNode:
// that needs per-item upstream source (NodeResult.InputSource, Tier 1.3),
// which the engine only knows after a node returns, not during its call -
// exposing it would mean changing Execute's signature, which this design
// avoids. $node('Name') covers the actual need.
func ResolveValue(ctx context.Context, rt *Runtime, raw any, item Item, itemIndex int) (any, error) {
	s, ok := raw.(string)
	if !ok || !strings.HasPrefix(s, "=") || rt == nil || rt.Expr == nil {
		return raw, nil
	}
	now := time.Now()
	bindings := map[string]any{
		"$json":      map[string]any(item),
		"$itemIndex": itemIndex,
		"$now":       now,
		"$today":     now.Truncate(24 * time.Hour),
		"$node":      rt.NodeAccessor(),
		"$vars":      rt.VarsMap(),
	}
	return rt.Expr.Eval(ctx, strings.TrimPrefix(s, "="), bindings)
}

// ResolveParam is ResolveValue(ctx, rt, params[key], item, itemIndex) - the
// common case of resolving one top-level parameter.
func ResolveParam(ctx context.Context, rt *Runtime, params map[string]any, key string, item Item, itemIndex int) (any, error) {
	return ResolveValue(ctx, rt, params[key], item, itemIndex)
}

// NodeAccessor returns the Go closure bound as the JS-callable $node(name)
// function - exported so a node type building its own bindings map (e.g.
// filter/if/switch, which evaluate a whole-field expression rather than
// going through ResolveValue) can offer $node too without duplicating this
// logic. A nil rt still returns a working (always-empty) accessor so
// ResolveValue's own nil-rt short-circuit is the only nil check callers
// need.
func (rt *Runtime) NodeAccessor() func(string) map[string]any {
	return func(name string) map[string]any {
		items, ok := rt.NodeOutput(name)
		if !ok || len(items) == 0 {
			return map[string]any{"json": nil, "all": []any{}}
		}
		all := make([]any, len(items))
		for i, it := range items {
			all[i] = map[string]any(it)
		}
		return map[string]any{"json": map[string]any(items[0]), "all": all}
	}
}
