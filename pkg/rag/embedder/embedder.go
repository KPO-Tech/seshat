package embedder

import internalembedder "github.com/EngineerProjects/nexus-engine/internal/rag/embedder"

type (
	Config   = internalembedder.Config
	Embedder = internalembedder.Embedder
	Provider = internalembedder.Provider
)

func New(cfg *Config) *Embedder {
	return internalembedder.New(cfg)
}

func DetectProviderPublic(baseURL string) Provider {
	return internalembedder.DetectProviderPublic(baseURL)
}
