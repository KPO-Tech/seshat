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
	// PDFMaxCharsPerRead bounds one read. It stops at a page boundary and says which page to continue
	// from, so a long document is read in several calls without any call being huge.
	PDFMaxCharsPerRead = 120_000

	// PDFMaxEnginePagesPerRead bounds how many pages with no text layer one read sends to the document
	// reader, since each of those is a slow call. Pages with a text layer cost almost nothing and are
	// not counted.
	PDFMaxEnginePagesPerRead = 20

	// pdfBatchPages is how many pages are read at a time, so a read can stop at a limit without having
	// paid for pages it will not return.
	pdfBatchPages = 10

	// renderPageToolName is the tool that shows a page as an image. It lives in a package that imports
	// this one, so the name is repeated here for the hints.
	renderPageToolName = "render_document_page"

	pdfCacheEntries = 16
)

// pdfPage is one page once read: its text (from the text layer, or from the document reader for a page
// without one) and whether it holds an image.
type pdfPage struct {
	text     string
	hasImage bool
}

// pdfDocument is what is kept about one PDF between reads. Pages are read when asked for and
// remembered, so a page read once (above all one an engine had to read) is never paid for again, and a
// read of pages 300 to 310 does not touch the other pages.
type pdfDocument struct {
	count int

	mu    sync.Mutex
	pages map[int]pdfPage
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

// pdfDocumentFor returns the remembered state of a PDF, starting it when the file is new or changed.
func pdfDocumentFor(path string, info os.FileInfo, data []byte) (*pdfDocument, error) {
	key := pdfCacheKey{path: path, size: info.Size(), modTime: info.ModTime()}
	pdfCache.Lock()
	if doc, ok := pdfCache.docs[key]; ok {
		pdfCache.Unlock()
		return doc, nil
	}
	pdfCache.Unlock()

	count, err := pdfsmart.PageCount(data)
	if err != nil {
		return nil, err
	}
	doc := &pdfDocument{count: count, pages: map[int]pdfPage{}}

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

func (d *pdfDocument) get(page int) (pdfPage, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.pages[page]
	return p, ok
}

func (d *pdfDocument) put(page int, p pdfPage) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pages[page] = p
}

// unread returns the pages of the batch that have not been read yet.
func (d *pdfDocument) unread(batch []int) []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	var missing []int
	for _, page := range batch {
		if _, ok := d.pages[page]; !ok {
			missing = append(missing, page)
		}
	}
	return missing
}

// pdfTargetPages turns the pages parameter into the pages to read: every page when there is none.
// How much of them one read returns is decided by the limits, not by the range.
func pdfTargetPages(pagesParam string, count int) ([]int, error) {
	if strings.TrimSpace(pagesParam) == "" {
		return allPages(count), nil
	}
	parsed, err := ParsePDFPageRange(pagesParam)
	if err != nil {
		return nil, fmt.Errorf("invalid pages parameter: %w", err)
	}
	last := parsed.LastPage
	if last == -1 || last > count {
		last = count
	}
	if parsed.FirstPage > count {
		return nil, fmt.Errorf("this PDF has %d pages, page %d does not exist", count, parsed.FirstPage)
	}
	pages := make([]int, 0, last-parsed.FirstPage+1)
	for page := parsed.FirstPage; page <= last; page++ {
		pages = append(pages, page)
	}
	return pages, nil
}

func allPages(count int) []int {
	pages := make([]int, count)
	for i := range pages {
		pages[i] = i + 1
	}
	return pages
}

// readPDFFile reads a PDF page by page. With no page range it reads from the start, as a normal read
// would; with one, those pages. Either way a read is bounded by cost, not by page count: it stops at a
// page boundary once it has returned PDFMaxCharsPerRead characters or has sent PDFMaxEnginePagesPerRead
// pages to the document reader, and says which page to continue from. A long document is therefore read
// in several calls, each with a page range, and no single call can run away.
//
// Only the pages a read returns are read, in batches, so the first read of a long document is quick.
// The result says which pages hold images or have no readable text. A page with no text layer goes to
// the configured document reader when there is one. When none of the pages read could be read at all,
// the PDF is handed over as a file so a model that can see it still can.
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
	targets, err := pdfTargetPages(pagesParam, doc.count)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	var body strings.Builder
	var shown, noText, visual []int
	continueAt := 0
	engineBudget := t.pdfMaxEnginePages()

	for next := 0; next < len(targets) && continueAt == 0; next += pdfBatchPages {
		batch := targets[next:min(next+pdfBatchPages, len(targets))]
		if missing := doc.unread(batch); len(missing) > 0 {
			result, _, err := pdfsmart.ReadPages(ctx, data, missing, pdfsmart.Options{MaxEnginePages: engineBudget}, t.documentReader, pdfsmart.VisionFallback{})
			if err != nil {
				if ctx.Err() != nil {
					return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
				}
				return tool.NewErrorResult(fmt.Errorf("failed to read PDF: %w", err)), nil
			}
			for _, page := range result.Pages {
				if page.Deferred {
					continue
				}
				doc.put(page.Page, pdfPage{text: page.Text, hasImage: page.HasEmbeddedImage})
				if page.Source != pdfsmart.PageSourceNative && t.documentReader != nil {
					engineBudget--
				}
			}
		}

		for _, number := range batch {
			page, read := doc.get(number)
			if !read {
				continueAt = number // it needs the document reader and this read has used its share
				break
			}
			text := strings.TrimSpace(page.text)
			if text == "" {
				noText = append(noText, number)
				continue
			}
			if len(shown) > 0 && body.Len()+len(text) > t.pdfMaxCharsPerRead() {
				continueAt = number
				break
			}
			fmt.Fprintf(&body, "--- page %d ---\n%s\n\n", number, text)
			shown = append(shown, number)
			if page.hasImage {
				visual = append(visual, number)
			}
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
			PageCount:    doc.count,
			VisualPages:  visual,
			ShownPages:   shown,
			NoTextPages:  noText,
			ContinueAt:   continueAt,
		},
	})), nil
}

func (t *Tool) pdfMaxEnginePages() int {
	if t.config != nil && t.config.MaxPDFEnginePagesPerRead > 0 {
		return t.config.MaxPDFEnginePagesPerRead
	}
	return PDFMaxEnginePagesPerRead
}

func (t *Tool) pdfMaxCharsPerRead() int {
	if t.config != nil && t.config.MaxPDFCharsPerRead > 0 {
		return t.config.MaxPDFCharsPerRead
	}
	return PDFMaxCharsPerRead
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
