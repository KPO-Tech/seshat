package pdfsmart

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// withPicture stamps a picture of the given size on the pages of a PDF, which gives a text page that holds a raster image.
func withPicture(t *testing.T, pdf []byte, pages []string, side int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))
	for x := 0; x < side; x++ {
		for y := 0; y < side; y++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	file := filepath.Join(t.TempDir(), "picture.png")
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	watermark, err := pdfcpu.ParseImageWatermarkDetails(file, "scale:0.3 abs, pos:c", true, types.POINTS)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(pdf), &out, pages, watermark, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestAPageWithTextAndAPictureKeepsItsTextAndSaysWhereThePictureIs(t *testing.T) {
	t.Parallel()
	data := withPicture(t, readTestdata(t, "text_layer.pdf"), []string{"1"}, 300)

	// Convert sends a page with a picture to the engine; with no engine the text is kept.
	result, ok, err := Convert(context.Background(), data, nil, VisionFallback{})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v: a page with text and a picture is readable without an engine", ok, err)
	}
	if !strings.Contains(result.Markdown, "Sample Report") {
		t.Errorf("the text of the page is missing:\n%s", result.Markdown)
	}
	if !strings.Contains(result.Markdown, "[Image 1 on page 1]") {
		t.Errorf("the picture is not marked:\n%s", result.Markdown)
	}
	page := result.Pages[0]
	if page.Source != PageSourceNative || !page.HasEmbeddedImage || len(page.Pictures) != 1 || page.Pictures[0].Number != 1 {
		t.Errorf("page = %+v", page)
	}
}

func TestAPageThatTheEngineReadsHasNoMarker(t *testing.T) {
	t.Parallel()
	data := withPicture(t, readTestdata(t, "text_layer.pdf"), []string{"1"}, 300)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/convert/file" {
			_, _ = w.Write([]byte(`{"status":"success","document":{"md_content":"Text the engine read, with the text of the picture","pages":[1]}}`))
			return
		}
		w.WriteHeader(http.StatusOK) // /health
	}))
	defer server.Close()
	result, ok, err := Convert(context.Background(), data, documentreader.NewDoclingClient(server.URL), VisionFallback{})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Contains(result.Markdown, "[Image") || len(result.Pages[0].Pictures) != 0 {
		t.Errorf("the engine read the picture, nothing is left to mark:\n%s", result.Markdown)
	}
}

func TestAPageWithNoTextIsStillAFailureWithoutAnEngine(t *testing.T) {
	t.Parallel()
	_, ok, err := Convert(context.Background(), readTestdata(t, "scanned.pdf"), nil, VisionFallback{})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v: a scan has nothing but its picture, and a marker is not its text", ok, err)
	}
}

func TestAPictureOnEveryPageIsALogoNotAFigure(t *testing.T) {
	t.Parallel()
	data := withPicture(t, readTestdata(t, "three_pages.pdf"), nil, 300)
	pages, _, pictures, err := pagesWithEmbeddedImages(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("the picture should be on 3 pages, got %v", pages)
	}
	if len(pictures) != 0 {
		t.Errorf("a picture repeated on every page is not marked, got %v", pictures)
	}
}

func TestAnIconIsNotMarked(t *testing.T) {
	t.Parallel()
	data := withPicture(t, readTestdata(t, "text_layer.pdf"), []string{"1"}, 40)
	result, ok, err := Convert(context.Background(), data, nil, VisionFallback{})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Contains(result.Markdown, "[Image") {
		t.Errorf("an icon is not a figure:\n%s", result.Markdown)
	}
}

func TestCaptionsAreAttachedOnlyWhenTheyCanBePaired(t *testing.T) {
	t.Parallel()
	text := "Some prose.\n\n**Figure 2.** Residual learning: a building block.\n\nMore prose.\nTable 1 is not a figure.\nFig. 3: The second one"
	cases := []struct {
		name  string
		count int
		want  []string
	}{
		{"as many pictures as captions", 2, []string{"Figure 2. Residual learning: a building block.", "Fig. 3: The second one"}},
		{"fewer pictures than captions: a guess, so no title", 1, []string{""}},
		{"more pictures than captions: no title", 3, []string{"", "", ""}},
	}
	for _, tc := range cases {
		marked, pictures := withImageMarkers(text, 4, tc.count)
		if len(pictures) != tc.count {
			t.Fatalf("%s: %d pictures", tc.name, len(pictures))
		}
		for i, picture := range pictures {
			if picture.Number != i+1 || picture.Title != tc.want[i] {
				t.Errorf("%s: picture %d = %+v, want title %q", tc.name, i, picture, tc.want[i])
			}
			if !strings.Contains(marked, ImageMarker(4, picture)) {
				t.Errorf("%s: %q is not in the text", tc.name, ImageMarker(4, picture))
			}
		}
	}
	if marked, pictures := withImageMarkers(text, 4, 0); marked != text || pictures != nil {
		t.Error("a page without pictures is left as it is")
	}
}

func TestAMarkerHasNoClosingBracketInItsTitle(t *testing.T) {
	t.Parallel()
	captions := captionsIn("Figure 1. Results [see appendix]")
	if len(captions) != 1 || strings.Count(ImageMarker(1, Picture{Number: 1, Title: captions[0]}), "]") != 1 {
		t.Errorf("captions = %q", captions)
	}
}
