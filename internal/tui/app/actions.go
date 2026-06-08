package app

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components"
	clipboard "github.com/atotto/clipboard"
)

// ─── Clipboard ───────────────────────────────────────────────────────────────

// copyToClipboard copies text using OSC 52 (tea.SetClipboard) and the native
// clipboard (atotto/clipboard), then shows a transient notice in the footer.
func (m *Model) copyToClipboard(text, notice string) tea.Cmd {
	m.copyNotice = copyNoticeForCapability(notice)
	return tea.Sequence(
		tea.SetClipboard(text),
		func() tea.Msg {
			_ = clipboard.WriteAll(text)
			return nil
		},
		tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return clearCopyNoticeMsg{}
		}),
	)
}

func copyNoticeForCapability(success string) string {
	return clipboardNotice(success, nativeClipboardLikelyAvailable(), terminalClipboardLikelyAvailable())
}

func clipboardNotice(success string, nativeAvailable, terminalAvailable bool) string {
	switch {
	case nativeAvailable:
		return success
	case terminalAvailable:
		return success + " (terminal clipboard requested)"
	default:
		return "Clipboard unavailable: install wl-clipboard or xclip"
	}
}

func nativeClipboardLikelyAvailable() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	}
	for _, name := range []string{"wl-copy", "xclip", "xsel", "pbcopy", "clip.exe", "powershell.exe"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func terminalClipboardLikelyAvailable() bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	return term != "" && term != "dumb"
}

// ─── Workspace commands ───────────────────────────────────────────────────────

func boolCmd(ok bool) tea.Cmd {
	if ok {
		return func() tea.Msg { return nil }
	}
	return nil
}

func (m *Model) syncComposerAssist() {
	if m.completions.IsOpen() {
		m.skillCompletions.Close()
		return
	}
	skills := m.loadSkillCatalog()
	m.skillCompletions.Sync(skills, m.input.Value())
}

func (m *Model) loadSkillCatalog() []tui.SkillInfo {
	if !m.skillCatalogLoaded {
		m.skillCatalog = m.workspace.LoadSkills(m.ctx)
		m.skillCatalogLoaded = true
	}
	return m.skillCatalog
}

func (m Model) loadSessions() tea.Cmd {
	return func() tea.Msg { m.workspace.ListSessions(m.ctx); return nil }
}

func (m Model) listModels() tea.Cmd {
	return func() tea.Msg { m.workspace.ListModels(m.ctx); return nil }
}

func (m Model) createSession() tea.Cmd {
	return func() tea.Msg { m.workspace.CreateSession(m.ctx); return nil }
}

func (m Model) loadSession(id string) tea.Cmd {
	return func() tea.Msg { m.workspace.LoadSession(m.ctx, id); return nil }
}

func (m Model) loadProviderConfig() tea.Cmd {
	return func() tea.Msg {
		providers := m.workspace.LoadProviderConfig(m.ctx)
		return providerConfigLoadedMsg{providers: providers}
	}
}

func (m Model) loadSearchConfig() tea.Cmd {
	return func() tea.Msg {
		cfg := m.workspace.LoadSearchConfig(m.ctx)
		return searchConfigLoadedMsg{config: cfg}
	}
}

func (m *Model) refreshSettingsHubData() {
	m.commands.SetSectionItems("commands", buildCommandSettingsItems(m.chat.VerboseInterim()))
	m.commands.SetSectionItems("tools", buildToolSettingsItems(m.workspace.LoadToolCatalog(m.ctx)))
	m.commands.SetSectionItems("mcp", buildMCPSettingsItems(m.workspace.LoadMCPServers(m.ctx)))
	m.skillCatalog = m.workspace.LoadSkills(m.ctx)
	m.skillCatalogLoaded = true
	m.commands.SetSectionItems("skills", buildSkillSettingsItems(m.skillCatalog))
}

func buildCommandSettingsItems(verboseInterim bool) []components.PaletteItem {
	verboseDesc := "Currently off · Keep assistant step narration compact between tools"
	if verboseInterim {
		verboseDesc = "Currently on · Show full assistant step narration between tools"
	}
	return []components.PaletteItem{
		{Kind: components.PaletteActionKind, ID: "new-session", Name: "New Session", Shortcut: "ctrl+n", Desc: "Start a fresh conversation"},
		{Kind: components.PaletteActionKind, ID: "sessions", Name: "Sessions", Shortcut: "ctrl+s", Desc: "Browse and resume past sessions"},
		{Kind: components.PaletteActionKind, ID: "copy-msg", Name: "Copy Last Message", Shortcut: "ctrl+u", Desc: "Copy your last message to clipboard"},
		{Kind: components.PaletteActionKind, ID: "toggle-verbose-steps", Name: "Verbose Agent Steps", Desc: verboseDesc},
		{Kind: components.PaletteActionKind, ID: "quit", Name: "Quit", Shortcut: "ctrl+c", Desc: "Exit Nexus"},
	}
}

func buildToolSettingsItems(items []tui.ToolInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "tools-empty",
			Name: "No tools found",
			Desc: "The current runtime did not expose any tools",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := strings.TrimSpace(item.Description)
		if category := strings.TrimSpace(item.Category); category != "" {
			if desc != "" {
				desc = category + " · " + desc
			} else {
				desc = category
			}
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "tool-" + item.Name,
			Name: item.Name,
			Desc: desc,
		})
	}
	return result
}

func buildMCPSettingsItems(items []tui.MCPServerInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "mcp-empty",
			Name: "No MCP servers configured",
			Desc: "Add MCP servers in config to expose them here",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := item.Status + " · " + strconv.Itoa(item.ToolsRegistered) + " tools"
		if item.Error != "" {
			desc += " · " + item.Error
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "mcp-" + item.Name,
			Name: item.Name,
			Desc: desc,
		})
	}
	return result
}

func buildSkillSettingsItems(items []tui.SkillInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "skills-empty",
			Name: "No skills found",
			Desc: "Add bundled, repo, or user skills to invoke them with /skill",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = strings.TrimSpace(item.WhenToUse)
		}
		if source := strings.TrimSpace(item.Source); source != "" {
			if desc != "" {
				desc = source + " · " + desc
			} else {
				desc = source
			}
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "skill-" + item.Name,
			Name: "/" + item.Name,
			Desc: desc,
		})
	}
	return result
}

func (m Model) deleteSession(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.workspace.DeleteSession(m.ctx, id)
		m.workspace.ListSessions(m.ctx)
		return sessionDeleteResultMsg{id: id, err: err}
	}
}
