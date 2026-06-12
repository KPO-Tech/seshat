package dialog

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/list"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/workspace"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// ─── toolGroup ─────────────────────────────────────────────────────────────

type toolGroup struct {
	*list.Versioned
	category string
	items    []*toolItem
	t        *styles.Styles
}

func (g *toolGroup) Finished() bool { return true }

func (g *toolGroup) Render(width int) string {
	title := ansi.Truncate(" "+g.category+" ", max(0, width-1), "…")
	return common.Section(g.t, title, width)
}

// ─── toolItem ──────────────────────────────────────────────────────────────

type toolItem struct {
	*list.Versioned
	name    string
	desc    string // kept for fuzzy filtering only
	focused bool
	m       fuzzy.Match
	t       *styles.Styles
	cache   map[int]string
}

func (i *toolItem) Finished() bool { return true }

func (i *toolItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.cache = nil
	i.Bump()
}

func (i *toolItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(i.m, m) {
		return
	}
	i.m = m
	i.cache = nil
	i.Bump()
}

func (i *toolItem) Render(width int) string {
	if cached, ok := i.cache[width]; ok {
		return cached
	}

	t := i.t
	style := t.Dialog.NormalItem
	if i.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
	}

	const prefix = "    " // 4-space indent under group header
	const prefixW = 4

	name := ansi.Truncate(i.name, max(0, width-prefixW), "…")
	nameWidth := ansi.StringWidth(name)

	// Apply fuzzy match underline highlighting.
	if len(i.m.MatchedIndexes) > 0 {
		var lastPos int
		var parts []string
		for _, rng := range matchedRanges(i.m.MatchedIndexes) {
			start, stop := bytePosToVisibleCharPos(name, rng)
			if start > lastPos {
				parts = append(parts, ansi.Cut(name, lastPos, start))
			}
			parts = append(parts,
				ansi.NewStyle().Underline(true).String(),
				ansi.Cut(name, start, stop+1),
				ansi.NewStyle().Underline(false).String(),
			)
			lastPos = stop + 1
		}
		if lastPos < ansi.StringWidth(name) {
			parts = append(parts, ansi.Cut(name, lastPos, ansi.StringWidth(name)))
		}
		name = strings.Join(parts, "")
	}

	gap := strings.Repeat(" ", max(0, width-prefixW-nameWidth))
	result := style.Render(prefix + name + gap)
	if i.cache == nil {
		i.cache = make(map[int]string)
	}
	i.cache[width] = result
	return result
}

// ─── ToolsList ─────────────────────────────────────────────────────────────

// ToolsList wraps list.List with grouped category/tool display and fuzzy filtering.
type ToolsList struct {
	*list.List
	groups []toolGroup
	t      *styles.Styles
}

func newToolsList(t *styles.Styles) *ToolsList {
	l := &ToolsList{
		List: list.NewList(),
		t:    t,
	}
	l.RegisterRenderCallback(list.FocusedRenderCallback(l.List))
	return l
}

// SetGroups replaces the tool groups and syncs the underlying list items.
func (l *ToolsList) SetGroups(groups ...toolGroup) {
	l.groups = groups
	l.syncItems("")
}

// SetFilter applies fuzzy filtering across all tool names/descs.
func (l *ToolsList) SetFilter(query string) {
	l.syncItems(query)
}

