package reranker

import internalreranker "github.com/KPO-Tech/seshat/internal/rag/reranker"

type LangSearchReranker = internalreranker.LangSearchReranker

func NewLangSearchRerankerWithKey(apiKey string) *LangSearchReranker {
	return internalreranker.NewLangSearchRerankerWithKey(apiKey)
}
