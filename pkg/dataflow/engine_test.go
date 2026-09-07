package dataflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/dataflow/expr"
)

// funcExecutor lets tests define a node's behavior as a plain function,
// mirroring pkg/workflow's ExecutorFunc.
type funcExecutor struct {
	desc     NodeDescription
	validate func(map[string]any) error
	execute  func(ctx context.Context, rt *Runtime, input []Item, params map[string]any) (Output, error)
}

func (f funcExecutor) Description() NodeDescription { return f.desc }

func (f funcExecutor) ValidateParameters(params map[string]any) error {
	if f.validate == nil {
		return nil
	}
	return f.validate(params)
}

func (f funcExecutor) Execute(ctx context.Context, rt *Runtime, input []Item, params map[string]any) (Output, error) {
	return f.execute(ctx, rt, input, params)
}

func passthrough(nodeType string) funcExecutor {
	return funcExecutor{
		desc: NodeDescription{Type: nodeType},
		execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
			return Main(input), nil
		},
	}
}

func TestRunExecutesDependenciesBeforeChildren(t *testing.T) {
	reg := NewRegistry()
	var seen []string
	var mu sync.Mutex
	reg.Register("a", funcExecutor{desc: NodeDescription{Type: "a"}, execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
		mu.Lock()
		seen = append(seen, "a")
		mu.Unlock()
		return Main([]Item{{"from": "a"}}), nil
	}})
	reg.Register("b", funcExecutor{desc: NodeDescription{Type: "b"}, execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
		mu.Lock()
		seen = append(seen, "b")
		mu.Unlock()
		if len(input) != 1 || input[0]["from"] != "a" {
			t.Fatalf("expected b to receive a's output, got %#v", input)
		}
		return Main(input), nil
	}})

	def := Definition{Name: "test", Nodes: []Node{
		{ID: "a", Type: "a", Connections: map[string][]string{"main": {"b"}}},
		{ID: "b", Type: "b"},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, results: %#v", result.Results)
	}
	if strings.Join(seen, ",") != "a,b" {
		t.Fatalf("unexpected execution order: %v", seen)
	}
}

