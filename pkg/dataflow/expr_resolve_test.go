package dataflow

import (
	"context"
	"errors"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/dataflow/expr"
)

// recordingEvaluator captures the last bindings it was called with, so tests
// can assert on exactly what ResolveValue hands the evaluator without
// depending on goja's own semantics.
type recordingEvaluator struct {
	lastSource   string
	lastBindings map[string]any
	result       any
	err          error
}

func (r *recordingEvaluator) Eval(_ context.Context, source string, bindings map[string]any) (any, error) {
	r.lastSource = source
	r.lastBindings = bindings
	return r.result, r.err
}

func TestResolveValueReturnsLiteralUnchangedWhenNotAnExpression(t *testing.T) {
	rt := &Runtime{Expr: &recordingEvaluator{result: "should not be used"}}
	got, err := ResolveValue(context.Background(), rt, "plain string", Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != "plain string" {
		t.Fatalf("expected literal passthrough, got %#v", got)
	}
}

func TestResolveValuePassesThroughNonStringRaw(t *testing.T) {
	rt := &Runtime{Expr: &recordingEvaluator{result: "should not be used"}}
	got, err := ResolveValue(context.Background(), rt, 42, Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected non-string raw to pass through unchanged, got %#v", got)
	}
}

func TestResolveValueEvaluatesLeadingEqualsPrefix(t *testing.T) {
	eval := &recordingEvaluator{result: "resolved"}
	rt := &Runtime{Expr: eval}
	item := Item{"x": 1}
	got, err := ResolveValue(context.Background(), rt, "=$json.x + 1", item, 3)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != "resolved" {
		t.Fatalf("expected the evaluator's result, got %#v", got)
	}
	if eval.lastSource != "$json.x + 1" {
		t.Fatalf("expected the leading '=' to be trimmed before evaluation, got %q", eval.lastSource)
	}
	if eval.lastBindings["$itemIndex"] != 3 {
		t.Fatalf("expected $itemIndex to be passed through, got %#v", eval.lastBindings["$itemIndex"])
	}
	if _, ok := eval.lastBindings["$now"]; !ok {
		t.Fatal("expected $now to be bound")
	}
	if _, ok := eval.lastBindings["$today"]; !ok {
		t.Fatal("expected $today to be bound")
	}
	if _, ok := eval.lastBindings["$node"].(func(string) map[string]any); !ok {
		t.Fatalf("expected $node to be bound as a func(string) map[string]any, got %#v", eval.lastBindings["$node"])
	}
	if _, ok := eval.lastBindings["$vars"].(map[string]string); !ok {
		t.Fatalf("expected $vars to be bound as a map[string]string, got %#v", eval.lastBindings["$vars"])
	}
}

func TestResolveValuePropagatesEvaluatorError(t *testing.T) {
	wantErr := errors.New("boom")
	rt := &Runtime{Expr: &recordingEvaluator{err: wantErr}}
	_, err := ResolveValue(context.Background(), rt, "=1", Item{}, 0)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected evaluator error to propagate, got %v", err)
	}
}

