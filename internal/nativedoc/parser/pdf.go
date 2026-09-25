//go:build cgo && nativedoc

// Package parser assembles seshat's native document primitives - pdf_oxide
// (text extraction), pdfium (page rendering, see nativedoc/pdfium's own doc
// comment for why rendering is split from pdf_oxide), nativedoc
// (OCR/layout/table-structure ONNX models), and internal/officetext (DOCX,
// XLSX) - into a real docling.DocumentConverterBackend implementation (see
// Converter in converter.go). This is what makes seshat's document-reading
// tools work with no external service (docling-serve, seshat-intelligence)
// configured, not just a validation harness.
//
// This is original orchestration code, not ported from RAGFlow - RAGFlow's
// own equivalent (internal/deepdoc/parser/) does considerably more
// (layout-aware reading order via DLA, table extraction via TSR, image
// extraction) than the deliberately-scoped v1 here. See
// docs/issues/document-intelligence-roadmap.md for what's intentionally
// not built yet.
package parser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pdfoxide "github.com/yfedoseev/pdf_oxide/go"

	"github.com/KPO-Tech/seshat/internal/docling"
	"github.com/KPO-Tech/seshat/internal/nativedoc"
	"github.com/KPO-Tech/seshat/internal/nativedoc/pdfium"
)

// convertPDF converts a PDF. Per page: native text extraction (pdf_oxide)
// is used when the page has a real text layer; otherwise the page is
// rendered (pdfium) and read via OCR (nativedoc's det/rec models). Reading
// order for OCR'd pages is a simple top-to-bottom, left-to-right sort of
// detected text boxes - correct for single-column pages, not yet
// layout-aware for multi-column ones (that needs DLA-based column/region
// detection, not built here yet).
func (c *Converter) convertPDF(ctx context.Context, data []byte, filename string) (*docling.ConversionResult, error) {
	doc, err := pdfoxide.OpenFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: open %s: %w", filename, err)
	}
	defer doc.Close()

	count, err := doc.PageCount()
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: page count for %s: %w", filename, err)
	}

	pages := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, textErr := doc.ExtractText(i)
		if textErr == nil && len(strings.TrimSpace(text)) >= c.minCharsPerPage() {
			pages = append(pages, strings.TrimSpace(text))
			continue
		}
		ocrText, ocrErr := c.ocrPDFPage(ctx, data, i)
		if ocrErr != nil {
			return nil, fmt.Errorf("nativedoc/parser: page %d of %s has no usable native text and OCR failed: %w", i, filename, ocrErr)
		}
		pages = append(pages, ocrText)
	}

	return &docling.ConversionResult{
		Markdown:  strings.Join(pages, "\n\n"),
		PageCount: count,
	}, nil
}

func (c *Converter) ocrPDFPage(ctx context.Context, pdfData []byte, pageIdx int) (string, error) {
	rgba, err := pdfium.RenderPage(pdfData, pageIdx, c.renderDPI())
	if err != nil {
		return "", fmt.Errorf("render page: %w", err)
	}
	img, err := nativedoc.FromImage(rgba)
	if err != nil {
		return "", fmt.Errorf("convert rendered page: %w", err)
	}

	det, err := nativedoc.RunDet(ctx, c.ModelDir, img)
	if err != nil {
		return "", fmt.Errorf("detect text: %w", err)
	}
	boxes := append([]nativedoc.DetBox(nil), det.Boxes...)
	sortReadingOrder(boxes)

	lines := make([]string, 0, len(boxes))
	for _, box := range boxes {
		crop := cropBox(img, box)
		rec, recErr := nativedoc.RunOCRRec(ctx, c.ModelDir, crop)
		if recErr != nil || strings.TrimSpace(rec.Text) == "" {
			continue
		}
		lines = append(lines, rec.Text)
	}
	return strings.Join(lines, "\n"), nil
}

// sortReadingOrder orders boxes top-to-bottom then left-to-right, using
// each box's top edge and left edge (Pts[0] is top-left, clockwise - see
// DetBox's own doc comment). Correct for single-column text; a multi-column
// layout needs DLA-based column detection to interleave correctly, which
// this package doesn't do yet.
func sortReadingOrder(boxes []nativedoc.DetBox) {
	sort.Slice(boxes, func(i, j int) bool {
		yi, yj := boxes[i].Pts[0][1], boxes[j].Pts[0][1]
		if yi != yj {
			return yi < yj
		}
		return boxes[i].Pts[0][0] < boxes[j].Pts[0][0]
	})
}

// cropBox extracts the axis-aligned bounding rectangle of a detected box
// from the source image. DetBox quads from this document type are
// typically already axis-aligned (horizontal text); a rotated/skewed quad
// would need a perspective-corrected crop instead, which isn't implemented
// here yet.
func cropBox(img *nativedoc.Image, box nativedoc.DetBox) *nativedoc.Image {
	minX, minY := box.Pts[0][0], box.Pts[0][1]
	maxX, maxY := box.Pts[0][0], box.Pts[0][1]
	for _, p := range box.Pts {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	x0, y0 := int(minX), int(minY)
	x1, y1 := int(maxX), int(maxY)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > img.W {
		x1 = img.W
	}
	if y1 > img.H {
		y1 = img.H
	}
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		return &nativedoc.Image{W: 1, H: 1, Pix: make([]byte, 3)}
	}
	pix := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		srcOff := ((y0+y)*img.W + x0) * 3
		dstOff := y * w * 3
		copy(pix[dstOff:dstOff+w*3], img.Pix[srcOff:srcOff+w*3])
	}
	return &nativedoc.Image{W: w, H: h, Pix: pix}
}