func TestRunRoutesConditionalPorts(t *testing.T) {
	reg := NewRegistry()
	reg.Register("split", funcExecutor{desc: NodeDescription{Type: "split"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		return Output{Ports: map[string][]Item{
			"true":  {{"branch": "true"}},
			"false": {{"branch": "false"}},
		}}, nil
	}})
	var trueGot, falseGot []Item
	reg.Register("sink", funcExecutor{desc: NodeDescription{Type: "sink"}, execute: func(_ context.Context, _ *Runtime, input []Item, params map[string]any) (Output, error) {
		if params["branch"] == "true" {
			trueGot = input
		} else {
			falseGot = input
		}
		return Main(input), nil
	}})

	def := Definition{Nodes: []Node{
		{ID: "split", Type: "split", Connections: map[string][]string{"true": {"sink-true"}, "false": {"sink-false"}}},
		{ID: "sink-true", Type: "sink", Parameters: map[string]any{"branch": "true"}},
		{ID: "sink-false", Type: "sink", Parameters: map[string]any{"branch": "false"}},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if len(trueGot) != 1 || trueGot[0]["branch"] != "true" {
		t.Fatalf("true branch got wrong items: %#v", trueGot)
	}
	if len(falseGot) != 1 || falseGot[0]["branch"] != "false" {
		t.Fatalf("false branch got wrong items: %#v", falseGot)
	}
}

func TestRunSkipsDownstreamOnFailure(t *testing.T) {
	reg := NewRegistry()
	reg.Register("boom", funcExecutor{desc: NodeDescription{Type: "boom"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		return Output{}, errors.New("boom")
	}})
	reg.Register("passthrough", passthrough("passthrough"))

	def := Definition{Nodes: []Node{
		{ID: "boom", Type: "boom", Connections: map[string][]string{"main": {"next"}}},
		{ID: "next", Type: "passthrough"},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Success {
		t.Fatal("expected overall failure")
	}
	if result.Results["boom"].Success {
		t.Fatal("expected boom to fail")
	}
	next := result.Results["next"]
	if !next.Skipped || next.Success {
		t.Fatalf("expected next to be skipped, got %#v", next)
	}
}

func TestValidateDetectsCycle(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "x", Connections: map[string][]string{"main": {"b"}}},
		{ID: "b", Type: "x", Connections: map[string][]string{"main": {"a"}}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestValidateDetectsDuplicateID(t *testing.T) {
	def := Definition{Nodes: []Node{{ID: "a", Type: "x"}, {ID: "a", Type: "y"}}}
	if err := Validate(def); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestValidateDetectsUnknownConnectionTarget(t *testing.T) {
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "x", Connections: map[string][]string{"main": {"missing"}}},
	}}
	if err := Validate(def); err == nil {
		t.Fatal("expected unknown target error")
	}
}

func TestValidateDetectsUnknownPinnedDataNode(t *testing.T) {
	def := Definition{
		Nodes:      []Node{{ID: "a", Type: "x"}},
		PinnedData: map[string][]Item{"missing": {{"x": 1}}},
	}
	if err := Validate(def); err == nil {
		t.Fatal("expected error for pinnedData referencing an unknown node")
	}
}

func TestRunFailsOnUnregisteredNodeType(t *testing.T) {
	def := Definition{Nodes: []Node{{ID: "a", Type: "does-not-exist"}}}
	result, err := Run(context.Background(), def, NewRegistry(), nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for unregistered node type")
	}
}

func TestRunSeedsStartingNodesWithInput(t *testing.T) {
	reg := NewRegistry()
	reg.Register("start", passthrough("start"))
	def := Definition{Nodes: []Node{{ID: "start", Type: "start"}}}
	result, err := Run(context.Background(), def, reg, nil, []Item{{"seed": true}}, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v", err, result.Success)
	}
	out := result.Results["start"].Output
	if len(out) != 1 || out[0]["seed"] != true {
		t.Fatalf("expected seeded input, got %#v", out)
	}
}

func TestRunRecordsInputSourceFromUpstreamNode(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", passthrough("a"))
	reg.Register("b", passthrough("b"))
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "a", Connections: map[string][]string{"main": {"b"}}},
		{ID: "b", Type: "b"},
	}}
	result, err := Run(context.Background(), def, reg, nil, []Item{{"x": 1}}, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	b := result.Results["b"]
	if len(b.Input) != 1 || b.Input[0]["x"] != 1 {
		t.Fatalf("expected b's Input to be a's output, got %#v", b.Input)
	}
	if len(b.InputSource) != 1 || b.InputSource[0] != (ItemSource{Node: "a", Port: "main"}) {
		t.Fatalf("expected b's InputSource to point at a's main port, got %#v", b.InputSource)
	}
}

func TestRunRecordsZeroValueInputSourceForSeedInput(t *testing.T) {
	reg := NewRegistry()
	reg.Register("start", passthrough("start"))
	def := Definition{Nodes: []Node{{ID: "start", Type: "start"}}}
	result, err := Run(context.Background(), def, reg, nil, []Item{{"seed": true}}, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v", err, result.Success)
	}
	src := result.Results["start"].InputSource
	if len(src) != 1 || src[0] != (ItemSource{}) {
		t.Fatalf("expected a zero-value ItemSource for seed input, got %#v", src)
	}
}

func TestRunRecordsInputSourceFromBothProducersOnFanIn(t *testing.T) {
	reg := NewRegistry()
	reg.Register("left", funcExecutor{desc: NodeDescription{Type: "left"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		return Main([]Item{{"from": "left"}}), nil
	}})
	reg.Register("right", funcExecutor{desc: NodeDescription{Type: "right"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		return Main([]Item{{"from": "right"}}), nil
	}})
	reg.Register("sink", passthrough("sink"))

	def := Definition{Nodes: []Node{
		{ID: "left", Type: "left", Connections: map[string][]string{"main": {"sink"}}},
		{ID: "right", Type: "right", Connections: map[string][]string{"main": {"sink"}}},
		{ID: "sink", Type: "sink"},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	sink := result.Results["sink"]
	if len(sink.Input) != 2 || len(sink.InputSource) != 2 {
		t.Fatalf("expected 2 items from 2 producers, got input=%#v sources=%#v", sink.Input, sink.InputSource)
	}
	// left/right run concurrently (same topological level) so arrival order
	// isn't guaranteed - assert both producers are represented and each
	// InputSource lines up with its own item, not just that two entries exist.
	seenSources := map[ItemSource]bool{}
	for i, item := range sink.Input {
		from, _ := item["from"].(string)
		src := sink.InputSource[i]
		if src.Node != from || src.Port != "main" {
			t.Fatalf("item %#v not index-aligned with its source %#v", item, src)
		}
		seenSources[src] = true
	}
	if !seenSources[ItemSource{Node: "left", Port: "main"}] || !seenSources[ItemSource{Node: "right", Port: "main"}] {
		t.Fatalf("expected sources from both left and right, got %#v", sink.InputSource)
	}
}

func TestRunRecordsOutputByPortWithoutChangingOutput(t *testing.T) {
	reg := NewRegistry()
	reg.Register("split", funcExecutor{desc: NodeDescription{Type: "split"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		return Output{Ports: map[string][]Item{
			"true":  {{"branch": "true"}},
			"false": {{"branch": "false"}},
		}}, nil
	}})
	def := Definition{Nodes: []Node{{ID: "split", Type: "split"}}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v", err, result.Success)
	}
	split := result.Results["split"]
	if len(split.OutputByPort["true"]) != 1 || split.OutputByPort["true"][0]["branch"] != "true" {
		t.Fatalf("expected OutputByPort[true] to have the true branch item, got %#v", split.OutputByPort)
	}
	if len(split.OutputByPort["false"]) != 1 || split.OutputByPort["false"][0]["branch"] != "false" {
		t.Fatalf("expected OutputByPort[false] to have the false branch item, got %#v", split.OutputByPort)
	}
	// Output still collapses to "every port concatenated" exactly as before -
	// no behavior change for existing consumers of this field.
	if len(split.Output) != 2 {
		t.Fatalf("expected Output to still collapse both ports (2 items), got %#v", split.Output)
	}
}

func TestRunRecordsInputOnValidationFailure(t *testing.T) {
	reg := NewRegistry()
	reg.Register("picky", funcExecutor{
		desc:     NodeDescription{Type: "picky"},
		validate: func(map[string]any) error { return errors.New("nope") },
	})
	def := Definition{Nodes: []Node{{ID: "picky", Type: "picky"}}}
	result, err := Run(context.Background(), def, reg, nil, []Item{{"x": 1}}, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	picky := result.Results["picky"]
	if picky.Success {
		t.Fatal("expected validation failure")
	}
	if len(picky.Input) != 1 || picky.Input[0]["x"] != 1 {
		t.Fatalf("expected Input to still be recorded on a validation failure, got %#v", picky.Input)
	}
	if len(picky.InputSource) != 1 || picky.InputSource[0] != (ItemSource{}) {
		t.Fatalf("expected InputSource to still be recorded on a validation failure, got %#v", picky.InputSource)
	}
}

func TestRunUsesPinnedDataInsteadOfExecutingTheRealNode(t *testing.T) {
	reg := NewRegistry()
	called := false
	reg.Register("a", funcExecutor{desc: NodeDescription{Type: "a"}, execute: func(_ context.Context, _ *Runtime, _ []Item, _ map[string]any) (Output, error) {
		called = true
		return Output{}, errors.New("the real node must never run when pinned")
	}})
	def := Definition{
		Nodes:      []Node{{ID: "a", Type: "a"}},
		PinnedData: map[string][]Item{"a": {{"pinned": true}}},
	}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if called {
		t.Fatal("expected the real executor to never run for a pinned node")
	}
	a := result.Results["a"]
	if !a.Pinned {
		t.Fatal("expected NodeResult.Pinned to be true")
	}
	if len(a.Output) != 1 || a.Output[0]["pinned"] != true {
		t.Fatalf("expected the pinned items as output, got %#v", a.Output)
	}
}

func TestRunPinnedNodeSkipsValidateParametersAndRegistryLookup(t *testing.T) {
	// No node type "does-not-exist" registered at all - a pinned node must
	// not even reach the registry lookup, let alone ValidateParameters.
	def := Definition{
		Nodes:      []Node{{ID: "a", Type: "does-not-exist"}},
		PinnedData: map[string][]Item{"a": {{"x": 1}}},
	}
	result, err := Run(context.Background(), def, NewRegistry(), nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	if !result.Results["a"].Pinned {
		t.Fatal("expected the unregistered-but-pinned node to still succeed as pinned")
	}
}

func TestRunPropagatesPinnedOutputToDownstreamNode(t *testing.T) {
	reg := NewRegistry()
	reg.Register("b", passthrough("b"))
	def := Definition{
		Nodes: []Node{
			{ID: "a", Type: "a", Connections: map[string][]string{"main": {"b"}}},
			{ID: "b", Type: "b"},
		},
		PinnedData: map[string][]Item{"a": {{"from": "pin"}}},
	}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	b := result.Results["b"]
	if len(b.Input) != 1 || b.Input[0]["from"] != "pin" {
		t.Fatalf("expected b to receive a's pinned output, got %#v", b.Input)
	}
	if len(b.InputSource) != 1 || b.InputSource[0] != (ItemSource{Node: "a", Port: "main"}) {
		t.Fatalf("expected b's InputSource to point at a's main port like a real run, got %#v", b.InputSource)
	}
}

// TestRunRetriesAFailingNodeUntilItSucceeds proves RetryOnFail (Tier 3.1)
// actually calls Execute more than once - not just accepts the field.
func TestRunRetriesAFailingNodeUntilItSucceeds(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	reg.Register("flaky", funcExecutor{desc: NodeDescription{Type: "flaky"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		calls++
		if calls < 3 {
			return Output{}, errors.New("not yet")
		}
		return Main([]Item{{"ok": true}}), nil
	}})
	def := Definition{Nodes: []Node{{ID: "a", Type: "flaky", RetryOnFail: true, MaxTries: 3}}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected the node to succeed on its 3rd attempt, results: %#v", result.Results)
	}
	a := result.Results["a"]
	if a.Attempts != 3 {
		t.Fatalf("expected Attempts=3, got %d", a.Attempts)
	}
	if calls != 3 {
		t.Fatalf("expected Execute to be called 3 times, got %d", calls)
	}
}

// TestRunDoesNotRetryWithoutRetryOnFail is the regression guard: every graph
// saved before this field existed has RetryOnFail=false, and must behave
// exactly as before - one call, immediate failure.
func TestRunDoesNotRetryWithoutRetryOnFail(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	reg.Register("boom", funcExecutor{desc: NodeDescription{Type: "boom"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		calls++
		return Output{}, errors.New("boom")
	}})
	def := Definition{Nodes: []Node{{ID: "a", Type: "boom"}}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call with RetryOnFail unset, got %d", calls)
	}
	if result.Results["a"].Attempts != 1 {
		t.Fatalf("expected Attempts=1, got %d", result.Results["a"].Attempts)
	}
}

// TestRunClampsMaxTriesToFive proves a node can't be configured to retry
// unbounded - MaxTries is clamped to n8n's own real 2-5 range.
func TestRunClampsMaxTriesToFive(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	reg.Register("boom", funcExecutor{desc: NodeDescription{Type: "boom"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		calls++
		return Output{}, errors.New("boom")
	}})
	def := Definition{Nodes: []Node{{ID: "a", Type: "boom", RetryOnFail: true, MaxTries: 999}}}
	if _, err := Run(context.Background(), def, reg, nil, nil, Options{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 5 {
		t.Fatalf("expected MaxTries clamped to 5, got %d calls", calls)
	}
}

// TestRunOnErrorContinueRegularOutputLetsDownstreamRun proves a node with
// OnError="continueRegularOutput" doesn't stop the run - downstream still
// executes, receiving no items (not skipped, not erroring itself).
func TestRunOnErrorContinueRegularOutputLetsDownstreamRun(t *testing.T) {
	reg := NewRegistry()
	reg.Register("boom", funcExecutor{desc: NodeDescription{Type: "boom"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		return Output{}, errors.New("boom")
	}})
	var nextRan bool
	var nextInput []Item
	reg.Register("next", funcExecutor{desc: NodeDescription{Type: "next"}, execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
		nextRan = true
		nextInput = input
		return Main(input), nil
	}})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "boom", OnError: "continueRegularOutput", Connections: map[string][]string{"main": {"next"}}},
		{ID: "next", Type: "next"},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Success {
		t.Fatal("expected the overall run to still report failure - a real error happened, it just didn't stop execution")
	}
	a := result.Results["a"]
	if a.Success || !a.Continued {
		t.Fatalf("expected a to be Success=false, Continued=true, got %#v", a)
	}
	if !nextRan {
		t.Fatal("expected downstream to run despite the upstream failure")
	}
	if len(nextInput) != 0 {
		t.Fatalf("expected downstream to receive no items, got %#v", nextInput)
	}
	if next := result.Results["next"]; next.Skipped {
		t.Fatal("expected downstream to not be skipped")
	}
}

// TestRunOnErrorContinueErrorOutputRoutesToErrorPort proves
// OnError="continueErrorOutput" sends a synthesized error item on a new
// "error" port, and a downstream node wired specifically to that port
// receives it.
func TestRunOnErrorContinueErrorOutputRoutesToErrorPort(t *testing.T) {
	reg := NewRegistry()
	reg.Register("boom", funcExecutor{desc: NodeDescription{Type: "boom"}, execute: func(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
		return Output{}, errors.New("boom detail")
	}})
	var errBranchGot []Item
	reg.Register("errsink", funcExecutor{desc: NodeDescription{Type: "errsink"}, execute: func(_ context.Context, _ *Runtime, input []Item, _ map[string]any) (Output, error) {
		errBranchGot = input
		return Main(input), nil
	}})
	def := Definition{Nodes: []Node{
		{ID: "a", Type: "boom", OnError: "continueErrorOutput", Connections: map[string][]string{"error": {"errsink"}}},
		{ID: "errsink", Type: "errsink"},
	}}
	result, err := Run(context.Background(), def, reg, nil, nil, Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	a := result.Results["a"]
	if a.Success || !a.Continued {
		t.Fatalf("expected a to be Success=false, Continued=true, got %#v", a)
	}
	if len(errBranchGot) != 1 || errBranchGot[0]["error"] != "boom detail" {
		t.Fatalf("expected the error branch to receive a synthesized error item, got %#v", errBranchGot)
	}
	if len(a.OutputByPort["error"]) != 1 {
		t.Fatalf("expected NodeResult.OutputByPort[\"error\"] to record the same item, got %#v", a.OutputByPort)
	}
}

func TestNodeAccessorSeesPinnedOutputOnceThatNodeCompletes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("b", funcExecutor{desc: NodeDescription{Type: "b"}, execute: func(ctx context.Context, rt *Runtime, _ []Item, params map[string]any) (Output, error) {
		val, err := ResolveParam(ctx, rt, params, "url", Item{}, 0)
		if err != nil {
			return Output{}, err
		}
		return Main([]Item{{"url": val}}), nil
	}})

	def := Definition{
		Nodes: []Node{
			{ID: "a", Type: "a", Connections: map[string][]string{"main": {"b"}}},
			{ID: "b", Type: "b", Parameters: map[string]any{"url": "=$node('a').json.id"}},
		},
		PinnedData: map[string][]Item{"a": {{"id": "pinned-id"}}},
	}
	rt := &Runtime{Expr: expr.NewPool(1)}
	result, err := Run(context.Background(), def, reg, rt, nil, Options{})
	if err != nil || !result.Success {
		t.Fatalf("run: err=%v success=%v results=%#v", err, result.Success, result.Results)
	}
	b := result.Results["b"]
	if len(b.Output) != 1 || b.Output[0]["url"] != "pinned-id" {
		t.Fatalf("expected $node('a') to see a's pinned output, got %#v", b.Output)
	}
}
