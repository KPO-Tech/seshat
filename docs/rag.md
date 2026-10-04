# RAG System in Seshat

Seshat includes a built-in Retrieval-Augmented Generation (RAG) system. Agents can ingest documents, query knowledge bases, and retrieve relevant content into their context without any external vector database or embedding service.

> Full documentation: [seshat-ai.com/docs/concepts/memory-rag](https://seshat-ai.com/docs/concepts/memory-rag)

---

## What RAG does

RAG lets an agent answer questions about a body of documents that is too large to fit in the context window. Instead of pasting the entire document, the agent retrieves only the relevant chunks.

Seshat's RAG pipeline:

1. **Ingestion** - documents are parsed, chunked, embedded and indexed.
2. **Retrieval** - at query time, the most relevant chunks are fetched using hybrid search.
3. **Augmentation** - retrieved chunks are injected into the agent's context alongside the user's question.

---

## Document ingestion

Seshat can convert rich documents before indexing through a configured document reader. The local/native readers are preferred when available; an HTTP document-reader service can be supplied through `document_reader_url` for OCR-heavy or layout-heavy formats:

| Format | Support |
|---|---|
| PDF | Full text extraction, tables, figures |
| DOCX / PPTX / XLSX | Full extraction |
| Markdown | Native |
| HTML | Via fetch |
| Audio (MP3, WAV) | Transcription via Whisper |

```bash
# Index a document
seshat rag add ./docs/architecture.pdf --collection "internal-docs"

# Index a URL
seshat rag add https://example.com/spec.pdf --collection "specs"

# List collections
seshat rag list
```

### Default reader

`pkg/documentreading` is the engine's default reader for a host. `Convert` / `ConvertBytes` read DOCX, PPTX and XLSX natively and PDFs page by page (`pdfsmart`): a page keeps its own text layer unless it carries an image, has almost no text, or has garbled text, and only those pages go to the optional external converter. Scans, audio and images need that converter; without one they yield no result rather than an error, so a host with nothing configured still reads every native format. External output is checked for garbled text as well. A caller that cannot accept a missed borderless table or chart (invoices, financial reports) should send those documents to the converter whole instead.

### Document-aware chunking

The core RAG service can now accept both extracted text and the original document bytes through `rag.IngestRequest.Data`. Plain text chunkers continue to split `IngestRequest.Text` as before. Chunkers that implement `rag.DocumentChunker` can use the original bytes to preserve document structure.

`rag.NewHybridDocumentChunker` provides a document-aware chunker. It calls the configured document reader's hybrid chunk path and maps returned structure onto neutral Seshat chunk metadata:

- headings;
- captions;
- page numbers;
- document item references;
- raw text when it differs from contextualized chunk text;
- token count when the backend returns it.

This path is intended for rich Knowledge ingestion, especially PDF, DOCX, PPTX, XLSX, and other structured enterprise documents. The existing paragraph and semantic chunkers remain useful fallbacks for local/simple text ingestion.

### Chunking markdown (`HeadingChunker`, `TableChunker`)

What the readers write is markdown, and `rag.HeadingChunker` (the fallback of the `structured` profile, 512 tokens at most with 64 of overlap) cuts it along its structure instead of at a fixed length:

- it reads blocks: headings (`#`, numbered outlines such as `1.2 Title`, and structural words such as Chapter/Chapitre/Capítulo/Kapitel, Article/Artigo/Artikel), paragraphs, lists, tables and fenced code. A numbered list is a list, not a series of headings, and the lines of a code block are never headings;
- a block is never cut in the middle of what it is: a paragraph is cut between sentences (the next piece starts with the last sentences of the one before, up to the profile's overlap), a list between items, a table between rows (each piece repeats the header row) and code between lines (each piece is fenced again);
- small sections are joined to their neighbours up to the size limit, their headings written in the text, so no chunk is a title and one line;
- each chunk starts with the path of headings it sits under, and carries it in `Metadata["heading_path"]` (titles only, joined with ` > `); a chunk that is only a table or only code has `chunk_type` `table` or `code`;
- a PDF's text has a marker line before each page (`documentreading.Result.PagedMarkdown`, `<!-- page 3 -->`); the chunker removes the markers from the text and gives the pages a chunk covers in `Metadata["page_numbers"]`, as a JSON array (`[3,4]`). Slides and sheets are already in the heading path (`Slide 3 > Title`, `Sheet1`);
- a heading with nothing under it is not lost: its title is in the path of what follows, or, when nothing follows it, it is written as text.

Both are public: `rag.NewHeadingChunker(profile)` and `rag.NewTableChunker(profile)`, with `rag.RecommendedChunkProfile(rag.ChunkProfileStructured)` for the profile hosts use; they are `DocumentChunker`s, so `rag.NewCachedDocumentChunker` wraps them.

`rag.TableChunker` is the same engine with every table kept apart, as its own chunk or chunks. `chunk_quality_test.go` holds both to measures on real reader output (`testdata/chunking`): no word of the source missing from the chunks, no table piece without its header, no unbalanced code fence, no chunk over the limit, and few chunks under 40 tokens; `RAG_CHUNK_CORPUS=<dir of .md files>` runs the same checks on a bigger corpus.

For repeated ingestion of the same document, wrap a document-aware chunker with `rag.NewCachedDocumentChunker`. The cache key includes the document content, filename, cache schema version, and chunker options when the chunker exposes a cache fingerprint. `rag.NewArtifactChunkCache` persists cached chunks through the runtime artifact store, while `rag.NewMemoryChunkCache` is useful for tests and short-lived local runs.

---

## Hybrid search

Seshat combines two retrieval strategies for better results than pure vector search:

| Strategy | How it works | Good for |
|---|---|---|
| **BM25** | Keyword-based, exact term matching | Named entities, code identifiers, precise terms |
| **Vector (semantic)** | Embedding similarity | Conceptual queries, paraphrase matching |

Results from both are fused and re-ranked before being presented to the agent.

---

## Vector backends

The CLI defaults to the embedded local-first path: HNSW when available, with a SQLite fallback. For larger knowledge bases or enterprise deployments, Seshat can use OpenSearch as the RAG vector store:

```bash
SESHAT_VECTOR_STORE=opensearch
OPENSEARCH_ADDRESSES=http://localhost:9200
OPENSEARCH_INDEX_PREFIX=seshat-rag
OPENSEARCH_KNN=true
OPENSEARCH_BULK_SIZE=500
```

Optional authentication:

```bash
OPENSEARCH_USERNAME=seshat
OPENSEARCH_PASSWORD=...

# or
OPENSEARCH_API_KEY=...
```

OpenSearch uses one index per Seshat namespace. Text search uses OpenSearch BM25, vector search uses `knn_vector` when `OPENSEARCH_KNN=true`, and hybrid search is fused by Seshat so ranking behavior remains consistent with the rest of the runtime. Ingestion uses the OpenSearch `_bulk` API; `OPENSEARCH_BULK_SIZE` controls how many records are sent per request and defaults to 500.

To run the optional integration test against a local or remote OpenSearch cluster:

```bash
OPENSEARCH_INTEGRATION_URL=http://localhost:9200 go test ./internal/vector -run TestOpenSearchStoreIntegration -count=1
```

---

## Embedding models

Seshat uses local embedding models via [Ollama](https://ollama.com) or remote APIs:

```go
client, _ := sdk.NewClient(&sdk.ClientConfig{
    RAGConfig: &sdk.RAGConfig{
        EmbeddingProvider: "ollama",
        EmbeddingModel:    "nomic-embed-text",
        ChunkSize:         512,
        ChunkOverlap:      64,
    },
})
```

Supported embedding providers: Ollama (local), OpenAI, Google, Mistral.

---

## Agent tools

The `search_knowledge` built-in tool is available in every session:

```
search_knowledge(query="architecture of the permission system", collection="internal-docs", top_k=5)
```

The agent calls this tool autonomously when it needs to retrieve information. Results are formatted as cited excerpts and injected into the context.

---

## Related docs

- [Memory and Compaction](./memory.md) - session memory and the agent memory tool
- [MCP Client](./mcp.md) - connect external knowledge servers via MCP
- [Planning Mode](./planning.md) - how the agent decides when to search vs act
- [Tools](./tools.md) - full built-in tool reference
