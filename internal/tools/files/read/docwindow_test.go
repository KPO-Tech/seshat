package read

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

func TestWindowDocumentReturnsTheWholeOfASmallDocument(t *testing.T) {
	w := windowDocument("one\ntwo\nthree", 0, 0, 100)
	if w.Text != "one\ntwo\nthree" || w.Partial() || w.continuation() != "" {
		t.Fatalf("got %+v", w)
	}
}

func TestWindowDocumentStopsAtTheSizeLimitAtTheEndOfALine(t *testing.T) {
	w := windowDocument("aaaa\nbbbb\ncccc\ndddd", 1, 0, 12)
	if w.Text != "aaaa\nbbbb" || w.First != 1 || w.Last != 2 || w.Total != 4 {
		t.Fatalf("got %+v", w)
	}
	if want := "Showing lines 1-2 of 4. Read the file again with offset=3 to continue."; w.continuation() != want {
		t.Fatalf("got %q, want %q", w.continuation(), want)
	}
	next := windowDocument("aaaa\nbbbb\ncccc\ndddd", 3, 0, 12)
	if next.Text != "cccc\ndddd" || next.Partial() == false || next.continuation() == "" {
		// lines 3-4 are the end, but the read did not start at the top, so it is still reported as a part
		t.Fatalf("got %+v", next)
	}
}

func TestWindowDocumentHonoursOffsetLimitAndATailOffset(t *testing.T) {
	doc := "1\n2\n3\n4\n5\n6"
	if w := windowDocument(doc, 2, 2, 1000); w.Text != "2\n3" || w.Last != 3 {
		t.Fatalf("limit: %+v", w)
	}
	if w := windowDocument(doc, -2, 0, 1000); w.Text != "5\n6" || w.First != 5 {
		t.Fatalf("tail: %+v", w)
	}
	if w := windowDocument(doc, 99, 0, 1000); w.First <= w.Total {
		t.Fatalf("an offset past the end must be recognisable: %+v", w)
	}
}

func TestWindowDocumentCutsAnImpossiblyLongLine(t *testing.T) {
	w := windowDocument(strings.Repeat("é", 50), 1, 0, 20)
	if !w.Cut || len(w.Text) > 20 || !strings.Contains(w.continuation(), "was cut") {
		t.Fatalf("got %+v", w)
	}
}

// A long converted document is read in pieces, as a PDF is, and the result says where to continue.
func TestAHugeConvertedDocumentIsReadInPiecesWithAWayToContinue(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := 1; i <= 5000; i++ {
		fmt.Fprintf(&sb, "Paragraph number %d of a very long document, with enough words in it to take up some room.\n\n", i)
	}
	tool := &Tool{config: DefaultToolConfig(), documentReader: bigReader{markdown: sb.String()}}

	dir := t.TempDir()
	path := filepath.Join(dir, "long.docx")
	if err := os.WriteFile(path, []byte("not a real docx, the reader is a fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)

	first, err := tool.readDocumentReaderFile(context.Background(), path, info, nil)
	if err != nil || first.IsError() {
		t.Fatalf("err=%v result=%+v", err, first)
	}
	if len(first.Content) > DocumentMaxCharsPerRead+2000 {
		t.Fatalf("a read must stay near the limit, got %d characters", len(first.Content))
	}
	if !strings.Contains(first.Content, "Read the file again with offset=") || !strings.Contains(first.Content, "Paragraph number 1 ") {
		t.Fatalf("missing the continuation or the start:\n%.400s", first.Content)
	}

	// A negative offset reads from the end, as for a text file.
	last, err := tool.readDocumentReaderFile(context.Background(), path, info, map[string]any{"offset": float64(-3)})
	if err != nil || last.IsError() || !strings.Contains(last.Content, "Paragraph number 5000 ") {
		t.Fatalf("the tail read failed: err=%v\n%.300s", err, last.Content)
	}
}

type bigReader struct{ markdown string }

func (bigReader) IsAvailable(context.Context) bool { return true }
func (b bigReader) ConvertFile(context.Context, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: b.markdown}, nil
}
func (b bigReader) ConvertBytes(context.Context, []byte, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: b.markdown}, nil
}
func (b bigReader) ConvertURL(context.Context, string) (*documentreader.ConversionResult, error) {
	return &documentreader.ConversionResult{Markdown: b.markdown}, nil
}

// slideDeck builds a one-slide PPTX with the given title: little text, as a deck of pictures has.
func slideDeck(t *testing.T, title string) string {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml":             `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/><Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`,
		"_rels/.rels":                     `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/presentation.xml":            `<?xml version="1.0" encoding="UTF-8"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           `<?xml version="1.0" encoding="UTF-8"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + title + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
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
	path := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A deck that is mostly pictures has a title or two of text. With no document reader to do better, that text is
// returned (with a note), not a message saying nothing could be read.
func TestADeckWithLittleTextIsStillReadWithNoDocumentReader(t *testing.T) {
	t.Parallel()
	path := slideDeck(t, "Quarterly review")
	info, _ := os.Stat(path)
	tool := &Tool{config: DefaultToolConfig()}
	result, err := tool.readDocumentReaderFile(context.Background(), path, info, nil)
	if err != nil || result.IsError() {
		t.Fatalf("err=%v result=%+v", err, result)
	}
	if !strings.Contains(result.Content, "Quarterly review") || !strings.Contains(result.Content, "mostly pictures") {
		t.Fatalf("the thin text and its note are expected:\n%s", result.Content)
	}
}

func TestADeckWithLittleTextIsStillReadWhenTheDocumentReaderFails(t *testing.T) {
	t.Parallel()
	path := slideDeck(t, "Quarterly review")
	info, _ := os.Stat(path)
	tool := &Tool{config: DefaultToolConfig(), documentReader: failingReader{}}
	result, err := tool.readDocumentReaderFile(context.Background(), path, info, nil)
	if err != nil || result.IsError() || !strings.Contains(result.Content, "Quarterly review") {
		t.Fatalf("err=%v result=%+v", err, result)
	}
}

type failingReader struct{ bigReader }

func (failingReader) ConvertFile(context.Context, string) (*documentreader.ConversionResult, error) {
	return nil, fmt.Errorf("unsupported format")
}
