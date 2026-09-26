package pdfsmart

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// Fixtures are copied from internal/pdftext's own testdata (already
// validated there): text_layer.pdf is a real multi-element text-layer PDF
// with no embedded images; scanned.pdf is a real pdfcpu-built PDF whose
// single page is nothing but an embedded screenshot image - exactly the
// "has an embedded image XObject" case this package routes to documentreader.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "pdftext", "testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}

func TestConvert_TextOnlyPDFExtractsNativelyWithNilDoclingClient(t *testing.T) {
	t.Parallel()
	result, ok, err := Convert(context.Background(), readTestdata(t, "text_layer.pdf"), nil, VisionFallback{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatalf("expected a text-only PDF (no embedded images) to succeed with no docling client, got pages: %+v", result.Pages)
	}
	if !strings.Contains(result.Markdown, "Sample Report") {
		t.Fatalf("expected extracted content, got:\n%s", result.Markdown)
	}
	for _, p := range result.Pages {
		if p.Source != PageSourceNative {
			t.Errorf("expected every page to be extracted natively, page %d got source %q", p.Page, p.Source)
		}
	}
	if result.DoclingPageCount() != 0 {
		t.Errorf("expected 0 pages routed to docling, got %d", result.DoclingPageCount())
	}
}

func TestConvert_ImagePDFFailsCleanlyWithNilDoclingClient(t *testing.T) {
	t.Parallel()
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, VisionFallback{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// The page has an embedded image, so it needs docling - with no
	// client available, ok must be false (the safety contract: never
	// return a result missing a page docling would have covered).
	if ok {
		t.Fatal("expected ok=false when a page needing docling has no docling client to use")
	}
}

func TestConvert_ImagePDFUsesDoclingWhenAvailable(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/convert/file":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"# This came from docling for this page","pages":[1]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := documentreader.NewDoclingClient(server.URL)
	result, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), client, VisionFallback{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatalf("expected success once docling is available for the image page, got pages: %+v", result.Pages)
	}
	if !strings.Contains(result.Markdown, "This came from docling for this page") {
		t.Fatalf("expected docling's markdown in the result, got:\n%s", result.Markdown)
	}
	if result.DoclingPageCount() == 0 {
		t.Error("expected at least one page to be routed to docling for an image-only PDF")
	}
}

func TestConvert_GarbledDoclingOutputForAnImagePageFailsCleanly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/convert/file":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"(cid:12)(cid:47)(cid:8)(cid:91)","pages":[1]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := documentreader.NewDoclingClient(server.URL)
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), client, VisionFallback{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if ok {
		t.Fatal("expected garbled docling output on the only page to be rejected, not accepted as success")
	}
}

// fakeRenderer returns a fixed, non-empty byte slice for any page - Convert
// never inspects the bytes themselves, only passes them to the transcriber.
type fakeRenderer struct{ err error }

func (r fakeRenderer) RenderPage(_ context.Context, _ []byte, pageNum int) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	return []byte(fmt.Sprintf("fake-png-page-%d", pageNum)), nil
}

// fakeTranscriber returns a fixed transcription, or simulates being
// unavailable/erroring, depending on its fields.
type fakeTranscriber struct {
	available    bool
	transcribed  string
	err          error
	transcribeFn func(pngImage []byte) (string, error)
}

func (t fakeTranscriber) IsAvailable(context.Context) bool { return t.available }

func (t fakeTranscriber) TranscribePage(_ context.Context, pngImage []byte) (string, error) {
	if t.transcribeFn != nil {
		return t.transcribeFn(pngImage)
	}
	if t.err != nil {
		return "", t.err
	}
	return t.transcribed, nil
}

func TestConvert_VisionFallbackUsedWhenDoclingUnavailable(t *testing.T) {
	t.Parallel()
	vision := VisionFallback{
		Renderer:    fakeRenderer{},
		Transcriber: fakeTranscriber{available: true, transcribed: "# Transcribed by vision"},
	}
	result, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, vision)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatalf("expected success via the vision fallback, got pages: %+v", result.Pages)
	}
	if !strings.Contains(result.Markdown, "Transcribed by vision") {
		t.Fatalf("expected vision's transcription in the result, got:\n%s", result.Markdown)
	}
	if result.VisionPageCount() == 0 {
		t.Error("expected at least one page to be routed to vision for an image-only PDF with no docling client")
	}
	for _, p := range result.Pages {
		if p.Source != PageSourceVision {
			t.Errorf("expected page %d to be sourced from vision, got %q", p.Page, p.Source)
		}
	}
}

func TestConvert_VisionFallbackTriedAfterDoclingFails(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/convert/file":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := documentreader.NewDoclingClient(server.URL)
	vision := VisionFallback{
		Renderer:    fakeRenderer{},
		Transcriber: fakeTranscriber{available: true, transcribed: "recovered via vision"},
	}
	result, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), client, vision)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !ok {
		t.Fatalf("expected the vision fallback to recover the page after docling failed, got pages: %+v", result.Pages)
	}
	if !strings.Contains(result.Markdown, "recovered via vision") {
		t.Fatalf("expected vision's transcription in the result, got:\n%s", result.Markdown)
	}
}

func TestConvert_VisionFallbackSkippedWhenTranscriberUnavailable(t *testing.T) {
	t.Parallel()
	vision := VisionFallback{
		Renderer:    fakeRenderer{},
		Transcriber: fakeTranscriber{available: false, transcribed: "should never be used"},
	}
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, vision)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when the vision transcriber reports itself unavailable (e.g. a text-only model)")
	}
}

func TestConvert_GarbledVisionOutputFailsCleanly(t *testing.T) {
	t.Parallel()
	vision := VisionFallback{
		Renderer:    fakeRenderer{},
		Transcriber: fakeTranscriber{available: true, transcribed: "(cid:12)(cid:47)(cid:8)(cid:91)"},
	}
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, vision)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if ok {
		t.Fatal("expected garbled vision output on the only page to be rejected, not accepted as success")
	}
}

func TestConvert_VisionRendererErrorFailsCleanly(t *testing.T) {
	t.Parallel()
	vision := VisionFallback{
		Renderer:    fakeRenderer{err: fmt.Errorf("simulated render failure")},
		Transcriber: fakeTranscriber{available: true, transcribed: "should never be reached"},
	}
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, vision)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when the page renderer errors")
	}
}
