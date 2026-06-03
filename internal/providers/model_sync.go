package providers

import (
	"github.com/EngineerProjects/nexus-engine/internal/model"
)

// init populates model.Global from AllProvidersInfo() so that every caller of
// model.Global.Lookup() gets accurate, up-to-date data.
//
// This is the single registration point: to add a model, update registry.go.
// model.Global is never populated from a separate hand-maintained catalog.
func init() {
	for provider, pinfo := range AllProvidersInfo() {
		for _, minfo := range pinfo.Models {
			model.Global.Register(model.Metadata{
				ID:          minfo.Identifier,
				Provider:    string(provider),
				Description: minfo.Description,
				ContextWindow: model.ContextWindow{
					MaxTokens:       minfo.ContextWindow,
					MaxOutputTokens: minfo.MaxOutput,
				},
				DefaultTemperature: minfo.DefaultTemperature,
				Capabilities:       minfo.Capabilities,
				Pricing:            model.Pricing{}, // detailed pricing tracked externally
			})
		}
	}
}
