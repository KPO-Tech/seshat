package pdftext

import (
	"fmt"
	"strings"
	"testing"
)

// namedLigatureFont is Helvetica whose codes 27 to 29 are the glyphs f_i, f_f_i and f_l, named by their parts, with a width for
// every character from 27.
func namedLigatureFont() string {
	widths := []string{"556", "556", "556", "556", "556"} // 27 to 31
	for _, w := range helvetica {
		widths = append(widths, fmt.Sprint(w))
	}
	return fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+Helvetica /FirstChar 27 /LastChar %d /Widths [%s] "+
		"/Encoding << /Type /Encoding /Differences [27 /f_i /f_f_i /f_l] >> >>", 26+len(widths), strings.Join(widths, " "))
}

func TestGlyphsNamedByTheirPartsAreWrittenAsLetters(t *testing.T) {
	t.Parallel()
	// \033, \034 and \035 are the octal escapes of the codes 27, 28 and 29 in a PDF string.
	content := `BT /F1 12 Tf 1 0 0 1 100 700 Tm (The \033le shows an e\034cient \035ow of money.) Tj ET` + "\n"
	md, err := PageMarkdown(firstPage(t, buildPDF(0, content, namedLigatureFont())))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(md), "The file shows an efficient flow of money."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLigatureCharactersAreWrittenAsTheLettersTheyStandFor(t *testing.T) {
	t.Parallel()
	in := "ﬁne-tuning, ﬂow, diﬀerent, oﬃce, baﬄe, ﬅop"
	if got, want := ligatureLetters.Replace(in), "fine-tuning, flow, different, office, baffle, stop"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOnlyGlyphNamesMadeOfSingleLettersAreLigatures(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{"f_i": "fi", "f_f_l": "ffl", "T_h": "Th", "f": "", "one_two": "", "a_1": "", "": "", "fi": ""} {
		got, ok := lettersOfGlyphName(name)
		if got != want || ok != (want != "") {
			t.Errorf("%q: got %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestControlCharactersLeftByASymbolFontAreDropped(t *testing.T) {
	t.Parallel()
	in := "allowed.\x02 DENIED:\x03 not\tallowed\nnext"
	if got, want := withoutControls(in), "allowed. DENIED: not\tallowed\nnext"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
