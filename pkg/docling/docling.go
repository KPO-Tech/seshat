package docling

import internaldocling "github.com/EngineerProjects/nexus-engine/internal/docling"

type (
	Client           = internaldocling.Client
	ConversionResult = internaldocling.ConversionResult
	ExtractedImage   = internaldocling.ExtractedImage
)

func NewClient(baseURL string) *Client {
	return internaldocling.NewClient(baseURL)
}
