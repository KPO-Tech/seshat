package docling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── NewGenericClient validation ───────────────────────────────────────────

func TestNewGenericClient_RequiresBaseURL(t *testing.T) {
	_, err := NewGenericClient(GenericConfig{
		FileField:    "file",
		ConvertPath:  "/v1/documents",
		ParseConvert: func([]byte) (*ConversionResult, error) { return nil, nil },
	})
	if err == nil {
		t.Fatal("expected an error for a missing BaseURL")
	}
}

func TestNewGenericClient_RequiresFileField(t *testing.T) {
	_, err := NewGenericClient(GenericConfig{
		BaseURL:      "http://localhost:5100",
		ConvertPath:  "/v1/documents",
		ParseConvert: func([]byte) (*ConversionResult, error) { return nil, nil },
	})
	if err == nil {
		t.Fatal("expected an error for a missing FileField")
	}
}

func TestNewGenericClient_RequiresAtLeastOneCapability(t *testing.T) {
	_, err := NewGenericClient(GenericConfig{
		BaseURL:   "http://localhost:5100",
		FileField: "file",
	})
	if err == nil {
		t.Fatal("expected an error when neither ConvertPath nor ChunkPath is set")
	}
}

func TestNewGenericClient_RejectsPathWithoutMatchingParser(t *testing.T) {
	_, err := NewGenericClient(GenericConfig{
		BaseURL:     "http://localhost:5100",
		FileField:   "file",
		ConvertPath: "/v1/documents",
		// ParseConvert deliberately omitted.
	})
	if err == nil {
		t.Fatal("expected an error for ConvertPath set without ParseConvert")
	}
}

func TestNewGenericClient_RejectsParserWithoutMatchingPath(t *testing.T) {
	_, err := NewGenericClient(GenericConfig{
		BaseURL:      "http://localhost:5100",
		FileField:    "file",
		ChunkPath:    "/v1/documents/chunks",
		ParseChunk:   func([]byte) ([]Chunk, error) { return nil, nil },
		ParseConvert: func([]byte) (*ConversionResult, error) { return nil, nil },
		// ConvertPath deliberately omitted.
	})
	if err == nil {
		t.Fatal("expected an error for ParseConvert set without ConvertPath")
	}
}

func TestNewGenericClient_ConvertOnlyIsValid(t *testing.T) {
	c, err := NewGenericClient(GenericConfig{
		BaseURL:      "http://localhost:5100",
		FileField:    "file",
		ConvertPath:  "/v1/documents",
		ParseConvert: func([]byte) (*ConversionResult, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("expected a convert-only config to be valid, got: %v", err)
	}
	if _, err := c.ChunkHybridBytes(context.Background(), []byte("x"), "x.pdf", ChunkOptions{}); err == nil {
		t.Fatal("expected ChunkHybridBytes to fail cleanly when ChunkPath isn't configured")
	}
}

func TestGenericClient_ConvertURLIsNotSupported(t *testing.T) {
	c, err := NewGenericClient(GenericConfig{
		BaseURL:      "http://localhost:5100",
		FileField:    "file",
		ConvertPath:  "/v1/documents",
		ParseConvert: func([]byte) (*ConversionResult, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("NewGenericClient: %v", err)
	}
	if _, err := c.ConvertURL(context.Background(), "https://example.com/doc.pdf"); err == nil {
		t.Fatal("expected ConvertURL to return an error")
	}
}

// ─── End-to-end against a seshat-intelligence-shaped server ───────────────
//
// The mock server and parser functions below mirror
// seshat-ai/seshat-intelligence's real routes.py/schemas.py as read at the
// time this was written: POST /v1/documents and POST /v1/documents/chunks,
// file field name "file" (FastAPI binds the multipart field to the
// UploadFile parameter's name), and the exact JSON field names from
// DocumentResponse/ChunkDocumentResponse/DocumentChunk - not a synthetic
// abstract shape, so a passing test here is real evidence GenericClient can
// actually front that service, not just a self-consistent round trip.

type siDocumentResponse struct {
	DocumentID string   `json:"document_id"`
	Filename   string   `json:"filename"`
	Status     string   `json:"status"`
	CreatedAt  string   `json:"created_at"`
	Markdown   string   `json:"markdown"`
	Errors     []string `json:"errors"`
}

type siDocumentChunk struct {
	Index       int      `json:"index"`
	Text        string   `json:"text"`
	RawText     *string  `json:"raw_text"`
	NumTokens   *int     `json:"num_tokens"`
	Headings    []string `json:"headings"`
	Captions    []string `json:"captions"`
	PageNumbers []int    `json:"page_numbers"`
	DocItems    []string `json:"doc_items"`
}

type siChunkDocumentResponse struct {
	Filename string            `json:"filename"`
	Chunks   []siDocumentChunk `json:"chunks"`
}

func parseSeshatIntelligenceConvert(rawBody []byte) (*ConversionResult, error) {
	var resp siDocumentResponse
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return nil, fmt.Errorf("decode seshat-intelligence document response: %w", err)
	}
	if resp.Status != "" && resp.Status != "success" && resp.Status != "completed" {
		return nil, fmt.Errorf("seshat-intelligence conversion failed (%s): %s", resp.Status, strings.Join(resp.Errors, "; "))
	}
	return &ConversionResult{Markdown: resp.Markdown}, nil
}

func parseSeshatIntelligenceChunks(rawBody []byte) ([]Chunk, error) {
	var resp siChunkDocumentResponse
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return nil, fmt.Errorf("decode seshat-intelligence chunk response: %w", err)
	}
	out := make([]Chunk, 0, len(resp.Chunks))
	for _, c := range resp.Chunks {
		rawText := ""
		if c.RawText != nil {
			rawText = *c.RawText
		}
		out = append(out, Chunk{
			Filename:    resp.Filename,
			ChunkIndex:  c.Index,
			Text:        c.Text,
			RawText:     rawText,
			NumTokens:   c.NumTokens,
			Headings:    c.Headings,
			Captions:    c.Captions,
			PageNumbers: c.PageNumbers,
			DocItems:    c.DocItems,
		})
	}
	return out, nil
}

func newSeshatIntelligenceGenericClient(t *testing.T, baseURL string) *GenericClient {
	t.Helper()
	c, err := NewGenericClient(GenericConfig{
		BaseURL:      baseURL,
		FileField:    "file",
		ConvertPath:  "/v1/documents",
		ParseConvert: parseSeshatIntelligenceConvert,
		ChunkPath:    "/v1/documents/chunks",
		ParseChunk:   parseSeshatIntelligenceChunks,
	})
	if err != nil {
		t.Fatalf("NewGenericClient: %v", err)
	}
	return c
}

func TestGenericClient_ConvertBytes_SeshatIntelligenceShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/documents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected the file under form field \"file\": %v", err)
		}
		defer file.Close()
		if header.Filename != "report.pdf" {
			t.Errorf("unexpected filename: %s", header.Filename)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(siDocumentResponse{
			DocumentID: "doc_1",
			Filename:   "report.pdf",
			Status:     "success",
			Markdown:   "# Report\n\nHello world.",
		})
	}))
	defer server.Close()

	c := newSeshatIntelligenceGenericClient(t, server.URL)
	result, err := c.ConvertBytes(context.Background(), []byte("%PDF-1.4 fake"), "report.pdf")
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if result.Markdown != "# Report\n\nHello world." {
		t.Errorf("unexpected markdown: %q", result.Markdown)
	}
}

