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

Seshat uses [docling-serve](https://github.com/DS4SD/docling) (a local Python process managed by `seshat setup`) to convert documents before indexing:

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

### Document-aware chunking

The core RAG service can now accept both extracted text and the original document bytes through `rag.IngestRequest.Data`. Plain text chunkers continue to split `IngestRequest.Text` as before. Chunkers that implement `rag.DocumentChunker` can use the original bytes to preserve document structure.

`rag.NewDoclingChunker` provides the first document-aware chunker. It calls docling-serve's hybrid chunk endpoint and maps Docling metadata onto Seshat chunk metadata:

- headings;
- captions;
- page numbers;
- Docling document item references;
- raw text when it differs from contextualized chunk text;
- token count when Docling returns it.

This path is intended for rich Knowledge ingestion, especially PDF, DOCX, PPTX, XLSX, and other structured enterprise documents. The existing paragraph and semantic chunkers remain useful fallbacks for local/simple text ingestion.

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
