package read

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// pdfcpu fills a package-level default configuration lazily and without a lock, so the first calls
// must not run concurrently. Doing one here, before any parallel test starts, is what the production
// code does with its own sync.Once.
func init() { _ = model.NewDefaultConfiguration() }

func fixturePDF(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "pdftext", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// writePDF stores data as a new file and returns its path and info, so each test has its own cache key.
func writePDF(t *testing.T, data []byte) (string, os.FileInfo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, info
}

func mergePDFs(t *testing.T, parts ...[]byte) []byte {
	t.Helper()
	inputs := make([]io.ReadSeeker, len(parts))
	for i, part := range parts {
		inputs[i] = bytes.NewReader(part)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(inputs, &out, false, nil); err != nil {
		t.Fatalf("merge PDFs: %v", err)
	}
	return out.Bytes()
}

func read(t *testing.T, tl *Tool, path string, info os.FileInfo, pages string) string {
	t.Helper()
	result, err := tl.readPDFFile(context.Background(), path, info, pages)
	if err != nil {
		t.Fatalf("readPDFFile: %v", err)
	}
	return result.Content
}

func TestReadPDF_AShortDocumentComesBackWholeWithPageMarkers(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "")

	for _, want := range []string{"Showing pages: 1-3 of 3", "--- page 1 ---", "--- page 3 ---", "Alpha section", "Bravo section", "Charlie section"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestReadPDF_APageRangeReturnsOnlyThosePages(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "2")

	if !strings.Contains(out, "Bravo section") || strings.Contains(out, "Alpha section") || strings.Contains(out, "Charlie section") {
		t.Fatalf("pages=2 must return only page 2:\n%s", out)
	}
	if !strings.Contains(out, "Showing pages: 2 of 3") {
		t.Errorf("missing the range header:\n%s", out)
	}
}

func TestReadPDF_AnOpenEndedRangeStopsAtTheLastPage(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "2-")
	if !strings.Contains(out, "Bravo section") || !strings.Contains(out, "Charlie section") || strings.Contains(out, "Alpha section") {
		t.Fatalf("pages=2- must return pages 2 and 3:\n%s", out)
	}
}

func TestReadPDF_APageBeyondTheEndIsAnErrorThatSaysHowManyPagesThereAre(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "9")
	if !strings.Contains(out, "3 pages") {
		t.Fatalf("want an error naming the page count, got:\n%s", out)
	}
}

func TestReadPDF_ALongDocumentIsReadWholeByDefaultWhenItFitsTheLimit(t *testing.T) {
	t.Parallel()
	part := fixturePDF(t, "three_pages.pdf")
	path, info := writePDF(t, mergePDFs(t, part, part, part, part, part, part, part, part)) // 24 pages
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "")
	if !strings.Contains(out, "Showing pages: 1-24 of 24") || strings.Contains(out, "Continue with") {
		t.Fatalf("a document under the limit is read whole, however many pages:\n%s", out)
	}
}

func TestReadPDF_AReadThatReachesTheLimitCanBeContinuedWithPageRanges(t *testing.T) {
	t.Parallel()
	part := fixturePDF(t, "three_pages.pdf")
	path, info := writePDF(t, mergePDFs(t, part, part, part, part, part, part, part, part))
	tl := &Tool{config: &ToolConfig{MaxFileSize: MaxFileSize, MaxPDFCharsPerRead: 1500}}

	first := read(t, tl, path, info, "")
	hint := "Continue with pages=\""
	at := strings.Index(first, hint)
	if !strings.Contains(first, "Showing pages: 1-") || at < 0 {
		t.Fatalf("a read with no range stops at the limit and says where to continue:\n%s", first)
	}
	var next int
	if _, err := fmt.Sscanf(first[at+len(hint):], "%d-", &next); err != nil || next < 2 {
		t.Fatalf("cannot find the continuation page in:\n%s", first)
	}

	second := read(t, tl, path, info, fmt.Sprintf("%d-", next))
	if !strings.Contains(second, fmt.Sprintf("--- page %d ---", next)) || strings.Contains(second, fmt.Sprintf("--- page %d ---", next-1)) {
		t.Fatalf("the continuation must start at page %d:\n%s", next, second)
	}
}

func TestReadPDF_APageRangeLongerThanFiftyPagesIsNoLongerAnError(t *testing.T) {
	t.Parallel()
	part := fixturePDF(t, "three_pages.pdf")
	path, info := writePDF(t, mergePDFs(t, part, part, part, part, part, part, part, part, part, part, part, part, part, part, part, part, part, part)) // 54 pages
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "1-54")
	if !strings.Contains(out, "Showing pages: 1-54 of 54") {
		t.Fatalf("limits, not page counts, bound a read:\n%s", out)
	}
}

