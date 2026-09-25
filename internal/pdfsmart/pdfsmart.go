// Package pdfsmart extracts a PDF's text page by page, sending only the
// pages that actually need it through docling instead of the whole
// document - most PDFs are mostly prose, and paying docling's per-page
// layout-detection pass (see its standard PDF pipeline) for every page of
// a large document is wasted work when only a handful of pages have an
// embedded image or an unreadable native text layer.
//
// A page is routed to docling when:
//   - it has at least one embedded raster image (a PDF XObject with
//     /Subtype /Image, detected via pdfcpu's own resource inspection -
//     a deterministic read of the file's structure, not a heuristic or
//     an ML classification), or
//   - its native text layer is too sparse (see pdftext.MinCharsPerPage),
//     or looks like broken font-encoding extraction (see
//     textquality.IsGarbledText).
//
// This deliberately does NOT attempt to detect borderless tables or
// vector-drawn diagrams/charts - those aren't reliably detectable without
// a trained layout model (this is exactly why docling itself runs layout
// detection on every page unconditionally, rather than trying to guess
// which pages need it). A page containing one is only caught here if its
// native text also happens to be sparse or garbled; otherwise it's
// extracted natively, and the caller gets prose-quality text for a page
// that visually contained a table. Callers that can't accept ever missing
// a table this way (e.g. financial documents, invoices) should keep
// sending those documents through docling wholesale instead of this
// package - see Convert's own doc comment for the exact safety contract.
package pdfsmart

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/KPO-Tech/seshat/internal/docling"
	"github.com/KPO-Tech/seshat/internal/pdftext"
	"github.com/KPO-Tech/seshat/internal/textquality"
)

// pdfcpuConfigWarmup works around a real data race inside pdfcpu itself:
// model.NewDefaultConfiguration() (called internally by api.ExtractImagesRaw
// and api.Trim whenever conf is nil, as it is everywhere in this file)
// lazily populates an unsynchronized package-level cache
// (model.loadedDefaultConfig) on first use - concurrent first calls from
// multiple goroutines race on that write. Once initialized, later calls
// only read the cached value, which is safe to do concurrently. Calling
// NewDefaultConfiguration once, non-concurrently, before any pdfcpu call
// this package makes establishes that happens-before via sync.Once,
// making every subsequent call - however concurrent - safe.
var pdfcpuConfigWarmup sync.Once

func warmUpPDFCPUConfig() {
	pdfcpuConfigWarmup.Do(func() {
		_ = pdfcpumodel.NewDefaultConfiguration()
	})
}

// PageSource identifies how a page's text was obtained.
type PageSource string

const (
	PageSourceNative  PageSource = "native"
	PageSourceDocling PageSource = "docling"
	// PageSourceVision marks a page that needed the vision-LLM fallback -
	// native text extraction was too sparse/absent AND docling was either
	// unavailable or also couldn't produce usable text for it. See
	// VisionFallback's doc comment for how this stage is configured.
	PageSourceVision PageSource = "vision"
)

// PageRenderer renders a single PDF page to a raster image, feeding the
// vision-LLM fallback (see Convert's own doc comment for where this slots
// into the page-processing pipeline). Implementations reuse Phase 1's
// internal/nativedoc/pdfium CGO rendering (internal/nativedoc/parser's
// Converter implements this directly), wired in as an interface exactly
// like docling.DocumentConverterBackend is, so this package itself never
// depends on CGO or the nativedoc build tag.
type PageRenderer interface {
	// RenderPage renders 1-indexed page pageNum of the PDF in data,
	// returning PNG-encoded image bytes.
	RenderPage(ctx context.Context, data []byte, pageNum int) ([]byte, error)
}

// VisionTranscriber sends a rendered page image to a vision-capable LLM and
// returns its transcription. Implementations must report IsAvailable=false
// (rather than attempting the call and failing) when the configured model
// isn't multimodal - see internal/model.Registry.VisionCapable, the
// capability check used elsewhere in this codebase for the same purpose.
type VisionTranscriber interface {
	IsAvailable(ctx context.Context) bool
	// TranscribePage returns the page's transcribed text/markdown given its
	// PNG-encoded image bytes.
	TranscribePage(ctx context.Context, pngImage []byte) (string, error)
}

// VisionFallback bundles the two collaborators the vision-LLM fallback
// stage needs. Both Renderer and Transcriber must be set (and Transcriber
// must report IsAvailable) for the fallback to actually run on a page - the
// zero value (or either field left nil) simply skips this stage, the same
// as a nil/unavailable docling client skips the docling stage. Passed as
// its own value rather than two more positional Convert parameters so a
// future fallback stage doesn't need another breaking signature change.
type VisionFallback struct {
	Renderer    PageRenderer
	Transcriber VisionTranscriber
}

func (v VisionFallback) usable(ctx context.Context) bool {
	return v.Renderer != nil && v.Transcriber != nil && v.Transcriber.IsAvailable(ctx)
}

// PageResult is one page's contribution to a Result.
type PageResult struct {
	Page   int
	Text   string
	Source PageSource
}

// Result is the outcome of a page-aware PDF-to-markdown conversion.
type Result struct {
	Markdown string
	Pages    []PageResult
}

// DoclingPageCount reports how many pages were actually routed to
// docling - a direct measure of how much of the document needed the
// expensive path, useful for logging/tuning.
func (r Result) DoclingPageCount() int {
	n := 0
	for _, p := range r.Pages {
		if p.Source == PageSourceDocling {
			n++
		}
	}
	return n
}

