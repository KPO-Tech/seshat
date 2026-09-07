package docling

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient_DefaultsTo120SecondTimeout(t *testing.T) {
	c := NewClient("http://localhost:5001")
	if c.httpClient.Timeout != 120*time.Second {
		t.Fatalf("expected default timeout of 120s, got %v", c.httpClient.Timeout)
	}
}

func TestNewClient_WithTimeoutOverridesTheDefault(t *testing.T) {
	c := NewClient("http://localhost:5001", WithTimeout(10*time.Minute))
	if c.httpClient.Timeout != 10*time.Minute {
		t.Fatalf("expected WithTimeout to override the default, got %v", c.httpClient.Timeout)
	}
}

func TestNewClient_WithAuthTenantAndUserAgentHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "sk-test" {
			t.Fatalf("missing API key header: %q", r.Header.Get("X-Api-Key"))
		}
		if r.Header.Get("X-Tenant-Id") != "tenant-1" {
			t.Fatalf("missing tenant header: %q", r.Header.Get("X-Tenant-Id"))
		}
		if r.Header.Get("User-Agent") != "seshat-test" {
			t.Fatalf("missing user-agent header: %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, WithAPIKey("sk-test"), WithTenantID("tenant-1"), WithUserAgent("seshat-test"))
	if !c.IsAvailable(context.Background()) {
		t.Fatal("expected client to be available")
	}
}

// A slow docling-serve response that would exceed the default 120s timeout
// must still succeed once WithTimeout raises the budget - this is the whole
// point of making it configurable: large/scan-heavy documents on a
// CPU-only deployment can genuinely take that long.
func TestConvertBytes_WithTimeoutAllowsASlowResponseToSucceed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/convert/file":
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"# slow but done","pages":[1]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, WithTimeout(200*time.Millisecond))
	result, err := c.ConvertBytes(context.Background(), []byte("dummy pdf bytes"), "report.pdf")
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if result.Markdown != "# slow but done" {
		t.Fatalf("unexpected markdown: %q", result.Markdown)
	}
}

func TestConvertBytesWithOptions_SendsMultipartOptions(t *testing.T) {
	wantOCR := true
	wantTables := true
	timeout := 42.5
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/convert/file" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]string{}
		sawFile := false
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if part.FileName() == "report.pdf" {
				sawFile = true
				continue
			}
			b, _ := io.ReadAll(part)
			fields[part.FormName()] = string(b)
		}
		if !sawFile {
			t.Fatal("missing file upload")
		}
		for key, want := range map[string]string{
			"do_ocr":             "true",
			"do_table_structure": "true",
			"pipeline":           "standard",
			"pdf_backend":        "dlparse_v4",
			"document_timeout":   "42.5",
		} {
			if fields[key] != want {
				t.Fatalf("%s = %q, want %q (all fields: %+v)", key, fields[key], want, fields)
			}
		}
		_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"# ok","pages":[1]}}`))
	}))
	defer server.Close()

	result, err := NewClient(server.URL).ConvertBytesWithOptions(
		context.Background(),
		[]byte("pdf"),
		"report.pdf",
		ConvertOptions{
			DoOCR:              &wantOCR,
			DoTableStructure:   &wantTables,
			Pipeline:           "standard",
			PDFBackend:         "dlparse_v4",
			DocumentTimeoutSec: &timeout,
		},
	)
	if err != nil {
		t.Fatalf("ConvertBytesWithOptions: %v", err)
	}
	if result.Markdown != "# ok" {
		t.Fatalf("unexpected markdown: %q", result.Markdown)
	}
}

func TestChunkHybrid_SendsConvertAndChunkOptions(t *testing.T) {
	mergePeers := true
	rawText := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]string{}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if part.FileName() != "" {
				continue
			}
			b, _ := io.ReadAll(part)
			fields[part.FormName()] = string(b)
		}
		for key, want := range map[string]string{
			"convert_pipeline":          "standard",
			"chunking_max_tokens":       "512",
			"chunking_tokenizer":        "sentence-transformers/all-MiniLM-L6-v2",
			"chunking_merge_peers":      "true",
			"chunking_include_raw_text": "true",
		} {
			if fields[key] != want {
				t.Fatalf("%s = %q, want %q (all fields: %+v)", key, fields[key], want, fields)
			}
		}
		_, _ = w.Write([]byte(`{"chunks":[{"chunk_index":0,"text":"hello"}]}`))
	}))
	defer server.Close()

	chunks, err := NewClient(server.URL).ChunkHybridBytes(context.Background(), []byte("pdf"), "report.pdf", ChunkOptions{
		MaxTokens:      512,
		Tokenizer:      "sentence-transformers/all-MiniLM-L6-v2",
		MergePeers:     &mergePeers,
		IncludeRawText: &rawText,
		ConvertOptions: ConvertOptions{Pipeline: "standard"},
	})
	if err != nil {
		t.Fatalf("ChunkHybridBytes: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Text != "hello" {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
}

func TestConvertBytes_ReturnsAPIErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).ConvertBytes(context.Background(), []byte("pdf"), "report.pdf")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusUnauthorized || apiErr.Body != "nope" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestConvertBytesWithOptions_RetriesTransientFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("temporary"))
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"# retried","pages":[1]}}`))
	}))
	defer server.Close()

	result, err := NewClient(server.URL, WithRetry(2, time.Millisecond, time.Millisecond)).
		ConvertBytes(context.Background(), []byte("pdf"), "report.pdf")
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if result.Markdown != "# retried" {
		t.Fatalf("unexpected markdown: %q", result.Markdown)
	}
}

func TestConvertBytesWithOptions_DoesNotRetryClientErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, WithRetry(3, time.Millisecond, time.Millisecond)).
		ConvertBytes(context.Background(), []byte("pdf"), "report.pdf")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt for 400, got %d", attempts)
	}
}

func TestIsAvailable_UsesHealthCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, WithHealthCacheTTL(time.Minute))
	if !c.IsAvailable(context.Background()) {
		t.Fatal("first IsAvailable should be true")
	}
	if !c.IsAvailable(context.Background()) {
		t.Fatal("second IsAvailable should be true")
	}
	if calls != 1 {
		t.Fatalf("expected one health call, got %d", calls)
	}
}

func TestIsAvailable_CacheCanBeDisabled(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, WithHealthCacheTTL(0))
	_ = c.IsAvailable(context.Background())
	_ = c.IsAvailable(context.Background())
	if calls != 2 {
		t.Fatalf("expected two health calls, got %d", calls)
	}
}
