package dialog

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/agent/tools/mcp"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/config"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/skills"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/list"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/workspace"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// SettingsID is the identifier for the settings dialog.
const SettingsID = "settings"

// settingsView tracks which sub-view is active inside the Settings dialog.
type settingsView uint

const (
	settingsViewRoot settingsView = iota
	settingsViewProviders
	settingsViewProvidersLLM
	settingsViewTheme
	settingsViewWebSearch
	settingsViewTools
	settingsViewMCP
	settingsViewMCPDetail
	settingsViewSkills
)

const (
	settingsDialogMaxWidth  = settingsCardMaxWidth
	settingsDialogMaxHeight = 24 // matches root view natural height (7 sections, no excess padding)
)

// settingsSection describes one row in the root hub list.
type settingsSection struct {
	id       string
	name     string
	desc     string
	shortcut string
	dialogID string       // if non-empty: dispatch ActionOpenDialog{dialogID} on select
	subView  settingsView // if dialogID == "": navigate here internally
}

// Settings is the Settings hub dialog opened by ctrl+p. It shows navigable
// sections that route to sub-views or existing dialogs.
type Settings struct {
	com  *common.Common
	view settingsView

	// root view state
	input    textinput.Model
	rootList *list.FilterableList
	sections []settingsSection

	// providers hub state
	providerSections []settingsSection
	providerList     *list.FilterableList

	// providers sub-view state
	providers []catwalk.Provider
	provList  *list.FilterableList
	provInput textinput.Model

	// theme sub-view state
	themeList *list.FilterableList

	// web-search sub-view state
	webSearchList *list.FilterableList

	// tools sub-view state
	toolsList  *ToolsList
	toolsInput textinput.Model
	toolsAll   []workspace.ToolInfo

	// mcp sub-view state
	mcpList           *list.FilterableList
	selectedMCPName   string
	mcpDetailViewport viewport.Model

	// skills sub-view state
	skillsList  *ToolsList
	skillsInput textinput.Model
	skillsAll   []skills.CatalogEntry

	keyMap struct {
		Select, Next, Previous, Back, Close key.Binding
	}
	help help.Model

	windowWidth int
}

var _ Dialog = (*Settings)(nil)

// NewSettings creates a new Settings hub dialog.
func NewSettings(com *common.Common) (*Settings, error) {
	t := com.Styles
	s := &Settings{
		com:              com,
		view:             settingsViewRoot,
		sections:         defaultSettingsSections(),
		providerSections: defaultProviderSections(),
	}

	// Root filter input.
	s.input = textinput.New()
	s.input.SetVirtualCursor(false)
	s.input.Placeholder = "Search settings..."
	s.input.SetStyles(t.TextInput)
	s.input.Focus()

	s.rootList = list.NewFilterableList()
	s.rootList.Focus()
	s.rootList.SetSelected(0)
	s.rootList.SetGap(1) // one blank line between each section for visual breathing room
	s.rebuildRootList("")

	s.providerList = list.NewFilterableList()
	s.providerList.Focus()
	s.providerList.SetSelected(0)
	s.providerList.SetGap(1)
	s.rebuildProviderList()

	// Providers filter input + list.
	s.provInput = textinput.New()
	s.provInput.SetVirtualCursor(false)
	s.provInput.Placeholder = "Filter providers..."
	s.provInput.SetStyles(t.TextInput)

	s.provList = list.NewFilterableList()
	s.provList.Focus()
	s.provList.SetSelected(0)
	s.provList.SetGap(1)

	s.themeList = list.NewFilterableList()
	s.themeList.Focus()
	s.themeList.SetGap(1)

	s.webSearchList = list.NewFilterableList()
	s.webSearchList.Focus()
	s.webSearchList.SetSelected(0)
	s.webSearchList.SetGap(1)
	s.rebuildWebSearchList()

	// Tools filter input + list.
	s.toolsInput = textinput.New()
	s.toolsInput.SetVirtualCursor(false)
	s.toolsInput.Placeholder = "Filter tools..."
	s.toolsInput.SetStyles(t.TextInput)
	s.toolsList = newToolsList(t)

	// MCP list.
	s.mcpList = list.NewFilterableList()
	s.mcpList.Focus()
	s.mcpList.SetSelected(0)
	s.mcpList.SetGap(1)
	s.mcpDetailViewport = viewport.New()

	// Skills filter input + list.
	s.skillsInput = textinput.New()
	s.skillsInput.SetVirtualCursor(false)
	s.skillsInput.Placeholder = "Filter skills..."
	s.skillsInput.SetStyles(t.TextInput)
	s.skillsList = newToolsList(t)

	providers, _ := config.Providers(com.Config()) // best-effort; nil on error → empty list
	s.providers = providers
	s.rebuildProvList("")

	// Key bindings.
	s.keyMap.Select = key.NewBinding(key.WithKeys("enter", "ctrl+y"), key.WithHelp("enter", "open"))
	s.keyMap.Next = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "next"))
	s.keyMap.Previous = key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "prev"))
	s.keyMap.Back = key.NewBinding(key.WithKeys("esc", "alt+esc", "left"), key.WithHelp("esc/←", "back"))
	s.keyMap.Close = key.NewBinding(key.WithKeys("esc", "alt+esc"), key.WithHelp("esc", "close"))

	h := help.New()
	h.Styles = t.DialogHelpStyles()
	s.help = h

	return s, nil
}

func defaultSettingsSections() []settingsSection {
	return []settingsSection{
		{id: "commands", name: "Commands", desc: "shortcuts, sessions, copy actions, app controls", dialogID: CommandsID},
		{id: "providers", name: "Providers", desc: "llm, web search, and future provider families", shortcut: "ctrl+,", subView: settingsViewProviders},
		{id: "models", name: "Models", desc: "switch the active AI model", shortcut: "ctrl+m", dialogID: ModelsID},
		{id: "theme", name: "Theme", desc: "background style and visual appearance", subView: settingsViewTheme},
		{id: "tools", name: "Tools", desc: "tool UX options and available tool reference", subView: settingsViewTools},
		{id: "mcp", name: "MCP", desc: "MCP server status and management notes", subView: settingsViewMCP},
		{id: "skills", name: "Skills", desc: "slash-skill workflow and skill path discovery", subView: settingsViewSkills},
	}
}

func defaultProviderSections() []settingsSection {
	return []settingsSection{
		{id: "providers_llm", name: "LLM", desc: "configure model providers and credentials", subView: settingsViewProvidersLLM},
		{id: "providers_web_search", name: "Web Search", desc: "configure web search providers and defaults", subView: settingsViewWebSearch},
	}
}

