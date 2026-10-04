package documentreading

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/documentreader"
	"github.com/KPO-Tech/seshat/pkg/officetext"
)

type fakeConverter struct {
	markdown string
}

func (f fakeConverter) IsAvailable(context.Context) bool { return true }

func (f fakeConverter) ConvertFile(context.Context, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: f.markdown}, nil
}

func (f fakeConverter) ConvertBytes(context.Context, []byte, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: f.markdown}, nil
}

func (f fakeConverter) ConvertURL(context.Context, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: f.markdown}, nil
}

// Fixtures here are copied from the seshat SDK's own officetext/pdftext
// testdata (real office/PDF parser fixtures, already validated there) - kept
// self-contained rather than referenced across repos so these tests still
// work in CI, which builds against the published SDK module, not a sibling
// checkout.

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}

func copyToDir(t *testing.T, name, dir string) string {
	t.Helper()
	dest := filepath.Join(dir, name)
	if err := os.WriteFile(dest, readTestdata(t, name), 0o600); err != nil {
		t.Fatalf("write %s: %v", dest, err)
	}
	return dest
}

func TestConvert_DOCXNativeWithNilExternalConverter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := copyToDir(t, "sample.docx", dir)

	result, ok, err := Convert(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a native-supported DOCX with no external converter")
	}
	if result.Source != SourceNative {
		t.Fatalf("expected SourceNative, got %q", result.Source)
	}
	if !strings.Contains(result.Markdown, "Sample Report") {
		t.Fatalf("expected extracted DOCX content, got:\n%s", result.Markdown)
	}
}

func TestConvert_XLSXNativeWithNilExternalConverter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := copyToDir(t, "sample.xlsx", dir)

	result, ok, err := Convert(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok || result.Source != SourceNative {
		t.Fatalf("expected native XLSX extraction, got ok=%v source=%q", ok, result.Source)
	}
	if !strings.Contains(result.Markdown, "Carol") {
		t.Fatalf("expected extracted XLSX content, got:\n%s", result.Markdown)
	}
}

func TestConvert_PDFTextLayerNativeWithNilExternalConverter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := copyToDir(t, "text_layer.pdf", dir)

	result, ok, err := Convert(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok || result.Source != SourceNative {
		t.Fatalf("expected native PDF extraction, got ok=%v source=%q", ok, result.Source)
	}
	if !strings.Contains(result.Markdown, "Sample Report") {
		t.Fatalf("expected extracted PDF content, got:\n%s", result.Markdown)
	}
}

func TestConvert_ScannedPDFWithNilExternalConverterYieldsNoResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := copyToDir(t, "scanned.pdf", dir)

	result, ok, err := Convert(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// No text layer and no external converter: nothing to extract, but this
	// is not an error condition. Callers decide whether to fail ingestion or
	// keep the attachment without a markdown sidecar.
	if ok {
		t.Fatalf("expected ok=false for a scanned PDF with no external converter, got markdown:\n%s", result.Markdown)
	}
}

func TestConvert_ScannedPDFUsesPDFSmartExternalFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := copyToDir(t, "scanned.pdf", dir)

	result, ok, err := Convert(context.Background(), path, fakeConverter{markdown: "OCR page text"})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for scanned PDF with external reader")
	}
	if result.Source != SourceExternal {
		t.Fatalf("expected external source for fully OCR-routed scanned PDF, got %q", result.Source)
	}
	if !strings.Contains(result.Markdown, "OCR page text") {
		t.Fatalf("expected external page text, got:\n%s", result.Markdown)
	}
}

