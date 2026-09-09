package dataflow

import (
	"context"
	"testing"
)

// plainExecutor implements NodeExecutor only — no TestConnection.
type plainExecutor struct{ t string }

func (p plainExecutor) Execute(context.Context, *Runtime, []Item, map[string]any) (Output, error) {
	return Output{}, nil
}
func (p plainExecutor) Description() NodeDescription            { return NodeDescription{Type: p.t} }
func (p plainExecutor) ValidateParameters(map[string]any) error { return nil }

// testableExecutor additionally implements TestableExecutor.
type testableExecutor struct{ plainExecutor }

func (testableExecutor) TestConnection(context.Context, *Runtime, map[string]any) error { return nil }

func TestRegistryTypesSetsIsTestableViaTypeAssertion(t *testing.T) {
	reg := NewRegistry()
	reg.Register("plain", plainExecutor{t: "plain"})
	reg.Register("testable", testableExecutor{plainExecutor{t: "testable"}})

	byType := map[string]NodeDescription{}
	for _, desc := range reg.Types() {
		byType[desc.Type] = desc
	}

	if byType["plain"].IsTestable {
		t.Fatal("expected IsTestable to be false for a node type with no TestConnection method")
	}
	if !byType["testable"].IsTestable {
		t.Fatal("expected IsTestable to be true for a node type implementing TestableExecutor")
	}
}
