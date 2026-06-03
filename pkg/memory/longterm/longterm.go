package longterm

import internallongterm "github.com/EngineerProjects/nexus-engine/internal/memory/longterm"

type (
	Entity            = internallongterm.Entity
	EntityInput       = internallongterm.EntityInput
	Extractor         = internallongterm.Extractor
	ExtractorConfig   = internallongterm.ExtractorConfig
	Graph             = internallongterm.Graph
	ObservationInput  = internallongterm.ObservationInput
	ObservationResult = internallongterm.ObservationResult
	Relation          = internallongterm.Relation
	RelationInput     = internallongterm.RelationInput
	Store             = internallongterm.Store
)

func DefaultExtractorConfig() ExtractorConfig {
	return internallongterm.DefaultExtractorConfig()
}

func NewExtractor(store Store, caller internallongterm.LLMCaller, cfg ExtractorConfig) *Extractor {
	return internallongterm.NewExtractor(store, caller, cfg)
}
