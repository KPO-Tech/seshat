package documentreader

import internalreader "github.com/KPO-Tech/seshat/internal/documentreader"

type (
	APIError         = internalreader.APIError
	ConversionResult = internalreader.ConversionResult
	Chunk            = internalreader.Chunk
	ChunkOptions     = internalreader.ChunkOptions
	ConvertOptions   = internalreader.ConvertOptions
	ExtractedImage   = internalreader.ExtractedImage
	RetryConfig      = internalreader.RetryConfig

	// Converter is anything that can convert documents to
	// markdown. See internalreader.Converter's own doc comment.
	Converter = internalreader.Converter

	// HybridChunker is anything that can produce document-aware hybrid
	// chunks. See internalreader.HybridChunker's own doc comment.
	HybridChunker = internalreader.HybridChunker

	// GenericClient is a configurable client for any document-intelligence
	// HTTP server that accepts a multipart file upload and returns JSON -
	// every protocol detail (paths, the multipart file field name, response
	// parsing) is supplied via GenericConfig.
	GenericClient = internalreader.GenericClient

	// GenericConfig configures a GenericClient.
	GenericConfig = internalreader.GenericConfig

	// ConvertParser translates a server's raw convert-endpoint response
	// body into ConversionResult.
	ConvertParser = internalreader.ConvertParser

	// ChunkParser translates a server's raw chunk-endpoint response body
	// into []Chunk.
	ChunkParser = internalreader.ChunkParser
)

// NewGenericClient validates cfg and builds a GenericClient - see
// GenericConfig's own field docs for what's required.
func NewGenericClient(cfg GenericConfig) (*GenericClient, error) {
	return internalreader.NewGenericClient(cfg)
}
