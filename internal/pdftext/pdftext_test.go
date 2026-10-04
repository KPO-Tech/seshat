package pdftext

import (
	"strings"
	"testing"
)

// testdata/text_layer.pdf is a real LibreOffice export of officetext's
// sample.md fixture (headings + paragraphs + a table). testdata/scanned.pdf
// is a real pdfcpu-built, image-only PDF (a screenshot of that same document
// rendered to PNG, then wrapped with no text layer) - it exercises the
// "looks like a scan" detection path with an actual scanned-style PDF, not a
// synthetic empty one.

func TestExtract_TextLayerPDF(t *testing.T) {
	res, err := Extract(readTestdata(t, "text_layer.pdf"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.PageCount < 1 {
		t.Fatalf("expected at least 1 page, got %d", res.PageCount)
	}
	if res.Sparse {
		t.Errorf("expected a text-layer PDF to not be flagged Sparse, got Sparse=true with text:\n%s", res.Text)
	}
	for _, want := range []string{"Sample Report", "Section One", "Alice"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("expected %q in extracted text, got:\n%s", want, res.Text)
		}
	}
}

func TestExtract_ScannedPDF(t *testing.T) {
	res, err := Extract(readTestdata(t, "scanned.pdf"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.PageCount < 1 {
		t.Fatalf("expected at least 1 page, got %d", res.PageCount)
	}
	if !res.Sparse {
		t.Errorf("expected an image-only PDF to be flagged Sparse, got text:\n%s", res.Text)
	}
}

func TestExtract_InvalidPDF(t *testing.T) {
	if _, err := Extract([]byte("not a pdf")); err == nil {
		t.Fatal("expected an error for invalid PDF bytes, got nil")
	}
}
