//go:build cgo && nativedoc

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nativedoc "github.com/KPO-Tech/seshat/internal/nativedoc"
)

// testdata/text_layer.pdf, sample.docx, and sample.xlsx are the same real
// fixtures internal/pdftext and internal/officetext already use (headings +
// paragraphs + a table: "Sample Report" / "Section One" / an Alice/Bob
// table). testdata/scanned.pdf is the same document with no text layer - a
// real pdfcpu-built, image-only PDF - so the OCR fallback path is tested
// against real scanned-style content, not a synthetic empty one.

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}

func TestConvertDOCX(t *testing.T) {
	c := New("")
	result, err := c.ConvertBytes(context.Background(), readTestdata(t, "sample.docx"), "sample.docx")
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	for _, want := range []string{"# Sample Report", "## Section One", "Alice"} {
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("expected %q in markdown, got:\n%s", want, result.Markdown)
		}
	}
}

func TestConvertXLSX(t *testing.T) {
	c := New("")
	result, err := c.ConvertBytes(context.Background(), readTestdata(t, "sample.xlsx"), "sample.xlsx")
	if err != nil {
		t.Fatalf("ConvertBytes: %v", err)
	}
	if !strings.Contains(result.Markdown, "Alice") {
		t.Errorf("expected sheet content in markdown, got:\n%s", result.Markdown)
	}
}

func TestConvertBytes_UnsupportedFormat(t *testing.T) {
	c := New("")
	if _, err := c.ConvertBytes(context.Background(), []byte("hello"), "notes.txt"); err == nil {
		t.Fatal("expected an error for an unsupported extension, got nil")
	}
}

// A native-text-layer PDF should convert with no models and no ONNX Runtime
// initialization at all - proves the fast path never touches OCR.
func TestConvertPDF_NativeTextLayer(t *testing.T) {
	c := New("")
	result, err := c.ConvertFile(context.Background(), filepath.Join("testdata", "text_layer.pdf"))
	if err != nil {
		t.Fatalf("ConvertFile: %v", err)
	}
	if result.PageCount < 1 {
		t.Fatalf("expected at least 1 page, got %d", result.PageCount)
	}
	for _, want := range []string{"Sample Report", "Section One", "Alice"} {
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("expected %q in markdown, got:\n%s", want, result.Markdown)
		}
	}
}

// Needs the real ONNX models (det.ort/rec.ort/ocr.res) - not vendored in
// this repo (see docs/issues/document-intelligence-roadmap.md Phase 0 for
// why: 70MB+ each). Run `./scripts/install-deepdoc-models.sh` and point
// SESHAT_NATIVEDOC_TEST_MODELS_DIR at the result to exercise this test;
// skipped otherwise rather than failing CI that hasn't provisioned models.
func TestConvertPDF_ScannedRequiresOCR(t *testing.T) {
	modelDir := os.Getenv("SESHAT_NATIVEDOC_TEST_MODELS_DIR")
	if modelDir == "" {
		t.Skip("SESHAT_NATIVEDOC_TEST_MODELS_DIR not set - see scripts/install-deepdoc-models.sh")
	}
	if err := nativedoc.InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}
	c := New(modelDir)
	result, err := c.ConvertFile(context.Background(), filepath.Join("testdata", "scanned.pdf"))
	if err != nil {
		t.Fatalf("ConvertFile: %v", err)
	}
	if result.PageCount < 1 {
		t.Fatalf("expected at least 1 page, got %d", result.PageCount)
	}
	if strings.TrimSpace(result.Markdown) == "" {
		t.Fatal("expected OCR to produce some text from the scanned PDF, got empty markdown")
	}
	// OCR isn't pixel-perfect - check for a recognizable word rather than
	// an exact match against the source document's real text.
	lower := strings.ToLower(result.Markdown)
	if !strings.Contains(lower, "report") {
		t.Errorf("expected recognizable text from the scanned page, got:\n%s", result.Markdown)
	}
}

func TestSortReadingOrder(t *testing.T) {
	boxes := []nativedoc.DetBox{
		{Pts: [4][2]float32{{100, 50}, {200, 50}, {200, 70}, {100, 70}}}, // bottom-right-ish
		{Pts: [4][2]float32{{10, 10}, {50, 10}, {50, 30}, {10, 30}}},     // top-left
		{Pts: [4][2]float32{{60, 10}, {90, 10}, {90, 30}, {60, 30}}},     // top, right of the previous one
	}
	sortReadingOrder(boxes)
	want := [][2]float32{{10, 10}, {60, 10}, {100, 50}}
	for i, w := range want {
		if boxes[i].Pts[0] != w {
			t.Errorf("box %d: got top-left %v, want %v", i, boxes[i].Pts[0], w)
		}
	}
}

func TestCropBox(t *testing.T) {
	img := &nativedoc.Image{W: 4, H: 2, Pix: []byte{
		1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 4, // row 0
		5, 5, 5, 6, 6, 6, 7, 7, 7, 8, 8, 8, // row 1
	}}
	box := nativedoc.DetBox{Pts: [4][2]float32{{1, 0}, {3, 0}, {3, 2}, {1, 2}}}
	crop := cropBox(img, box)
	if crop.W != 2 || crop.H != 2 {
		t.Fatalf("expected a 2x2 crop, got %dx%d", crop.W, crop.H)
	}
	want := []byte{2, 2, 2, 3, 3, 3, 6, 6, 6, 7, 7, 7}
	if string(crop.Pix) != string(want) {
		t.Errorf("crop pixels = %v, want %v", crop.Pix, want)
	}
}
