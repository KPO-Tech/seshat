package officetext

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat/internal/mdtable"
)

// ExtractPPTX converts a slide deck to markdown: one "## Slide N" section per slide, with the slide's title
// promoted to a heading, bullets with their depth, tables with their merged cells, charts as tables of their
// data, the description of pictures, the text of SmartArt diagrams, and the speaker notes. Slide order follows the
// presentation's slide list (ppt/presentation.xml's sldIdLst, resolved through the rels part), not filename
// sort: a deck whose slides were reordered after the files were first created would come out in the wrong order.
//
// slideCount is the deck's total slide count (including slides that render to nothing, e.g. a slide that is
// entirely a picture) - the caller (officetext.Extract) uses it to flag decks whose extracted text is sparse
// relative to how many slides actually exist, not just whether it is literally empty.
func ExtractPPTX(data []byte) (markdown string, slideCount int, err error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", 0, fmt.Errorf("not a valid PPTX (zip open failed): %w", err)
	}
	slidePaths, err := orderedSlidePaths(zr)
	if err != nil {
		return "", 0, err
	}
	if len(slidePaths) == 0 {
		return "", 0, fmt.Errorf("not a valid PPTX: no slides found")
	}

	var sb strings.Builder
	for i, slidePath := range slidePaths {
		tree := readPart(zr, slidePath)
		if tree == nil {
			continue
		}
		slide := &pptxSlide{zr: zr, path: slidePath, rels: loadSlideRels(zr, slidePath)}
		body := slide.render(tree)
		if strings.TrimSpace(body) == "" {
			continue
		}
		heading := fmt.Sprintf("## Slide %d", i+1)
		if sld := findFirst(tree, "sld"); sld != nil && sld.attr("show") == "0" {
			heading += " (hidden)"
		}
		sb.WriteString(heading + "\n\n")
		sb.WriteString(body)
	}
	return strings.TrimSpace(sb.String()), len(slidePaths), nil
}

// orderedSlidePaths resolves ppt/presentation.xml's <p:sldId> list (in document order) to zip entry paths via
// ppt/_rels/presentation.xml.rels. Falls back to a numeric sort of ppt/slides/slideN.xml if either part is
// missing or malformed, so a still-valid-but-unusual PPTX degrades to filename order instead of failing outright.
func orderedSlidePaths(zr *zip.Reader) ([]string, error) {
	presTree := readPart(zr, "ppt/presentation.xml")
	relsTree := readPart(zr, "ppt/_rels/presentation.xml.rels")
	if presTree == nil || relsTree == nil {
		return fallbackSlidePaths(zr), nil
	}
	targets := make(map[string]string) // rId -> "ppt/slides/slideN.xml"
	for _, rel := range findAll(relsTree, "Relationship") {
		if id, target := rel.attr("Id"), rel.attr("Target"); id != "" && target != "" {
			targets[id] = resolveTarget("ppt/presentation.xml", target)
		}
	}
	sldIDLst := findFirst(presTree, "sldIdLst")
	if sldIDLst == nil {
		return fallbackSlidePaths(zr), nil
	}
	var paths []string
	for _, sldID := range findAll(sldIDLst, "sldId") {
		// <p:sldId id="256" r:id="rId2"/> - "id" and "r:id" collide on local name, so this must go through
		// nsAttr, not attr, to get the relationship id rather than the arbitrary sldId number.
		if target, ok := targets[sldID.nsAttr("id")]; ok {
			paths = append(paths, target)
		}
	}
	if len(paths) == 0 {
		return fallbackSlidePaths(zr), nil
	}
	return paths, nil
}