// ID implements Dialog.
func (s *Settings) ID() string { return SettingsID }

// Cursor implements Dialog.
func (s *Settings) Cursor() *tea.Cursor {
	switch s.view {
	case settingsViewRoot:
		return InputCursor(s.com.Styles, s.input.Cursor())
	case settingsViewProvidersLLM:
		return InputCursor(s.com.Styles, s.provInput.Cursor())
	case settingsViewTools:
		return InputCursor(s.com.Styles, s.toolsInput.Cursor())
	case settingsViewSkills:
		return InputCursor(s.com.Styles, s.skillsInput.Cursor())
	}
	return nil
}

// HandleMsg implements Dialog.
func (s *Settings) HandleMsg(msg tea.Msg) Action {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if s.view == settingsViewRoot {
		return s.handleRootKey(kp)
	}
	return s.handleSubKey(kp)
}

func (s *Settings) handleRootKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Close):
		return ActionClose{}
	case key.Matches(msg, s.keyMap.Previous):
		if s.rootList.IsSelectedFirst() {
			s.rootList.SelectLast()
		} else {
			s.rootList.SelectPrev()
		}
		s.rootList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.rootList.IsSelectedLast() {
			s.rootList.SelectFirst()
		} else {
			s.rootList.SelectNext()
		}
		s.rootList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		return s.activateSelected()
	default:
		// Per-item shortcut triggers (when filter is empty).
		if s.input.Value() == "" {
			for _, fi := range s.rootList.FilteredItems() {
				if si, ok := fi.(*settingsSectionItem); ok && si.shortcut != "" && msg.String() == si.shortcut {
					return s.activateSI(si)
				}
			}
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		s.rebuildRootList(s.input.Value())
		return ActionCmd{cmd}
	}
	return nil
}

func (s *Settings) handleSubKey(msg tea.KeyPressMsg) Action {
	if key.Matches(msg, s.keyMap.Back) {
		s.gotoParent()
		return nil
	}
	switch s.view {
	case settingsViewProviders:
		return s.handleProviderKey(msg)
	case settingsViewProvidersLLM:
		return s.handleProvKey(msg)
	case settingsViewTheme:
		return s.handleThemeKey(msg)
	case settingsViewWebSearch:
		return s.handleWebSearchKey(msg)
	case settingsViewTools:
		return s.handleToolsKey(msg)
	case settingsViewMCP:
		return s.handleMCPKey(msg)
	case settingsViewMCPDetail:
		return s.handleMCPDetailKey(msg)
	case settingsViewSkills:
		return s.handleSkillsKey(msg)
	}
	return nil
}

func (s *Settings) handleProvKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.provList.IsSelectedFirst() {
			s.provList.SelectLast()
		} else {
			s.provList.SelectPrev()
		}
		s.provList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.provList.IsSelectedLast() {
			s.provList.SelectFirst()
		} else {
			s.provList.SelectNext()
		}
		s.provList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		if item := s.provList.SelectedItem(); item != nil {
			if pi, ok := item.(*settingsProviderItem); ok {
				return ActionOpenProviderConfig{Provider: pi.provider}
			}
		}
	default:
		var cmd tea.Cmd
		s.provInput, cmd = s.provInput.Update(msg)
		s.rebuildProvList(s.provInput.Value())
		return ActionCmd{cmd}
	}
	return nil
}

func (s *Settings) handleProviderKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.providerList.IsSelectedFirst() {
			s.providerList.SelectLast()
		} else {
			s.providerList.SelectPrev()
		}
		s.providerList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.providerList.IsSelectedLast() {
			s.providerList.SelectFirst()
		} else {
			s.providerList.SelectNext()
		}
		s.providerList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		if item := s.providerList.SelectedItem(); item != nil {
			if si, ok := item.(*settingsSectionItem); ok {
				return s.activateSI(si)
			}
		}
	}
	return nil
}

func (s *Settings) handleWebSearchKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.webSearchList.IsSelectedFirst() {
			s.webSearchList.SelectLast()
		} else {
			s.webSearchList.SelectPrev()
		}
		s.webSearchList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.webSearchList.IsSelectedLast() {
			s.webSearchList.SelectFirst()
		} else {
			s.webSearchList.SelectNext()
		}
		s.webSearchList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		if item := s.webSearchList.SelectedItem(); item != nil {
			if wi, ok := item.(*settingsWebSearchItem); ok {
				if wi.providerID == "auto" {
					return ActionSelectWebSearchProvider{ProviderID: wi.providerID}
				}
				return ActionOpenWebSearchConfig{ProviderID: wi.providerID}
			}
		}
	}
	return nil
}

func (s *Settings) handleThemeKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.themeList.IsSelectedFirst() {
			s.themeList.SelectLast()
		} else {
			s.themeList.SelectPrev()
		}
		s.themeList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.themeList.IsSelectedLast() {
			s.themeList.SelectFirst()
		} else {
			s.themeList.SelectNext()
		}
		s.themeList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		if item := s.themeList.SelectedItem(); item != nil {
			if ti, ok := item.(*settingsThemeItem); ok {
				cfg := s.com.Config()
				isTransparent := cfg != nil && cfg.Options != nil && cfg.Options.TUI.Transparent != nil && *cfg.Options.TUI.Transparent
				// Only dispatch if the user chose a DIFFERENT option.
				if ti.transparent != isTransparent {
					return ActionToggleTransparentBackground{}
				}
			}
		}
	}
	return nil
}

func (s *Settings) handleToolsKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.toolsList.IsSelectedFirst() {
			s.toolsList.SelectLast()
		} else {
			s.toolsList.SelectPrev()
		}
		s.toolsList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.toolsList.IsSelectedLast() {
			s.toolsList.SelectFirst()
		} else {
			s.toolsList.SelectNext()
		}
		s.toolsList.ScrollToSelected()
	default:
		var cmd tea.Cmd
		s.toolsInput, cmd = s.toolsInput.Update(msg)
		s.toolsList.SetFilter(s.toolsInput.Value())
		s.toolsList.SelectFirst()
		return ActionCmd{cmd}
	}
	return nil
}

func (s *Settings) handleMCPKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.mcpList.IsSelectedFirst() {
			s.mcpList.SelectLast()
		} else {
			s.mcpList.SelectPrev()
		}
		s.mcpList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.mcpList.IsSelectedLast() {
			s.mcpList.SelectFirst()
		} else {
			s.mcpList.SelectNext()
		}
		s.mcpList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Select):
		if item := s.mcpList.SelectedItem(); item != nil {
			if mi, ok := item.(*settingsMCPItem); ok {
				s.selectedMCPName = mi.name
				s.gotoView(settingsViewMCPDetail)
			}
		}
	}
	return nil
}

