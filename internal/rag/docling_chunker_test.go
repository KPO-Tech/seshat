package rag

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/docling"
	"github.com/KPO-Tech/seshat/internal/vector"
)

func TestDoclingChunker_SplitDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chunk/hybrid/file":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Fatalf("content-type = %q", r.Header.Get("Content-Type"))
			}
			mr, err := r.MultipartReader()
			if err != nil {
				t.Fatal(err)
			}
			var sawFile bool
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if part.FileName() == "policy.pdf" {
					sawFile = true
				}
			}
			if !sawFile {
				t.Fatal("missing uploaded file")
			}
			_, _ = w.Write([]byte(`{
				"chunks": [
					{
						"filename": "policy.pdf",
						"chunk_index": 0,
						"text": "Security policy introduction",
						"raw_text": "Security policy introduction",
						"num_tokens": 12,
						"headings": ["Security Policy"],
						"page_numbers": [1],
						"doc_items": ["#/texts/0"]
					},
					{
						"filename": "policy.pdf",
						"chunk_index": 1,
						"text": "Access review table",
						"captions": ["Table 1"],
						"page_numbers": [2],
						"doc_items": ["#/tables/0"]
					}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	chunker := NewDoclingChunker(docling.NewClient(srv.URL), docling.ChunkOptions{MaxTokens: 512})
	chunks, err := chunker.SplitDocument(context.Background(), Document{
		Filename: "policy.pdf",
		Text:     "fallback text",
		Data:     []byte("%PDF"),
	})
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Metadata["chunker"] != "docling_hybrid" {
		t.Fatalf("expected docling metadata, got %+v", chunks[0].Metadata)
	}
	if chunks[0].Metadata["page_numbers"] != "[1]" {
		t.Errorf("page_numbers metadata = %q", chunks[0].Metadata["page_numbers"])
	}
	if chunks[1].Metadata["captions"] != `["Table 1"]` {
		t.Errorf("captions metadata = %q", chunks[1].Metadata["captions"])
	}
}

func TestServiceIngest_UsesDocumentChunkerMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chunk/hybrid/file":
			_, _ = w.Write([]byte(`{
				"chunks": [
					{
						"filename": "manual.pdf",
						"chunk_index": 0,
						"text": "Alpha installation steps",
						"headings": ["Install"],
						"page_numbers": [3]
					}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := vector.NewMemoryStore()
	svc := NewService(nil, store, fakeEmbedder{}, NewDoclingChunker(docling.NewClient(srv.URL), docling.ChunkOptions{}))

	result, err := svc.Ingest(context.Background(), IngestRequest{
		CorpusID: "kb",
		FileID:   "manual",
		Filename: "manual.pdf",
		Data:     []byte("%PDF"),
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Chunks != 1 {
		t.Fatalf("expected 1 chunk, got %d", result.Chunks)
	}

	records, err := store.Get(context.Background(), "kb", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Metadata["chunker"] != "docling_hybrid" {
		t.Fatalf("missing docling chunker metadata: %+v", records[0].Metadata)
	}
	if records[0].Metadata["page_numbers"] != "[3]" {
		t.Errorf("page_numbers metadata = %q", records[0].Metadata["page_numbers"])
	}
}

func TestDoclingChunker_FallsBackToTextChunkerWhenUnavailable(t *testing.T) {
	chunker := &DoclingChunker{Fallback: ParagraphChunker{MaxChunkChars: 50}}
	chunks, err := chunker.SplitDocument(context.Background(), Document{
		Filename: "notes.pdf",
		Text:     "First paragraph.\n\nSecond paragraph.",
		Data:     []byte("%PDF"),
	})
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected fallback paragraph chunks, got %d", len(chunks))
	}
}
