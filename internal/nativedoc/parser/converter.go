//go:build cgo && nativedoc

package parser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat/internal/documentreader"
	"github.com/KPO-Tech/seshat/internal/nativedoc"
)

// defaultRenderDPI matches deepdoc's own default rendering resolution for
// the OCR path - high enough for det/rec to read normal body text cleanly
// without the memory/latency cost of a much higher DPI.
const defaultRenderDPI = 150

// defaultMinCharsPerPage is the native-text-layer length below which a PDF
// page is treated as scanned/sparse and routed through OCR instead.
// Deliberately simple (a length check, not internal/pdfsmart's fuller
// image-XObject-or-garbled-text heuristic) - this package has no dependency
// on pdfsmart/pdftext; unifying the two heuristics is future work, not a
// correctness requirement for a first working version.
const defaultMinCharsPerPage = 40

// Converter implements documentreader.Converter using only native
// Go components - no docling-serve, no seshat-intelligence. It dispatches
// by file extension: .pdf goes through pdf_oxide/pdfium/OCR (see pdf.go,
// CGO required for the OCR fallback path); .docx and .xlsx delegate to
// seshat's existing pure-Go, zero-CGO internal/officetext extractors (see
// docx.go) rather than porting RAGFlow's CGO office_oxide wrapper - tested
// head-to-head and the output was identical, so the CGO dependency
// wouldn't have bought anything. Other formats aren't supported yet - see
// docs/issues/document-intelligence-roadmap.md for what's still open.
type Converter struct {
	// ModelDir must contain det.ort, rec.ort, and ocr.res (see nativedoc's
	// own doc comment for the full model list and where to fetch them).
	// Only needed for PDF pages that fall back to OCR.
	ModelDir string
	// RenderDPI overrides defaultRenderDPI when set (> 0). PDF only.
	RenderDPI float64
	// MinCharsPerPage overrides defaultMinCharsPerPage when set (> 0). PDF only.
	MinCharsPerPage int
}

// New creates a Converter backed by the ONNX models under modelDir.
func New(modelDir string) *Converter {
	return &Converter{ModelDir: modelDir}
}

func (c *Converter) renderDPI() float64 {
	if c.RenderDPI > 0 {
		return c.RenderDPI
	}
	return defaultRenderDPI
}

func (c *Converter) minCharsPerPage() int {
	if c.MinCharsPerPage > 0 {
		return c.MinCharsPerPage
	}
	return defaultMinCharsPerPage
}

// IsAvailable reports whether the ONNX Runtime environment is ready. DOCX
// and native-text-layer PDF pages would still work even if this is false
// (neither needs ONNX), but callers use this as a simple readiness gate for
// the whole backend, so it also validates against the OCR path being usable.
func (c *Converter) IsAvailable(_ context.Context) bool {
	return nativedoc.Initialized()
}

func (c *Converter) ConvertFile(ctx context.Context, filePath string) (*documentreader.ConversionResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: read %s: %w", filePath, err)
	}
	return c.ConvertBytes(ctx, data, filePath)
}

// ConvertURL fetches the document itself - unlike docling-serve, which
// fetches server-side, there's no separate service here to delegate to.
func (c *Converter) ConvertURL(ctx context.Context, docURL string) (*documentreader.ConversionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: build request for %s: %w", docURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: fetch %s: %w", docURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nativedoc/parser: fetch %s: HTTP %d", docURL, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: read response from %s: %w", docURL, err)
	}
	return c.ConvertBytes(ctx, data, docURL)
}

func (c *Converter) ConvertBytes(ctx context.Context, data []byte, filename string) (*documentreader.ConversionResult, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return c.convertPDF(ctx, data, filename)
	case ".docx":
		return c.convertDOCX(data, filename)
	case ".xlsx":
		return c.convertXLSX(data, filename)
	default:
		return nil, fmt.Errorf("nativedoc/parser: unsupported format for %s (only .pdf, .docx, .xlsx so far)", filename)
	}
}

var _ documentreader.Converter = (*Converter)(nil)
