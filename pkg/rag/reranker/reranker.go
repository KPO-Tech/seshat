package reranker

import internalreranker "github.com/EngineerProjects/nexus-engine/internal/rag/reranker"

type LangSearchReranker = internalreranker.LangSearchReranker

func NewLangSearchRerankerWithKey(apiKey string) *LangSearchReranker {
	return internalreranker.NewLangSearchRerankerWithKey(apiKey)
}
