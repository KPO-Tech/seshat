package officetext

import (
	"strings"
	"testing"
)

const pNS = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" ` +
	`xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram"`

// oneSlideDeck is a deck of one slide whose shapes (and relationships) the test writes.
func oneSlideDeck(t *testing.T, shapes string, extra map[string]string, rels string) []byte {
	t.Helper()
	parts := map[string]string{
		"ppt/slides/slide1.xml": `<p:sld ` + pNS + `><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:sld>`,
	}
	if rels != "" {
		parts["ppt/slides/_rels/slide1.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels + `</Relationships>`
	}
	for name, content := range extra {
		parts[name] = content
	}
	return buildDOCX(t, parts)
}

func extractSlide(t *testing.T, deck []byte) string {
	t.Helper()
	md, _, err := ExtractPPTX(deck)
	if err != nil {
		t.Fatalf("ExtractPPTX: %v", err)
	}
	return md
}

func textShape(ph, bodyProps, text string) string {
	nvPr := `<p:nvPr/>`
	if ph != "" {
		nvPr = `<p:nvPr><p:ph type="` + ph + `"/></p:nvPr>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="2" name="x"/><p:cNvSpPr/>` + nvPr + `</p:nvSpPr><p:txBody>` + bodyProps + text + `</p:txBody></p:sp>`
}

func slidePara(props, text string) string {
	return `<a:p>` + props + `<a:r><a:t>` + text + `</a:t></a:r></a:p>`
}

func TestPPTXRealDeckWithTablesChartsNotesAndPictures(t *testing.T) {
	// testdata/features.pptx was written by python-pptx: placeholders, nested bullets, a table with a merged
	// header, a chart, a picture with a description, a group, speaker notes and a hidden slide.
	md, count, err := ExtractPPTX(readTestdata(t, "features.pptx"))
	if err != nil || count != 6 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	for _, want := range []string{
		"### Quarterly Review\n\nFinance team, Q1",
		"- Grow revenue\n  - North America\n  - Europe\n- Reduce costs",
		"> **Notes:** Mention the Berlin office closing.",
		"| Region | 2024 | 2024 |\n| --- | --- | --- |\n| North | 10 | 12 |",
		"**Chart: Revenue (M EUR)**",
		"| Category | 2023 | 2024 |\n| --- | --- | --- |\n| North | 10.5 | 12 |\n| South | 7 | 9 |\n| East | 3 | 4.25 |",
		"[Image: Office floor plan]",
		"Grouped caption",
		"## Slide 6 (hidden)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "- Plain text box") {
		t.Errorf("a text box is text, not a bullet:\n%s", md)
	}
}

func TestPPTXFootersSlideNumbersAndDatesAreNotContent(t *testing.T) {
	shapes := textShape("title", "", slidePara("", "Real title")) +
		textShape("sldNum", "", slidePara("", "7")) +
		textShape("dt", "", slidePara("", "3/10/2026")) +
		textShape("ftr", "", slidePara("", "Confidential")) +
		textShape("body", "", slidePara("", "The point"))
	md := extractSlide(t, oneSlideDeck(t, shapes, nil, ""))
	if !strings.Contains(md, "### Real title") || !strings.Contains(md, "- The point") {
		t.Fatalf("got:\n%s", md)
	}
	for _, noise := range []string{"Confidential", "3/10/2026", "\n7\n"} {
		if strings.Contains(md, noise) {
			t.Errorf("%q is not slide content:\n%s", noise, md)
		}
	}
}

func TestPPTXNumberedAndUnbulletedParagraphs(t *testing.T) {
	body := slidePara(`<a:pPr><a:buAutoNum type="arabicPeriod"/></a:pPr>`, "First") +
		slidePara(`<a:pPr><a:buAutoNum type="arabicPeriod"/></a:pPr>`, "Second") +
		slidePara(`<a:pPr lvl="1"><a:buAutoNum type="alphaLcParenR"/></a:pPr>`, "Sub") +
		slidePara(`<a:pPr><a:buNone/></a:pPr>`, "Just a sentence.")
	md := extractSlide(t, oneSlideDeck(t, textShape("body", "", body), nil, ""))
	if !strings.Contains(md, "1. First\n2. Second\n  a) Sub\n\nJust a sentence.") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestPPTXHyperlinks(t *testing.T) {
	run := `<a:p><a:r><a:rPr><a:hlinkClick r:id="rId9"/></a:rPr><a:t>our site</a:t></a:r></a:p>`
	rels := `<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com" TargetMode="External"/>`
	md := extractSlide(t, oneSlideDeck(t, textShape("body", "", run), nil, rels))
	if !strings.Contains(md, "- [our site](https://example.com)") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestPPTXTableMergesAndHeaderRow(t *testing.T) {
	cell := func(attrs, text string) string {
		return `<a:tc ` + attrs + `><a:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></a:txBody></a:tc>`
	}
	tbl := `<p:graphicFrame><a:graphic><a:graphicData><a:tbl><a:tblPr firstRow="1"/>` +
		`<a:tr>` + cell(`rowSpan="2"`, "Team") + cell(`gridSpan="2"`, "Q1") + cell(`hMerge="1"`, "") + `</a:tr>` +
		`<a:tr>` + cell(`vMerge="1"`, "") + cell("", "Plan") + cell("", "Actual") + `</a:tr>` +
		`<a:tr>` + cell("", "Red") + cell("", "5") + cell("", "6") + `</a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	md := extractSlide(t, oneSlideDeck(t, tbl, nil, ""))
	for _, want := range []string{"| Team | Q1 | Q1 |", "| Team | Plan | Actual |", "| Red | 5 | 6 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestPPTXSmartArtTextIsKept(t *testing.T) {
	frame := `<p:graphicFrame><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/diagram"><dgm:relIds r:dm="rId3" r:lo="rId4"/></a:graphicData></a:graphic></p:graphicFrame>`
	data := `<dgm:dataModel ` + pNS + `><dgm:ptLst>` +
		`<dgm:pt modelId="1" type="doc"><dgm:t><a:p><a:r><a:t>diagram root</a:t></a:r></a:p></dgm:t></dgm:pt>` +
		`<dgm:pt modelId="2"><dgm:t><a:p><a:r><a:t>Plan</a:t></a:r></a:p></dgm:t></dgm:pt>` +
		`<dgm:pt modelId="3"><dgm:t><a:p><a:r><a:t>Build</a:t></a:r></a:p></dgm:t></dgm:pt>` +
		`<dgm:pt modelId="4" type="parTrans"><dgm:t><a:p><a:r><a:t>link</a:t></a:r></a:p></dgm:t></dgm:pt>` +
		`</dgm:ptLst></dgm:dataModel>`
	rels := `<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/diagramData" Target="../diagrams/data1.xml"/>`
	md := extractSlide(t, oneSlideDeck(t, frame, map[string]string{"ppt/diagrams/data1.xml": data}, rels))
	if !strings.Contains(md, "- Plan\n- Build") || strings.Contains(md, "diagram root") || strings.Contains(md, "link") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestPPTXChartWithXYSeriesAndGaps(t *testing.T) {
	frame := `<p:graphicFrame><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart r:id="rId7"/></a:graphicData></a:graphic></p:graphicFrame>`
	chart := `<c:chartSpace ` + pNS + `><c:chart><c:plotArea><c:scatterChart><c:ser>` +
		`<c:tx><c:strRef><c:strCache><c:pt idx="0"><c:v>Speed</c:v></c:pt></c:strCache></c:strRef></c:tx>` +
		`<c:xVal><c:numRef><c:numCache><c:ptCount val="3"/><c:pt idx="0"><c:v>1</c:v></c:pt><c:pt idx="2"><c:v>3</c:v></c:pt></c:numCache></c:numRef></c:xVal>` +
		`<c:yVal><c:numRef><c:numCache><c:ptCount val="3"/><c:pt idx="0"><c:v>0.30000000000000004</c:v></c:pt><c:pt idx="1"><c:v>0.5</c:v></c:pt><c:pt idx="2"><c:v>0.9</c:v></c:pt></c:numCache></c:numRef></c:yVal>` +
		`</c:ser></c:scatterChart></c:plotArea></c:chart></c:chartSpace>`
	rels := `<Relationship Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart1.xml"/>`
	md := extractSlide(t, oneSlideDeck(t, frame, map[string]string{"ppt/charts/chart1.xml": chart}, rels))
	if !strings.Contains(md, "| Category | Speed |\n| --- | --- |\n| 1 | 0.3 |\n|  | 0.5 |\n| 3 | 0.9 |") {
		t.Fatalf("got:\n%s", md)
	}
}