// VisionPageCount reports how many pages needed the vision-LLM fallback -
// pages where neither native extraction nor docling produced usable text.
// Like DoclingPageCount, a direct measure of how much of the document
// needed the most expensive path, useful for logging/tuning.
func (r Result) VisionPageCount() int {
	n := 0
	for _, p := range r.Pages {
		if p.Source == PageSourceVision {
			n++
		}
	}
	return n
}

// Convert extracts data's text page by page (see package doc for the
// native-vs-docling routing rule per page). A page that still has no usable
// text after docling gets one more attempt via vision, when configured (see
// VisionFallback) - render the page and transcribe it with a vision-capable
// LLM, gated on that model actually reporting vision support (see
// VisionTranscriber.IsAvailable) so this is never attempted against a
// text-only model.
//
// ok is true only when EVERY page produced usable text, whether native, via
// docling, or via the vision fallback - if even one page that needed one of
// those couldn't get it, ok is false and Result should be discarded. This
// is a deliberate safety contract, not an oversight: a partial result
// missing one page's content is exactly the silent, permanent data loss
// this package exists to avoid on the pages it CAN cheaply skip the
// expensive paths for - it must never produce that same failure mode on a
// page it couldn't. Callers should fall back to sending the whole document
// through docling when ok is false, the same as if this package didn't
// exist.
func Convert(ctx context.Context, data []byte, doclingClient docling.DocumentConverterBackend, vision VisionFallback) (Result, bool, error) {
	warmUpPDFCPUConfig()

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, false, fmt.Errorf("pdfsmart: parse pdf: %w", err)
	}
	pageCount := reader.NumPage()
	if pageCount == 0 {
		return Result{}, false, fmt.Errorf("pdfsmart: pdf has no pages")
	}

	pagesWithImages, err := pagesWithEmbeddedImages(data)
	if err != nil {
		// Can't tell which pages have images - conservatively treat every
		// page as needing docling rather than silently extracting
		// natively past an image detection failed to find.
		pagesWithImages = allPages(pageCount)
	}

	result := Result{Pages: make([]PageResult, 0, pageCount)}
	var sb strings.Builder
	allOK := true

	for i := 1; i <= pageCount; i++ {
		needsDocling := pagesWithImages[i]
		var text string

		if !needsDocling {
			page := reader.Page(i)
			if page.V.IsNull() {
				needsDocling = true
			} else if native, extractErr := page.GetPlainText(nil); extractErr != nil ||
				len(strings.TrimSpace(native)) < pdftext.MinCharsPerPage || textquality.IsGarbledText(native) {
				needsDocling = true
			} else {
				text = native
			}
		}

		source := PageSourceNative
		if needsDocling {
			source = PageSourceDocling
			text = ""
			if doclingClient != nil && doclingClient.IsAvailable(ctx) {
				if pageData, extractErr := extractSinglePage(data, i); extractErr == nil {
					if conversion, convErr := doclingClient.ConvertBytes(ctx, pageData, fmt.Sprintf("page-%d.pdf", i)); convErr == nil &&
						strings.TrimSpace(conversion.Markdown) != "" && !textquality.IsGarbledText(conversion.Markdown) {
						text = conversion.Markdown
					}
				}
			}
			if strings.TrimSpace(text) == "" && vision.usable(ctx) {
				if img, renderErr := vision.Renderer.RenderPage(ctx, data, i); renderErr == nil {
					if transcribed, transErr := vision.Transcriber.TranscribePage(ctx, img); transErr == nil &&
						strings.TrimSpace(transcribed) != "" && !textquality.IsGarbledText(transcribed) {
						text = transcribed
						source = PageSourceVision
					}
				}
			}
			if strings.TrimSpace(text) == "" {
				allOK = false
			}
		}

		result.Pages = append(result.Pages, PageResult{Page: i, Text: text, Source: source})
		if text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(text)
		}
	}

	result.Markdown = sb.String()
	return result, allOK, nil
}

func allPages(n int) map[int]bool {
	m := make(map[int]bool, n)
	for i := 1; i <= n; i++ {
		m[i] = true
	}
	return m
}

// pagesWithEmbeddedImages reports which 1-indexed pages have at least one
// embedded raster image (a PDF XObject with /Subtype /Image), via
// pdfcpu's own resource inspection - a deterministic read of the file's
// structure, not a heuristic or an ML classification.
func pagesWithEmbeddedImages(data []byte) (map[int]bool, error) {
	imageSets, err := api.ExtractImagesRaw(bytes.NewReader(data), nil, nil)
	if err != nil {
		return nil, err
	}
	pages := make(map[int]bool)
	for _, set := range imageSets {
		for _, img := range set {
			if img.PageNr > 0 {
				pages[img.PageNr] = true
			}
		}
	}
	return pages, nil
}

// extractSinglePage produces a standalone one-page PDF containing just
// page n, so doclingClient only has to process that one page instead of
// the whole document.
func extractSinglePage(data []byte, n int) ([]byte, error) {
	var buf bytes.Buffer
	if err := api.Trim(bytes.NewReader(data), &buf, []string{strconv.Itoa(n)}, nil); err != nil {
		return nil, fmt.Errorf("pdfsmart: extract page %d: %w", n, err)
	}
	return buf.Bytes(), nil
}
