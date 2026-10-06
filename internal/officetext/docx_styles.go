package officetext

import (
	"archive/zip"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// wordStyle is what a paragraph style says about a paragraph that matters for text: how deep a heading it is,
// whether it is a list item, and whether it is code or a quotation. Word writes none of this on the paragraph
// itself when a style carries it, so it is read from styles.xml and followed along the style's basedOn chain.
type wordStyle struct {
	name      string
	basedOn   string
	outline   int // outline level, 0 for none, 1..9 for a heading
	hasOutln  bool
	numID     string
	numLevel  int
	hasNumPr  bool
	monospace bool
}

type wordStyles map[string]*wordStyle

var (
	headingNameRe = regexp.MustCompile(`(?i)^(?:heading|titre|überschrift|ueberschrift|título|titulo|intestazione|kop|rubrik|otsikko)\s*(\d)$`)
	monoFontRe    = regexp.MustCompile(`(?i)(consolas|courier|monaco|menlo|lucida console|source code|fira (code|mono)|dejavu sans mono|liberation mono|cascadia)`)
)

func loadWordStyles(zr *zip.Reader) wordStyles {
	styles := wordStyles{}
	tree := readPart(zr, "word/styles.xml")
	if tree == nil {
		return styles
	}
	for _, s := range findAll(tree, "style") {
		if s.attr("type") != "paragraph" && s.attr("type") != "" {
			continue
		}
		style := &wordStyle{}
		if n := s.child("name"); n != nil {
			style.name = n.attr("val")
		}
		if b := s.child("basedOn"); b != nil {
			style.basedOn = b.attr("val")
		}
		if pPr := s.child("pPr"); pPr != nil {
			if o := pPr.child("outlineLvl"); o != nil {
				if level, err := strconv.Atoi(o.attr("val")); err == nil {
					style.outline, style.hasOutln = level+1, true
				}
			}
			if numPr := pPr.child("numPr"); numPr != nil {
				style.numID, style.numLevel, style.hasNumPr = numPrOf(numPr)
			}
		}
		if rPr := s.child("rPr"); rPr != nil {
			style.monospace = runIsMonospace(rPr)
		}
		styles[s.attr("styleId")] = style
	}
	return styles
}

// chain returns the style and the styles it is based on, nearest first. A cycle in a damaged file ends it.
func (s wordStyles) chain(id string) []*wordStyle {
	var out []*wordStyle
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		style, ok := s[id]
		if !ok {
			break
		}
		out = append(out, style)
		id = style.basedOn
	}
	return out
}

// headingLevel is 1..6 for a heading paragraph and 0 for body text. The outline level a style declares is what
// Word itself uses to build its navigation pane, and it is the same in every language, which the style's name
// ("Heading 1", "Titre 1", "Überschrift 1") is not; the name is only a fallback for files without styles.xml.
func (s wordStyles) headingLevel(styleID string, directOutline int) int {
	if directOutline > 0 && directOutline <= 9 {
		return min(directOutline, 6)
	}
	for _, style := range s.chain(styleID) {
		if style.hasOutln {
			if style.outline >= 1 && style.outline <= 9 {
				return min(style.outline, 6)
			}
			return 0
		}
	}
	if s.isTitle(styleID) {
		return 1
	}
	for _, name := range append(s.names(styleID), styleID) {
		if m := headingNameRe.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
			level, _ := strconv.Atoi(m[1])
			return min(level, 6)
		}
	}
	return 0
}

func (s wordStyles) names(id string) []string {
	var names []string
	for _, style := range s.chain(id) {
		names = append(names, style.name)
	}
	return names
}

// numbering returns the list a style puts its paragraphs in, if any. Word's built-in nested list styles
// ("List Bullet 2", "List Number 3") carry their depth in the name rather than in the list level.
func (s wordStyles) numbering(styleID string) (numID string, level int, ok bool) {
	for _, style := range s.chain(styleID) {
		if style.hasNumPr {
			level = style.numLevel
			if depth := listStyleDepth(s.names(styleID)); level == 0 && depth > 1 {
				level = depth - 1
			}
			return style.numID, level, true
		}
	}
	return "", 0, false
}

