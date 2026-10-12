package nodes

import (
	"context"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
)

func TestWaitPassesInputThroughAfterDelay(t *testing.T) {
	n := NewWait()
	start := time.Now()
	out, err := n.Execute(context.Background(), nil, []dataflow.Item{{"x": 1}}, map[string]any{"seconds": 1})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected wait to actually delay, elapsed=%v", elapsed)
	}
	if len(out.Ports["main"]) != 1 {
		t.Fatalf("expected input passed through, got %#v", out.Ports)
	}
}

func TestWaitValidateParametersRejectsNonPositive(t *testing.T) {
	n := NewWait()
	if err := n.ValidateParameters(map[string]any{"seconds": 0}); err == nil {
		t.Fatal("expected error for seconds=0")
	}
	if err := n.ValidateParameters(map[string]any{"seconds": 601}); err == nil {
		t.Fatal("expected error for seconds exceeding the max")
	}
}

func TestWaitRespectsContextCancellation(t *testing.T) {
	n := NewWait()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := n.Execute(ctx, nil, nil, map[string]any{"seconds": 5})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}