func TestConvert_UnsupportedExtensionWithNilExternalConverterYieldsNoResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.wav")
	if err := os.WriteFile(path, []byte("not really audio"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, ok, err := Convert(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for an external-only format with no external converter")
	}
}

func TestConvertBytes_GarbledExternalOutputIsRejected(t *testing.T) {
	t.Parallel()

	client := fakeConverter{markdown: "Report (cid:12)(cid:47) Summary (cid:8)(cid:91)"}
	result, ok, err := ConvertBytes(context.Background(), []byte("not really audio, only the extension matters"), "voicenote.wav", client)
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if ok {
		t.Fatalf("expected garbled external output to be rejected, got markdown:\n%s", result.Markdown)
	}
}

func TestConvertBytes_DOCXNative(t *testing.T) {
	t.Parallel()
	result, ok, err := ConvertBytes(context.Background(), readTestdata(t, "sample.docx"), "sample.docx", nil)
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if !ok || result.Source != SourceNative {
		t.Fatalf("expected native DOCX extraction, got ok=%v source=%q", ok, result.Source)
	}
	if !strings.Contains(result.Markdown, "Sample Report") {
		t.Fatalf("expected extracted content, got:\n%s", result.Markdown)
	}
}

func TestExtensionSetsAreDisjointAndConsistent(t *testing.T) {
	t.Parallel()
	for ext := range NativeExtensions {
		if ExternalOnlyExtensions[ext] {
			t.Errorf("%q is in both NativeExtensions and ExternalOnlyExtensions", ext)
		}
		if !AllConvertibleExtensions[ext] {
			t.Errorf("%q from NativeExtensions is missing from AllConvertibleExtensions", ext)
		}
	}
	for ext := range ExternalOnlyExtensions {
		if !AllConvertibleExtensions[ext] {
			t.Errorf("%q from ExternalOnlyExtensions is missing from AllConvertibleExtensions", ext)
		}
	}
	if !AllConvertibleExtensions[".pdf"] {
		t.Error(`".pdf" must be in AllConvertibleExtensions`)
	}
}

func TestHTMLIsConvertedToMarkdownLocally(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")
	page := `<html><head><title>Q1</title><script>track()</script></head><body><h1>Q1 report</h1><p>Revenue grew.</p>` +
		`<table><tr><th>Region</th><th>Sales</th></tr><tr><td>North</td><td>10</td></tr></table></body></html>`
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}

	result, ok, err := Convert(context.Background(), path, nil)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if result.Source != SourceNative {
		t.Errorf("source = %q, want native", result.Source)
	}
	for _, want := range []string{"# Q1 report", "Revenue grew.", "| Region | Sales |", "| North | 10 |"} {
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("missing %q in:\n%s", want, result.Markdown)
		}
	}
	if strings.Contains(result.Markdown, "track()") {
		t.Errorf("a script leaked into the text:\n%s", result.Markdown)
	}

	again, ok, err := ConvertBytes(context.Background(), []byte(page), "report.HTM", nil)
	if err != nil || !ok || again.Markdown != result.Markdown {
		t.Errorf("ConvertBytes differs: ok=%v err=%v", ok, err)
	}
}

func TestHTMLIsConvertibleButTheReadToolKeepsItsSource(t *testing.T) {
	for _, ext := range []string{".html", ".htm", ".xhtml"} {
		if !NativeExtensions[ext] || !AllConvertibleExtensions[ext] {
			t.Errorf("%s should be convertible", ext)
		}
		if officetext.SupportedExtensions[ext] {
			t.Errorf("%s must not be an officetext format: the Read tool would stop showing its source", ext)
		}
	}
}

func TestAnEmptyHTMLPageIsNotAResult(t *testing.T) {
	if _, ok, err := ConvertBytes(context.Background(), []byte("<html><body><script>x()</script></body></html>"), "blank.html", nil); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

// slideDeck builds a one-slide PPTX whose slide holds the given title, or no text at all when it is empty.
func slideDeck(t *testing.T, title string) []byte {
	t.Helper()
	body := ""
	if title != "" {
		body = `<p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + title + `</a:t></a:r></a:p></p:txBody></p:sp>`
	}
	files := map[string]string{
		"[Content_Types].xml":             `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/><Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`,
		"_rels/.rels":                     `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/presentation.xml":            `<?xml version="1.0" encoding="UTF-8"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           `<?xml version="1.0" encoding="UTF-8"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>` + body + `</p:spTree></p:cSld></p:sld>`,
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type failingConverter struct{ fakeConverter }

func (failingConverter) ConvertBytes(context.Context, []byte, string) (*documentreader.ConversionResult, error) {
	return nil, errors.New("unsupported format")
}

func TestADeckWithLittleTextIsReadWhenNothingCanDoBetter(t *testing.T) {
	deck := slideDeck(t, "Quarterly review")
	for name, converter := range map[string]documentreader.Converter{
		"no reader":       nil,
		"reader fails":    failingConverter{},
		"reader is empty": fakeConverter{markdown: ""},
	} {
		result, ok, err := ConvertBytes(context.Background(), deck, "deck.pptx", converter)
		if err != nil || !ok || !strings.Contains(result.Markdown, "Quarterly review") || result.Source != SourceNative {
			t.Errorf("%s: ok=%v err=%v source=%q markdown=%q", name, ok, err, result.Source, result.Markdown)
		}
	}
}

func TestAReaderThatDoesBetterThanThinNativeTextWins(t *testing.T) {
	result, ok, err := ConvertBytes(context.Background(), slideDeck(t, "Quarterly review"), "deck.pptx", fakeConverter{markdown: "# Quarterly review\n\nRevenue grew 12% (read from the slide pictures)."})
	if err != nil || !ok || result.Source != SourceExternal || !strings.Contains(result.Markdown, "Revenue grew") {
		t.Fatalf("ok=%v err=%v %+v", ok, err, result)
	}
}

func TestADeckWithNoTextIsNothingNotAnError(t *testing.T) {
	_, ok, err := ConvertBytes(context.Background(), slideDeck(t, ""), "deck.pptx", failingConverter{})
	if err != nil || ok {
		t.Fatalf("a deck of pictures that nothing can read is no result, not an error: ok=%v err=%v", ok, err)
	}
}

func TestACorruptOfficeFileStillReportsTheReadersError(t *testing.T) {
	_, ok, err := ConvertBytes(context.Background(), []byte("not a zip"), "deck.pptx", failingConverter{})
	if err == nil || ok {
		t.Fatalf("a corrupt file whose reader fails is an error: ok=%v err=%v", ok, err)
	}
}