func TestGenericClient_ChunkHybridBytes_SeshatIntelligenceShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/documents/chunks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Fatalf("expected the file under form field \"file\": %v", err)
		}
		rawText := "Section One\nAlice is 30."
		numTokens := 6
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(siChunkDocumentResponse{
			Filename: "report.pdf",
			Chunks: []siDocumentChunk{
				{
					Index:       0,
					Text:        "Section One\n\nAlice is 30.",
					RawText:     &rawText,
					NumTokens:   &numTokens,
					Headings:    []string{"Section One"},
					PageNumbers: []int{1},
				},
			},
		})
	}))
	defer server.Close()

	c := newSeshatIntelligenceGenericClient(t, server.URL)
	chunks, err := c.ChunkHybridBytes(context.Background(), []byte("%PDF-1.4 fake"), "report.pdf", ChunkOptions{})
	if err != nil {
		t.Fatalf("ChunkHybridBytes: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	got := chunks[0]
	if got.Filename != "report.pdf" || got.ChunkIndex != 0 || got.Text != "Section One\n\nAlice is 30." {
		t.Errorf("unexpected chunk: %+v", got)
	}
	if got.NumTokens == nil || *got.NumTokens != 6 {
		t.Errorf("expected NumTokens=6, got %v", got.NumTokens)
	}
	if len(got.Headings) != 1 || got.Headings[0] != "Section One" {
		t.Errorf("unexpected headings: %v", got.Headings)
	}
}

func TestGenericClient_IsAvailable_CustomHealthPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	disableCache := time.Duration(0)
	c, err := NewGenericClient(GenericConfig{
		BaseURL:        server.URL,
		FileField:      "file",
		ConvertPath:    "/v1/documents",
		ParseConvert:   parseSeshatIntelligenceConvert,
		HealthCacheTTL: &disableCache, // disable caching so this test isn't order-sensitive
	})
	if err != nil {
		t.Fatalf("NewGenericClient: %v", err)
	}
	if !c.IsAvailable(context.Background()) {
		t.Fatal("expected IsAvailable to be true when /health returns 200")
	}
}

func TestGenericClient_ConvertBytes_SurfacesAPIErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"uploaded file is empty"}`))
	}))
	defer server.Close()

	c := newSeshatIntelligenceGenericClient(t, server.URL)
	_, err := c.ConvertBytes(context.Background(), []byte{}, "empty.pdf")
	if err == nil {
		t.Fatal("expected an error for a non-2xx response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", apiErr.Status)
	}
}
