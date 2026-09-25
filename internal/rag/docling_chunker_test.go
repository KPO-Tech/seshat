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

func TestDoclingChunkerForProfileSendsProfileMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chunk/hybrid/file":
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
			if fields["chunking_max_tokens"] != "1536" {
				t.Fatalf("chunking_max_tokens = %q, want 1536 (fields: %+v)", fields["chunking_max_tokens"], fields)
			}
			_, _ = w.Write([]byte(`{"chunks":[{"chunk_index":0,"text":"enterprise policy"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	profile, ok := RecommendedChunkProfile(ChunkProfileLarge)
	if !ok {
		t.Fatal("large profile not found")
	}
	chunker := NewDoclingChunkerForProfile(docling.NewClient(server.URL), profile, docling.ChunkOptions{})
	chunks, err := chunker.SplitDocument(context.Background(), Document{
		Filename: "policy.pdf",
		Text:     "fallback",
		Data:     []byte("%PDF"),
	})
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %d", len(chunks))
	}
	if chunks[0].Metadata["chunk_profile"] != "large" {
		t.Fatalf("missing chunk profile metadata: %+v", chunks[0].Metadata)
	}
	if chunks[0].Metadata["chunk_max_tokens"] != "1536" {
		t.Fatalf("missing chunk max tokens metadata: %+v", chunks[0].Metadata)
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

// TestNewDoclingChunkerForProfile_StructuredProfileGetsHeadingFallback is a
// regression guard: only the "structured" profile should get the
// heading-aware fallback - every other profile keeps plain ParagraphChunker,
// matching today's behavior.
func TestNewDoclingChunkerForProfile_StructuredProfileGetsHeadingFallback(t *testing.T) {
	structured := NewDoclingChunkerForProfile(nil, ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 1024}, docling.ChunkOptions{})
	if _, ok := structured.Fallback.(*HeadingChunker); !ok {
		t.Fatalf("expected the structured profile's fallback to be *HeadingChunker, got %T", structured.Fallback)
	}

	medium := NewDoclingChunkerForProfile(nil, ChunkProfile{Name: ChunkProfileMedium, MaxTokens: 768}, docling.ChunkOptions{})
	if _, ok := medium.Fallback.(*HeadingChunker); ok {
		t.Fatal("expected a non-structured profile's fallback to stay the plain chunker, not *HeadingChunker")
	}

	// End-to-end: with no client (docling path skipped entirely), the
	// structured profile's SplitDocument call should still produce
	// heading-aware output, not a flat paragraph split.
	chunks, err := structured.SplitDocument(context.Background(), Document{
		Filename: "policy.txt",
		Text:     "Article 1 Scope\n\nApplies to all employees.\n",
	})
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Metadata["heading_path"] != "Article 1 Scope" {
		t.Fatalf("expected heading-aware fallback output, got %+v", chunks)
	}
}

// TestNewDoclingChunkerForProfile_TableAndQAProfilesGetMatchingFallbacks is
// the same regression guard as the structured-profile test above, extended
// for the two chunkers added in document-intelligence roadmap Phase 4.
func TestNewDoclingChunkerForProfile_TableAndQAProfilesGetMatchingFallbacks(t *testing.T) {
	table := NewDoclingChunkerForProfile(nil, ChunkProfile{Name: ChunkProfileTable, MaxTokens: 1536}, docling.ChunkOptions{})
	if _, ok := table.Fallback.(*TableChunker); !ok {
		t.Fatalf("expected the table profile's fallback to be *TableChunker, got %T", table.Fallback)
	}

	qa := NewDoclingChunkerForProfile(nil, ChunkProfile{Name: ChunkProfileQA, MaxTokens: 512}, docling.ChunkOptions{})
	if _, ok := qa.Fallback.(*QAChunker); !ok {
		t.Fatalf("expected the qa profile's fallback to be *QAChunker, got %T", qa.Fallback)
	}

	// End-to-end: with no client (docling path skipped entirely), the table
	// profile's SplitDocument call should still keep a table's rows intact.
	tableChunks, err := table.SplitDocument(context.Background(), Document{
		Filename: "data.txt",
		Text:     "| A | B |\n| --- | --- |\n| 1 | 2 |\n",
	})
	if err != nil {
		t.Fatalf("SplitDocument (table): %v", err)
	}
	if len(tableChunks) != 1 || tableChunks[0].Metadata["chunk_type"] != "table" {
		t.Fatalf("expected table-aware fallback output, got %+v", tableChunks)
	}

	// End-to-end: same for the qa profile - one chunk per Q/A pair.
	qaChunks, err := qa.SplitDocument(context.Background(), Document{
		Filename: "faq.txt",
		Text:     "Q: What is X?\nA: X is a thing.\n",
	})
	if err != nil {
		t.Fatalf("SplitDocument (qa): %v", err)
	}
	if len(qaChunks) != 1 || qaChunks[0].Metadata["qa_question"] != "What is X?" {
		t.Fatalf("expected qa-aware fallback output, got %+v", qaChunks)
	}
}
