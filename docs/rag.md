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

---

## Hybrid search

Seshat combines two retrieval strategies for better results than pure vector search:

| Strategy | How it works | Good for |
|---|---|---|
| **BM25** | Keyword-based, exact term matching | Named entities, code identifiers, precise terms |
| **Vector (semantic)** | Embedding similarity | Conceptual queries, paraphrase matching |

Results from both are fused and re-ranked before being presented to the agent.

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
