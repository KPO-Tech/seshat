package read

import (
	"strings"
	"testing"
)

func TestFormatPDFMarkdownResultOmitsImageBytes(t *testing.T) {
	t.Parallel()

	reader := &Tool{}
	out := reader.formatPDFMarkdownResult(&FileReadResult{
		Type: FileTypePDFMarkdown,
		PDFMarkdown: &PDFMarkdownFileResult{
			FilePath:     "cv.pdf",
			Markdown:     "# CV\n\nStructured content",
			OriginalSize: 1024,
			PageCount:    2,
			Images: []PDFImage{{
				Filename: "picture-1.png",
				MimeType: "image/png",
				Base64:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB",
			}},
		},
	})

	if !strings.Contains(out, "# CV") {
		t.Fatalf("expected markdown content in output, got:\n%s", out)
	}
	if !strings.Contains(out, "picture-1.png (image/png)") {
		t.Fatalf("expected image metadata in output, got:\n%s", out)
	}
	if strings.Contains(out, "iVBORw0KGgo") || strings.Contains(out, "base64") {
		t.Fatalf("expected image bytes to be omitted, got:\n%s", out)
	}
}

func TestFormatDocumentReaderResultOmitsImageBytes(t *testing.T) {
	t.Parallel()

	reader := &Tool{}
	out := reader.formatDocumentReaderResult(&FileReadResult{
		Type: FileTypeDocumentReader,
		DocumentReader: &DocumentReaderFileResult{
			FilePath:     "deck.pptx",
			Format:       "pptx",
			Markdown:     "# Deck\n\nSlide notes",
			OriginalSize: 2048,
			PageCount:    3,
			Images: []PDFImage{{
				Filename: "slide-image.jpg",
				MimeType: "image/jpeg",
				Base64:   "/9j/4AAQSkZJRgABAQ",
			}},
		},
	})

	if !strings.Contains(out, "# Deck") {
		t.Fatalf("expected markdown content in output, got:\n%s", out)
	}
	if !strings.Contains(out, "slide-image.jpg (image/jpeg)") {
		t.Fatalf("expected image metadata in output, got:\n%s", out)
	}
	if strings.Contains(out, "/9j/4AAQ") || strings.Contains(out, "base64") {
		t.Fatalf("expected image bytes to be omitted, got:\n%s", out)
	}
}