func TestReadPDF_APageWithoutATextLayerCountsAgainstTheEngineBudget(t *testing.T) {
	t.Parallel()
	scan := fixturePDF(t, "scanned.pdf")
	path, info := writePDF(t, mergePDFs(t, scan, scan, scan, scan))
	engine := &fakeEngine{markdown: "Text the engine read from a scan."}
	tl := &Tool{config: &ToolConfig{MaxFileSize: MaxFileSize, MaxPDFEnginePagesPerRead: 2}, documentReader: engine}

	out := read(t, tl, path, info, "")
	if engine.calls != 2 {
		t.Fatalf("a read may send only 2 pages to the reader, got %d calls", engine.calls)
	}
	if !strings.Contains(out, "Showing pages: 1-2 of 4") || !strings.Contains(out, "Continue with pages=\"3-\"") {
		t.Fatalf("want pages 1-2 and a hint to continue at 3:\n%s", out)
	}
}

func TestReadPDF_ARangeOverTheSizeLimitStopsAtAPageAndSaysWhereToContinue(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	tl := &Tool{config: &ToolConfig{MaxFileSize: MaxFileSize, MaxPDFCharsPerRead: 200}}

	out := read(t, tl, path, info, "1-3")
	if !strings.Contains(out, "Alpha section") || strings.Contains(out, "Charlie section") {
		t.Fatalf("the read should stop before the last page:\n%s", out)
	}
	if !strings.Contains(out, "Continue with pages=\"2-\"") && !strings.Contains(out, "Continue with pages=\"3-\"") {
		t.Fatalf("want a continuation hint, got:\n%s", out)
	}
}

type fakeEngine struct {
	markdown string
	calls    int
}

func (e *fakeEngine) IsAvailable(context.Context) bool { return true }
func (e *fakeEngine) ConvertFile(context.Context, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: e.markdown}, nil
}
func (e *fakeEngine) ConvertBytes(context.Context, []byte, string) (*documentreader.ConversionResult, error) {
	e.calls++
	return &documentreader.ConversionResult{Markdown: e.markdown}, nil
}
func (e *fakeEngine) ConvertURL(context.Context, string) (*documentreader.ConversionResult, error) {
	return nil, fmt.Errorf("not supported")
}

func TestReadPDF_AScannedPageIsReadByTheEngineOnceAndRemembered(t *testing.T) {
	t.Parallel()
	mixed := mergePDFs(t, fixturePDF(t, "text_layer.pdf"), fixturePDF(t, "scanned.pdf"))
	path, info := writePDF(t, mixed)
	engine := &fakeEngine{markdown: "Text the engine read from the scan."}
	tl := &Tool{config: DefaultToolConfig(), documentReader: engine}

	out := read(t, tl, path, info, "")
	if !strings.Contains(out, "Sample Report") || !strings.Contains(out, "Text the engine read from the scan.") {
		t.Fatalf("want both pages read:\n%s", out)
	}
	if engine.calls != 1 {
		t.Fatalf("only the scanned page should reach the engine, got %d calls", engine.calls)
	}

	read(t, tl, path, info, "2")
	if engine.calls != 1 {
		t.Fatalf("the engine's text for page 2 should be remembered, got %d calls", engine.calls)
	}
}

func TestReadPDF_APageWithoutTextAndNoEngineIsReportedNotDropped(t *testing.T) {
	t.Parallel()
	mixed := mergePDFs(t, fixturePDF(t, "text_layer.pdf"), fixturePDF(t, "scanned.pdf"))
	path, info := writePDF(t, mixed)
	out := read(t, &Tool{config: DefaultToolConfig()}, path, info, "")

	if !strings.Contains(out, "Sample Report") {
		t.Fatalf("page 1 should be read:\n%s", out)
	}
	if !strings.Contains(out, "Pages with no readable text (likely scanned): 2") {
		t.Fatalf("page 2 should be reported as unreadable:\n%s", out)
	}
}

func TestReadPDF_ChangingTheFileInvalidatesWhatWasRemembered(t *testing.T) {
	t.Parallel()
	path, info := writePDF(t, fixturePDF(t, "three_pages.pdf"))
	tl := &Tool{config: DefaultToolConfig()}
	if out := read(t, tl, path, info, ""); !strings.Contains(out, "Pages: 3") {
		t.Fatalf("first read:\n%s", out)
	}

	if err := os.WriteFile(path, fixturePDF(t, "text_layer.pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if out := read(t, tl, path, info, ""); !strings.Contains(out, "Pages: 1") || !strings.Contains(out, "Sample Report") {
		t.Fatalf("a changed file must be read again:\n%s", out)
	}
}
