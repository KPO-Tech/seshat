package read

import (
	"context"
	"errors"
	"testing"
)

// The PDF helpers run under the context of the read: a read that was cancelled does not parse the file. They used to hand pdfcpu a
// context.Background(), so a cancelled call still paid for the whole parse.
func TestPDFHelpersStopWithTheirContext(t *testing.T) {
	t.Parallel()
	path, _ := writePDF(t, fixturePDF(t, "text_layer.pdf"))

	if n, err := GetPDFPageCount(context.Background(), path); err != nil || n < 1 {
		t.Fatalf("GetPDFPageCount with a live context = (%d, %v)", n, err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := GetPDFPageCount(cancelled, path); err == nil {
		t.Errorf("GetPDFPageCount with a cancelled context must fail, got %d pages", n)
	} else if !errors.Is(err, context.Canceled) {
		t.Errorf("the error must say the context was cancelled: %v", err)
	}
	if _, err := ReadPDF(cancelled, path); err == nil {
		t.Error("ReadPDF with a cancelled context must fail")
	}
}

// The page count is the real one. pdfcpu.Read alone leaves it at 0, so the "too many pages to read at once" check of the fallback
// readers never fired and a very large PDF went through whole.
func TestPDFPageCountIsTheNumberOfPages(t *testing.T) {
	t.Parallel()
	one := fixturePDF(t, "text_layer.pdf")
	path, _ := writePDF(t, mergePDFs(t, one, one, one))

	if n, err := GetPDFPageCount(context.Background(), path); err != nil || n != 3 {
		t.Errorf("GetPDFPageCount = (%d, %v), want 3 pages", n, err)
	}
	result, err := ReadPDF(context.Background(), path)
	if err != nil {
		t.Fatalf("ReadPDF: %v", err)
	}
	if result.PageCount != 3 {
		t.Errorf("ReadPDF page count = %d, want 3", result.PageCount)
	}
}
