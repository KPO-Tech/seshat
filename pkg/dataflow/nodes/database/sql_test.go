package database

import (
	"context"
	"testing"

	_ "modernc.org/sqlite" // registers "sqlite" for this package's own tests — see sql.go's doc comment for why production code doesn't import it directly

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

type staticSecrets map[string]string

func (s staticSecrets) Resolve(_ context.Context, ref string) (string, error) {
	return s[ref], nil
}

func TestSQLiteExecuteQueryReturnsRows(t *testing.T) {
	rt := &dataflow.Runtime{Secrets: staticSecrets{"db": "file::memory:?cache=shared"}}
	n := NewSQLite()

	_, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "executeQuery",
		"query": "CREATE TABLE t (id INTEGER, name TEXT)",
	})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = n.Execute(context.Background(), rt, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "insert",
		"query": "INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b')",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	out, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "executeQuery",
		"query": "SELECT id, name FROM t ORDER BY id",
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	items := out.Ports["main"]
	if len(items) != 2 {
		t.Fatalf("expected 2 rows, got %d: %#v", len(items), items)
	}
	if items[0]["name"] != "a" || items[1]["name"] != "b" {
		t.Fatalf("unexpected rows: %#v", items)
	}
}

func TestSQLiteInsertReportsRowsAffected(t *testing.T) {
	rt := &dataflow.Runtime{Secrets: staticSecrets{"db": "file::memory:?cache=shared&mode=rwc"}}
	n := NewSQLite()
	if _, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "insert", "query": "CREATE TABLE t2 (id INTEGER)",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	out, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "insert", "query": "INSERT INTO t2 (id) VALUES (1), (2), (3)",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := out.Ports["main"][0]["rows_affected"]; got != int64(3) {
		t.Fatalf("expected rows_affected=3, got %#v", got)
	}
}

func TestSQLValidateParametersRequiresDSNRefAndOperation(t *testing.T) {
	n := NewPostgres()
	if err := n.ValidateParameters(map[string]any{}); err == nil {
		t.Fatal("expected error for missing dsnSecretRef")
	}
	if err := n.ValidateParameters(map[string]any{"dsnSecretRef": "x", "operation": "bogus", "query": "SELECT 1"}); err == nil {
		t.Fatal("expected error for invalid operation")
	}
	if err := n.ValidateParameters(map[string]any{"dsnSecretRef": "x", "operation": "executeQuery", "query": "SELECT 1"}); err != nil {
		t.Fatalf("expected valid params to pass, got %v", err)
	}
}

func TestSQLTestConnectionSucceedsWithoutOperationOrQuery(t *testing.T) {
	rt := &dataflow.Runtime{Secrets: staticSecrets{"db": "file::memory:?cache=shared"}}
	n := NewSQLite()
	// Deliberately no "operation"/"query" - TestConnection must not need them.
	if err := n.TestConnection(context.Background(), rt, map[string]any{"dsnSecretRef": "db"}); err != nil {
		t.Fatalf("expected TestConnection to succeed, got %v", err)
	}
}

func TestSQLTestConnectionFailsForUnreachableHost(t *testing.T) {
	rt := &dataflow.Runtime{Secrets: staticSecrets{"db": "postgres://user:pass@127.0.0.1:1/nonexistent"}}
	n := NewPostgres()
	if err := n.TestConnection(context.Background(), rt, map[string]any{"dsnSecretRef": "db"}); err == nil {
		t.Fatal("expected TestConnection to fail against an unreachable host")
	}
}

func TestSQLExecuteRequiresSecretResolver(t *testing.T) {
	n := NewMySQL()
	_, err := n.Execute(context.Background(), nil, nil, map[string]any{
		"dsnSecretRef": "db", "operation": "executeQuery", "query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error without a configured Runtime.Secrets")
	}
}
