package documentreading

import (
	"context"
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
