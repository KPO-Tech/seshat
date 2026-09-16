package ragtool

const (
	ToolSearchName = "rag_search"
	ToolIngestName = "rag_ingest"
	ToolDeleteName = "rag_delete"

	SearchHint = "Search a document corpus using semantic similarity. Use when documents have been ingested and you need to find relevant passages."
	IngestHint = "Ingest a text document into a named corpus for later semantic search. Returns the number of indexed chunks."
	DeleteHint = "Delete an entire corpus, or a single file's chunks within a corpus."

	DefaultTopK = 5
)
