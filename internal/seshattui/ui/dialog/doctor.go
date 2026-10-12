package dialog

import (
	"bytes"
	"context"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/common"
	"github.com/KPO-Tech/seshat-runtime/pkg/doctor"
	uv "github.com/charmbracelet/ultraviolet"
)

const DoctorID = "doctor"

type Doctor struct {
	com      *common.Common
	report   doctor.Report
	viewport viewport.Model
	help     help.Model
	width    int
	height   int
	keyMap   struct {
		Close, Refresh, Up, Down key.Binding
	}
}

var _ Dialog = (*Doctor)(nil)

func NewDoctor(com *common.Common, report doctor.Report) *Doctor {
	d := &Doctor{
		com:      com,
		report:   report,
		viewport: viewport.New(),
	}
	d.keyMap.Close = key.NewBinding(key.WithKeys("esc", "alt+esc"), key.WithHelp("esc", "close"))
	d.keyMap.Refresh = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	d.keyMap.Up = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	d.keyMap.Down = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	d.help = help.New()
	d.help.Styles = com.Styles.DialogHelpStyles()
	d.setContent(80)
	return d
}

func (d *Doctor) ID() string { return DoctorID }

func (d *Doctor) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, d.keyMap.Refresh):
			// DoctorReport does blocking I/O (SQLite ping, git/uv subprocess
			// calls); defer it via ActionCmd so it runs off the Bubble Tea
			// event loop instead of freezing the UI while it runs.
			return ActionCmd{Cmd: func() tea.Msg {
				return ActionOpenDoctor{Report: d.com.Workspace.DoctorReport(context.Background())}
			}}
		}
	}
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return ActionCmd{Cmd: cmd}
}

func (d *Doctor) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := min(settingsCardMaxWidth, max(48, area.Dx()-8))
	height := min(settingsCardMaxHeight, max(14, area.Dy()-4))
	innerW := width - t.Dialog.View.GetHorizontalFrameSize()
	innerH := height - t.Dialog.View.GetVerticalFrameSize()
	contentH := max(1, innerH-4)

	if width != d.width || contentH != d.height {
		d.width = width
		d.height = contentH
		d.viewport.SetWidth(innerW)
		d.viewport.SetHeight(contentH)
		d.setContent(innerW)
	}

	title := t.Dialog.Title.Render("Doctor")
	sep := t.Header.Separator.Render(strings.Repeat("─", max(1, innerW)))
	helpView := t.Dialog.HelpView.Width(innerW).Render(d.help.View(d))
	view := t.Dialog.View.Width(width).Height(height).Render(strings.Join([]string{
		title,
		sep,
		d.viewport.View(),
		helpView,
	}, "\n"))
	DrawCenter(scr, area, view)
	return nil
}

func (d *Doctor) setContent(width int) {
	var buf bytes.Buffer
	doctor.PrintText(&buf, d.report)
	content := strings.TrimRight(buf.String(), "\n")
	d.viewport.SetContent(lipgloss.NewStyle().Width(max(1, width)).Render(content))
}

func (d *Doctor) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Up, d.keyMap.Down, d.keyMap.Refresh, d.keyMap.Close}
}

func (d *Doctor) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
