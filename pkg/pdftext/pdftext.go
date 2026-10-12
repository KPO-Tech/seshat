// Package pdftext re-exports internal/pdftext's native PDF text extraction
// for external consumers (e.g. seshat-ai) that want the same no-docling
// fallback the read_file tool uses.
package pdftext

import internalpdftext "github.com/KPO-Tech/seshat-runtime/internal/pdftext"

// MinCharsPerPage mirrors internal/pdftext.MinCharsPerPage.
const MinCharsPerPage = internalpdftext.MinCharsPerPage

// Result mirrors internal/pdftext.Result.
type Result = internalpdftext.Result

// Extract reads a PDF's embedded text layer natively (no OCR, no external
// service). See internal/pdftext.Extract for details.
func Extract(data []byte) (*Result, error) {
	return internalpdftext.Extract(data)
}
