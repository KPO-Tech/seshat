package read

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KPO-Tech/seshat/internal/pdfsmart"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

const (
	// PDFInlineMaxChars is how much text a PDF may hold and still be returned whole when no pages are
	// asked for. A longer one gets a map of its pages instead, and the agent reads the parts it needs.
	PDFInlineMaxChars = 100_000

	// PDFMaxCharsPerRead bounds a single read of a page range. The read stops at a page boundary and
	// says which page to continue from.
	PDFMaxCharsPerRead = 120_000

	// renderPageToolName is the tool that shows a page as an image. It lives in a package that imports
	// this one, so the name is repeated here for the hints.
	renderPageToolName = "render_document_page"

	pdfCacheEntries = 16
)

// pdfDocument is what is worked out about one PDF once and kept: the per-page map, and the text an
// engine produced for pages that had no text layer, which is the expensive part to redo.
type pdfDocument struct {
	pages  []pdfsmart.PageInfo
	engine map[int]string
}

type pdfCacheKey struct {
	path    string
	size    int64
	modTime time.Time
}

var pdfCache = struct {
	sync.Mutex
	docs  map[pdfCacheKey]*pdfDocument
	order []pdfCacheKey
}{docs: map[pdfCacheKey]*pdfDocument{}}

// pdfDocumentFor returns the cached map of a PDF, building it when the file is new or has changed.
func pdfDocumentFor(path string, info os.FileInfo, data []byte) (*pdfDocument, error) {
	key := pdfCacheKey{path: path, size: info.Size(), modTime: info.ModTime()}
	pdfCache.Lock()
	if doc, ok := pdfCache.docs[key]; ok {
		pdfCache.Unlock()
		return doc, nil
	}
	pdfCache.Unlock()

	pages, err := pdfsmart.Analyze(data)
	if err != nil {
		return nil, err
	}
	doc := &pdfDocument{pages: pages, engine: map[int]string{}}

	pdfCache.Lock()
	defer pdfCache.Unlock()
	if existing, ok := pdfCache.docs[key]; ok {
		return existing, nil
	}
	pdfCache.docs[key] = doc
	pdfCache.order = append(pdfCache.order, key)
	for len(pdfCache.order) > pdfCacheEntries {
		delete(pdfCache.docs, pdfCache.order[0])
		pdfCache.order = pdfCache.order[1:]
	}
	return doc, nil
}

func (d *pdfDocument) engineText(page int) (string, bool) {
	pdfCache.Lock()
	defer pdfCache.Unlock()
	text, ok := d.engine[page]
	return text, ok
}

func (d *pdfDocument) rememberEngineText(page int, text string) {
	pdfCache.Lock()
	defer pdfCache.Unlock()
	d.engine[page] = text
}

func (d *pdfDocument) totalChars() int {
	total := 0
	for _, page := range d.pages {
		total += page.Chars
	}
	return total
}

// pdfTargetPages turns the pages parameter into the pages to read. With no parameter it is every page
// when the document is short, and none (mapOnly) when it is not.
func pdfTargetPages(pagesParam string, doc *pdfDocument) (pages []int, mapOnly bool, err error) {
	count := len(doc.pages)
	if strings.TrimSpace(pagesParam) == "" {
		if count <= PDFATMentionInlineThreshold && doc.totalChars() <= PDFInlineMaxChars {
			return allPages(count), false, nil
		}
		return nil, true, nil
	}
	parsed, err := ParsePDFPageRange(pagesParam)
	if err != nil {
		return nil, false, fmt.Errorf("invalid pages parameter: %w", err)
	}
	last := parsed.LastPage
	if last == -1 || last > count {
		last = count
	}
	if parsed.FirstPage > count {
		return nil, false, fmt.Errorf("this PDF has %d pages, page %d does not exist", count, parsed.FirstPage)
	}
	if last-parsed.FirstPage+1 > MaxPagesPerRead {
		return nil, false, fmt.Errorf("page range %q exceeds the maximum of %d pages per request, use a smaller range", pagesParam, MaxPagesPerRead)
	}
	for page := parsed.FirstPage; page <= last; page++ {
		pages = append(pages, page)
	}
	return pages, false, nil
}

func allPages(count int) []int {
	pages := make([]int, count)
	for i := range pages {
		pages[i] = i + 1
	}
	return pages
}

