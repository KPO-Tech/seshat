package dialog

import (
	"fmt"
	"image"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/planreview"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/util"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	// PlanReviewID is the identifier for the plan review dialog.
	PlanReviewID = "plan_review"

	planReviewMaxWidth  = 150
	planReviewMaxHeight = 38
	planReviewMinHeight = 18
)

type planReviewEditorMode uint8

const (
	planReviewModeView planReviewEditorMode = iota
	planReviewModeLineComment
	planReviewModeGlobalComment
)

type planReviewKeyMap struct {
	Up               key.Binding
	Down             key.Binding
	Approve          key.Binding
	RequestChanges   key.Binding
	EditLineComment  key.Binding
	EditGlobal       key.Binding
	Save             key.Binding
	Cancel           key.Binding
	PrevVersion      key.Binding
	NextVersion      key.Binding
	ToggleFullscreen key.Binding
	Close            key.Binding
}

func defaultPlanReviewKeyMap() planReviewKeyMap {
	return planReviewKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "line up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "line down"),
		),
		Approve: key.NewBinding(
			key.WithKeys("ctrl+y", "a"),
			key.WithHelp("ctrl+y", "approve"),
		),
		RequestChanges: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "send changes"),
		),
		EditLineComment: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "comment line"),
		),
		EditGlobal: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "global comment"),
		),
		Save: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "save"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		PrevVersion: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev version"),
		),
		NextVersion: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next version"),
		),
		ToggleFullscreen: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "fullscreen"),
		),
		Close: CloseKey,
	}
}

// PlanReview is the submit_plan review surface shown outside the transcript.
type PlanReview struct {
	com *common.Common

	windowWidth  int
	windowHeight int
	fullscreen   bool

	submissions   []planreview.Submission
	activeVersion int

	selectedLine  int
	lineComments  map[int]string
	globalComment string

	viewport      viewport.Model
	viewportDirty bool

	input      textinput.Model
	editorMode planReviewEditorMode

	help   help.Model
	keyMap planReviewKeyMap

	planArea image.Rectangle
}

var _ Dialog = (*PlanReview)(nil)

// NewPlanReview creates a new plan review dialog for a submitted plan.
func NewPlanReview(com *common.Common, submission planreview.Submission) *PlanReview {
	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()

	km := defaultPlanReviewKeyMap()

	vp := viewport.New()
	vp.KeyMap = viewport.KeyMap{
		Up:           km.Up,
		Down:         km.Down,
		Left:         key.NewBinding(key.WithDisabled()),
		Right:        key.NewBinding(key.WithDisabled()),
		PageUp:       key.NewBinding(key.WithDisabled()),
		PageDown:     key.NewBinding(key.WithDisabled()),
		HalfPageUp:   key.NewBinding(key.WithDisabled()),
		HalfPageDown: key.NewBinding(key.WithDisabled()),
	}

	input := textinput.New()
	input.SetVirtualCursor(false)
	input.SetStyles(com.Styles.TextInput)
	input.Prompt = "> "

	p := &PlanReview{
		com:           com,
		submissions:   []planreview.Submission{submission},
		activeVersion: 0,
		lineComments:  make(map[int]string),
		viewport:      vp,
		input:         input,
		help:          h,
		keyMap:        km,
		viewportDirty: true,
	}
	return p
}

func (p *PlanReview) ID() string {
	return PlanReviewID
}

// AddSubmission appends or replaces the active submission and switches focus to
// the latest version.
func (p *PlanReview) AddSubmission(submission planreview.Submission) {
	if len(p.submissions) == 0 || p.submissions[0].PlanID != submission.PlanID {
		p.submissions = []planreview.Submission{submission}
	} else {
		replaced := false
		for i, existing := range p.submissions {
			if existing.Version == submission.Version {
				p.submissions[i] = submission
				replaced = true
				break
			}
		}
		if !replaced {
			p.submissions = append(p.submissions, submission)
		}
	}
	p.activeVersion = len(p.submissions) - 1
	p.selectedLine = 0
	p.lineComments = make(map[int]string)
	p.globalComment = ""
	p.editorMode = planReviewModeView
	p.input.SetValue("")
	p.input.Blur()
	p.viewport.SetYOffset(0)
	p.viewportDirty = true
	p.ensureSelectionInBounds()
}

func (p *PlanReview) currentSubmission() planreview.Submission {
	if len(p.submissions) == 0 {
		return planreview.Submission{}
	}
	if p.activeVersion < 0 {
		p.activeVersion = 0
	}
	if p.activeVersion >= len(p.submissions) {
		p.activeVersion = len(p.submissions) - 1
	}
	return p.submissions[p.activeVersion]
}

