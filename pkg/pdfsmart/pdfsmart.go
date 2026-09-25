// Package pdfsmart re-exports internal/pdfsmart's page-aware PDF
// conversion for external consumers (e.g. seshat-ai).
package pdfsmart

import (
	"context"

	internalpdfsmart "github.com/KPO-Tech/seshat/internal/pdfsmart"
	"github.com/KPO-Tech/seshat/pkg/docling"
)

type (
	PageSource = internalpdfsmart.PageSource
	PageResult = internalpdfsmart.PageResult
	Result     = internalpdfsmart.Result
)

const (
	PageSourceNative  = internalpdfsmart.PageSourceNative
	PageSourceDocling = internalpdfsmart.PageSourceDocling
)

// Convert extracts a PDF's text page by page, sending only the pages that
// actually need it (an embedded image, or a sparse/garbled native text
// layer) through doclingClient instead of the whole document. See
// internal/pdfsmart's package doc for the exact routing rule, and
// Convert's own doc comment there for the ok=false safety contract.
func Convert(ctx context.Context, data []byte, doclingClient docling.DocumentConverterBackend) (Result, bool, error) {
	return internalpdfsmart.Convert(ctx, data, doclingClient)
}