// readPDFFile reads a PDF page by page. The agent gets the text of the pages it asked for (all of them
// for a short document), a note on which pages hold images or have no readable text, and for a long
// document with no page range, a map of the pages instead of everything at once.
//
// A page with no text layer goes to the configured document reader when there is one. When none of the
// requested pages can be read at all, the PDF is handed over as a file so a model that can see it still can.
func (t *Tool) readPDFFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
	pagesParam string,
) (tool.CallResult, error) {
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
	default:
	}

	if fileInfo.Size() > t.config.MaxFileSize {
		return tool.NewErrorResult(fmt.Errorf("PDF too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxFileSize)), nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to read PDF: %w", err)), nil
	}
	doc, err := pdfDocumentFor(filePath, fileInfo, data)
	if err != nil {
		// Encrypted or malformed: nothing page-aware can be done, hand the file over as before.
		return t.readPDFAsFile(ctx, filePath, fileInfo, pagesParam)
	}

	targets, mapOnly, err := pdfTargetPages(pagesParam, doc)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if mapOnly {
		return tool.NewTextResult(t.formatPDFMarkdownResult(&FileReadResult{
			Type: FileTypePDFMarkdown,
			PDFMarkdown: &PDFMarkdownFileResult{
				FilePath:     filePath,
				OriginalSize: fileInfo.Size(),
				PageCount:    len(doc.pages),
				TotalChars:   doc.totalChars(),
				Map:          pdfPageMap(doc),
			},
		})), nil
	}

	texts, err := t.pdfPageTexts(ctx, data, doc, targets)
	if err != nil {
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(err), nil
	}

	var body strings.Builder
	var shown, noText, visual []int
	continueAt := 0
	for _, page := range targets {
		text := strings.TrimSpace(texts[page])
		if text == "" {
			noText = append(noText, page)
			continue
		}
		if len(shown) > 0 && body.Len()+len(text) > t.pdfMaxCharsPerRead() {
			continueAt = page
			break
		}
		fmt.Fprintf(&body, "--- page %d ---\n%s\n\n", page, text)
		shown = append(shown, page)
		if doc.pages[page-1].HasEmbeddedImage {
			visual = append(visual, page)
		}
	}
	if len(shown) == 0 {
		return t.readPDFAsFile(ctx, filePath, fileInfo, pagesParam)
	}

	return tool.NewTextResult(t.formatPDFMarkdownResult(&FileReadResult{
		Type: FileTypePDFMarkdown,
		PDFMarkdown: &PDFMarkdownFileResult{
			FilePath:     filePath,
			Markdown:     strings.TrimSpace(body.String()),
			OriginalSize: fileInfo.Size(),
			PageCount:    len(doc.pages),
			VisualPages:  visual,
			ShownPages:   shown,
			NoTextPages:  noText,
			ContinueAt:   continueAt,
		},
	})), nil
}

func (t *Tool) pdfMaxCharsPerRead() int {
	if t.config != nil && t.config.MaxPDFCharsPerRead > 0 {
		return t.config.MaxPDFCharsPerRead
	}
	return PDFMaxCharsPerRead
}

// pdfPageTexts returns the text of each requested page: the native text layer, or for a page without
// one the engine's text (remembered, since an engine call is the slow part).
func (t *Tool) pdfPageTexts(ctx context.Context, data []byte, doc *pdfDocument, targets []int) (map[int]string, error) {
	texts := make(map[int]string, len(targets))
	var toRead []int
	for _, page := range targets {
		if doc.pages[page-1].NoText {
			if cached, ok := doc.engineText(page); ok {
				texts[page] = cached
				continue
			}
		}
		toRead = append(toRead, page)
	}
	if len(toRead) == 0 {
		return texts, nil
	}
	sort.Ints(toRead)
	result, _, err := pdfsmart.ReadPages(ctx, data, toRead, pdfsmart.Options{}, t.documentReader, pdfsmart.VisionFallback{})
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}
	for _, page := range result.Pages {
		texts[page.Page] = page.Text
		if doc.pages[page.Page-1].NoText && strings.TrimSpace(page.Text) != "" {
			doc.rememberEngineText(page.Page, page.Text)
		}
	}
	return texts, nil
}

func pdfPageMap(doc *pdfDocument) []PDFPageInfo {
	pages := make([]PDFPageInfo, 0, len(doc.pages))
	for _, page := range doc.pages {
		pages = append(pages, PDFPageInfo{Page: page.Page, Chars: page.Chars, HasImage: page.HasEmbeddedImage, NoText: page.NoText})
	}
	return pages
}

// formatPageRanges writes 1,2,3,5 as "1-3, 5".
func formatPageRanges(pages []int) string {
	if len(pages) == 0 {
		return ""
	}
	sorted := append([]int(nil), pages...)
	sort.Ints(sorted)
	var parts []string
	start, prev := sorted[0], sorted[0]
	flush := func() {
		if start == prev {
			parts = append(parts, fmt.Sprintf("%d", start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for _, page := range sorted[1:] {
		if page == prev+1 {
			prev = page
			continue
		}
		flush()
		start, prev = page, page
	}
	flush()
	return strings.Join(parts, ", ")
}