func (p *PlanReview) currentLines() []string {
	content := strings.ReplaceAll(p.currentSubmission().Content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (p *PlanReview) ensureSelectionInBounds() {
	lines := p.currentLines()
	if p.selectedLine < 0 {
		p.selectedLine = 0
	}
	if p.selectedLine >= len(lines) {
		p.selectedLine = len(lines) - 1
	}
	if p.selectedLine < 0 {
		p.selectedLine = 0
	}
}

func (p *PlanReview) ensureSelectionVisible() {
	if p.viewport.Height() <= 0 {
		return
	}
	line := p.selectedLine
	top := p.viewport.YOffset()
	bottom := top + p.viewport.Height() - 1
	switch {
	case line < top:
		p.viewport.SetYOffset(line)
	case line > bottom:
		p.viewport.SetYOffset(line - p.viewport.Height() + 1)
	}
}

func (p *PlanReview) setEditor(mode planReviewEditorMode) {
	p.editorMode = mode
	p.input.Focus()
	switch mode {
	case planReviewModeLineComment:
		line := p.selectedLine + 1
		p.input.Placeholder = fmt.Sprintf("Comment on line %d", line)
		p.input.SetValue(p.lineComments[line])
	case planReviewModeGlobalComment:
		p.input.Placeholder = "Global review comment"
		p.input.SetValue(p.globalComment)
	}
	p.input.CursorEnd()
}

func (p *PlanReview) finishEditor(save bool) {
	if save {
		value := strings.TrimSpace(p.input.Value())
		switch p.editorMode {
		case planReviewModeLineComment:
			line := p.selectedLine + 1
			if value == "" {
				delete(p.lineComments, line)
			} else {
				p.lineComments[line] = value
			}
		case planReviewModeGlobalComment:
			p.globalComment = value
		}
	}
	p.input.Blur()
	p.input.SetValue("")
	p.editorMode = planReviewModeView
	p.viewportDirty = true
}

func (p *PlanReview) collectLineComments() []planreview.LineComment {
	comments := make([]planreview.LineComment, 0, len(p.lineComments))
	for line, comment := range p.lineComments {
		comment = strings.TrimSpace(comment)
		if comment == "" {
			continue
		}
		comments = append(comments, planreview.LineComment{Line: line, Comment: comment})
	}
	return comments
}

func (p *PlanReview) hasFeedback() bool {
	if strings.TrimSpace(p.globalComment) != "" {
		return true
	}
	for _, c := range p.lineComments {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}

func (p *PlanReview) submitReview(approved bool) Action {
	review := planreview.Review{
		Submission:    p.currentSubmission(),
		Approved:      approved,
		GlobalComment: strings.TrimSpace(p.globalComment),
		LineComments:  p.collectLineComments(),
	}
	if approved {
		return ActionPlanReviewSubmit{Review: review}
	}
	if !review.HasFeedback() {
		return ActionCmd{Cmd: util.ReportWarn("Add a global comment or a line comment before sending changes.")}
	}
	return ActionPlanReviewRequestChanges{Review: review}
}

func (p *PlanReview) switchVersion(delta int) {
	if len(p.submissions) <= 1 {
		return
	}
	p.activeVersion += delta
	if p.activeVersion < 0 {
		p.activeVersion = 0
	}
	if p.activeVersion >= len(p.submissions) {
		p.activeVersion = len(p.submissions) - 1
	}
	p.ensureSelectionInBounds()
	p.ensureSelectionVisible()
	p.viewportDirty = true
}

func (p *PlanReview) handleViewingKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, p.keyMap.Close):
		return ActionClose{}
	case key.Matches(msg, p.keyMap.Up):
		p.selectedLine--
		p.ensureSelectionInBounds()
		p.ensureSelectionVisible()
		p.viewportDirty = true
	case key.Matches(msg, p.keyMap.Down):
		p.selectedLine++
		p.ensureSelectionInBounds()
		p.ensureSelectionVisible()
		p.viewportDirty = true
	case key.Matches(msg, p.keyMap.EditLineComment):
		p.setEditor(planReviewModeLineComment)
	case key.Matches(msg, p.keyMap.EditGlobal):
		p.setEditor(planReviewModeGlobalComment)
	case key.Matches(msg, p.keyMap.Approve):
		return p.submitReview(true)
	case key.Matches(msg, p.keyMap.RequestChanges):
		return p.submitReview(false)
	case key.Matches(msg, p.keyMap.PrevVersion):
		p.switchVersion(-1)
	case key.Matches(msg, p.keyMap.NextVersion):
		p.switchVersion(1)
	case key.Matches(msg, p.keyMap.ToggleFullscreen):
		p.fullscreen = !p.fullscreen
	}
	return nil
}

