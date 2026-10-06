package officetext

import (
	"sort"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat/internal/mdtable"
)

// chartTable turns a chart part (ppt/charts/chartN.xml, which Word and Excel use too) into the table of numbers it
// was drawn from: one row per category, one column per series, and the chart's title as the caption. The picture
// of a chart says nothing to a text reader, but the numbers behind it are stored in the file.
func chartTable(chart *node) mdtable.Table {
	t := mdtable.Table{Caption: chartTitle(chart), HeaderRows: 1}
	plot := findFirst(chart, "plotArea")
	if plot == nil {
		return t
	}
	var series []*node
	for _, kind := range plot.Children {
		series = append(series, kind.childrenNamed("ser")...)
	}
	if len(series) == 0 {
		return t
	}

	type column struct {
		name   string
		values []string
	}
	var categories []string
	var columns []column
	for i, ser := range series {
		name := firstValue(ser.child("tx"))
		if name == "" {
			name = "Series " + strconv.Itoa(i+1)
		}
		cats := cachePoints(firstOf(ser, "cat", "xVal"))
		if len(cats) > len(categories) {
			categories = cats
		}
		columns = append(columns, column{name: name, values: cachePoints(firstOf(ser, "val", "yVal"))})
	}

	header := []string{"Category"}
	for _, c := range columns {
		header = append(header, c.name)
	}
	t.Rows = append(t.Rows, header)
	rows := len(categories)
	for _, c := range columns {
		rows = max(rows, len(c.values))
	}
	for r := 0; r < rows; r++ {
		row := make([]string, 0, len(columns)+1)
		label := ""
		if r < len(categories) {
			label = categories[r]
		}
		row = append(row, label)
		for _, c := range columns {
			value := ""
			if r < len(c.values) {
				value = c.values[r]
			}
			row = append(row, value)
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}

func chartTitle(chart *node) string {
	title := chart.child("chartSpace")
	if title == nil {
		title = chart
	}
	c := findFirst(title, "chart")
	if c == nil {
		return ""
	}
	titleNode := c.child("title")
	if titleNode == nil {
		return ""
	}
	var parts []string
	for _, p := range findAll(titleNode, "p") {
		var sb strings.Builder
		for _, r := range p.childrenNamed("r") {
			if t := r.child("t"); t != nil {
				sb.WriteString(t.Text)
			}
		}
		if text := strings.TrimSpace(sb.String()); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Chart: " + strings.Join(parts, " ")
}

func firstOf(n *node, names ...string) *node {
	for _, name := range names {
		if c := n.child(name); c != nil {
			return c
		}
	}
	return nil
}

func firstValue(n *node) string {
	if n == nil {
		return ""
	}
	if v := findFirst(n, "v"); v != nil {
		return strings.TrimSpace(v.Text)
	}
	return ""
}

// cachePoints reads the cached values of a series part (c:cat, c:val...) in order, leaving "" where the file has
// a gap. Numbers are written without the noise of floating point ("0.30000000000000004").
func cachePoints(n *node) []string {
	if n == nil {
		return nil
	}
	cache := findFirst(n, "strCache")
	if cache == nil {
		cache = findFirst(n, "numCache")
	}
	if cache == nil {
		return nil
	}
	count := 0
	if pc := cache.child("ptCount"); pc != nil {
		count, _ = strconv.Atoi(pc.attr("val"))
	}
	points := map[int]string{}
	for _, pt := range cache.childrenNamed("pt") {
		idx, err := strconv.Atoi(pt.attr("idx"))
		if err != nil {
			continue
		}
		if v := pt.child("v"); v != nil {
			points[idx] = tidyNumber(strings.TrimSpace(v.Text))
		}
		count = max(count, idx+1)
	}
	out := make([]string, count)
	for idx, value := range points {
		if idx >= 0 && idx < count {
			out[idx] = value
		}
	}
	return out
}

func tidyNumber(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'g', 10, 64)
}

// smartArtLines lists the text of the nodes of a SmartArt diagram (its data part), in the order the diagram
// stores them. The drawing is lost, but the words of a process or a hierarchy are not.
func smartArtLines(data *node) []string {
	type item struct {
		order int
		text  string
	}
	var items []item
	for i, pt := range findAll(data, "pt") {
		if kind := pt.attr("type"); kind != "" && kind != "node" {
			continue
		}
		t := pt.child("t")
		if t == nil {
			continue
		}
		var parts []string
		for _, p := range t.childrenNamed("p") {
			var sb strings.Builder
			for _, r := range p.childrenNamed("r") {
				if rt := r.child("t"); rt != nil {
					sb.WriteString(rt.Text)
				}
			}
			if text := strings.TrimSpace(sb.String()); text != "" {
				parts = append(parts, text)
			}
		}
		if text := strings.Join(parts, " "); text != "" {
			items = append(items, item{i, text})
		}
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].order < items[b].order })
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = it.text
	}
	return lines
}
