package dialog

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/commands"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/list"
	uv "github.com/charmbracelet/ultraviolet"
)

const SkillsPickerID = "skills_picker"

type SkillsPicker struct {
	com *common.Common

	keyMap struct {
		Select, Next, Previous, Close key.Binding
	}

	help  help.Model
	input textinput.Model
	list  *list.FilterableList

	skills []commands.CustomCommand
}

var _ Dialog = (*SkillsPicker)(nil)

func NewSkillsPicker(com *common.Common, skills []commands.CustomCommand) (*SkillsPicker, error) {
	s := &SkillsPicker{com: com, skills: skills}
	s.help = help.New()
	s.help.Styles = com.Styles.DialogHelpStyles()
	s.list = list.NewFilterableList()
	s.list.Focus()
	s.list.SetSelected(0)
	s.input = textinput.New()
	s.input.SetVirtualCursor(false)
	s.input.Placeholder = "Type a skill name"
	s.input.SetStyles(com.Styles.TextInput)
	s.input.Focus()
	s.keyMap.Select = key.NewBinding(key.WithKeys("enter", "ctrl+y"), key.WithHelp("enter", "run"))
	s.keyMap.Next = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "next item"))
	s.keyMap.Previous = key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "previous item"))
	closeKey := CloseKey
	closeKey.SetHelp("esc", "cancel")
	s.keyMap.Close = closeKey
	s.setSkillItems(skills)
	return s, nil
}

func (s *SkillsPicker) ID() string { return SkillsPickerID }

func (s *SkillsPicker) SetSkills(skills []commands.CustomCommand) {
	s.skills = skills
	s.setSkillItems(skills)
}

func (s *SkillsPicker) setSkillItems(skills []commands.CustomCommand) {
	items := make([]list.FilterableItem, 0, len(skills))
	for _, cmd := range skills {
		if cmd.Skill == nil {
			continue
		}
		action := ActionRunCustomCommand{Skill: cmd.Skill}
		item := NewCommandItem(s.com.Styles, "skill_"+cmd.ID, "/"+cmd.Skill.Name, "", action)
		if cmd.Skill.Description != "" {
			item = item.WithDescription(cmd.Skill.Description)
		}
		items = append(items, item)
	}
	s.list.SetItems(items...)
	s.list.SetFilter("")
	s.list.ScrollToTop()
	s.list.SetSelected(0)
	s.input.SetValue("")
}

func (s *SkillsPicker) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, s.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, s.keyMap.Previous):
			if s.list.IsSelectedFirst() {
				s.list.SelectLast()
			} else {
				s.list.SelectPrev()
			}
			s.list.ScrollToSelected()
		case key.Matches(msg, s.keyMap.Next):
			if s.list.IsSelectedLast() {
				s.list.SelectFirst()
			} else {
				s.list.SelectNext()
			}
			s.list.ScrollToSelected()
		case key.Matches(msg, s.keyMap.Select):
			if item := s.list.SelectedItem(); item != nil {
				if commandItem, ok := item.(*CommandItem); ok {
					return commandItem.Action()
				}
			}
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			s.list.SetFilter(s.input.Value())
			s.list.ScrollToTop()
			s.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

func (s *SkillsPicker) Cursor() *tea.Cursor {
	return InputCursor(s.com.Styles, s.input.Cursor())
}

func (s *SkillsPicker) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := s.com.Styles
	width := max(0, min(settingsCardMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	inputW := max(0, innerWidth-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1)
	s.input.SetWidth(inputW)
	s.help.SetWidth(innerWidth)
	titleH := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight
	inputH := t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight
	helpH := t.Dialog.HelpView.GetVerticalFrameSize()
	viewFrameH := t.Dialog.View.GetVerticalFrameSize()
	listMarginH := t.Dialog.List.GetVerticalMargins()
	const subBlock = 3
	const sepAbove = 1
	overhead := titleH + inputH + subBlock + sepAbove + helpH + viewFrameH + listMarginH
	s.list.SetSize(innerWidth, 9999)
	measuredContentH := s.list.TotalHeight()
	maxTermH := max(0, area.Dy()-t.Dialog.View.GetVerticalBorderSize())
	height := max(overhead+1, min(settingsCardMaxHeight, min(maxTermH, overhead+measuredContentH)))
	finalContentH := max(1, height-overhead)
	s.list.SetSize(innerWidth, finalContentH)
	sep := t.Header.Separator.Render(strings.Repeat("─", innerWidth))
	rc := NewRenderContext(t, width)
	rc.Parts = []string{rc.TitleStyle.Render("Chat  ›  Skills")}
	rc.AddPart(t.Dialog.InputPrompt.Render(s.input.View()))
	rc.AddPart(sep)
	rc.AddPart(t.Dialog.SecondaryText.Render("  choose a skill to invoke"))
	rc.Parts = append(rc.Parts, "")
	visibleCount := len(s.list.FilteredItems())
	if s.list.Height() >= visibleCount {
		s.list.ScrollToTop()
	} else {
		s.list.ScrollToSelected()
	}
	rc.AddPart(t.Dialog.List.Height(s.list.Height()).Render(s.list.Render()))
	rc.Parts = append(rc.Parts, sep)
	rc.Help = s.help.View(s)
	view := rc.Render()
	cur := s.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

func (s *SkillsPicker) ShortHelp() []key.Binding {
	return []key.Binding{s.keyMap.Next, s.keyMap.Select, s.keyMap.Close}
}

func (s *SkillsPicker) FullHelp() [][]key.Binding {
	return [][]key.Binding{{s.keyMap.Select, s.keyMap.Next, s.keyMap.Previous}, {s.keyMap.Close}}
}
