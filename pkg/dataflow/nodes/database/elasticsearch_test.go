package database

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

func TestElasticsearchValidateParameters(t *testing.T) {
	n := NewElasticsearch()
	if err := n.ValidateParameters(map[string]any{}); err == nil {
		t.Fatal("expected error for missing baseURLSecretRef")
	}
	if err := n.ValidateParameters(map[string]any{"baseURLSecretRef": "x", "index": "i", "operation": "get"}); err == nil {
		t.Fatal("expected error: get requires id")
	}
	if err := n.ValidateParameters(map[string]any{"baseURLSecretRef": "x", "index": "i", "operation": "search"}); err != nil {
		t.Fatalf("expected valid params, got %v", err)
	}
}

func TestElasticsearchSearchParsesHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"1","_score":1.2,"_source":{"title":"hello"}}]}}`))
	}))
	defer srv.Close()

	n := NewElasticsearch()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"es": srv.URL}}
	out, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"baseURLSecretRef": "es", "index": "docs", "operation": "search",
		"query": map[string]any{"match_all": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	items := out.Ports["main"]
	if len(items) != 1 || items[0]["title"] != "hello" || items[0]["_id"] != "1" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestElasticsearchTestConnectionSucceedsWithoutIndexOrOperation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewElasticsearch()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"es": srv.URL}}
	// Deliberately no "index"/"operation" - TestConnection must not need them.
	if err := n.TestConnection(context.Background(), rt, map[string]any{"baseURLSecretRef": "es"}); err != nil {
		t.Fatalf("expected TestConnection to succeed, got %v", err)
	}
}

func TestElasticsearchTestConnectionFailsOnErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	n := NewElasticsearch()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"es": srv.URL}}
	if err := n.TestConnection(context.Background(), rt, map[string]any{"baseURLSecretRef": "es"}); err == nil {
		t.Fatal("expected TestConnection to fail on a non-2xx response")
	}
}

func TestElasticsearchSurfacesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	n := NewElasticsearch()
	rt := &dataflow.Runtime{Secrets: staticSecrets{"es": srv.URL}}
	_, err := n.Execute(context.Background(), rt, nil, map[string]any{
		"baseURLSecretRef": "es", "index": "docs", "operation": "get", "id": "missing",
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