func fallbackSlidePaths(zr *zip.Reader) []string {
	type indexed struct {
		n    int
		path string
	}
	var slides []indexed
	for _, f := range zr.File {
		if path.Dir(f.Name) != "ppt/slides" {
			continue
		}
		base := path.Base(f.Name)
		if !strings.HasPrefix(base, "slide") || !strings.HasSuffix(base, ".xml") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "slide"), ".xml"))
		if err != nil {
			continue
		}
		slides = append(slides, indexed{n: n, path: f.Name})
	}
	for i := 0; i < len(slides); i++ {
		for j := i + 1; j < len(slides); j++ {
			if slides[j].n < slides[i].n {
				slides[i], slides[j] = slides[j], slides[i]
			}
		}
	}
	paths := make([]string, len(slides))
	for i, s := range slides {
		paths[i] = s.path
	}
	return paths
}

// resolveTarget turns a relationship target into a path in the package: absolute ("/ppt/x.xml") or relative to
// the part that holds the relationship.
func resolveTarget(from, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Join(path.Dir(from), target)
}

type slideRel struct {
	kind   string // the last element of the relationship type: "chart", "notesSlide", "hyperlink", "diagramData"...
	target string // a package path, or an address for an external target
	web    bool
}

func loadSlideRels(zr *zip.Reader, slidePath string) map[string]slideRel {
	rels := map[string]slideRel{}
	tree := readPart(zr, path.Join(path.Dir(slidePath), "_rels", path.Base(slidePath)+".rels"))
	if tree == nil {
		return rels
	}
	for _, rel := range findAll(tree, "Relationship") {
		kind := rel.attr("Type")
		kind = kind[strings.LastIndex(kind, "/")+1:]
		external := rel.attr("TargetMode") == "External"
		target := rel.attr("Target")
		if !external {
			target = resolveTarget(slidePath, target)
		}
		rels[rel.attr("Id")] = slideRel{kind: kind, target: target, web: external}
	}
	return rels
}

type pptxSlide struct {
	zr   *zip.Reader
	path string
	rels map[string]slideRel
	out  strings.Builder
	list listState
}

// listState counts the numbered items of the current text shape, level by level.
type listState struct {
	counters map[int]int
}

func (l *listState) reset() { l.counters = map[int]int{} }

func (l *listState) next(level int) int {
	if l.counters == nil {
		l.counters = map[int]int{}
	}
	l.counters[level]++
	for deeper := range l.counters {
		if deeper > level {
			delete(l.counters, deeper)
		}
	}
	return l.counters[level]
}

// render writes a slide: the title first, then the shapes in the order the file lists them, then the notes.
func (s *pptxSlide) render(tree *node) string {
	spTree := findFirst(tree, "spTree")
	if spTree == nil {
		return ""
	}
	var title, subtitle string
	s.shapes(spTree, &title, &subtitle)

	var out strings.Builder
	if title != "" {
		out.WriteString("### " + title + "\n\n")
	}
	if subtitle != "" {
		out.WriteString(subtitle + "\n\n")
	}
	out.WriteString(s.out.String())
	if notes := s.notes(); notes != "" {
		out.WriteString("> **Notes:** " + strings.ReplaceAll(notes, "\n", "\n> ") + "\n\n")
	}
	return out.String()
}

func (s *pptxSlide) shapes(container *node, title, subtitle *string) {
	for _, shape := range container.Children {
		switch shape.Local {
		case "sp":
			s.textShape(shape, title, subtitle)
		case "grpSp":
			s.shapes(shape, title, subtitle)
		case "AlternateContent":
			if choice := shape.child("Choice"); choice != nil {
				s.shapes(choice, title, subtitle)
			}
		case "pic":
			if alt := pictureAlt(shape); alt != "" {
				s.out.WriteString("[Image: " + alt + "]\n\n")
			}
		case "graphicFrame":
			s.graphicFrame(shape)
		}
	}
}

