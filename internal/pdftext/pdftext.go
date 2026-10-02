// Package pdftext extracts plain text from PDFs that carry a real text
// layer (i.e. not a scan) without any external service. It exists so
// readPDFFile has a better no-docling fallback than raw base64 pass-through:
// most PDFs - reports, exports, invoices - are text-native, and pdfcpu
// (already a dependency here) only exposes page/content-stream manipulation,
// not decoded text.
package pdftext

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MinCharsPerPage is the threshold below which extracted text is treated as
// "not really there" - a scanned/image-only PDF will still yield a handful
// of stray characters from stamps or embedded metadata, but nowhere near
// this per page on average. Below it, callers should prefer OCR (docling)
// or the base64/vision fallback instead of trusting this near-empty text.
const MinCharsPerPage = 20

// Result holds the outcome of a native PDF text extraction attempt.
type Result struct {
	Text      string
	PageCount int
	// Sparse is true when the extracted text is too little relative to the
	// page count to be a real text layer - most likely a scanned PDF.
	Sparse bool
}

// Extract reads a PDF's embedded text layer. It does not attempt OCR: a
// scanned PDF with no text layer will return a Sparse result with little or
// no text, not an error - callers decide what to do next (try docling, or
// fall back to sending the raw PDF to a vision-capable model).
func Extract(data []byte) (*Result, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	pageCount := r.NumPage()
	var sb strings.Builder
	for i := 1; i <= pageCount; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := PageText(page)
		if err != nil {
			// A single malformed page shouldn't sink extraction for the
			// whole document - skip it and keep going.
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	text := strings.TrimSpace(sb.String())
	sparse := pageCount == 0 || len(text) < MinCharsPerPage*pageCount

	return &Result{
		Text:      text,
		PageCount: pageCount,
		Sparse:    sparse,
	}, nil
}
