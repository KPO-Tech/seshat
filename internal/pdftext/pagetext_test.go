package pdftext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

// glyph builds one drawn character 10pt wide at x on baseline y, in a 10pt font.
func glyph(s string, x, y float64) pdf.Text {
	return pdf.Text{FontSize: 10, X: x, Y: y, W: 5, S: s}
}

// word lays out the letters of w from x, each 5pt wide with no gap.
func word(w string, x, y float64) []pdf.Text {
	var out []pdf.Text
	for _, r := range w {
		out = append(out, glyph(string(r), x, y))
		x += 5
	}
	return out
}

func join(parts ...[]pdf.Text) []pdf.Text {
	var all []pdf.Text
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

func TestLayoutText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		glyphs []pdf.Text
		want   string
	}{
		{
			name:   "a gap wider than a kern puts a space back between words that were drawn separately",
			glyphs: join(word("dans", 0, 100), word("le", 25, 100), word("cas", 40, 100)), // 5pt gaps: half an em
			want:   "dans le cas",
		},
		{
			name:   "letters with no gap stay one word",
			glyphs: word("kerning", 0, 100),
			want:   "kerning",
		},
		{
			name:   "a small kerning gap is not a space",
			glyphs: join(word("AV", 0, 100), word("ATAR", 10.5, 100)), // 0.5pt gap, 5% of the font size
			want:   "AVATAR",
		},
		{
			name:   "a vertical move starts a new line",
			glyphs: join(word("one", 0, 100), word("two", 0, 88)),
			want:   "one\ntwo",
		},
		{
			name:   "a superscript is not a new line",
			glyphs: join(word("x", 0, 100), word("2", 5, 103)),
			want:   "x2",
		},
		{
			name:   "returning to the left on the same baseline is a new line",
			glyphs: join(word("right", 100, 100), word("left", 0, 100)),
			want:   "right\nleft",
		},
		{
			name:   "an explicit space is kept once",
			glyphs: join(word("a", 0, 100), []pdf.Text{glyph(" ", 5, 100), glyph(" ", 10, 100)}, word("b", 15, 100)),
			want:   "a b",
		},
		{
			name:   "the replacement character and newline markers the library adds are dropped",
			glyphs: join(word("ab", 0, 100), []pdf.Text{glyph("�", 10, 100), glyph("\n", 10, 100)}, word("cd", 10, 100)),
			want:   "abcd",
		},
		{
			name:   "an accent drawn as its own glyph joins its letter and does not split the word",
			glyphs: join(word("E", 0, 100), []pdf.Text{glyph("́", 0, 106)}, word("TE", 5, 100)),
			want:   "ÉTE",
		},
		{
			name:   "no space at the start of a line or after a line break",
			glyphs: join(word("a", 0, 100), word("b", 20, 88)),
			want:   "a\nb",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := layoutText(tc.glyphs); got != tc.want {
				t.Errorf("layoutText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageText_ReadsARealPDFWithItsSpaces(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "three_pages.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := pdf.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	text, err := PageText(reader.Page(2))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Bravo section. The second page covers the warranty period") {
		t.Fatalf("page text lost its words:\n%s", text)
	}
}