func TestResolveValueSkipsEvaluationWhenRuntimeIsNil(t *testing.T) {
	got, err := ResolveValue(context.Background(), nil, "=$json.x", Item{"x": 1}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != "=$json.x" {
		t.Fatalf("expected a nil Runtime to leave the expression unresolved, got %#v", got)
	}
}

func TestResolveValueSkipsEvaluationWhenExprIsNil(t *testing.T) {
	got, err := ResolveValue(context.Background(), &Runtime{}, "=$json.x", Item{"x": 1}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != "=$json.x" {
		t.Fatalf("expected a nil Runtime.Expr to leave the expression unresolved, got %#v", got)
	}
}

func TestResolveParamResolvesOneKeyFromParams(t *testing.T) {
	eval := &recordingEvaluator{result: "resolved"}
	rt := &Runtime{Expr: eval}
	params := map[string]any{"url": "=$json.id", "method": "GET"}
	got, err := ResolveParam(context.Background(), rt, params, "url", Item{"id": "abc"}, 0)
	if err != nil {
		t.Fatalf("ResolveParam: %v", err)
	}
	if got != "resolved" {
		t.Fatalf("expected resolved value, got %#v", got)
	}
	if eval.lastSource != "$json.id" {
		t.Fatalf("expected params[%q] to be resolved, got source %q", "url", eval.lastSource)
	}

	got, err = ResolveParam(context.Background(), rt, params, "method", Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveParam: %v", err)
	}
	if got != "GET" {
		t.Fatalf("expected a non-'=' param to pass through literally, got %#v", got)
	}
}

func TestRuntimeNodeOutputIsFalseForAnUnrecordedName(t *testing.T) {
	rt := &Runtime{}
	if _, ok := rt.NodeOutput("nope"); ok {
		t.Fatal("expected ok=false for a node that never ran")
	}
}

func TestRuntimeNodeOutputReturnsWhatWasRecorded(t *testing.T) {
	rt := &Runtime{}
	items := []Item{{"foo": "bar"}}
	rt.recordNodeOutput("a", items)
	got, ok := rt.NodeOutput("a")
	if !ok {
		t.Fatal("expected ok=true after recordNodeOutput")
	}
	if len(got) != 1 || got[0]["foo"] != "bar" {
		t.Fatalf("expected the recorded items back, got %#v", got)
	}
}

func TestRuntimeNodeOutputAndRecordAreNilSafe(t *testing.T) {
	var rt *Runtime
	if _, ok := rt.NodeOutput("a"); ok {
		t.Fatal("expected ok=false on a nil Runtime")
	}
	rt.recordNodeOutput("a", []Item{{"x": 1}}) // must not panic
}

// TestVarsMapExposesNamedVariablesToExpressions is an end-to-end proof (real
// expr.Pool, not a mocked evaluator) that $vars.NAME resolves to a value set
// on Runtime.Vars - the actual JS property-access syntax Tier 3.2's
// dataflowvariables.Service.ResolveAll result feeds this with.
func TestVarsMapExposesNamedVariablesToExpressions(t *testing.T) {
	rt := &Runtime{Expr: expr.NewPool(1), Vars: map[string]string{"API_BASE": "https://api.example.com"}}
	got, err := ResolveValue(context.Background(), rt, "=$vars.API_BASE + '/v1'", Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != "https://api.example.com/v1" {
		t.Fatalf("expected $vars.API_BASE to resolve, got %#v", got)
	}
}

// TestVarsMapIsNilSafe proves a nil Runtime.Vars (the default for every
// caller that hasn't wired variable resolution in) makes $vars.ANYTHING
// resolve to undefined rather than erroring - same "off, not broken"
// posture Expr itself already has when unset.
func TestVarsMapIsNilSafe(t *testing.T) {
	rt := &Runtime{Expr: expr.NewPool(1)}
	got, err := ResolveValue(context.Background(), rt, "=$vars.MISSING === undefined", Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if got != true {
		t.Fatalf("expected $vars.MISSING to be undefined on a Runtime with no Vars set, got %#v", got)
	}
}

// TestNodeAccessorSeesUpstreamNodeOutputAfterItCompletes proves $node('a')
// resolves against node "a"'s real, engine-recorded output - end to end
// through Run, a real expr.Pool, and ResolveValue - not just a mocked
// evaluator, so this also guards the Run -> recordNodeOutput wiring itself.
func TestNodeAccessorSeesUpstreamNodeOutputAfterItCompletes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", funcExecutor{desc: NodeDescription{Type: "a"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		return Main([]Item{{"id": "abc123"}}), nil
	}})
	reg.Register("b", funcExecutor{desc: NodeDescription{Type: "b"}, execute: func(ctx context.Context, rt *Runtime, _ []Item, params map[string]any) (Output, error) {
		val, err := ResolveParam(ctx, rt, params, "url", Item{}, 0)
		if err != nil {
			return Output{}, err
		}
		return Main([]Item{{"url": val}}), nil
	}})

	def := Definition{Nodes: []Node{
		{ID: "a", Type: "a", Connections: map[string][]string{"main": {"b"}}},
		{ID: "b", Type: "b", Parameters: map[string]any{"url": "=`https://example.com/` + $node('a').json.id"}},
	}}
	rt := &Runtime{Expr: expr.NewPool(2)}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	b := result.Results["b"]
	if len(b.Output) != 1 || b.Output[0]["url"] != "https://example.com/abc123" {
		t.Fatalf("expected $node('a') to see a's real output, got %#v", b.Output)
	}
}

// TestNodeAccessorReturnsEmptyForANodeThatHasNotRunYet proves $node(name)
// for an unknown/not-yet-run node resolves to {json: null, all: []} rather
// than erroring - a graph-authoring mistake (typo'd node name) should
// surface as a JS-level null, not a Go panic or an opaque failure deep in
// goja.
func TestNodeAccessorReturnsEmptyForANodeThatHasNotRunYet(t *testing.T) {
	rt := &Runtime{Expr: expr.NewPool(1)}
	got, err := ResolveValue(context.Background(), rt, "=$node('missing').json === null && $node('missing').all.length", Item{}, 0)
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	var n int64
	switch v := got.(type) {
	case int64:
		n = v
	case float64:
		n = int64(v)
	default:
		t.Fatalf("expected a numeric result (json===null is true, so the expression reduces to all.length), got %#v (%T)", got, got)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}
