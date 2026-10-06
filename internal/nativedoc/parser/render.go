//go:build cgo && nativedoc

package parser

import (
	"bytes"
	"context"
	"fmt"
	"image/png"

	"github.com/KPO-Tech/seshat/internal/nativedoc/pdfium"
)

// RenderPage renders 1-indexed page pageNum of the PDF in data to a
// PNG-encoded image, using pdfium (see nativedoc/pdfium's own doc comment
// for why rendering goes through pdfium rather than pdf_oxide). Satisfies
// pdfsmart.PageRenderer (duck-typed - see that interface's own doc comment
// for why no import is needed here), so a Converter doubles as the
// vision-LLM fallback's page renderer with no extra wiring beyond the
// existing RenderDPI field convertPDF's OCR path already uses.
func (c *Converter) RenderPage(ctx context.Context, data []byte, pageNum int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, err := pdfium.RenderPage(data, pageNum-1, c.renderDPI())
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: render page %d: %w", pageNum, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("nativedoc/parser: encode page %d as png: %w", pageNum, err)
	}
	return buf.Bytes(), nil
}
