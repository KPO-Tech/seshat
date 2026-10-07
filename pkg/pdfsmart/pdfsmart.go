// Package pdfsmart re-exports internal/pdfsmart's page-aware PDF
// conversion for external consumers (e.g. seshat-ai).
package pdfsmart

import (
	"context"

	internalpdfsmart "github.com/KPO-Tech/seshat/internal/pdfsmart"
	"github.com/KPO-Tech/seshat/internal/pdftext"
	"github.com/KPO-Tech/seshat/pkg/documentreader"
)

type (
	PageSource        = internalpdfsmart.PageSource
	PageResult        = internalpdfsmart.PageResult
	Result            = internalpdfsmart.Result
	PageRenderer      = internalpdfsmart.PageRenderer
	VisionTranscriber = internalpdfsmart.VisionTranscriber
	VisionFallback    = internalpdfsmart.VisionFallback

	// TableFinder is a document reader that can also locate and lay out the tables with no ruling lines on a PDF
	// page, from a layout model (the native reader, with its models, is one). pdfsmart asks it for pages whose text
	// is columnar and fills the cells it finds from the page's own text. TableStructure is what it answers with.
	TableFinder    = internalpdfsmart.TableFinder
	TableStructure = pdftext.TableStructure
)

// ErrPasswordRequired is returned for a PDF protected by a user password that was not given, or was given wrongly.
var ErrPasswordRequired = internalpdfsmart.ErrPasswordRequired

// Unlock returns a PDF in a form the page reader can open: unchanged when it is not encrypted or the reader already
// opens it, a decrypted copy otherwise (AES-256 and anything with a user password). An empty password is what every
// viewer tries first and opens a PDF whose owner password only limits printing and copying. Convert and the page
// readers do this themselves with an empty password; call it with the user's password for a protected PDF.
func Unlock(data []byte, password string) ([]byte, error) {
	return internalpdfsmart.Unlock(context.Background(), data, password)
}

// UnlockContext is Unlock with a context: cancelling it stops the decryption of a large PDF.
func UnlockContext(ctx context.Context, data []byte, password string) ([]byte, error) {
	return internalpdfsmart.Unlock(ctx, data, password)
}

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
