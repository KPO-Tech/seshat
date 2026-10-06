//go:build cgo && nativedoc

// Package nativedoc re-exports internal/nativedoc's fully native (no
// external service) document-conversion backend for external consumers
// (e.g. seshat-ai/seshat-backend).
//
// This is opt-in, not wired in as a default anywhere in seshat itself:
// internal/tools/builtin and pkg/sdk stay CGO-independent (a plain
// `go build ./...` works with no native OCR dependencies at all). A caller
// that wants native PDF/DOCX/XLSX conversion - including OCR for scanned
// PDFs - constructs a Converter here and passes it as
// sdk.ClientConfig.DocumentConverter, the same injection point used for any
// custom Converter. See
// docs/issues/document-intelligence-roadmap.md for the full picture
// (model provisioning, build tooling, Windows support).
package nativedoc

import (
	internalnativedoc "github.com/KPO-Tech/seshat/internal/nativedoc"
	"github.com/KPO-Tech/seshat/internal/nativedoc/parser"
	"github.com/KPO-Tech/seshat/pkg/pdfsmart"
)

// Converter implements documentreader.Converter for PDF, DOCX, and
// XLSX using only native Go/CGO components. See parser.Converter's own doc
// comment for exactly what each format path does and doesn't do yet. It also
// implements pdfsmart.PageRenderer (RenderPage, see render.go in the parser
// package) - the same Converter doubles as the page renderer for
// pdfsmart's vision-LLM fallback, no extra wiring needed.
type Converter = parser.Converter

var _ pdfsmart.PageRenderer = (*Converter)(nil)

// New creates a Converter. modelDir must contain det.ort, rec.ort, and
// ocr.res (the ONNX models used for OCR on scanned/image-only PDF pages) -
// DOCX/XLSX and PDFs with a native text layer don't need them. Fetch the
// models from https://huggingface.co/InfiniFlow/deepdoc (Apache-2.0).
func New(modelDir string) *Converter {
	return parser.New(modelDir)
}

// InitORT initializes the process-global ONNX Runtime environment. Call
// once at process start, before using a Converter on any scanned PDF page -
// see internal/nativedoc.InitORT's own doc comment. Safe to call even if
// you'll only ever convert DOCX/XLSX/native-text PDFs (which don't need
// ONNX Runtime at all), but there's no reason to call it if you know you
// won't need OCR.
func InitORT() error {
	return internalnativedoc.InitORT()
}

// Initialized reports whether InitORT has already succeeded.
func Initialized() bool {
	return internalnativedoc.Initialized()
}
