package ragtool

const (
	ToolSearchName = "rag_search"
	ToolIngestName = "rag_ingest"

	SearchHint = "Search a document corpus using semantic similarity. Use when documents have been ingested and you need to find relevant passages."
	IngestHint = "Ingest a text document into a named corpus for later semantic search. Returns the number of indexed chunks."

	DefaultTopK = 5
)