func (s *Settings) handleMCPDetailKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Next):
		s.mcpDetailViewport.ScrollDown(1)
	case key.Matches(msg, s.keyMap.Previous):
		s.mcpDetailViewport.ScrollUp(1)
	}
	return nil
}

func (s *Settings) rebuildToolsList(filter string) {
	groups := buildToolGroups(s.com.Styles, s.toolsAll)
	s.toolsList.SetGroups(groups...)
	s.toolsList.SetFilter(filter)
	s.toolsList.Focus()
	s.toolsList.SelectFirst()
	s.toolsList.ScrollToTop()
}

func (s *Settings) handleSkillsKey(msg tea.KeyPressMsg) Action {
	switch {
	case key.Matches(msg, s.keyMap.Previous):
		if s.skillsList.IsSelectedFirst() {
			s.skillsList.SelectLast()
		} else {
			s.skillsList.SelectPrev()
		}
		s.skillsList.ScrollToSelected()
	case key.Matches(msg, s.keyMap.Next):
		if s.skillsList.IsSelectedLast() {
			s.skillsList.SelectFirst()
		} else {
			s.skillsList.SelectNext()
		}
		s.skillsList.ScrollToSelected()
	default:
		var cmd tea.Cmd
		s.skillsInput, cmd = s.skillsInput.Update(msg)
		s.skillsList.SetFilter(s.skillsInput.Value())
		s.skillsList.SelectFirst()
		return ActionCmd{cmd}
	}
	return nil
}

func (s *Settings) rebuildSkillsList(filter string) {
	groups := buildSkillGroups(s.com.Styles, s.skillsAll)
	s.skillsList.SetGroups(groups...)
	s.skillsList.SetFilter(filter)
	s.skillsList.Focus()
	s.skillsList.SelectFirst()
	s.skillsList.ScrollToTop()
}

func (s *Settings) rebuildMCPList() {
	cfg := s.com.Config()
	states := s.com.Workspace.MCPGetStates()

	var items []list.FilterableItem
	if cfg != nil {
		for _, server := range cfg.MCP.Sorted() {
			info := states[server.Name]
			info.Name = server.Name
			items = append(items, &settingsMCPItem{
				Versioned: list.NewVersioned(),
				name:      server.Name,
				info:      info,
				t:         s.com.Styles,
			})
		}
	}
	s.mcpList.SetItems(items...)
	s.mcpList.ScrollToTop()
	s.mcpList.Focus()
	s.mcpList.SetSelected(0)
}

func (s *Settings) buildMCPDetail(serverName string, width int) string {
	t := s.com.Styles
	accent := lipgloss.NewStyle().Foreground(t.Logo.FieldColor).Bold(true)
	muted := t.Sidebar.WorkingDir
	bold := lipgloss.NewStyle().Bold(true)

	states := s.com.Workspace.MCPGetStates()
	info, hasInfo := states[serverName]

	// Full-width heading so the orange foreground spans the entire line.
	heading := accent
	if width > 0 {
		heading = accent.Width(width)
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, heading.Render("  "+serverName))
	lines = append(lines, "")

	if hasInfo {
		switch info.State {
		case mcp.StateConnected:
			statusStr := fmt.Sprintf("connected · %d tools · %d prompts · %d resources",
				info.Counts.Tools, info.Counts.Prompts, info.Counts.Resources)
			connStyle := lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground())
			lines = append(lines, muted.Render("  Status  ")+connStyle.Render(statusStr))
		case mcp.StateStarting:
			lines = append(lines, muted.Render("  Status  starting…"))
		case mcp.StateError:
			errStr := "error"
			if info.Error != nil {
				errStr = "error: " + info.Error.Error()
			}
			errStyle := lipgloss.NewStyle().Foreground(t.Tool.IconError.GetForeground())
			lines = append(lines, muted.Render("  Status  ")+errStyle.Render(errStr))
		case mcp.StateDisabled:
			lines = append(lines, muted.Render("  Status  disabled"))
		}
	} else {
		lines = append(lines, muted.Render("  Status  offline"))
	}

	// Tools list for this server.
	var serverTools []*mcp.Tool
	for name, tools := range mcp.Tools() {
		if name == serverName {
			serverTools = tools
			break
		}
	}

	const descIndent = "      "
	const descIndentW = 6
	descWrapW := width - descIndentW
	if descWrapW < 20 {
		descWrapW = 0 // no wrapping if too narrow
	}

	if len(serverTools) > 0 {
		lines = append(lines, "")
		lines = append(lines, accent.Render(fmt.Sprintf("  Tools (%d)", len(serverTools))))
		for _, tool := range serverTools {
			lines = append(lines, "")
			lines = append(lines, bold.Render("    "+tool.Name))
			if desc := strings.TrimSpace(tool.Description); desc != "" {
				// First paragraph only.
				if idx := strings.Index(desc, "\n\n"); idx >= 0 {
					desc = desc[:idx]
				}
				desc = strings.ReplaceAll(desc, "\n", " ")
				if descWrapW > 0 {
					wrapped := ansi.Wordwrap(desc, descWrapW, "")
					for _, wline := range strings.Split(wrapped, "\n") {
						lines = append(lines, muted.Render(descIndent+wline))
					}
				} else {
					lines = append(lines, muted.Render(descIndent+desc))
				}
			}
		}
	} else if hasInfo && info.State == mcp.StateConnected {
		lines = append(lines, "")
		lines = append(lines, muted.Render("  No tools available."))
	}

	lines = append(lines, "")
	lines = append(lines, muted.Render("  esc or ← to go back"))
	return strings.Join(lines, "\n")
}

func (s *Settings) activateSelected() Action {
	if item := s.rootList.SelectedItem(); item != nil {
		if si, ok := item.(*settingsSectionItem); ok {
			return s.activateSI(si)
		}
	}
	return nil
}

func (s *Settings) activateSI(si *settingsSectionItem) Action {
	if si.dialogID != "" {
		return ActionOpenDialog{DialogID: si.dialogID}
	}
	s.gotoView(si.subView)
	return nil
}