func (p *PlanReview) handleEditorKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, p.keyMap.Cancel):
		p.finishEditor(false)
		return nil
	case key.Matches(msg, p.keyMap.Save):
		p.finishEditor(true)
		return nil
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return ActionCmd{Cmd: cmd}
}

func (p *PlanReview) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if p.editorMode != planReviewModeView {
			return p.handleEditorKey(msg)
		}
		return p.handleViewingKey(msg)
	case tea.MouseWheelMsg:
		p.viewport, _ = p.viewport.Update(msg)
	case tea.MouseClickMsg:
		if p.editorMode != planReviewModeView {
			return nil
		}
		point := image.Pt(msg.X, msg.Y)
		if !point.In(p.planArea) {
			return nil
		}
		line := p.viewport.YOffset() + (msg.Y - p.planArea.Min.Y)
		if line < 0 {
			line = 0
		}
		p.selectedLine = line
		p.ensureSelectionInBounds()
		p.ensureSelectionVisible()
		p.viewportDirty = true
	}
	return nil
}

func (p *PlanReview) renderViewportContent(width int) string {
	lines := p.currentLines()
	p.ensureSelectionInBounds()

	accent := lipgloss.NewStyle().Foreground(p.com.Styles.Logo.FieldColor).Bold(true)
	commentStyle := p.com.Styles.Status.SuccessMessage.UnsetBackground().Padding(0)
	numberStyle := p.com.Styles.Dialog.SecondaryText.Padding(0)
	selectedStyle := p.com.Styles.Dialog.PrimaryText.Bold(true).Padding(0)
	normalStyle := p.com.Styles.Dialog.PrimaryText.Padding(0)

	digits := len(fmt.Sprintf("%d", len(lines)))
	var rendered []string
	for idx, rawLine := range lines {
		prefix := "  "
		if idx == p.selectedLine {
			prefix = "› "
		}
		if _, ok := p.lineComments[idx+1]; ok {
			prefix = prefix[:1] + "●"
		}

		textWidth := max(1, width-digits-4)
		content := ansi.Truncate(strings.ReplaceAll(rawLine, "	", "    "), textWidth, "")
		line := fmt.Sprintf("%s %s %s",
			accent.Render(prefix),
			numberStyle.Render(fmt.Sprintf("%*d", digits, idx+1)),
			content,
		)
		if idx == p.selectedLine {
			line = selectedStyle.Render(line)
		} else {
			line = normalStyle.Render(line)
		}
		rendered = append(rendered, line)
	}
	if len(rendered) == 0 {
		return p.com.Styles.Dialog.SecondaryText.Render("No plan content")
	}
	_ = commentStyle
	return strings.Join(rendered, "\n")
}

func (p *PlanReview) currentLineCommentPreview(width int) string {
	comment := strings.TrimSpace(p.lineComments[p.selectedLine+1])
	if comment == "" {
		return p.com.Styles.Dialog.SecondaryText.Render("No line comment on selected line")
	}
	label := fmt.Sprintf("Line %d: %s", p.selectedLine+1, comment)
	return p.com.Styles.Dialog.PrimaryText.Render(ansi.Truncate(label, width, ""))
}

func (p *PlanReview) globalCommentPreview(width int) string {
	if strings.TrimSpace(p.globalComment) == "" {
		return p.com.Styles.Dialog.SecondaryText.Render("Global review comment not set")
	}
	label := "Global: " + p.globalComment
	return p.com.Styles.Dialog.PrimaryText.Render(ansi.Truncate(label, width, ""))
}

func (p *PlanReview) helpView() string {
	if p.editorMode != planReviewModeView {
		return p.help.ShortHelpView([]key.Binding{p.keyMap.Save, p.keyMap.Cancel})
	}
	bindings := []key.Binding{
		p.keyMap.Up,
		p.keyMap.Down,
		p.keyMap.EditLineComment,
		p.keyMap.EditGlobal,
	}
	if p.hasFeedback() {
		bindings = append(bindings, p.keyMap.RequestChanges)
	}
	bindings = append(bindings, p.keyMap.Approve)
	if len(p.submissions) > 1 {
		bindings = append(bindings, p.keyMap.PrevVersion, p.keyMap.NextVersion)
	}
	bindings = append(bindings, p.keyMap.ToggleFullscreen, p.keyMap.Close)
	return p.help.ShortHelpView(bindings)
}

