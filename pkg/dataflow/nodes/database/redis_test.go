package database

import (
	"context"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

func TestRedisTestConnectionFailsForUnreachableAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"addr": "127.0.0.1:1"}}
	n := NewRedis()
	// Deliberately no "operation"/"key" - TestConnection must not need them.
	if err := n.TestConnection(ctx, rt, map[string]any{"addrSecretRef": "addr"}); err == nil {
		t.Fatal("expected TestConnection to fail against an unreachable address")
	}
}

func TestRedisValidateParameters(t *testing.T) {
	n := NewRedis()
	if err := n.ValidateParameters(map[string]any{}); err == nil {
		t.Fatal("expected error for missing addrSecretRef")
	}
	if err := n.ValidateParameters(map[string]any{"addrSecretRef": "x", "operation": "bogus", "key": "k"}); err == nil {
		t.Fatal("expected error for invalid operation")
	}
	if err := n.ValidateParameters(map[string]any{"addrSecretRef": "x", "operation": "get", "key": "k"}); err != nil {
		t.Fatalf("expected valid params, got %v", err)
	}
}