func (s *Settings) gotoView(v settingsView) {
	s.view = v
	switch v {
	case settingsViewProviders:
		s.rebuildProviderList()
	case settingsViewProvidersLLM:
		s.provInput.SetValue("")
		s.provInput.Focus()
		s.rebuildProvList("")
	case settingsViewTheme:
		s.rebuildThemeList()
	case settingsViewWebSearch:
		s.rebuildWebSearchList()
	case settingsViewTools:
		tools, _ := s.com.Workspace.ListTools(context.Background())
		s.toolsAll = tools
		s.toolsInput.SetValue("")
		s.toolsInput.Focus()
		s.rebuildToolsList("")
	case settingsViewMCP:
		s.rebuildMCPList()
	case settingsViewMCPDetail:
		content := s.buildMCPDetail(s.selectedMCPName, s.mcpDetailViewport.Width())
		s.mcpDetailViewport.SetContent(content)
		s.mcpDetailViewport.GotoTop()
	case settingsViewSkills:
		entries, _ := s.com.Workspace.ListSkills(context.Background())
		s.skillsAll = entries
		s.skillsInput.SetValue("")
		s.skillsInput.Focus()
		s.rebuildSkillsList("")
	}
}

func (s *Settings) gotoParent() {
	switch s.view {
	case settingsViewProvidersLLM, settingsViewWebSearch:
		s.gotoView(settingsViewProviders)
	case settingsViewMCPDetail:
		s.gotoView(settingsViewMCP)
	default:
		s.gotoRoot()
	}
}

func (s *Settings) gotoRoot() {
	s.view = settingsViewRoot
	s.input.Focus()
	s.rebuildRootList(s.input.Value())
}

// ─── List rebuild ──────────────────────────────────────────────────────────

func (s *Settings) rebuildRootList(filter string) {
	items := make([]list.FilterableItem, 0, len(s.sections))
	for _, sec := range s.sections {
		sec := sec
		items = append(items, &settingsSectionItem{
			Versioned: list.NewVersioned(),
			id:        sec.id, name: sec.name, desc: sec.desc,
			shortcut: sec.shortcut, dialogID: sec.dialogID, subView: sec.subView,
			t: s.com.Styles,
		})
	}
	s.rootList.SetItems(items...)
	s.rootList.SetFilter(filter)
	s.rootList.ScrollToTop()
	if filter == "" {
		s.rootList.SetSelected(0)
	}
}

func (s *Settings) rebuildProviderList() {
	items := make([]list.FilterableItem, 0, len(s.providerSections))
	for _, sec := range s.providerSections {
		sec := sec
		items = append(items, &settingsSectionItem{
			Versioned: list.NewVersioned(),
			id:        sec.id, name: sec.name, desc: sec.desc,
			shortcut: sec.shortcut, dialogID: sec.dialogID, subView: sec.subView,
			t: s.com.Styles,
		})
	}
	s.providerList.SetItems(items...)
	s.providerList.ScrollToTop()
	s.providerList.SetSelected(0)
}

func (s *Settings) rebuildWebSearchList() {
	items := make([]list.FilterableItem, 0, len(defaultWebSearchProviders()))
	for _, provider := range defaultWebSearchProviders() {
		provider := provider
		items = append(items, &settingsWebSearchItem{
			Versioned:  list.NewVersioned(),
			providerID: provider.id,
			name:       provider.name,
			desc:       provider.desc,
			t:          s.com.Styles,
		})
	}
	s.webSearchList.SetItems(items...)
	s.webSearchList.ScrollToTop()
	s.webSearchList.SetSelected(0)
}

func (s *Settings) rebuildProvList(filter string) {
	cfg := s.com.Config()
	items := make([]list.FilterableItem, 0, len(s.providers))
	for _, p := range s.providers {
		p := p
		configured := false
		if cfg != nil {
			if pc, ok := cfg.Providers.Get(string(p.ID)); ok {
				configured = pc.APIKey != "" || pc.OAuthToken != nil || (!providerNeedsAPIKey(string(p.ID)) && pc.BaseURL != "")
			}
		}
		items = append(items, &settingsProviderItem{
			Versioned: list.NewVersioned(),
			provider:  p, configured: configured,
			t: s.com.Styles,
		})
	}
	s.provList.SetItems(items...)
	s.provList.SetFilter(filter)
	s.provList.ScrollToTop()
	s.provList.SetSelected(0)
}

func (s *Settings) rebuildThemeList() {
	cfg := s.com.Config()
	isTransparent := cfg != nil && cfg.Options != nil &&
		cfg.Options.TUI.Transparent != nil && *cfg.Options.TUI.Transparent
	items := []list.FilterableItem{
		&settingsThemeItem{Versioned: list.NewVersioned(), transparent: true, com: s.com},
		&settingsThemeItem{Versioned: list.NewVersioned(), transparent: false, com: s.com},
	}
	s.themeList.SetItems(items...)
	s.themeList.ScrollToTop()
	if isTransparent {
		s.themeList.SetSelected(0)
	} else {
		s.themeList.SetSelected(1)
	}
}

// ─── Draw ──────────────────────────────────────────────────────────────────