var listStyleRe = regexp.MustCompile(`(?i)^list\s+(?:bullet|number|continue)?\s*(\d)$`)

func listStyleDepth(names []string) int {
	for _, name := range names {
		if m := listStyleRe.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
			depth, _ := strconv.Atoi(m[1])
			return depth
		}
	}
	return 0
}

// isTitle is true for the document title style, which sits above the first-level headings.
func (s wordStyles) isTitle(styleID string) bool {
	for _, name := range append(s.names(styleID), styleID) {
		if strings.EqualFold(name, "title") || strings.EqualFold(name, "titre") {
			return true
		}
	}
	return false
}

func (s wordStyles) isCode(styleID string) bool {
	for _, style := range s.chain(styleID) {
		if style.monospace {
			return true
		}
		lower := strings.ToLower(style.name)
		if strings.Contains(lower, "source code") || strings.Contains(lower, "preformatted") || lower == "code" || lower == "verbatim" || strings.HasPrefix(lower, "html code") {
			return true
		}
	}
	return false
}

func (s wordStyles) isQuote(styleID string) bool {
	for _, name := range s.names(styleID) {
		switch strings.ToLower(name) {
		case "quote", "intense quote", "block text", "blockquote", "citation", "citation intense":
			return true
		}
	}
	return false
}

func numPrOf(numPr *node) (numID string, level int, ok bool) {
	if id := numPr.child("numId"); id != nil {
		numID = id.attr("val")
	}
	if l := numPr.child("ilvl"); l != nil {
		level, _ = strconv.Atoi(l.attr("val"))
	}
	return numID, level, numID != ""
}

func runIsMonospace(rPr *node) bool {
	fonts := rPr.child("rFonts")
	if fonts == nil {
		return false
	}
	for _, key := range []string{"ascii", "hAnsi", "cs"} {
		if v := fonts.attr(key); v != "" && monoFontRe.MatchString(v) {
			return true
		}
	}
	return false
}

// numberLevel is one level of a list definition: how its items are numbered and how the number is written.
type numberLevel struct {
	format string // decimal, lowerLetter, upperRoman, bullet, none...
	text   string // the pattern, "%1.%2" or "(%3)"
	start  int
}

// wordNumbering holds the list definitions of numbering.xml and counts the items of each list as the document
// is read, because the number in front of a clause ("3.2") is not in the text of the paragraph: Word writes it
// from the list.
type wordNumbering struct {
	nums     map[string]string              // numId -> abstractNumId
	abstract map[string]map[int]numberLevel // abstractNumId -> level -> definition
	override map[string]map[int]numberLevel // numId -> level -> overriding definition (start values)
	counters map[string]map[int]int         // numId -> level -> current count
}

func loadWordNumbering(zr *zip.Reader) *wordNumbering {
	n := &wordNumbering{
		nums:     map[string]string{},
		abstract: map[string]map[int]numberLevel{},
		override: map[string]map[int]numberLevel{},
		counters: map[string]map[int]int{},
	}
	tree := readPart(zr, "word/numbering.xml")
	if tree == nil {
		return n
	}
	for _, a := range findAll(tree, "abstractNum") {
		levels := map[int]numberLevel{}
		for _, lvl := range a.childrenNamed("lvl") {
			index, _ := strconv.Atoi(lvl.attr("ilvl"))
			levels[index] = levelOf(lvl)
		}
		n.abstract[a.attr("abstractNumId")] = levels
	}
	for _, num := range findAll(tree, "num") {
		id := num.attr("numId")
		if a := num.child("abstractNumId"); a != nil {
			n.nums[id] = a.attr("val")
		}
		for _, o := range num.childrenNamed("lvlOverride") {
			index, _ := strconv.Atoi(o.attr("ilvl"))
			level := n.abstract[n.nums[id]][index]
			if s := o.child("startOverride"); s != nil {
				level.start, _ = strconv.Atoi(s.attr("val"))
			}
			if lvl := o.child("lvl"); lvl != nil {
				level = levelOf(lvl)
			}
			if n.override[id] == nil {
				n.override[id] = map[int]numberLevel{}
			}
			n.override[id][index] = level
		}
	}
	return n
}

