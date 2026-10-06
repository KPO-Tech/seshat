// Package officetext re-exports internal/officetext's native DOCX/PPTX/XLSX
// text extraction for external consumers (e.g. seshat-ai's upload and RAG
// ingestion pipelines) that want the same conversion the read_file tool uses
// without going through an external document-reader service.
package officetext

import internalofficetext "github.com/KPO-Tech/seshat/internal/officetext"

// SupportedExtensions lists the formats Extract can handle natively.
var SupportedExtensions = internalofficetext.SupportedExtensions

// ErrEmpty is returned when a document parses but yields no extractable text.
var ErrEmpty = internalofficetext.ErrEmpty

// MinCharsPerSlide is re-exported for callers that want to reason about the
// PPTX sparse threshold directly (e.g. logging/diagnostics); Extract itself
// applies it automatically via sparse.
const MinCharsPerSlide = internalofficetext.MinCharsPerSlide

// Extract converts a DOCX/PPTX/XLSX file's bytes to markdown. ok is false
// when filename's extension isn't natively supported. sparse is true when
// extraction "succeeded" but yielded implausibly little text for the
// format - currently only computed for PPTX, where a deck that's mostly
// screenshots/diagrams can parse fine while still needing OCR to
// get anything useful out of it. Callers should treat sparse the same way
// they already treat pdftext.Result.Sparse: prefer a document reader over
// trusting a sparse native result.
func Extract(filename string, data []byte) (markdown string, ok bool, sparse bool, err error) {
	return internalofficetext.Extract(filename, data)
}