func (s *Settings) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := s.com.Styles
	s.windowWidth = area.Dx()

	// Outer width — capped, respects dialog frame.
	width := max(0, min(settingsDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerW := width - t.Dialog.View.GetHorizontalFrameSize()
	inputW := max(0, innerW-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1)

	// Fixed height budget components.
	titleH := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight
	inputH := t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight
	helpH := t.Dialog.HelpView.GetVerticalFrameSize()
	viewFrameH := t.Dialog.View.GetVerticalFrameSize()
	listMarginH := t.Dialog.List.GetVerticalMargins() // = 1 (bottom margin)

	// Structural constants for the consistent layout every view uses:
	//   sep(1) + subtitle(1) + blank(1) = subBlock
	//   sep(1) above help = sepAbove
	const subBlock = 3
	const sepAbove = 1

	// Thin horizontal separator (header separator style).
	sep := t.Header.Separator.Render(strings.Repeat("─", innerW))

	// ── Fixed height: all sub-views use the same max dimensions as the root ──

	// Overhead per view type (determines how much height the list content gets).
	//   search views (root/providers): title+input+subBlock+sepAbove+help+frame+listMargin
	//   theme (no input):              title+subBlock+sepAbove+help+frame+listMargin
	//   info views (no input, no sub): title+sep+sepAbove+help+frame
	var overhead int
	switch s.view {
	case settingsViewRoot, settingsViewProvidersLLM, settingsViewTools, settingsViewSkills:
		overhead = titleH + inputH + subBlock + sepAbove + helpH + viewFrameH + listMarginH
	case settingsViewProviders, settingsViewTheme, settingsViewWebSearch, settingsViewMCP:
		overhead = titleH + subBlock + sepAbove + helpH + viewFrameH + listMarginH
	default:
		overhead = titleH + 1 + sepAbove + helpH + viewFrameH
	}

	maxTermH := max(0, area.Dy()-t.Dialog.View.GetVerticalBorderSize())
	height := max(overhead+1, min(settingsDialogMaxHeight, maxTermH))
	finalContentH := max(1, height-overhead)

	// Phase 3 — set final list sizes.
	switch s.view {
	case settingsViewRoot:
		s.rootList.SetSize(innerW, finalContentH)
	case settingsViewProviders:
		s.providerList.SetSize(innerW, finalContentH)
	case settingsViewProvidersLLM:
		s.provList.SetSize(innerW, finalContentH)
	case settingsViewTheme:
		s.themeList.SetSize(innerW, finalContentH)
	case settingsViewWebSearch:
		s.webSearchList.SetSize(innerW, finalContentH)
	case settingsViewTools:
		s.toolsList.SetSize(innerW, finalContentH)
	case settingsViewMCP:
		s.mcpList.SetSize(innerW, finalContentH)
	case settingsViewMCPDetail:
		s.mcpDetailViewport.SetWidth(innerW)
		s.mcpDetailViewport.SetHeight(finalContentH)
	case settingsViewSkills:
		s.skillsList.SetSize(innerW, finalContentH)
	}

	// ── Build render context ──────────────────────────────────────────────────

	rc := NewRenderContext(t, width)
	orange := lipgloss.NewStyle().Bold(true).Foreground(t.Logo.FieldColor)
	rc.Parts = []string{rc.TitleStyle.Render(orange.Render(s.viewTitle()))}

	switch s.view {
	case settingsViewRoot:
		s.input.SetWidth(inputW)
		rc.AddPart(t.Dialog.InputPrompt.Render(s.input.View()))
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render("  choose a section"))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.rootList.Height()).Render(s.rootList.Render()))

	case settingsViewProviders:
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render("  choose a provider family"))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.providerList.Height()).Render(s.providerList.Render()))

	case settingsViewProvidersLLM:
		s.provInput.SetWidth(inputW)
		rc.AddPart(t.Dialog.InputPrompt.Render(s.provInput.View()))
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render("  select a provider — configure first, then choose a model"))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.provList.Height()).Render(s.provList.Render()))

	case settingsViewTheme:
		// Theme has no search input — sep → subtitle → blank → list.
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render("  choose a background style"))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.themeList.Height()).Render(s.themeList.Render()))

	case settingsViewWebSearch:
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render("  choose and configure a web search provider"))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.webSearchList.Height()).Render(s.webSearchList.Render()))

	case settingsViewTools:
		s.toolsInput.SetWidth(inputW)
		rc.AddPart(t.Dialog.InputPrompt.Render(s.toolsInput.View()))
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render(
			fmt.Sprintf("  %d tools — ↑↓ navigate · type to filter", len(s.toolsAll)),
		))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.toolsList.Height()).Render(s.toolsList.Render()))

	case settingsViewSkills:
		s.skillsInput.SetWidth(inputW)
		rc.AddPart(t.Dialog.InputPrompt.Render(s.skillsInput.View()))
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render(
			fmt.Sprintf("  %d skills — ↑↓ navigate · type to filter", len(s.skillsAll)),
		))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.skillsList.Height()).Render(s.skillsList.Render()))

	case settingsViewMCP:
		cfg := s.com.Config()
		serverCount := 0
		if cfg != nil {
			serverCount = len(cfg.MCP)
		}
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.SecondaryText.Render(
			fmt.Sprintf("  %d servers — ↑↓ navigate · enter to inspect", serverCount),
		))
		rc.Parts = append(rc.Parts, "")
		rc.AddPart(t.Dialog.List.Height(s.mcpList.Height()).Render(s.mcpList.Render()))

	case settingsViewMCPDetail:
		rc.AddPart(sep)
		rc.AddPart(t.Dialog.List.Height(finalContentH).Render(s.mcpDetailViewport.View()))
	}

	rc.Parts = append(rc.Parts, sep)
	rc.Help = s.help.View(s)

	view := rc.Render()
	cur := s.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

func (s *Settings) viewTitle() string {
	switch s.view {
	case settingsViewProviders:
		return "Settings  ›  Providers"
	case settingsViewProvidersLLM:
		return "Settings  ›  Providers  ›  LLM"
	case settingsViewTheme:
		return "Settings  ›  Theme"
	case settingsViewWebSearch:
		return "Settings  ›  Providers  ›  Web Search"
	case settingsViewTools:
		return "Settings  ›  Tools"
	case settingsViewMCP:
		return "Settings  ›  MCP"
	case settingsViewMCPDetail:
		return "Settings  ›  MCP  ›  " + s.selectedMCPName
	case settingsViewSkills:
		return "Settings  ›  Skills"
	default:
		return "Settings"
	}
}

// ─── Info builders ─────────────────────────────────────────────────────────

func (s *Settings) infoWebSearch() []string {
	t := s.com.Styles
	accent := lipgloss.NewStyle().Foreground(t.Logo.FieldColor).Bold(true)
	muted := t.Sidebar.WorkingDir
	return []string{
		"",
		muted.Render("  Web search is available to all agents via the web_search tool."),
		muted.Render("  Configure a provider below to enable search."),
		"",
		accent.Render("  Providers:"),
		muted.Render("    Tavily, Exa, Jina, LangSearch — API key required"),
		muted.Render("    SearXNG — self-hosted, no third-party key required"),
		"",
		muted.Render("  Press esc or ← to go back."),
	}
}

