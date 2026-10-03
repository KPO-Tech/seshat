package officetext

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat/internal/mdtable"
)

// ExtractDOCX converts a Word document to markdown: headings (whatever language the styles are named in),
// paragraphs, bulleted and numbered lists with the numbers Word writes from the list ("3.2"), tables with their
// merged cells, hyperlinks, footnotes, the alternative text of pictures, the text in text boxes, and code. What it
// leaves out is what is not the document's own text: headers and footers (the same line on every page),
// comments, tracked deletions, and the pictures themselves.
func ExtractDOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("not a valid DOCX (zip open failed): %w", err)
	}
	f := findZipFile(zr, "word/document.xml")
	if f == nil {
		return "", fmt.Errorf("not a valid DOCX: missing word/document.xml")
	}
	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open word/document.xml: %w", err)
	}
	defer rc.Close()
	tree, err := parseXMLTree(rc)
	if err != nil {
		return "", fmt.Errorf("failed to parse word/document.xml: %w", err)
	}
	body := findFirst(tree, "body")
	if body == nil {
		return "", fmt.Errorf("not a valid DOCX: missing w:body")
	}

	doc := &wordDoc{
		styles:    loadWordStyles(zr),
		numbering: loadWordNumbering(zr),
		links:     loadRelationships(zr, "word/_rels/document.xml.rels"),
		footnotes: loadNotes(zr, "word/footnotes.xml", ""),
	}
	for id, text := range loadNotes(zr, "word/endnotes.xml", "e") {
		doc.footnotes[id] = text
	}
	doc.hasTitle = doc.findTitle(body)
	doc.blocks(body)
	doc.out.flush()
	doc.writeFootnotes()
	return strings.TrimSpace(doc.out.String()), nil
}

type wordDoc struct {
	styles    wordStyles
	numbering *wordNumbering
	links     map[string]string // relationship id -> target
	footnotes map[string]string // note id -> text
	cited     []string          // note ids in the order the text refers to them
	hasTitle  bool              // the document has a title, so its headings start one level below it
	out       blockWriter
}

// loadRelationships maps relationship ids to their targets (the address behind a hyperlink).
func loadRelationships(zr *zip.Reader, name string) map[string]string {
	links := map[string]string{}
	if tree := readPart(zr, name); tree != nil {
		for _, rel := range findAll(tree, "Relationship") {
			if rel.attr("Id") != "" {
				links[rel.attr("Id")] = rel.attr("Target")
			}
		}
	}
	return links
}

// loadNotes reads the footnotes (or endnotes) part into id -> text. prefix keeps endnote ids apart from footnote ids.
func loadNotes(zr *zip.Reader, name, prefix string) map[string]string {
	notes := map[string]string{}
	tree := readPart(zr, name)
	if tree == nil {
		return notes
	}
	for _, note := range findAll(tree, "footnote") {
		notes[prefix+note.attr("id")] = noteText(note)
	}
	for _, note := range findAll(tree, "endnote") {
		notes[prefix+note.attr("id")] = noteText(note)
	}
	for id, text := range notes {
		if text == "" {
			delete(notes, id)
		}
	}
	return notes
}

