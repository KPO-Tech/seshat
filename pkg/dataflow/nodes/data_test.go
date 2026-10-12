package nodes

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow/expr"
)

func TestSetAddsLiteralAndExpressionFields(t *testing.T) {
	n := NewSet(expr.NewPool(2))
	out, err := n.Execute(context.Background(), nil, []dataflow.Item{{"n": 3}}, map[string]any{
		"fields": map[string]any{
			"label":  "constant",
			"double": "=$json.n * 2",
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	item := out.Ports["main"][0]
	if item["n"] != 3 {
		t.Fatalf("expected original field preserved, got %#v", item)
	}
	if item["label"] != "constant" {
		t.Fatalf("expected literal field, got %#v", item["label"])
	}
	if got := toFloat(t, item["double"]); got != 6 {
		t.Fatalf("expected evaluated field 6, got %#v (%T)", item["double"], item["double"])
	}
}

func TestSetPrefersRuntimeExprAndOffersNodeAndNowBindings(t *testing.T) {
	// Registered with its own pool (as Register always does), but called
	// with a Runtime whose Expr is a *different* pool - the Runtime-level
	// evaluator must win, and it must additionally offer $node/$now/$today,
	// which the node's own pre-Tier-2.1 fallback path does not.
	n := NewSet(expr.NewPool(2))
	rt := &dataflow.Runtime{Expr: expr.NewPool(2)}
	out, err := n.Execute(context.Background(), rt, []dataflow.Item{{"n": 3}}, map[string]any{
		"fields": map[string]any{
			"hasNow":  "=typeof $now !== 'undefined'",
			"hasNode": "=typeof $node === 'function'",
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	item := out.Ports["main"][0]
	if item["hasNow"] != true {
		t.Fatalf("expected $now to be bound via Runtime.Expr, got %#v", item["hasNow"])
	}
	if item["hasNode"] != true {
		t.Fatalf("expected $node to be bound via Runtime.Expr, got %#v", item["hasNode"])
	}
}

func toFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	case int:
		return float64(n)
	default:
		t.Fatalf("expected a numeric value, got %#v (%T)", v, v)
		return 0
	}
}

func TestMergeIsIdentityPassthrough(t *testing.T) {
	n := NewMerge()
	input := []dataflow.Item{{"a": 1}, {"b": 2}}
	out, err := n.Execute(context.Background(), nil, input, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(out.Ports["main"]) != 2 {
		t.Fatalf("expected pass-through of both items, got %#v", out.Ports["main"])
	}
}