func (s *Settings) infoMCP() []string {
	t := s.com.Styles
	accent := lipgloss.NewStyle().Foreground(t.Logo.FieldColor).Bold(true)
	muted := t.Sidebar.WorkingDir
	cfg := s.com.Config()
	states := s.com.Workspace.MCPGetStates()

	// Config file locations the user edits to add/remove MCP servers.
	globalCfg := config.GlobalConfig()
	globalData := config.GlobalConfigData()
	configPaths := []string{globalCfg}
	if globalData != globalCfg {
		configPaths = append(configPaths, globalData)
	}

	if cfg == nil || len(cfg.MCP) == 0 {
		lines := []string{
			"",
			accent.Render("  MCP servers"),
			muted.Render("    No MCP servers configured."),
			"",
			muted.Render("  Add servers to one of these files:"),
		}
		for _, path := range configPaths {
			lines = append(lines, muted.Render("    "+path))
		}
		lines = append(lines, "", muted.Render("  Press esc or ← to go back."))
		return lines
	}
	lines := []string{"", accent.Render(fmt.Sprintf("  Configured MCP servers: %d", len(cfg.MCP)))}
	if len(configPaths) == 1 {
		lines = append(lines, muted.Render("  Config: "+configPaths[0]))
	} else {
		lines = append(lines, muted.Render("  Config: "+configPaths[0]))
		for _, path := range configPaths[1:] {
			lines = append(lines, muted.Render("         "+path))
		}
	}
	lines = append(lines, "", accent.Render("  Current runtime state:"))
	for _, server := range cfg.MCP.Sorted() {
		state, ok := states[server.Name]
		if !ok {
			lines = append(lines, muted.Render("    "+server.Name+" — offline"))
			continue
		}
		status := "offline"
		switch state.State {
		case mcp.StateStarting:
			status = "starting"
		case mcp.StateConnected:
			status = fmt.Sprintf("connected · %d tools · %d prompts · %d resources", state.Counts.Tools, state.Counts.Prompts, state.Counts.Resources)
		case mcp.StateError:
			status = "error"
			if state.Error != nil {
				status += ": " + state.Error.Error()
			}
		case mcp.StateDisabled:
			status = "disabled"
		}
		lines = append(lines, muted.Render("    "+server.Name+" — "+status))
	}
	lines = append(lines, "", muted.Render("  Press esc or ← to go back."))
	return lines
}

// ─── settingsMCPItem ───────────────────────────────────────────────────────────

type settingsMCPItem struct {
	*list.Versioned
	name    string
	info    mcp.ClientInfo
	focused bool
	match   fuzzy.Match
	t       *styles.Styles
}

func (i *settingsMCPItem) Filter() string { return i.name }
func (i *settingsMCPItem) ID() string     { return i.name }
func (i *settingsMCPItem) Finished() bool { return false }

func (i *settingsMCPItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.Bump()
}

func (i *settingsMCPItem) SetMatch(m fuzzy.Match) {
	i.match = m
	i.Bump()
}

func (i *settingsMCPItem) Render(width int) string {
	t := i.t
	style := t.Dialog.NormalItem
	if i.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
	}
	style = style.Width(width)
	hpad := style.GetHorizontalPadding()
	lineWidth := width - hpad

	const prefix = "    "
	const prefixW = 4

	// Status badge at far right — use attribute-only ANSI codes (fg reset = \x1b[39m)
	// so the outer background is unbroken, matching settingsSectionItem's approach.
	var statusText string
	var statusFg color.Color
	switch i.info.State {
	case mcp.StateConnected:
		statusFg = t.ToolCallSuccess.GetForeground()
		if i.info.Counts.Tools > 0 {
			statusText = fmt.Sprintf("✓ %d tools", i.info.Counts.Tools)
		} else {
			statusText = "✓ connected"
		}
	case mcp.StateStarting:
		statusFg = t.Tool.IconPending.GetForeground()
		statusText = "● starting"
	case mcp.StateError:
		statusFg = t.Tool.IconError.GetForeground()
		statusText = "✗ error"
	case mcp.StateDisabled:
		statusFg = t.Sidebar.WorkingDir.GetForeground()
		statusText = "○ disabled"
	default:
		statusFg = t.Sidebar.WorkingDir.GetForeground()
		statusText = "– offline"
	}
	fgOn := ansi.Style{}.ForegroundColor(statusFg).String()
	fgOff := ansi.Style{}.ForegroundColor(nil).String()
	infoText := " " + fgOn + statusText + fgOff + "  "
	infoWidth := lipgloss.Width(infoText)

	boldOn := ansi.Style{}.Bold().String()
	boldOff := ansi.Style{}.Normal().String()
	nameStr := boldOn + i.name + boldOff
	nameWidth := lipgloss.Width(i.name)

	gap := strings.Repeat(" ", max(0, lineWidth-prefixW-nameWidth-infoWidth))
	return style.Render(prefix + nameStr + gap + infoText)
}