func (p *PlanReview) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := p.com.Styles

	forceFullscreen := area.Dx() <= 96 || area.Dy() <= 24
	width := min(planReviewMaxWidth, area.Dx())
	height := min(planReviewMaxHeight, area.Dy())
	if !p.fullscreen && !forceFullscreen {
		width = min(width, max(96, area.Dx()-6))
		height = min(height, max(planReviewMinHeight, area.Dy()-4))
	}
	if p.fullscreen || forceFullscreen {
		width = area.Dx()
		height = area.Dy()
	}

	innerWidth := max(1, width-t.Dialog.View.GetHorizontalFrameSize())
	viewFrameH := t.Dialog.View.GetVerticalFrameSize()
	helpHeight := 1
	metaHeight := 3
	footerHeight := 2
	editorHeight := 0
	editorLabel := ""
	if p.editorMode != planReviewModeView {
		editorHeight = 2
		switch p.editorMode {
		case planReviewModeLineComment:
			editorLabel = fmt.Sprintf("Comment for line %d", p.selectedLine+1)
		case planReviewModeGlobalComment:
			editorLabel = "Global review comment"
		}
	}
	titleHeight := 1
	separatorCount := 2
	viewportHeight := max(5, height-viewFrameH-titleHeight-metaHeight-footerHeight-editorHeight-helpHeight-separatorCount)

	p.viewport.SetWidth(innerWidth)
	p.viewport.SetHeight(viewportHeight)
	if p.viewportDirty {
		p.viewport.SetContent(p.renderViewportContent(innerWidth))
		p.viewportDirty = false
	}
	p.ensureSelectionVisible()

	versionInfo := fmt.Sprintf("v%d", p.currentSubmission().Version)
	if len(p.submissions) > 1 {
		versionInfo = fmt.Sprintf("v%d (%d/%d)", p.currentSubmission().Version, p.activeVersion+1, len(p.submissions))
	}
	titleInfo := t.Dialog.SecondaryText.Padding(0).Render(versionInfo)

	title := common.DialogTitle(t, "Plan Review", max(0, innerWidth-lipgloss.Width(titleInfo)), t.Dialog.TitleGradFromColor, t.Dialog.TitleGradToColor)
	titleLine := t.Dialog.Title.Render(title) + titleInfo
	separator := t.Header.Separator.Render(strings.Repeat("─", innerWidth))

	metadata := []string{
		t.Dialog.PrimaryText.Render(fmt.Sprintf("File: %s", p.currentSubmission().Filename)),
		t.Dialog.SecondaryText.Render(fmt.Sprintf("Slug: %s   Status: %s   Lines: %d", p.currentSubmission().Slug, p.currentSubmission().Status, len(p.currentLines()))),
		t.Dialog.SecondaryText.Render("Review this plan outside the transcript, then approve or request changes."),
	}

	summary := []string{
		t.Dialog.SecondaryText.Render(fmt.Sprintf("Selected line %d   Line comments: %d", p.selectedLine+1, len(p.lineComments))),
		p.globalCommentPreview(innerWidth),
		p.currentLineCommentPreview(innerWidth),
	}

	parts := []string{titleLine}
	parts = append(parts, metadata...)
	parts = append(parts, separator)
	parts = append(parts, p.viewport.View())
	parts = append(parts, separator)
	parts = append(parts, summary...)

	inputOffsetY := 0
	if p.editorMode != planReviewModeView {
		parts = append(parts, t.Dialog.SecondaryText.Render(editorLabel))
		inputOffsetY = len(parts)
		p.input.SetWidth(max(10, innerWidth-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1))
		parts = append(parts, t.Dialog.InputPrompt.Render(p.input.View()))
	}

	parts = append(parts, t.Dialog.HelpView.Width(innerWidth).Render(p.helpView()))

	view := t.Dialog.View.Width(width).Render(strings.Join(parts, "\n"))

	dialogWidth, dialogHeight := lipgloss.Size(view)
	rect := common.CenterRect(area, dialogWidth, dialogHeight)

	contentMinX := rect.Min.X + t.Dialog.View.GetBorderLeftSize() + t.Dialog.View.GetPaddingLeft() + t.Dialog.View.GetMarginLeft()
	contentMinY := rect.Min.Y + t.Dialog.View.GetBorderTopSize() + t.Dialog.View.GetPaddingTop() + t.Dialog.View.GetMarginTop()
	viewportTop := contentMinY + titleHeight + len(metadata) + 1
	p.planArea = image.Rect(contentMinX, viewportTop, contentMinX+innerWidth, viewportTop+viewportHeight)

	var cur *tea.Cursor
	if p.editorMode != planReviewModeView {
		cur = InputCursor(t, p.input.Cursor())
		if cur != nil {
			// inputOffsetY counts parts items; the viewport part contributes
			// viewportHeight rendered lines, not 1. Add the extra lines here.
			cur.Y += inputOffsetY - 2 + p.viewport.Height()
		}
	}

	DrawCenterCursor(scr, area, view, cur)
	return cur
}