func noteText(note *node) string {
	if t := note.attr("type"); t != "" && t != "normal" {
		return "" // separators
	}
	doc := &wordDoc{footnotes: map[string]string{}}
	var parts []string
	for _, p := range findAll(note, "p") {
		if text := strings.TrimSpace(doc.runs(p).text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func (d *wordDoc) writeFootnotes() {
	if len(d.cited) == 0 {
		return
	}
	var sb strings.Builder
	for _, id := range d.cited {
		if text := d.footnotes[id]; text != "" {
			fmt.Fprintf(&sb, "[^%s]: %s\n", id, text)
		}
	}
	if sb.Len() > 0 {
		d.out.WriteString("\n")
		d.out.WriteString(sb.String())
	}
}

// blocks writes the block-level children of a container (the body, a content control, a tracked insertion).
func (d *wordDoc) blocks(container *node) {
	for _, c := range container.Children {
		switch c.Local {
		case "p":
			d.paragraph(c)
		case "tbl":
			d.out.flush()
			d.table(c).RenderMarkdown(&d.out.Builder)
		case "sdt":
			if content := c.child("sdtContent"); content != nil {
				d.blocks(content)
			}
		case "AlternateContent":
			if choice := c.child("Choice"); choice != nil {
				d.blocks(choice)
			}
		case "ins", "moveTo", "customXml", "smartTag", "sdtContent":
			d.blocks(c)
		}
	}
}

func (d *wordDoc) paragraph(p *node) {
	styleID, direct := "", 0
	numID, level, listed := "", 0, false
	if pPr := p.child("pPr"); pPr != nil {
		if s := pPr.child("pStyle"); s != nil {
			styleID = s.attr("val")
		}
		if o := pPr.child("outlineLvl"); o != nil {
			if v, err := strconv.Atoi(o.attr("val")); err == nil {
				direct = v + 1
			}
		}
		if numPr := pPr.child("numPr"); numPr != nil {
			numID, level, listed = numPrOf(numPr)
		}
	}
	if !listed {
		numID, level, listed = d.styles.numbering(styleID)
	}

	content := d.runs(p)
	text := strings.TrimSpace(content.text)
	defer func() {
		for _, box := range content.boxes {
			d.textBox(box)
		}
	}()
	if text == "" && len(content.images) == 0 {
		return
	}
	for _, image := range content.images {
		if text != "" {
			text += "\n\n"
		}
		text += image
	}

	if heading := d.styles.headingLevel(styleID, direct); heading > 0 {
		if d.hasTitle && !d.styles.isTitle(styleID) {
			heading = min(heading+1, 6)
		}
		d.out.flush()
		label := ""
		if listed {
			label, _, _ = d.numbering.next(numID, level)
		}
		d.out.block(strings.Repeat("#", heading) + " " + strings.TrimSpace(label+" "+oneLine(text)))
		return
	}
	if listed {
		label, bullet, _ := d.numbering.next(numID, level)
		marker := label
		if bullet {
			marker = "-"
		}
		d.out.listItem(level, marker, text)
		return
	}
	if content.mono || d.styles.isCode(styleID) {
		d.out.codeLine(content.text)
		return
	}
	if d.styles.isQuote(styleID) {
		d.out.block("> " + strings.ReplaceAll(text, "\n", "\n> "))
		return
	}
	d.out.block(text)
}

// findTitle says whether any paragraph of the body is in the title style.
func (d *wordDoc) findTitle(body *node) bool {
	for _, p := range findAll(body, "p") {
		if pPr := p.child("pPr"); pPr != nil {
			if s := pPr.child("pStyle"); s != nil && d.styles.isTitle(s.attr("val")) {
				return true
			}
		}
	}
	return false
}

func (d *wordDoc) textBox(paragraphs []string) {
	for _, text := range paragraphs {
		d.out.block(text)
	}
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// paragraphContent is what the runs of one paragraph add up to.
type paragraphContent struct {
	text   string
	mono   bool       // every run with text is in a monospaced font: a line of code
	images []string   // "[Image: ...]" for each picture that has a description
	boxes  [][]string // paragraphs of the text boxes anchored in this paragraph
}

func (d *wordDoc) runs(p *node) paragraphContent {
	var c paragraphContent
	var sb strings.Builder
	sawText, allMono := false, true
	d.collect(p, &sb, &c, &sawText, &allMono)
	c.text = sb.String()
	c.mono = sawText && allMono
	return c
}

func (d *wordDoc) collect(n *node, sb *strings.Builder, c *paragraphContent, sawText, allMono *bool) {
	for _, child := range n.Children {
		switch child.Local {
		case "r":
			d.run(child, sb, c, sawText, allMono)
		case "hyperlink":
			var inner strings.Builder
			d.collect(child, &inner, c, sawText, allMono)
			text, target := inner.String(), d.links[child.nsAttr("id")]
			if strings.TrimSpace(text) != "" && (strings.HasPrefix(target, "http") || strings.HasPrefix(target, "mailto:")) {
				fmt.Fprintf(sb, "[%s](%s)", strings.TrimSpace(text), target)
			} else {
				sb.WriteString(text)
			}
		case "ins", "smartTag", "sdt", "sdtContent", "customXml", "moveTo", "fldSimple", "oMath", "oMathPara":
			d.collect(child, sb, c, sawText, allMono)
		case "AlternateContent":
			if choice := child.child("Choice"); choice != nil {
				d.collect(choice, sb, c, sawText, allMono)
			}
		}
	}
}

func (d *wordDoc) run(r *node, sb *strings.Builder, c *paragraphContent, sawText, allMono *bool) {
	hadText := false
	for _, e := range r.Children {
		switch e.Local {
		case "t":
			sb.WriteString(e.Text)
			hadText = hadText || strings.TrimSpace(e.Text) != ""
		case "tab":
			sb.WriteByte(' ')
		case "br", "cr":
			if t := e.attr("type"); t != "page" && t != "column" {
				sb.WriteByte('\n')
			}
		case "noBreakHyphen":
			sb.WriteByte('-')
		case "footnoteReference", "endnoteReference":
			id := e.attr("id")
			if e.Local == "endnoteReference" {
				id = "e" + id
			}
			if _, ok := d.footnotes[id]; ok {
				fmt.Fprintf(sb, "[^%s]", id)
				d.cited = append(d.cited, id)
			}
		case "drawing", "pict", "object":
			d.drawing(e, c)
		case "AlternateContent":
			if choice := e.child("Choice"); choice != nil {
				d.drawing(choice, c)
			}
		}
	}
	if hadText {
		*sawText = true
		if rPr := r.child("rPr"); rPr == nil || !runIsMonospace(rPr) {
			*allMono = false
		}
	}
}

// drawing finds what a picture, a shape or a text box contributes to the text: the description the author
// gave a picture, and the paragraphs of a text box.
func (d *wordDoc) drawing(n *node, c *paragraphContent) {
	boxes := findAll(n, "txbxContent")
	for _, box := range boxes {
		var paragraphs []string
		for _, p := range findAll(box, "p") {
			if text := strings.TrimSpace(d.runs(p).text); text != "" {
				paragraphs = append(paragraphs, text)
			}
		}
		if len(paragraphs) > 0 {
			c.boxes = append(c.boxes, paragraphs)
		}
	}
	if len(boxes) > 0 {
		return // a text box is a shape with text, not a picture with a description
	}
	if docPr := findFirst(n, "docPr"); docPr != nil {
		alt := strings.TrimSpace(docPr.attr("descr"))
		if alt == "" {
			alt = strings.TrimSpace(docPr.attr("title"))
		}
		if alt != "" {
			c.images = append(c.images, "[Image: "+oneLine(alt)+"]")
		}
	}
}

// table reads a w:tbl into a Table with its merged cells resolved: gridSpan repeats a cell's text across the
// columns it covers, and a vertical merge repeats the text of the cell it continues.
func (d *wordDoc) table(tbl *node) mdtable.Table {
	t := mdtable.Table{}
	if look := findFirst(tbl, "tblLook"); look != nil && look.attr("firstRow") == "1" {
		t.HeaderRows = 1
	}
	flaggedHeaders, leading := 0, true
	for _, tr := range tbl.childrenNamed("tr") {
		row := []string{}
		for _, tc := range tr.childrenNamed("tc") {
			span, continues := 1, false
			if pr := tc.child("tcPr"); pr != nil {
				if g := pr.child("gridSpan"); g != nil {
					if v, err := strconv.Atoi(g.attr("val")); err == nil && v > 1 {
						span = v
					}
				}
				if m := pr.child("vMerge"); m != nil {
					continues = m.attr("val") != "restart"
				}
			}
			text := d.cellText(tc)
			if continues && len(t.Rows) > 0 {
				if above := t.Rows[len(t.Rows)-1]; len(row) < len(above) {
					text = above[len(row)]
				}
			}
			for i := 0; i < span; i++ {
				row = append(row, text)
			}
		}
		t.Rows = append(t.Rows, row)
		if pr := tr.child("trPr"); pr != nil && pr.child("tblHeader") != nil && leading {
			flaggedHeaders++
		} else {
			leading = false
		}
	}
	if flaggedHeaders > 0 {
		t.HeaderRows = flaggedHeaders
	}
	return t
}

// cellText is the text of a table cell: its paragraphs on separate lines, a nested table flattened to lines.
func (d *wordDoc) cellText(tc *node) string {
	var lines []string
	for _, c := range tc.Children {
		switch c.Local {
		case "p":
			content := d.runs(c)
			line := strings.TrimSpace(content.text)
			if line == "" {
				continue
			}
			if _, _, listed := d.cellListed(c); listed {
				line = "- " + line
			}
			lines = append(lines, line)
		case "tbl":
			for _, row := range d.table(c).Rows {
				var cells []string
				for _, cell := range row {
					if cell = strings.TrimSpace(cell); cell != "" {
						cells = append(cells, oneLine(cell))
					}
				}
				if len(cells) > 0 {
					lines = append(lines, strings.Join(cells, "; "))
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

// cellListed says whether a paragraph in a table cell is a list item. The numbers are not counted there: a
// list in a cell is written as bullets, because the cell is one line of a markdown row.
func (d *wordDoc) cellListed(p *node) (string, int, bool) {
	styleID := ""
	if pPr := p.child("pPr"); pPr != nil {
		if s := pPr.child("pStyle"); s != nil {
			styleID = s.attr("val")
		}
		if numPr := pPr.child("numPr"); numPr != nil {
			if id, level, ok := numPrOf(numPr); ok {
				return id, level, id != "0"
			}
		}
	}
	id, level, ok := d.styles.numbering(styleID)
	return id, level, ok && id != "0"
}

// blockWriter writes markdown blocks, keeping a list together and a run of code lines in one fence.
type blockWriter struct {
	strings.Builder
	state string // "", "list", "code"
}

func (w *blockWriter) flush() {
	switch w.state {
	case "code":
		w.WriteString("```\n\n")
	case "list":
		w.WriteString("\n")
	}
	w.state = ""
}

func (w *blockWriter) block(text string) {
	w.flush()
	w.WriteString(text)
	w.WriteString("\n\n")
}

func (w *blockWriter) listItem(level int, marker, text string) {
	if w.state != "list" {
		w.flush()
		w.state = "list"
	}
	w.WriteString(strings.Repeat("  ", level))
	if marker != "" {
		w.WriteString(marker)
		w.WriteByte(' ')
	}
	w.WriteString(strings.ReplaceAll(text, "\n", " "))
	w.WriteByte('\n')
}

func (w *blockWriter) codeLine(text string) {
	if w.state != "code" {
		w.flush()
		w.WriteString("```\n")
		w.state = "code"
	}
	w.WriteString(strings.TrimRight(text, " \t\r\n"))
	w.WriteByte('\n')
}

func findZipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}