// textShape writes a shape's paragraphs. Title and subtitle placeholders are returned to the caller to put at
// the top; slide numbers, dates and footers are not content and are left out.
func (s *pptxSlide) textShape(sp *node, title, subtitle *string) {
	kind, plain := placeholderKind(sp)
	switch kind {
	case "sldNum", "dt", "ftr", "hdr":
		return
	}
	txBody := sp.child("txBody")
	if txBody == nil {
		return
	}
	paragraphs := s.paragraphs(txBody)
	switch kind {
	case "title", "ctrTitle":
		if text := strings.Join(oneLines(paragraphs), " "); text != "" && *title == "" {
			*title = text
			return
		}
	case "subTitle":
		if text := strings.Join(oneLines(paragraphs), " "); text != "" && *subtitle == "" {
			*subtitle = text
			return
		}
	}

	s.list.reset()
	wrote := false
	for _, p := range paragraphs {
		if p.text == "" {
			continue
		}
		bullet := (!plain && !p.noBullet) || p.bullet != ""
		switch {
		case p.numbered:
			label := formatBulletNumber(p.numberType, s.list.next(p.level)+p.numberStart-1)
			s.out.WriteString(strings.Repeat("  ", p.level) + label + " " + p.text + "\n")
		case bullet:
			s.list.next(p.level)
			s.out.WriteString(strings.Repeat("  ", p.level) + "- " + p.text + "\n")
		default:
			if wrote {
				s.out.WriteString("\n")
			}
			s.out.WriteString(p.text + "\n")
		}
		wrote = true
	}
	if wrote {
		s.out.WriteString("\n")
	}
}

// placeholderKind returns the placeholder type of a shape ("title", "body", "sldNum"...), and whether the shape is
// a plain text box, whose paragraphs are text rather than bullets unless they say otherwise.
func placeholderKind(sp *node) (kind string, plainBox bool) {
	if nv := sp.child("nvSpPr"); nv != nil {
		if c := nv.child("cNvSpPr"); c != nil && c.attr("txBox") == "1" {
			plainBox = true
		}
		if nvPr := nv.child("nvPr"); nvPr != nil {
			if ph := nvPr.child("ph"); ph != nil {
				return ph.attr("type"), false
			}
		}
	}
	return "", plainBox
}

type pptxParagraph struct {
	text        string
	level       int
	noBullet    bool
	bullet      string // an explicit bullet character
	numbered    bool
	numberType  string
	numberStart int
}

func (s *pptxSlide) paragraphs(txBody *node) []pptxParagraph {
	var out []pptxParagraph
	for _, p := range txBody.childrenNamed("p") {
		para := pptxParagraph{numberStart: 1}
		if pPr := p.child("pPr"); pPr != nil {
			para.level, _ = strconv.Atoi(pPr.attr("lvl"))
			para.noBullet = pPr.child("buNone") != nil
			if b := pPr.child("buChar"); b != nil {
				para.bullet = b.attr("char")
			}
			if n := pPr.child("buAutoNum"); n != nil {
				para.numbered, para.numberType = true, n.attr("type")
				if v, err := strconv.Atoi(n.attr("startAt")); err == nil {
					para.numberStart = v
				}
			}
		}
		para.text = s.paragraphText(p)
		out = append(out, para)
	}
	return out
}

// paragraphText joins the runs of a paragraph; a run with a hyperlink becomes a markdown link.
func (s *pptxSlide) paragraphText(p *node) string {
	var sb strings.Builder
	for _, c := range p.Children {
		switch c.Local {
		case "r", "fld":
			text := ""
			if t := c.child("t"); t != nil {
				text = t.Text
			}
			if rPr := c.child("rPr"); rPr != nil && text != "" {
				if link := rPr.child("hlinkClick"); link != nil {
					if rel, ok := s.rels[link.nsAttr("id")]; ok && rel.web && strings.TrimSpace(text) != "" {
						text = "[" + strings.TrimSpace(text) + "](" + rel.target + ")"
					}
				}
			}
			sb.WriteString(text)
		case "br":
			sb.WriteByte(' ')
		}
	}
	return strings.TrimSpace(sb.String())
}

func oneLines(paragraphs []pptxParagraph) []string {
	var out []string
	for _, p := range paragraphs {
		if p.text != "" {
			out = append(out, p.text)
		}
	}
	return out
}

