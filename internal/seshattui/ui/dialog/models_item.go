package dialog

import (
	"fmt"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat/internal/seshattui/config"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/common"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/list"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// ModelGroup represents a group of model items.
type ModelGroup struct {
	*list.Versioned
	Title      string
	Items      []*ModelItem
	configured bool
	t          *styles.Styles
}

// NewModelGroup creates a new ModelGroup.
func NewModelGroup(t *styles.Styles, title string, configured bool, items ...*ModelItem) ModelGroup {
	return ModelGroup{
		Versioned:  list.NewVersioned(),
		Title:      title,
		Items:      items,
		configured: configured,
		t:          t,
	}
}

// Finished implements list.Item. Model groups are immutable headers.
func (m *ModelGroup) Finished() bool {
	return true
}

// AppendItems appends [ModelItem]s to the group.
func (m *ModelGroup) AppendItems(items ...*ModelItem) {
	m.Items = append(m.Items, items...)
}

// Render implements [list.Item].
func (m *ModelGroup) Render(width int) string {
	var configured string
	if m.configured {
		configuredIcon := m.t.ToolCallSuccess.Render()
		configuredText := m.t.Dialog.Models.ConfiguredText.Render("Configured")
		configured = configuredIcon + " " + configuredText
	}

	title := m.Title
	if m.configured {
		greenOn := ansi.Style{}.ForegroundColor(m.t.ToolCallSuccess.GetForeground()).String()
		greenOff := ansi.Style{}.ForegroundColor(nil).String()
		title = greenOn + title + greenOff
	}
	title = " " + title + " "
	title = ansi.Truncate(title, max(0, width-lipgloss.Width(configured)-1), "…")

	return common.Section(m.t, title, width, configured)
}

// ModelItem represents a list item for a model type.
type ModelItem struct {
	*list.Versioned

	prov      catwalk.Provider
	model     catwalk.Model
	modelType ModelType

	cache        map[int]string
	t            *styles.Styles
	m            fuzzy.Match
	focused      bool
	showProvider bool
}

// Finished implements list.Item. Model items are render-stable
// outside of explicit SetFocused / SetMatch.
func (m *ModelItem) Finished() bool {
	return true
}

// SelectedModel returns this model item as a [config.SelectedModel] instance.
func (m *ModelItem) SelectedModel() config.SelectedModel {
	return config.SelectedModel{
		Model:           m.model.ID,
		Provider:        string(m.prov.ID),
		ReasoningEffort: m.model.DefaultReasoningEffort,
		MaxTokens:       m.model.DefaultMaxTokens,
	}
}

// SelectedModelType returns the type of model represented by this item.
func (m *ModelItem) SelectedModelType() config.SelectedModelType {
	return m.modelType.Config()
}

var _ ListItem = &ModelItem{}

// NewModelItem creates a new ModelItem.
func NewModelItem(t *styles.Styles, prov catwalk.Provider, model catwalk.Model, typ ModelType, showProvider bool) *ModelItem {
	return &ModelItem{
		Versioned:    list.NewVersioned(),
		prov:         prov,
		model:        model,
		modelType:    typ,
		t:            t,
		cache:        make(map[int]string),
		showProvider: showProvider,
	}
}

// Filter implements ListItem.
func (m *ModelItem) Filter() string {
	return m.model.Name
}

// ID implements ListItem.
func (m *ModelItem) ID() string {
	return modelKey(string(m.prov.ID), m.model.ID)
}

// Render implements ListItem.
func (m *ModelItem) Render(width int) string {
	if cached, ok := m.cache[width]; ok {
		return cached
	}

	t := m.t
	style := t.Dialog.NormalItem
	infoStyle := t.Dialog.ListItem.InfoBlurred
	if m.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
		infoStyle = t.Dialog.ListItem.InfoFocused
	}

	// Provider badge at far right (optional).
	var infoText string
	var infoWidth int
	if m.showProvider {
		infoText = infoStyle.Render(" " + string(m.prov.Name) + " ")
		infoWidth = lipgloss.Width(infoText)
	}

	// Context window in muted gray (left of provider badge).
	var ctxStr string
	ctxWidth := 0
	if m.model.ContextWindow > 0 {
		ctxText := fmtContextWindow(m.model.ContextWindow)
		greyColor := t.Sidebar.WorkingDir.GetForeground()
		greyOn := ansi.Style{}.ForegroundColor(greyColor).String()
		greyOff := ansi.Style{}.ForegroundColor(nil).String()
		ctxStr = " " + greyOn + ctxText + greyOff
		ctxWidth = 1 + ansi.StringWidth(ctxText)
	}

	const prefix = "    "
	const prefixW = 4

	// Model name truncated to available space.
	nameAvail := max(0, width-prefixW-ctxWidth-infoWidth)
	name := ansi.Truncate(m.model.Name, nameAvail, "…")
	nameWidth := ansi.StringWidth(name)

	// Apply fuzzy match underline.
	if len(m.m.MatchedIndexes) > 0 {
		var lastPos int
		var parts []string
		for _, rng := range matchedRanges(m.m.MatchedIndexes) {
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

	gap := strings.Repeat(" ", max(0, width-prefixW-nameWidth-ctxWidth-infoWidth))
	result := style.Render(prefix + name + gap + ctxStr + infoText)
	if m.cache == nil {
		m.cache = make(map[int]string)
	}
	m.cache[width] = result
	return result
}

// fmtContextWindow formats a context window size as a human-readable string.
func fmtContextWindow(n int64) string {
	switch {
	case n >= 1_000_000:
		v := float64(n) / 1_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dM", int64(v))
		}
		return fmt.Sprintf("%.1fM", v)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// SetFocused implements ListItem.
func (m *ModelItem) SetFocused(focused bool) {
	if m.focused == focused {
		return
	}
	m.cache = nil
	m.focused = focused
	if m.Versioned != nil {
		m.Bump()
	}
}

// SetMatch implements ListItem.
func (m *ModelItem) SetMatch(fm fuzzy.Match) {
	if sameFuzzyMatch(m.m, fm) {
		return
	}
	m.cache = nil
	m.m = fm
	if m.Versioned != nil {
		m.Bump()
	}
}
