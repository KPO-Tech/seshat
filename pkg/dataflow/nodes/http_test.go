package nodes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/dataflow/expr"
)

func TestValidateURLForSSRFBlocksPrivateAndMetadataHosts(t *testing.T) {
	cases := []string{
		"http://localhost/",
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080/",
		"ftp://example.com/",
	}
	for _, url := range cases {
		if err := validateURLForSSRF(url); err == nil {
			t.Errorf("expected %q to be blocked", url)
		}
	}
}

func TestValidateURLForSSRFAllowsPublicHTTPS(t *testing.T) {
	if err := validateURLForSSRF("https://example.com/path"); err != nil {
		t.Fatalf("expected example.com to be allowed, got %v", err)
	}
}

func TestHTTPRequestValidateParametersRequiresURL(t *testing.T) {
	node := NewHTTPRequest()
	if err := node.ValidateParameters(map[string]any{}); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestHTTPRequestValidateParametersEnforcesSSRFGuard(t *testing.T) {
	node := NewHTTPRequest()
	if err := node.ValidateParameters(map[string]any{"url": "http://169.254.169.254/"}); err == nil {
		t.Fatal("expected the real SSRF guard to reject a metadata URL")
	}
}

func TestHTTPRequestExecuteParsesJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Errorf("expected header X-Test=yes, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"n":3}`))
	}))
	defer srv.Close()

	node := NewHTTPRequest()
	allowLocal(node)

	output, err := node.Execute(context.Background(), nil, nil, map[string]any{
		"url":     srv.URL,
		"method":  "GET",
		"headers": map[string]any{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	items := output.Ports["main"]
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["status"] != 200 {
		t.Fatalf("expected status 200, got %#v", items[0]["status"])
	}
	parsed, ok := items[0]["json"].(map[string]any)
	if !ok || parsed["ok"] != true {
		t.Fatalf("expected parsed json body, got %#v", items[0]["json"])
	}
}

func TestHTTPRequestExecuteResolvesExpressionsInURLBodyAndHeaders(t *testing.T) {
	var gotPath, gotBody, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Item-Id")
		body := make([]byte, 64)
		n, _ := r.Body.Read(body)
		gotBody = string(body[:n])
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	node := NewHTTPRequest()
	allowLocal(node)
	rt := &dataflow.Runtime{Expr: expr.NewPool(1)}
	input := []dataflow.Item{{"id": "42"}}

	_, err := node.Execute(context.Background(), rt, input, map[string]any{
		"url":     "=" + `"` + srv.URL + `/items/" + $json.id`,
		"method":  "POST",
		"body":    "=`id=${$json.id}`",
		"headers": map[string]any{"X-Item-Id": "=$json.id", "X-Literal": "unchanged"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotPath != "/items/42" {
		t.Fatalf("expected the '=' URL expression to resolve against $json, got path %q", gotPath)
	}
	if gotBody != "id=42" {
		t.Fatalf("expected the '=' body expression to resolve against $json, got %q", gotBody)
	}
	if gotHeader != "42" {
		t.Fatalf("expected the '=' header value to resolve against $json, got %q", gotHeader)
	}
}

func TestHTTPRequestExecuteLeavesLiteralParamsUnresolvedWithNoRuntimeExpr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	node := NewHTTPRequest()
	allowLocal(node)
	// No Runtime at all (nil) - must behave exactly as before Tier 2.1 for a
	// literal URL, not error out just because expressions aren't wired up.
	_, err := node.Execute(context.Background(), nil, nil, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("expected a literal url to work with a nil Runtime, got %v", err)
	}
}

func TestHTTPRequestExecuteRejectsBlockedURL(t *testing.T) {
	node := NewHTTPRequest()
	_, err := node.Execute(context.Background(), nil, nil, map[string]any{"url": "http://169.254.169.254/"})
	if err == nil {
		t.Fatal("expected Execute to enforce the SSRF guard even if ValidateParameters was skipped")
	}
}
