// Package pdfsmart re-exports internal/pdfsmart's page-aware PDF
// conversion for external consumers (e.g. seshat-ai).
package pdfsmart

import (
	"context"

	internalpdfsmart "github.com/KPO-Tech/seshat/internal/pdfsmart"
	"github.com/KPO-Tech/seshat/pkg/documentreader"
)

type (
	PageSource        = internalpdfsmart.PageSource
	PageResult        = internalpdfsmart.PageResult
	Result            = internalpdfsmart.Result
	PageRenderer      = internalpdfsmart.PageRenderer
	VisionTranscriber = internalpdfsmart.VisionTranscriber
	VisionFallback    = internalpdfsmart.VisionFallback
)

const (
	PageSourceNative  = internalpdfsmart.PageSourceNative
	PageSourceDocling = internalpdfsmart.PageSourceDocling
	PageSourceVision  = internalpdfsmart.PageSourceVision
)

// Convert extracts a PDF's text page by page, sending only the pages that
// actually need it (an embedded image, or a sparse/garbled native text
// layer) through documentReader instead of the whole document, with an
// optional vision-LLM fallback (see VisionFallback) for a page that still
// has no usable text afterward. See internal/pdfsmart's package doc for the
// exact routing rule, and Convert's own doc comment there for the ok=false
// safety contract. Pass VisionFallback{} to disable the vision stage.
func Convert(ctx context.Context, data []byte, documentReader documentreader.Converter, vision VisionFallback) (Result, bool, error) {
	return internalpdfsmart.Convert(ctx, data, documentReader, vision)
}
