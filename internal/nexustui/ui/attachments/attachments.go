package attachments

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/charmbracelet/x/ansi"
)

const maxFilename = 15

type Keymap struct {
	DeleteMode,
	DeleteAll,
	Escape key.Binding
}

func New(renderer *Renderer, keyMap Keymap) *Attachments {
	return &Attachments{
		keyMap:   keyMap,
		renderer: renderer,
	}
}

type Attachments struct {
	renderer *Renderer
	keyMap   Keymap
	list     []message.Attachment
	deleting bool
}

func (m *Attachments) List() []message.Attachment { return m.list }
func (m *Attachments) Reset()                     { m.list = nil }

func (m *Attachments) HandleClick(x, y, width int) bool {
	if idx, ok := m.renderer.DeleteHit(m.list, m.deleting, width, x, y); ok {
		m.list = slices.Delete(m.list, idx, idx+1)
		m.deleting = false
		return true
	}
	return false
}

func (m *Attachments) Update(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case message.Attachment:
		m.list = append(m.list, msg)
		return true
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.DeleteMode):
			if len(m.list) > 0 {
				m.deleting = true
			}
			return true
		case m.deleting && key.Matches(msg, m.keyMap.Escape):
			m.deleting = false
			return true
		case m.deleting && key.Matches(msg, m.keyMap.DeleteAll):
			m.deleting = false
			m.list = nil
			return true
		case m.deleting:
			// Handle digit keys for individual attachment deletion.
			r := msg.Code
			if r >= '0' && r <= '9' {
				num := int(r - '0')
				if num < len(m.list) {
					m.list = slices.Delete(m.list, num, num+1)
				}
				m.deleting = false
			}
			return true
		}
	}
	return false
}

func (m *Attachments) Render(width int) string {
	return m.renderer.RenderComposer(m.list, m.deleting, width)
}

// Renderer returns the attachment renderer so callers can update its
// styles in place.
func (m *Attachments) Renderer() *Renderer { return m.renderer }

func NewRenderer(normalStyle, deletingStyle, imageStyle, textStyle, skillStyle lipgloss.Style) *Renderer {
	return &Renderer{
		normalStyle:   normalStyle,
		textStyle:     textStyle,
		imageStyle:    imageStyle,
		skillStyle:    skillStyle,
		deletingStyle: deletingStyle,
	}
}

// SetStyles updates the renderer styles in place.
func (r *Renderer) SetStyles(normalStyle, deletingStyle, imageStyle, textStyle, skillStyle lipgloss.Style) {
	r.normalStyle = normalStyle
	r.textStyle = textStyle
	r.imageStyle = imageStyle
	r.skillStyle = skillStyle
	r.deletingStyle = deletingStyle
}

type Renderer struct {
	normalStyle, textStyle, imageStyle, skillStyle, deletingStyle lipgloss.Style
	deleteHits                                                    []deleteHit
}

type deleteHit struct {
	index  int
	startX int
	endX   int
}

func (r *Renderer) Render(attachments []message.Attachment, deleting bool, width int) string {
	rendered, _ := r.renderLayout(attachments, deleting, false, width)
	return rendered
}

func (r *Renderer) RenderComposer(attachments []message.Attachment, deleting bool, width int) string {
	rendered, hits := r.renderLayout(attachments, deleting, true, width)
	r.deleteHits = hits
	return rendered
}

func (r *Renderer) DeleteHit(attachments []message.Attachment, deleting bool, width, x, y int) (int, bool) {
	if y != 0 || deleting {
		return 0, false
	}
	_, hits := r.renderLayout(attachments, deleting, true, width)
	for _, hit := range hits {
		if x >= hit.startX && x < hit.endX {
			return hit.index, true
		}
	}
	return 0, false
}

func (r *Renderer) renderLayout(attachments []message.Attachment, deleting, showRemove bool, width int) (string, []deleteHit) {
	var (
		chips []string
		hits  []deleteHit
		curX  int
	)

	nameStyle := r.normalStyle
	maxItemWidth := lipgloss.Width(r.imageStyle.String() + nameStyle.Render(strings.Repeat("x", maxFilename)))
	if showRemove && !deleting {
		nameStyle = nameStyle.Copy().MarginRight(0)
		maxItemWidth = lipgloss.Width(r.imageStyle.String()+nameStyle.Render(strings.Repeat("x", maxFilename))) + lipgloss.Width(r.deletingStyle.Render("×")) + 1
	}
	fits := int(math.Floor(float64(width)/float64(maxItemWidth))) - 1

	for i, att := range attachments {
		filename := filepath.Base(att.FileName)
		if ansi.StringWidth(filename) > maxFilename {
			filename = ansi.Truncate(filename, maxFilename, "…")
		}

		if deleting {
			for _, chip := range []string{
				r.deletingStyle.Render(fmt.Sprintf("%d", i)),
				nameStyle.Render(filename),
			} {
				chips = append(chips, chip)
				curX += lipgloss.Width(chip)
			}
		} else {
			for _, chip := range []string{r.icon(att).String(), nameStyle.Render(filename)} {
				chips = append(chips, chip)
				curX += lipgloss.Width(chip)
			}
			if showRemove {
				deleteChip := r.deletingStyle.Render("×")
				chips = append(chips, deleteChip)
				deleteWidth := lipgloss.Width(deleteChip)
				hits = append(hits, deleteHit{index: i, startX: curX, endX: curX + deleteWidth})
				curX += deleteWidth
				if i < len(attachments)-1 {
					chips = append(chips, " ")
					curX++
				}
			}
		}

		if i == fits && len(attachments) > i+1 {
			chips = append(chips, lipgloss.NewStyle().Width(maxItemWidth).Render(fmt.Sprintf("%d more…", len(attachments)-fits)))
			break
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, chips...), hits
}

func (r *Renderer) icon(a message.Attachment) lipgloss.Style {
	if a.IsImage() {
		return r.imageStyle
	}
	if a.IsMarkdown() {
		return r.skillStyle
	}
	return r.textStyle
}
