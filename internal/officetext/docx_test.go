package officetext

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

const (
	wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
		`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
)

// buildDOCX writes a package from its parts, so a test can say exactly which OOXML it is about.
func buildDOCX(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func docxBody(body string) map[string]string {
	return map[string]string{"word/document.xml": `<?xml version="1.0"?><w:document ` + wNS + `><w:body>` + body + `</w:body></w:document>`}
}

func para(style, text string) string {
	pPr := ""
	if style != "" {
		pPr = `<w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`
	}
	return `<w:p>` + pPr + `<w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func listPara(numID, level int, text string) string {
	return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + itoa(level) + `"/><w:numId w:val="` + itoa(numID) + `"/></w:numPr></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func itoa(n int) string { return strconv.Itoa(n) }

func extractDOCX(t *testing.T, parts map[string]string) string {
	t.Helper()
	md, err := ExtractDOCX(buildDOCX(t, parts))
	if err != nil {
		t.Fatalf("ExtractDOCX: %v", err)
	}
	return md
}

func TestDOCXHeadingsComeFromTheOutlineLevelNotTheStyleName(t *testing.T) {
	// A French Word names its heading style "heading 1" but gives it the id "Titre1"; a custom style can have any
	// name at all and still be a heading because it has an outline level.
	parts := docxBody(para("Titre1", "Introduction") + para("Titre2", "Contexte") + para("MonStyle", "Annexe") + para("Corps", "Du texte."))
	parts["word/styles.xml"] = `<w:styles ` + wNS + `>
	  <w:style w:type="paragraph" w:styleId="Titre1"><w:name w:val="heading 1"/><w:pPr><w:outlineLvl w:val="0"/></w:pPr></w:style>
	  <w:style w:type="paragraph" w:styleId="Titre2"><w:name w:val="heading 2"/><w:basedOn w:val="Titre1"/><w:pPr><w:outlineLvl w:val="1"/></w:pPr></w:style>
	  <w:style w:type="paragraph" w:styleId="MonStyle"><w:name w:val="Mon style"/><w:basedOn w:val="Titre1"/></w:style>
	  <w:style w:type="paragraph" w:styleId="Corps"><w:name w:val="Corps de texte"/><w:pPr><w:outlineLvl w:val="9"/></w:pPr></w:style>
	</w:styles>`
	got := extractDOCX(t, parts)
	for _, want := range []string{"# Introduction", "## Contexte", "# Annexe", "\nDu texte."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "# Du texte") {
		t.Errorf("body text became a heading:\n%s", got)
	}
}

func TestDOCXWithoutStylesFallsBackToTheStyleName(t *testing.T) {
	got := extractDOCX(t, docxBody(para("Heading2", "Scope")+para("Normal", "Text")))
	if !strings.Contains(got, "## Scope") || strings.Contains(got, "# Text") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDOCXHeadingsStartBelowTheTitle(t *testing.T) {
	got := extractDOCX(t, docxBody(para("Title", "Annual Report")+para("Heading1", "Summary")))
	if !strings.Contains(got, "# Annual Report") || !strings.Contains(got, "## Summary") {
		t.Fatalf("got:\n%s", got)
	}
}

func numberingXML() string {
	return `<w:numbering ` + wNS + `>
	  <w:abstractNum w:abstractNumId="0">
	    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
	    <w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="(%2)"/></w:lvl>
	  </w:abstractNum>
	  <w:abstractNum w:abstractNumId="1">
	    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="*"/></w:lvl>
	  </w:abstractNum>
	  <w:abstractNum w:abstractNumId="2">
	    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="Article %1"/></w:lvl>
	    <w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/></w:lvl>
	  </w:abstractNum>
	  <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
	  <w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
	  <w:num w:numId="3"><w:abstractNumId w:val="2"/></w:num>
	  <w:num w:numId="4"><w:abstractNumId w:val="0"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="5"/></w:lvlOverride></w:num>
	</w:numbering>`
}

func TestDOCXListsKeepTheirKindDepthAndNumbers(t *testing.T) {
	parts := docxBody(
		listPara(1, 0, "First") + listPara(1, 1, "Sub a") + listPara(1, 1, "Sub b") + listPara(1, 0, "Second") + listPara(1, 1, "Restarted") +
			para("", "Between") +
			listPara(2, 0, "Bullet") + listPara(2, 0, "Bullet two"))
	parts["word/numbering.xml"] = numberingXML()
	got := extractDOCX(t, parts)
	want := "1. First\n  (a) Sub a\n  (b) Sub b\n2. Second\n  (a) Restarted\n\nBetween\n\n- Bullet\n- Bullet two"
	if !strings.Contains(got, want) {
		t.Fatalf("got:\n%s\nwant it to contain:\n%s", got, want)
	}
}

func TestDOCXAListCanStartAtAnotherNumber(t *testing.T) {
	parts := docxBody(listPara(4, 0, "Fifth") + listPara(4, 0, "Sixth"))
	parts["word/numbering.xml"] = numberingXML()
	if got := extractDOCX(t, parts); !strings.Contains(got, "5. Fifth\n6. Sixth") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDOCXNumberedHeadingsKeepTheirNumbers(t *testing.T) {
	// The "Article 1" and "1.1" are not in the text of the paragraphs: Word writes them from the list.
	heading := func(level int, text string) string {
		return `<w:p><w:pPr><w:pStyle w:val="Heading1"/><w:numPr><w:ilvl w:val="` + itoa(level) + `"/><w:numId w:val="3"/></w:numPr></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
	}
	parts := docxBody(heading(0, "Definitions") + heading(1, "Scope") + heading(1, "Term") + heading(0, "Payment"))
	parts["word/numbering.xml"] = numberingXML()
	got := extractDOCX(t, parts)
	for _, want := range []string{"# Article 1 Definitions", "# 1.1 Scope", "# 1.2 Term", "# Article 2 Payment"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestDOCXHyperlinksFootnotesAndPictures(t *testing.T) {
	body := `<w:p><w:r><w:t xml:space="preserve">See </w:t></w:r>` +
		`<w:hyperlink r:id="rId5"><w:r><w:t>the site</w:t></w:r></w:hyperlink>` +
		`<w:r><w:t>.</w:t></w:r><w:r><w:footnoteReference w:id="2"/></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:inline><wp:docPr id="1" name="Picture 1" descr="A bar chart of sales by region"/></wp:inline></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:inline><wp:docPr id="2" name="Picture 2"/></wp:inline></w:drawing></w:r></w:p>`
	parts := docxBody(body)
	parts["word/_rels/document.xml.rels"] = `<Relationships><Relationship Id="rId5" Type="x" Target="https://example.com/a" TargetMode="External"/></Relationships>`
	parts["word/footnotes.xml"] = `<w:footnotes ` + wNS + `>` +
		`<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>` +
		`<w:footnote w:id="2"><w:p><w:r><w:t>Source: annual accounts.</w:t></w:r></w:p></w:footnote></w:footnotes>`
	got := extractDOCX(t, parts)
	for _, want := range []string{"See [the site](https://example.com/a).[^2]", "[Image: A bar chart of sales by region]", "[^2]: Source: annual accounts."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Picture 2") {
		t.Errorf("a picture with no description is not text:\n%s", got)
	}
}

func TestDOCXTextBoxIsReadOnceAndTrackedDeletionsAreNot(t *testing.T) {
	box := `<w:p><w:r><mc:AlternateContent>` +
		`<mc:Choice Requires="wps"><w:drawing><wp:anchor><a:graphic><a:graphicData><w:txbxContent><w:p><w:r><w:t>Boxed note</w:t></w:r></w:p></w:txbxContent></a:graphicData></a:graphic></wp:anchor></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><w:txbxContent><w:p><w:r><w:t>Boxed note</w:t></w:r></w:p></w:txbxContent></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r><w:r><w:t>Body text</w:t></w:r></w:p>`
	tracked := `<w:p><w:r><w:t>Kept </w:t></w:r><w:del><w:r><w:delText>removed</w:delText></w:r></w:del><w:ins><w:r><w:t>added</w:t></w:r></w:ins></w:p>`
	got := extractDOCX(t, docxBody(box+tracked))
	if strings.Count(got, "Boxed note") != 1 {
		t.Errorf("the text box should appear once:\n%s", got)
	}
	if !strings.Contains(got, "Body text") || !strings.Contains(got, "Kept added") || strings.Contains(got, "removed") {
		t.Errorf("tracked changes:\n%s", got)
	}
}

func TestDOCXTablesResolveMergedCellsAndHeaderRows(t *testing.T) {
	cell := func(text string, props string) string {
		return `<w:tc><w:tcPr>` + props + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc>`
	}
	row := func(header bool, cells ...string) string {
		pr := ""
		if header {
			pr = `<w:trPr><w:tblHeader/></w:trPr>`
		}
		return `<w:tr>` + pr + strings.Join(cells, "") + `</w:tr>`
	}
	table := `<w:tbl>` +
		row(true, cell("Team", `<w:vMerge w:val="restart"/>`), cell("Q1", `<w:gridSpan w:val="2"/>`)) +
		row(true, cell("", `<w:vMerge/>`), cell("Plan", ""), cell("Actual", "")) +
		row(false, cell("Red", ""), cell("5", ""), cell("6", "")) +
		`</w:tbl>`
	want := "| Team | Q1 / Plan | Q1 / Actual |\n| --- | --- | --- |\n| Red | 5 | 6 |"
	if got := extractDOCX(t, docxBody(table)); !strings.Contains(got, want) {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDOCXTableCellsKeepParagraphsListsAndNestedTables(t *testing.T) {
	inner := `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>in1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>in2</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	table := `<w:tbl>` +
		`<w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>one</w:t></w:r></w:p><w:p><w:r><w:t>two</w:t></w:r></w:p></w:tc><w:tc>` + inner + `<w:p/></w:tc></w:tr>` +
		`</w:tbl>`
	got := extractDOCX(t, docxBody(table))
	if !strings.Contains(got, "| one<br>two | in1; in2 |") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDOCXHeadersAndFootersAreNotPartOfTheText(t *testing.T) {
	parts := docxBody(para("", "Body"))
	parts["word/header1.xml"] = `<w:hdr ` + wNS + `><w:p><w:r><w:t>CONFIDENTIAL</w:t></w:r></w:p></w:hdr>`
	if got := extractDOCX(t, parts); strings.Contains(got, "CONFIDENTIAL") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDOCXCodeRunsAreFenced(t *testing.T) {
	code := func(text string) string {
		return `<w:p><w:r><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/></w:rPr><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
	}
	got := extractDOCX(t, docxBody(para("", "Run this:")+code("x = 1")+code("    y = 2")+para("", "Done.")))
	if !strings.Contains(got, "```\nx = 1\n    y = 2\n```\n\nDone.") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDOCXRealFileWithEverythingWordWrites(t *testing.T) {
	// testdata/features.docx was written by python-docx from the default Word template: list styles that carry
	// their numbering, a table with merged header cells, code in Consolas, a quote and a hyperlink.
	md, err := ExtractDOCX(readTestdata(t, "features.docx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Quarterly Report", "## Overview", "### Goals",
		"- Grow revenue\n  - North America\n  - Europe\n- Reduce costs",
		"1. Collect data\n2. Clean it\n3. Publish",
		"| Region | 2024 / Sales | 2024 / Cost | Notes |",
		"| South | 7 | 3 | a \\| b |",
		"```\ndef total(xs):\n    return sum(xs)\n```",
		"> Numbers are what they are.",
		"[the site](https://example.com/report)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestDOCXEveryTextBoxOfAGroupIsRead(t *testing.T) {
	box := func(text string) string {
		return `<w:txbxContent><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:txbxContent>`
	}
	body := `<w:p><w:r><w:drawing><wp:anchor><a:graphic><a:graphicData>` + box("Left box") + box("Right box") + `</a:graphicData></a:graphic></wp:anchor></w:drawing></w:r></w:p>`
	got := extractDOCX(t, docxBody(body))
	if !strings.Contains(got, "Left box") || !strings.Contains(got, "Right box") {
		t.Fatalf("got:\n%s", got)
	}
}
