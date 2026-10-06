package pdftext

import (
	"bytes"
	"fmt"
	"strings"
)

// helveticaFont is the font dictionary of the tests: Helvetica with a width table for the characters from space to tilde, as the
// fonts of most files have one. extra is added to the dictionary (an /Encoding, for instance).
func helveticaFont(extra string) string {
	widths := make([]string, len(helvetica))
	for i, w := range helvetica {
		widths[i] = fmt.Sprint(w)
	}
	return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 32 /LastChar %d /Widths [%s] %s >>", 31+len(helvetica), strings.Join(widths, " "), extra)
}

// buildPDF is a one page PDF with a content stream and a font F1, shown rotated by `rotate` degrees.
func buildPDF(rotate int, content, font string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate %d /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>", rotate),
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
		font,
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}