// buildSkillGroups converts a flat CatalogEntry slice into sorted toolGroups
// keyed by source (Built-in, Project, User).
func buildSkillGroups(t *styles.Styles, entries []skills.CatalogEntry) []toolGroup {
	bySource := make(map[string][]*toolItem)
	for _, entry := range entries {
		src := string(entry.Source)
		name := entry.Name
		if entry.UserInvocable {
			name = "/" + name
		}
		bySource[src] = append(bySource[src], &toolItem{
			Versioned: list.NewVersioned(),
			name:      name,
			desc:      entry.Description,
			t:         t,
			cache:     make(map[int]string),
		})
	}
	// Emit groups in preferred source order; sort items within each group.
	preferred := []struct{ key, label string }{
		{string(skills.SourceSystem), "Built-in"},
		{string(skills.SourceProject), "Project"},
		{string(skills.SourceUser), "User"},
	}
	groups := make([]toolGroup, 0, len(bySource))
	seen := make(map[string]bool)
	for _, p := range preferred {
		items, ok := bySource[p.key]
		if !ok {
			continue
		}
		seen[p.key] = true
		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
		groups = append(groups, toolGroup{
			Versioned: list.NewVersioned(),
			category:  p.label,
			items:     items,
			t:         t,
		})
	}
	// Any unknown source types appended at end.
	for src, items := range bySource {
		if seen[src] {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
		groups = append(groups, toolGroup{
			Versioned: list.NewVersioned(),
			category:  src,
			items:     items,
			t:         t,
		})
	}
	return groups
}

// ─── help.KeyMap ──────────────────────────────────────────────────────────

func (s *Settings) ShortHelp() []key.Binding {
	switch s.view {
	case settingsViewRoot:
		return []key.Binding{s.keyMap.Next, s.keyMap.Select, s.keyMap.Close}
	case settingsViewMCPDetail:
		return []key.Binding{s.keyMap.Next, s.keyMap.Back}
	default:
		return []key.Binding{s.keyMap.Next, s.keyMap.Select, s.keyMap.Back}
	}
}

func (s *Settings) FullHelp() [][]key.Binding {
	switch s.view {
	case settingsViewRoot:
		return [][]key.Binding{{s.keyMap.Next, s.keyMap.Previous, s.keyMap.Select}, {s.keyMap.Close}}
	case settingsViewMCPDetail:
		return [][]key.Binding{{s.keyMap.Next, s.keyMap.Previous}, {s.keyMap.Back}}
	default:
		return [][]key.Binding{{s.keyMap.Next, s.keyMap.Previous, s.keyMap.Select}, {s.keyMap.Back}}
	}
}

// ─── settingsSectionItem ───────────────────────────────────────────────────

type settingsSectionItem struct {
	*list.Versioned
	id, name, desc, shortcut, dialogID string
	subView                            settingsView
	focused                            bool
	match                              fuzzy.Match
	t                                  *styles.Styles
}

func (i *settingsSectionItem) Filter() string { return i.name + " " + i.desc }
func (i *settingsSectionItem) ID() string     { return i.id }
func (i *settingsSectionItem) Finished() bool { return true }

func (i *settingsSectionItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.Bump()
}

func (i *settingsSectionItem) SetMatch(m fuzzy.Match) {
	i.match = m
	i.Bump()
}

// Render builds a full-width row: bold name on the left, muted description
// and shortcut on the right. Inline ANSI attribute codes (not full resets)
// are used for bold and grey-fg so that the outer style's orange background
// is never interrupted — this is what produces the continuous highlight bar.
//
// Items are indented by 3 spaces so they nest visually under the "choose a
// section" subtitle (which sits at a 2-space indent), matching the layout
// from the original command palette design.
func (i *settingsSectionItem) Render(width int) string {
	t := i.t
	style := t.Dialog.NormalItem
	scStyle := t.Dialog.ListItem.InfoBlurred
	if i.focused {
		// Soft selection: dark warm-orange background, normal text — readable, not harsh.
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
		scStyle = t.Dialog.ListItem.InfoFocused
	}
	// Width must be set explicitly so lipgloss fills the full row with background color.
	style = style.Width(width)

	// Available inner width (style has Padding(0,1) = 2 chars total).
	hpad := style.GetHorizontalPadding()
	lineWidth := width - hpad

	// 4-space indent prefix: combined with the 1-char left padding from the
	// style, items sit at col 5 from the dialog edge — visually nested under
	// the "choose a section" subtitle at col 3.
	const prefix = "    "
	const prefixW = 4

	// Right: shortcut in info style (pre-rendered, placed at far-right edge).
	var infoText string
	infoWidth := 0
	if i.shortcut != "" {
		infoText = scStyle.Render(" " + i.shortcut + " ")
		infoWidth = lipgloss.Width(infoText)
	}

	// Name: bold via attribute-only codes so the outer background is unbroken.
	// ansi.Style{}.Bold().String()   = "\x1b[1m"  (bold on)
	// ansi.Style{}.Normal().String() = "\x1b[22m" (intensity reset, NOT a full reset)
	boldOn := ansi.Style{}.Bold().String()
	boldOff := ansi.Style{}.Normal().String()
	nameStr := boldOn + i.name + boldOff
	nameWidth := lipgloss.Width(i.name) // visual width ignores escape codes

	// Description: grey foreground via attribute-only codes.
	// ansi.Style{}.ForegroundColor(c).String()      = "\x1b[38;2;r;g;bm"  (set fg)
	// ansi.Style{}.ForegroundColor(nil).String() = "\x1b[39m"          (reset fg only)
	var descStr string
	descWidth := 0
	if i.desc != "" {
		greyColor := t.Sidebar.WorkingDir.GetForeground()
		greyOn := ansi.Style{}.ForegroundColor(greyColor).String()
		greyOff := ansi.Style{}.ForegroundColor(nil).String()
		const sep = "  "
		maxDesc := lineWidth - prefixW - nameWidth - len(sep) - infoWidth - 1
		if maxDesc > 2 {
			desc := ansi.Truncate(i.desc, maxDesc, "…")
			descStr = sep + greyOn + desc + greyOff
			descWidth = len(sep) + lipgloss.Width(desc)
		}
	}

	// Gap fills remaining space; comes between left content and right info.
	gap := strings.Repeat(" ", max(0, lineWidth-prefixW-nameWidth-descWidth-infoWidth))

	// Single Render call: outer style applies bg/fg uniformly; inner ANSI codes
	// only toggle attributes without full resets, so the orange bg is unbroken.
	return style.Render(prefix + nameStr + descStr + gap + infoText)
}

// ─── settingsProviderItem ──────────────────────────────────────────────────

type settingsProviderItem struct {
	*list.Versioned
	provider   catwalk.Provider
	configured bool
	focused    bool
	match      fuzzy.Match
	t          *styles.Styles
}

func (i *settingsProviderItem) Filter() string { return i.provider.Name + " " + string(i.provider.ID) }
func (i *settingsProviderItem) ID() string     { return string(i.provider.ID) }
func (i *settingsProviderItem) Finished() bool { return false }

func (i *settingsProviderItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.Bump()
}

func (i *settingsProviderItem) SetMatch(m fuzzy.Match) {
	i.match = m
	i.Bump()
}

func (i *settingsProviderItem) Render(width int) string {
	t := i.t
	style := t.Dialog.NormalItem
	if i.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
	}
	style = style.Width(width)

	hpad := style.GetHorizontalPadding()
	lineWidth := width - hpad

	const prefix = "    "
	const prefixW = 4

	// Status badge at far right.
	var statusStyle lipgloss.Style
	var statusText string
	if i.configured {
		statusStyle = lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground())
		statusText = "✓ configured"
	} else {
		statusStyle = lipgloss.NewStyle().Foreground(t.Tool.IconError.GetForeground())
		statusText = "✗ not configured"
	}
	infoText := statusStyle.Render(" "+statusText) + "  "
	infoWidth := lipgloss.Width(infoText)

	// Provider name: bold via attribute-only codes.
	boldOn := ansi.Style{}.Bold().String()
	boldOff := ansi.Style{}.Normal().String()
	nameStr := boldOn + i.provider.Name + boldOff
	nameWidth := lipgloss.Width(i.provider.Name)

	gap := strings.Repeat(" ", max(0, lineWidth-prefixW-nameWidth-infoWidth))
	return style.Render(prefix + nameStr + gap + infoText)
}

// ─── settingsThemeItem ─────────────────────────────────────────────────────

// settingsThemeItem represents one background style option in the Theme sub-view.
// It reads the live config on every Render so the active indicator (●/○) always
// reflects the current setting without requiring an explicit list rebuild.
type settingsThemeItem struct {
	*list.Versioned
	// transparent=true → "Terminal background" option; false → "Solid background".
	transparent bool
	com         *common.Common
	focused     bool
	match       fuzzy.Match
}

func (i *settingsThemeItem) Filter() string {
	if i.transparent {
		return "terminal background transparent theme"
	}
	return "solid background dark theme"
}
func (i *settingsThemeItem) ID() string {
	if i.transparent {
		return "theme_transparent"
	}
	return "theme_solid"
}

// Finished returns false so the list always calls Render and picks up config changes.
func (i *settingsThemeItem) Finished() bool { return false }

func (i *settingsThemeItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.Bump()
}