func (l *ToolsList) syncItems(query string) {
	q := strings.ToLower(strings.ReplaceAll(query, " ", ""))

	if q == "" {
		items := make([]list.Item, 0)
		for gi := range l.groups {
			g := &l.groups[gi]
			items = append(items, g)
			for _, item := range g.items {
				item.SetMatch(fuzzy.Match{})
				items = append(items, item)
			}
			items = append(items, list.NewSpacerItem(1))
		}
		l.List.SetItems(items...)
		return
	}

	// Build a flat slice for fuzzy matching.
	flat := make([]*toolItem, 0, l.totalItems())
	for gi := range l.groups {
		flat = append(flat, l.groups[gi].items...)
	}
	filterStrs := make([]string, len(flat))
	for idx, it := range flat {
		filterStrs[idx] = it.name + " " + it.desc
	}

	matches := fuzzy.Find(query, filterStrs)
	matchByIdx := make(map[int]fuzzy.Match, len(matches))
	for _, m := range matches {
		matchByIdx[m.Index] = m
	}

	items := make([]list.Item, 0)
	flatIdx := 0
	for gi := range l.groups {
		g := &l.groups[gi]
		var groupItems []list.Item
		for _, item := range g.items {
			if m, ok := matchByIdx[flatIdx]; ok {
				item.SetMatch(m)
				groupItems = append(groupItems, item)
			} else {
				item.SetMatch(fuzzy.Match{})
			}
			flatIdx++
		}
		if len(groupItems) > 0 {
			items = append(items, g)
			items = append(items, groupItems...)
			items = append(items, list.NewSpacerItem(1))
		}
	}
	l.List.SetItems(items...)
}

func (l *ToolsList) totalItems() int {
	n := 0
	for _, g := range l.groups {
		n += len(g.items)
	}
	return n
}

// SelectNext skips non-toolItem entries (group headers, spacers).
func (l *ToolsList) SelectNext() bool {
	v := l.List.SelectNext()
	for v {
		if _, ok := l.SelectedItem().(*toolItem); ok {
			return v
		}
		v = l.List.SelectNext()
	}
	return v
}

// SelectPrev skips non-toolItem entries.
func (l *ToolsList) SelectPrev() bool {
	v := l.List.SelectPrev()
	for v {
		if _, ok := l.SelectedItem().(*toolItem); ok {
			return v
		}
		v = l.List.SelectPrev()
	}
	return v
}

// SelectFirst selects the first toolItem.
func (l *ToolsList) SelectFirst() bool {
	v := l.List.SelectFirst()
	for v {
		if _, ok := l.SelectedItem().(*toolItem); ok {
			return v
		}
		v = l.List.SelectNext()
	}
	return v
}

// SelectLast selects the last toolItem.
func (l *ToolsList) SelectLast() bool {
	v := l.List.SelectLast()
	for v {
		if _, ok := l.SelectedItem().(*toolItem); ok {
			return v
		}
		v = l.List.SelectPrev()
	}
	return v
}

// IsSelectedFirst reports whether the selection is on the first toolItem.
func (l *ToolsList) IsSelectedFirst() bool {
	orig := l.Selected()
	l.SelectFirst()
	isFirst := l.Selected() == orig
	l.List.SetSelected(orig)
	return isFirst
}

// IsSelectedLast reports whether the selection is on the last toolItem.
func (l *ToolsList) IsSelectedLast() bool {
	orig := l.Selected()
	l.SelectLast()
	isLast := l.Selected() == orig
	l.List.SetSelected(orig)
	return isLast
}

// buildToolGroups converts a flat ToolInfo slice into sorted toolGroups.
func buildToolGroups(t *styles.Styles, tools []workspace.ToolInfo) []toolGroup {
	byCategory := make(map[string][]*toolItem)
	order := make([]string, 0)
	for _, tool := range tools {
		cat := tool.Category
		if cat == "" {
			cat = "general"
		}
		if _, exists := byCategory[cat]; !exists {
			order = append(order, cat)
		}
		byCategory[cat] = append(byCategory[cat], &toolItem{
			Versioned: list.NewVersioned(),
			name:      tool.Name,
			desc:      tool.Description,
			t:         t,
			cache:     make(map[int]string),
		})
	}
	sort.Strings(order)
	groups := make([]toolGroup, 0, len(order))
	for _, cat := range order {
		items := byCategory[cat]
		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
		groups = append(groups, toolGroup{
			Versioned: list.NewVersioned(),
			category:  cat,
			items:     items,
			t:         t,
		})
	}
	return groups
}