func levelOf(lvl *node) numberLevel {
	out := numberLevel{start: 1}
	if f := lvl.child("numFmt"); f != nil {
		out.format = f.attr("val")
	}
	if t := lvl.child("lvlText"); t != nil {
		out.text = t.attr("val")
	}
	if s := lvl.child("start"); s != nil {
		if v, err := strconv.Atoi(s.attr("val")); err == nil {
			out.start = v
		}
	}
	return out
}

func (n *wordNumbering) level(numID string, level int) numberLevel {
	if o, ok := n.override[numID][level]; ok {
		return o
	}
	return n.abstract[n.nums[numID]][level]
}

// next counts one more item at this level of the list (restarting the levels below it, as Word does) and
// returns how the item is labelled and what kind of list it is in. A numId of "0" is Word's way to say that a
// paragraph is not in a list.
func (n *wordNumbering) next(numID string, level int) (label string, bullet bool, listed bool) {
	if numID == "" || numID == "0" {
		return "", false, false
	}
	def := n.level(numID, level)
	if def.format == "" && def.text == "" {
		// A list the file does not define: still a list, written as bullets.
		return "", true, true
	}
	counters := n.counters[numID]
	if counters == nil {
		counters = map[int]int{}
		n.counters[numID] = counters
	}
	if _, started := counters[level]; started {
		counters[level]++
	} else {
		counters[level] = def.start
	}
	for deeper := range counters {
		if deeper > level {
			delete(counters, deeper)
		}
	}
	switch def.format {
	case "bullet":
		return "", true, true
	case "none":
		return "", false, true
	}
	return n.label(numID, level, def), false, true
}

// label writes a level's pattern with the counters of the levels it names: "%1.%2" at 3.2 is "3.2".
func (n *wordNumbering) label(numID string, level int, def numberLevel) string {
	pattern := def.text
	if pattern == "" {
		pattern = fmt.Sprintf("%%%d.", level+1)
	}
	for l := 0; l <= level; l++ {
		marker := fmt.Sprintf("%%%d", l+1)
		if !strings.Contains(pattern, marker) {
			continue
		}
		count := n.counters[numID][l]
		if count == 0 {
			count = n.level(numID, l).start
		}
		pattern = strings.ReplaceAll(pattern, marker, formatCount(n.level(numID, l).format, count))
	}
	return strings.TrimSpace(pattern)
}

func formatCount(format string, count int) string {
	switch format {
	case "lowerLetter":
		return letters(count, 'a')
	case "upperLetter":
		return letters(count, 'A')
	case "lowerRoman":
		return strings.ToLower(roman(count))
	case "upperRoman":
		return roman(count)
	case "decimalZero":
		return fmt.Sprintf("%02d", count)
	default:
		return strconv.Itoa(count)
	}
}

// letters writes 1 as a, 26 as z, 27 as aa.
func letters(count int, first rune) string {
	if count < 1 {
		return strconv.Itoa(count)
	}
	var out []rune
	for count > 0 {
		count--
		out = append([]rune{first + rune(count%26)}, out...)
		count /= 26
	}
	return string(out)
}

func roman(count int) string {
	if count < 1 || count > 3999 {
		return strconv.Itoa(count)
	}
	values := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var sb strings.Builder
	for i, v := range values {
		for count >= v {
			sb.WriteString(symbols[i])
			count -= v
		}
	}
	return sb.String()
}

// readPart parses one part of the package, or returns nil when it is not there or not XML.
func readPart(zr *zip.Reader, name string) *node {
	f := findZipFile(zr, name)
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	tree, err := parseXMLTree(rc)
	if err != nil {
		return nil
	}
	return tree
}
