package enricher

import internalenricher "github.com/KPO-Tech/seshat/internal/rag/enricher"

type (
	// LLMCaller is the minimal interface LLMEnricher needs to call the
	// provider API - normally satisfied by *providers.Client, but defined
	// as an interface so a caller can supply anything with a matching
	// CreateMessage method (or a fake, for tests).
	LLMCaller = internalenricher.LLMCaller

	// Config configures an LLMEnricher: Model, QuestionsPerChunk,
	// MaxConcurrency, and Timeout.
	Config = internalenricher.Config

	// LLMEnricher generates synthetic questions per chunk via an LLM - see
	// internal/rag/enricher's package doc for the rationale and
	// internal/rag.Enricher for the interface it implements.
	LLMEnricher = internalenricher.LLMEnricher
)

// New creates an LLMEnricher. caller is normally the same *providers.Client
// already used elsewhere for chat completions.
func New(caller LLMCaller, cfg Config) *LLMEnricher {
	return internalenricher.New(caller, cfg)
}