func formatBulletNumber(kind string, n int) string {
	suffix := "."
	if strings.Contains(kind, "ParenR") {
		suffix = ")"
	} else if strings.Contains(kind, "ParenBoth") {
		return "(" + formatCount(bulletFormat(kind), n) + ")"
	}
	return formatCount(bulletFormat(kind), n) + suffix
}

func bulletFormat(kind string) string {
	switch {
	case strings.HasPrefix(kind, "alphaLc"):
		return "lowerLetter"
	case strings.HasPrefix(kind, "alphaUc"):
		return "upperLetter"
	case strings.HasPrefix(kind, "romanLc"):
		return "lowerRoman"
	case strings.HasPrefix(kind, "romanUc"):
		return "upperRoman"
	}
	return "decimal"
}

func pictureAlt(pic *node) string {
	if nv := pic.child("nvPicPr"); nv != nil {
		if c := nv.child("cNvPr"); c != nil {
			alt := strings.TrimSpace(c.attr("descr"))
			if alt == "" {
				alt = strings.TrimSpace(c.attr("title"))
			}
			return oneLine(alt)
		}
	}
	return ""
}

// graphicFrame writes what a frame holds: a table, a chart (as a table of its data) or a SmartArt diagram.
func (s *pptxSlide) graphicFrame(frame *node) {
	if tbl := findFirst(frame, "tbl"); tbl != nil {
		s.pptxTable(tbl).RenderMarkdown(&s.out)
		return
	}
	if chart := findFirst(frame, "chart"); chart != nil {
		if rel, ok := s.rels[chart.nsAttr("id")]; ok {
			if tree := readPart(s.zr, rel.target); tree != nil {
				chartTable(tree).RenderMarkdown(&s.out)
			}
		}
		return
	}
	if ids := findFirst(frame, "relIds"); ids != nil {
		if rel, ok := s.rels[ids.nsAttr("dm")]; ok {
			if tree := readPart(s.zr, rel.target); tree != nil {
				for _, line := range smartArtLines(tree) {
					s.out.WriteString("- " + line + "\n")
				}
				s.out.WriteString("\n")
			}
		}
	}
}

// pptxTable reads an a:tbl. DrawingML writes a merged cell once, with gridSpan/rowSpan, and leaves a placeholder
// (hMerge/vMerge) where the cell continues; the placeholders take the text of the cell they continue.
func (s *pptxSlide) pptxTable(tbl *node) mdtable.Table {
	t := mdtable.Table{}
	if pr := tbl.child("tblPr"); pr != nil && pr.attr("firstRow") == "1" {
		t.HeaderRows = 1
	}
	for _, tr := range tbl.childrenNamed("tr") {
		var row []string
		for _, tc := range tr.childrenNamed("tc") {
			text := ""
			if body := tc.child("txBody"); body != nil {
				var lines []string
				for _, p := range body.childrenNamed("p") {
					if line := s.paragraphText(p); line != "" {
						lines = append(lines, line)
					}
				}
				text = strings.Join(lines, "\n")
			}
			switch {
			case tc.attr("hMerge") == "1" && len(row) > 0:
				text = row[len(row)-1]
			case tc.attr("vMerge") == "1" && len(t.Rows) > 0 && len(row) < len(t.Rows[len(t.Rows)-1]):
				text = t.Rows[len(t.Rows)-1][len(row)]
			}
			row = append(row, text)
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

// notes returns the speaker notes of the slide.
func (s *pptxSlide) notes() string {
	for _, rel := range s.rels {
		if rel.kind != "notesSlide" {
			continue
		}
		tree := readPart(s.zr, rel.target)
		if tree == nil {
			return ""
		}
		var lines []string
		for _, sp := range findAll(tree, "sp") {
			if kind, _ := placeholderKind(sp); kind != "body" {
				continue
			}
			if body := sp.child("txBody"); body != nil {
				lines = append(lines, oneLines(s.paragraphs(body))...)
			}
		}
		return strings.Join(lines, "\n")
	}
	return ""
}
