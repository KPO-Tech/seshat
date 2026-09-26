package read

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests exercise readDocumentReaderFile/readPDFFile through the actual
// production entrypoints (not just internal/officetext or internal/pdftext
// in isolation) with documentReader == nil, proving the native extraction
// path is really wired into the tool and not just unit-tested in a vacuum.
// Fixtures are copied from the sibling internal/officetext and
// internal/pdftext packages' testdata (real LibreOffice-produced files),
// not hand-rolled here, so this stays in sync with what those packages
// actually validate against.

func copyFixture(t *testing.T, srcRel, destDir, destName string) (string, os.FileInfo) {
	t.Helper()
	data, err := os.ReadFile(srcRel)
	if err != nil {
		t.Fatalf("read fixture %s: %v", srcRel, err)
	}
	destPath := filepath.Join(destDir, destName)
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		t.Fatalf("write fixture copy %s: %v", destPath, err)
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat fixture copy: %v", err)
	}
	return destPath, info
}

func TestReadDocumentReaderFile_NativeDOCXWithoutDocumentReader(t *testing.T) {
	t.Parallel()
	tool := &Tool{config: DefaultToolConfig()} // documentReader intentionally nil
	dir := t.TempDir()
	path, info := copyFixture(t, "../../../officetext/testdata/sample.docx", dir, "report.docx")

	result, err := tool.readDocumentReaderFile(context.Background(), path, info)
	if err != nil {
		t.Fatalf("readDocumentReaderFile: %v", err)
	}
	if strings.Contains(result.Content, "requires the configured document reader") {
		t.Fatalf("expected native extraction to succeed without DocumentReader, got the DocumentReader-required message:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "Sample Report") || !strings.Contains(result.Content, "Alice") {
		t.Fatalf("expected extracted DOCX content, got:\n%s", result.Content)
	}
}

func TestReadDocumentReaderFile_NativeXLSXWithoutDocumentReader(t *testing.T) {
	t.Parallel()
	tool := &Tool{config: DefaultToolConfig()}
	dir := t.TempDir()
	path, info := copyFixture(t, "../../../officetext/testdata/sample.xlsx", dir, "data.xlsx")

	result, err := tool.readDocumentReaderFile(context.Background(), path, info)
	if err != nil {
		t.Fatalf("readDocumentReaderFile: %v", err)
	}
	if strings.Contains(result.Content, "requires the configured document reader") {
		t.Fatalf("expected native extraction to succeed without DocumentReader, got the DocumentReader-required message:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") || !strings.Contains(result.Content, "90") {
		t.Fatalf("expected extracted XLSX content, got:\n%s", result.Content)
	}
}

func TestReadPDFFile_NativeTextLayerWithoutDocumentReader(t *testing.T) {
	t.Parallel()
	tool := &Tool{config: DefaultToolConfig()}
	dir := t.TempDir()
	path, info := copyFixture(t, "../../../pdftext/testdata/text_layer.pdf", dir, "report.pdf")

	result, err := tool.readPDFFile(context.Background(), path, info, "")
	if err != nil {
		t.Fatalf("readPDFFile: %v", err)
	}
	if !strings.Contains(result.Content, "Sample Report") {
		t.Fatalf("expected native PDF text extraction, got:\n%s", result.Content)
	}
	// Native text extraction must win over the raw base64 pass-through path
	// for a PDF that actually has a text layer.
	if strings.Contains(result.Content, "data:application/pdf;base64") {
		t.Fatalf("expected the markdown/text result, not the base64 fallback, got:\n%s", result.Content)
	}
}

func TestReadPDFFile_ScannedFallsBackToBase64WithoutDocumentReader(t *testing.T) {
	t.Parallel()
	tool := &Tool{config: DefaultToolConfig()}
	dir := t.TempDir()
	path, info := copyFixture(t, "../../../pdftext/testdata/scanned.pdf", dir, "scan.pdf")

	result, err := tool.readPDFFile(context.Background(), path, info, "")
	if err != nil {
		t.Fatalf("readPDFFile: %v", err)
	}
	// No text layer and no DocumentReader client configured - this must fall all
	// the way through to the base64 path, not silently return empty text.
	if !strings.Contains(result.Content, "data:application/pdf;base64") {
		t.Fatalf("expected a scanned PDF with no DocumentReader client to fall back to base64, got:\n%s", result.Content)
	}
}
