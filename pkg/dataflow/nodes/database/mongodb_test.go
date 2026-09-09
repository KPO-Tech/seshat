package database

import (
	"context"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

func TestMongoDBTestConnectionFailsForUnreachableHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"uri": "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=500"}}
	n := NewMongoDB()
	// Deliberately no "database"/"collection"/"operation" - TestConnection
	// must not need them.
	if err := n.TestConnection(ctx, rt, map[string]any{"uriSecretRef": "uri"}); err == nil {
		t.Fatal("expected TestConnection to fail against an unreachable host")
	}
}

func TestMongoDBValidateParameters(t *testing.T) {
	n := NewMongoDB()
	if err := n.ValidateParameters(map[string]any{}); err == nil {
		t.Fatal("expected error for missing uriSecretRef")
	}
	if err := n.ValidateParameters(map[string]any{"uriSecretRef": "x", "database": "d", "collection": "c", "operation": "bogus"}); err == nil {
		t.Fatal("expected error for invalid operation")
	}
	if err := n.ValidateParameters(map[string]any{"uriSecretRef": "x", "database": "d", "collection": "c", "operation": "find"}); err != nil {
		t.Fatalf("expected valid params, got %v", err)
	}
}

func TestToBsonM(t *testing.T) {
	if len(toBsonM(nil)) != 0 {
		t.Fatal("expected empty bson.M for nil input")
	}
	m := toBsonM(map[string]any{"a": 1})
	if m["a"] != 1 {
		t.Fatalf("unexpected conversion: %#v", m)
	}
}