func (i *settingsThemeItem) SetMatch(m fuzzy.Match) {
	i.match = m
	i.Bump()
}

func (i *settingsThemeItem) Render(width int) string {
	t := i.com.Styles
	style := t.Dialog.NormalItem
	if i.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
	}
	style = style.Width(width)

	// Determine whether this option is currently active.
	cfg := i.com.Config()
	isTransparent := cfg != nil && cfg.Options != nil &&
		cfg.Options.TUI.Transparent != nil && *cfg.Options.TUI.Transparent
	isActive := i.transparent == isTransparent

	hpad := style.GetHorizontalPadding()
	lineWidth := width - hpad

	const prefix = "    "
	const prefixW = 4

	// Name and description.
	var name, desc string
	if i.transparent {
		name = "Terminal background"
		desc = "use your terminal's own color scheme (default)"
	} else {
		name = "Solid background"
		desc = "use nexus built-in dark color scheme"
	}

	boldOn := ansi.Style{}.Bold().String()
	boldOff := ansi.Style{}.Normal().String()
	nameStr := boldOn + name + boldOff
	nameWidth := lipgloss.Width(name)

	// Active indicator at far right: ● green when active, nothing when inactive.
	// Placed at the right edge like the provider status badge so it never
	// interrupts the orange background fill.
	var infoText string
	infoWidth := 0
	if isActive {
		checkStyle := lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground())
		infoText = checkStyle.Render("●") + "  "
		infoWidth = 3
	}

	// Description in grey using fg-only ANSI codes (preserves outer background).
	greyColor := t.Sidebar.WorkingDir.GetForeground()
	greyOn := ansi.Style{}.ForegroundColor(greyColor).String()
	greyOff := ansi.Style{}.ForegroundColor(nil).String()
	const sep = "  "
	maxDesc := lineWidth - prefixW - nameWidth - len(sep) - infoWidth - 1
	var descStr string
	descWidth := 0
	if maxDesc > 2 {
		desc = ansi.Truncate(desc, maxDesc, "…")
		descStr = sep + greyOn + desc + greyOff
		descWidth = len(sep) + lipgloss.Width(desc)
	}

	gap := strings.Repeat(" ", max(0, lineWidth-prefixW-nameWidth-descWidth-infoWidth))
	return style.Render(prefix + nameStr + descStr + gap + infoText)
}

type webSearchProviderOption struct {
	id   string
	name string
	desc string
}

func defaultWebSearchProviders() []webSearchProviderOption {
	return []webSearchProviderOption{
		{id: "auto", name: "Auto", desc: "try configured providers in priority order"},
		{id: "tavily", name: "Tavily", desc: "API key required"},
		{id: "exa", name: "Exa", desc: "API key required"},
		{id: "jina", name: "Jina", desc: "API key required"},
		{id: "langsearch", name: "LangSearch", desc: "API key required"},
		{id: "searxng", name: "SearXNG", desc: "Base URL required"},
	}
}

func currentWebSearchProviderID() string {
	providerID := strings.ToLower(strings.TrimSpace(os.Getenv("WEB_SEARCH_PROVIDER")))
	if providerID == "" {
		return "auto"
	}
	return providerID
}

func webSearchConfigured(providerID string) bool {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "auto":
		return true
	case "tavily":
		return strings.TrimSpace(os.Getenv("TAVILY_API_KEY")) != ""
	case "exa":
		return strings.TrimSpace(os.Getenv("EXA_API_KEY")) != ""
	case "jina":
		return strings.TrimSpace(os.Getenv("JINA_API_KEY")) != ""
	case "langsearch":
		return strings.TrimSpace(os.Getenv("LANGSEARCH_API_KEY")) != ""
	case "searxng":
		return strings.TrimSpace(os.Getenv("SEARXNG_BASE_URL")) != ""
	default:
		return false
	}
}

// settingsWebSearchItem represents one web-search provider in settings.
type settingsWebSearchItem struct {
	*list.Versioned
	providerID string
	name       string
	desc       string
	focused    bool
	match      fuzzy.Match
	t          *styles.Styles
}

func (i *settingsWebSearchItem) Filter() string { return i.name + " " + i.desc }
func (i *settingsWebSearchItem) ID() string     { return i.providerID }
func (i *settingsWebSearchItem) Finished() bool { return false }

func (i *settingsWebSearchItem) SetFocused(f bool) {
	if i.focused == f {
		return
	}
	i.focused = f
	i.Bump()
}

func (i *settingsWebSearchItem) SetMatch(m fuzzy.Match) {
	i.match = m
	i.Bump()
}

func (i *settingsWebSearchItem) Render(width int) string {
	t := i.t
	style := t.Dialog.NormalItem
	if i.focused {
		style = t.Dialog.SelectedItem.
			Background(lipgloss.Color(settingsCardSelectedBg)).
			Foreground(t.Dialog.NormalItem.GetForeground())
	}
	style = style.Width(width)
	hpad := style.GetHorizontalPadding()
	lineWidth := width - hpad
	const prefix = "    "
	const prefixW = 4

	active := currentWebSearchProviderID() == i.providerID
	configured := webSearchConfigured(i.providerID)
	statusStyle := lipgloss.NewStyle().Foreground(t.Sidebar.WorkingDir.GetForeground())
	statusText := "not configured"
	if configured {
		statusStyle = lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground())
		statusText = "configured"
	}
	if active {
		statusStyle = lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground()).Bold(true)
		statusText = "active"
	}
	infoText := statusStyle.Render(" "+statusText) + "  "
	infoWidth := lipgloss.Width(infoText)

	boldOn := ansi.Style{}.Bold().String()
	boldOff := ansi.Style{}.Normal().String()
	nameStr := boldOn + i.name + boldOff
	nameWidth := lipgloss.Width(i.name)

	var descStr string
	descWidth := 0
	if i.desc != "" {
		greyColor := t.Sidebar.WorkingDir.GetForeground()
		greyOn := ansi.Style{}.ForegroundColor(greyColor).String()
		greyOff := ansi.Style{}.ForegroundColor(nil).String()
		const sep = "  "
		maxDesc := lineWidth - prefixW - nameWidth - len(sep) - infoWidth - 1
		if maxDesc > 2 {
			desc := ansi.Truncate(i.desc, maxDesc, "…")
			descStr = sep + greyOn + desc + greyOff
			descWidth = len(sep) + lipgloss.Width(desc)
		}
	}
	gap := strings.Repeat(" ", max(0, lineWidth-prefixW-nameWidth-descWidth-infoWidth))
	return style.Render(prefix + nameStr + descStr + gap + infoText)
}
