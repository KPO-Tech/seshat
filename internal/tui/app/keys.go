package app

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components"
	clipboard "github.com/atotto/clipboard"
)

// pendingSubmitMsg is used to queue a prompt while session creation is pending.
// openSettingsMsg triggers the provider settings panel — sent on first run when
// no provider is configured.
type openSettingsMsg struct{}

type pendingSubmitMsg struct{ prompt string }

// clearCopyNoticeMsg clears the transient "Copied!" footer message.
type clearCopyNoticeMsg struct{}

// cfgSaveResultMsg is sent after attempting to save a provider credential.
type cfgSaveResultMsg struct{ err error }

// providerConfigLoadedMsg carries a refreshed provider list.
type providerConfigLoadedMsg struct{ providers []tui.ProviderStatus }

type sessionDeleteResultMsg struct {
	id  string
	err error
}

// searchConfigLoadedMsg carries the refreshed search configuration.
type searchConfigLoadedMsg struct{ config tui.SearchConfig }

// searchKeySaveResultMsg is sent after attempting to save a search provider key.
type searchKeySaveResultMsg struct{ err error }

// searchModeSaveResultMsg is sent after attempting to save the search mode.
type searchModeSaveResultMsg struct{ err error }

// handleKey processes a keypress. Returns (consumed, cmd):
//   - consumed=true  → key was handled; do NOT forward to textarea
//   - consumed=false → key was not handled; forward to textarea for normal input
func (m *Model) handleKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	k := msg.String()
	stroke := msg.Keystroke()

	if stroke == "ctrl+shift+c" {
		if text := m.chat.SelectedText(); text != "" {
			return true, m.copyToClipboard(text, "Selection copied")
		}
		return true, nil
	}

	// ── Global quit/cancel — always handled before any overlay state ─────
	// These must come first so ctrl+c / ctrl+q work even when a picker or
	// panel is open (those state blocks all do "return true, nil" and would
	// swallow the key otherwise).
	switch k {
	case "ctrl+c":
		if m.busy {
			m.workspace.Cancel()
			m.cancelling = true
			return true, nil
		}
		m.cancel()
		return true, tea.Quit
	case "ctrl+q":
		m.cancel()
		return true, tea.Quit
	}

	// ── Permission dialog (all keys consumed) ────────────────────────────
	if m.state == statePermission && m.permission.HasPending() {
		// Scroll keys are handled by the dialog itself first.
		if m.permission.HandleKey(k) {
			return true, nil
		}
		switch {
		case k == "y" || k == "Y":
			m.permission.Resolve(true, false)
			m.state = stateChat
		case k == "n" || k == "N" || k == "esc":
			m.permission.Resolve(false, true)
			m.state = stateChat
		case k == "a" || k == "A":
			m.permission.Resolve("always", false)
			m.state = stateChat
		default:
			m.permInput += k
		}
		return true, nil
	}

	// ── Model selection (all keys consumed) ─────────────────────────────
	if m.state == stateModelSelect {
		switch k {
		case "esc", "ctrl+m":
			if m.returnState == stateCommands {
				m.refreshSettingsHubData()
				m.state = stateCommands
				m.commands.Open("")
			} else {
				m.state = m.prevChatState()
			}
		case "left":
			// Navigate back to the settings hub if that's where we came from,
			// otherwise close to the chat/welcome state.
			if m.returnState == stateCommands {
				m.refreshSettingsHubData()
				m.state = stateCommands
				m.commands.Open("")
			} else {
				m.state = m.prevChatState()
			}
		case "up":
			m.modelSelect.Up()
		case "down":
			m.modelSelect.Down()
		case "enter":
			if sel := m.modelSelect.Selected(); sel != nil {
				m.workspace.SetModel(sel.Provider, sel.Identifier)
				m.state = m.prevChatState()
			}
		case "backspace":
			m.modelSelect.DeleteFilter()
		default:
			if len(k) == 1 {
				m.modelSelect.TypeFilter(k)
			}
		}
		return true, nil
	}

	// ── Settings hub (all keys consumed) ────────────────────────────────
	if m.state == stateCommands {
		switch k {
		case "esc", "ctrl+p":
			if !m.commands.Back() {
				m.state = m.prevChatState()
			}
		case "left":
			if !m.commands.Back() {
				m.state = m.prevChatState()
			}
		case "up":
			m.commands.Up()
		case "down":
			m.commands.Down()
		case "enter":
			return true, m.activateSettingsSelection()
		case "backspace":
			m.commands.DeleteFilter()
		default:
			if len(k) == 1 {
				m.commands.TypeFilter(k)
			}
		}
		return true, nil
	}

	// ── Provider config panel (all keys consumed) ───────────────────────
	if m.state == stateProviderConfig {
		cp := m.configPanel
		if cp.IsEditing() {
			switch k {
			case "esc":
				m.state = m.prevChatState()
			case "left":
				cp.ExitEdit()
				// Reload provider status after editing.
				return true, m.loadProviderConfig()
			case "up":
				cp.Up()
			case "down":
				cp.Down()
			case "tab":
				cp.Down()
			case "enter":
				draft, _, fieldKey := cp.CurrentFieldDraft()
				if strings.TrimSpace(draft) == "" {
					return true, nil
				}
				providerID := cp.EditedProviderID()
				return true, func() tea.Msg {
					err := m.workspace.SaveProviderField(m.ctx, providerID, fieldKey, strings.TrimSpace(draft))
					if err != nil {
						return cfgSaveResultMsg{err: err}
					}
					return cfgSaveResultMsg{}
				}
			case "backspace":
				cp.DeleteChar()
			case "ctrl+v":
				if text, err := clipboard.ReadAll(); err == nil && text != "" {
					cp.TypeString(text)
				}
				return true, nil
			case "ctrl+r":
				cp.ToggleReveal()
			default:
				if len(k) == 1 {
					cp.TypeChar(k)
				}
			}
		} else {
			switch k {
			case "esc", "ctrl+,":
				if m.returnState == stateCommands {
					m.refreshSettingsHubData()
					m.state = stateCommands
					m.commands.Open("")
				} else {
					m.state = m.prevChatState()
				}
			case "up":
				cp.Up()
			case "down":
				cp.Down()
			case "enter":
				cp.EnterEdit()
			case "backspace":
				cp.DeleteFilter()
			default:
				if len(k) == 1 {
					cp.TypeFilter(k)
				}
			}
		}
		return true, nil
	}

	// ── Search config panel (all keys consumed) ──────────────────────────
	if m.state == stateSearchConfig {
		sp := m.searchPanel
		switch {
		case sp.IsEditingKey():
			switch k {
			case "esc":
				m.state = m.prevChatState()
			case "left":
				sp.ExitKeyEdit()
			case "enter":
				draft, dbKey := sp.CurrentDraft()
				if strings.TrimSpace(draft) == "" {
					sp.ExitKeyEdit()
					return true, nil
				}
				return true, func() tea.Msg {
					err := m.workspace.SaveSearchKey(m.ctx, dbKey, strings.TrimSpace(draft))
					return searchKeySaveResultMsg{err: err}
				}
			case "backspace":
				sp.DeleteChar()
			case "ctrl+v":
				if text, err := clipboard.ReadAll(); err == nil && text != "" {
					sp.TypeString(text)
				}
				return true, nil
			case "ctrl+r":
				sp.ToggleReveal()
			default:
				if len(k) == 1 {
					sp.TypeChar(k)
				}
			}
		case sp.IsEditingMode():
			switch k {
			case "esc":
				m.state = m.prevChatState()
			case "left":
				sp.ExitModeEdit()
			case "up":
				sp.Up()
			case "down":
				sp.Down()
			case "enter":
				chosen := sp.ConfirmMode()
				if chosen != "" {
					return true, func() tea.Msg {
						err := m.workspace.SaveSearchMode(m.ctx, chosen)
						return searchModeSaveResultMsg{err: err}
					}
				}
			}
		default:
			switch k {
			case "esc":
				if m.returnState == stateCommands {
					m.refreshSettingsHubData()
					m.state = stateCommands
					m.commands.Open("")
				} else {
					m.state = m.prevChatState()
				}
			case "up":
				sp.Up()
			case "down":
				sp.Down()
			case "enter":
				sp.EnterList()
			}
		}
		return true, nil
	}

	// ── Session browser (all keys consumed) ─────────────────────────────
	if m.state == stateSessions {
		switch k {
		case "esc", "ctrl+s":
			m.state = m.prevChatState()
		case "up":
			m.sessions.Up()
		case "down":
			m.sessions.Down()
		case "enter":
			id := m.sessions.Selected()
			if id != "" {
				m.state = stateChat
				return true, m.loadSession(id)
			}
		case "d", "delete":
			id := m.sessions.DeleteSelected()
			if id != "" {
				if id == m.activeSession {
					m.activeSession = ""
					m.lastTurnErr = ""
					m.lastErr = nil
					m.busy = false
					m.chat.Clear()
					m.state = stateWelcome
				}
				return true, m.deleteSession(id)
			}
		case "backspace":
			m.sessions.DeleteFilter()
		default:
			if len(k) == 1 {
				m.sessions.TypeFilter(k)
			}
		}
		return true, nil
	}

	// ── Global shortcuts (always consumed) ──────────────────────────────
	// Note: ctrl+c and ctrl+q are already handled at the top of handleKey.
	switch k {
	case "ctrl+p":
		if m.state != stateCommands {
			m.refreshSettingsHubData()
			m.commands.Open("")
			m.state = stateCommands
		}
		return true, nil
	case "ctrl+,":
		if m.state != stateProviderConfig {
			m.state = stateProviderConfig
			return true, m.loadProviderConfig()
		}
		return true, nil
	case "ctrl+s":
		if m.state == stateChat || m.state == stateWelcome {
			m.state = stateSessions
			return true, m.loadSessions()
		}
	case "ctrl+n":
		return true, m.createSession()
	case "ctrl+m":
		if m.state != stateModelSelect {
			m.returnState = m.prevChatState()
			m.state = stateModelSelect
			m.modelSelect.ClearFilter()
			return true, m.listModels()
		}
	case "tab":
		// Tab toggles between editor focus (typing) and main focus (scrolling).
		if m.state == stateChat {
			if m.focus == uiFocusEditor {
				m.focus = uiFocusMain
				m.input.Blur()
			} else {
				m.focus = uiFocusEditor
				return true, m.input.Focus()
			}
			return true, nil
		}
	case "ctrl+o":
		if m.state == stateChat || m.state == stateWelcome {
			opened := m.chat.ToggleDetails()
			*m = m.relayout()
			// Auto-switch focus so arrow keys scroll the sidebar immediately
			// without requiring an extra Tab press.
			if opened {
				m.focus = uiFocusMain
				m.input.Blur()
			} else if m.focus == uiFocusMain {
				m.focus = uiFocusEditor
				return true, m.input.Focus()
			}
			return true, boolCmd(opened)
		}
		return false, nil
	case "esc":
		if m.busy && (m.state == stateChat || m.state == stateWelcome) {
			m.workspace.Cancel()
			m.cancelling = true
			return true, nil
		}
	}

	// ── Chat / welcome: dispatch by focus state (crush pattern) ──────────
	if m.state == stateChat || m.state == stateWelcome {

		// When focus is on the chat list, arrow keys scroll rather than move cursor.
		if m.focus == uiFocusMain {
			switch k {
			case "up":
				if m.chat.DetailsOpen() {
					m.chat.DetailScrollUp(3)
				} else {
					m.chat.ScrollUp(3)
				}
				return true, nil
			case "down":
				if m.chat.DetailsOpen() {
					m.chat.DetailScrollDown(3)
				} else {
					m.chat.ScrollDown(3)
				}
				return true, nil
			case "pgup":
				if m.chat.DetailsOpen() {
					m.chat.DetailPageUp()
				} else {
					m.chat.PageUp()
				}
				return true, nil
			case "pgdown":
				if m.chat.DetailsOpen() {
					m.chat.DetailPageDown()
				} else {
					m.chat.PageDown()
				}
				return true, nil
			case "home":
				if m.chat.DetailsOpen() {
					m.chat.DetailGotoTop()
				} else {
					m.chat.GotoTop()
				}
				return true, nil
			case "end":
				if m.chat.DetailsOpen() {
					m.chat.DetailGotoBottom()
				} else {
					m.chat.GotoBottom()
				}
				return true, nil
			case "n":
				return true, boolCmd(m.chat.SelectNextTool())
			case "p":
				return true, boolCmd(m.chat.SelectPrevTool())
			case "space":
				return true, boolCmd(m.chat.ToggleSelectedToolExpanded())
			case "o", "enter", "right":
				opened := m.chat.ToggleDetails()
				*m = m.relayout()
				return true, boolCmd(opened)
			case "left", "esc":
				if m.chat.DetailsOpen() {
					m.chat.CloseDetails()
					*m = m.relayout()
					// Return focus to editor when sidebar closes.
					m.focus = uiFocusEditor
					return true, m.input.Focus()
				}
				if !m.busy {
					m.state = stateWelcome
				}
				return true, nil
			}
			m.focus = uiFocusEditor
			return true, m.input.Focus()
		}
		// ── Editor focus (default) ────────────────────────────────────────

		// Slash-skill suggestions intercept keys while open.
		if m.skillCompletions.IsOpen() {
			switch k {
			case "esc":
				m.skillCompletions.Close()
				return true, nil
			case "up":
				m.skillCompletions.Up()
				return true, nil
			case "down":
				m.skillCompletions.Down()
				return true, nil
			case "enter", "tab":
				if sel := m.skillCompletions.Selected(); sel != "" {
					m.input.SetValue(sel + " ")
					m.input.CursorEnd()
					m.skillCompletions.Close()
					*m = m.resizeInput()
					return true, nil
				}
				m.skillCompletions.Close()
				return false, nil
			default:
				return false, nil
			}
		}

		// File completions popup intercepts keys while open.
		if m.completions.IsOpen() {
			switch k {
			case "esc":
				m.completions.Close()
			case "up":
				m.completions.Up()
			case "down":
				m.completions.Down()
			case "enter", "tab":
				if sel := m.completions.Selected(); sel != "" {
					query := m.completions.Query()
					val := m.input.Value()
					atIdx := strings.LastIndex(val, "@"+query)
					if atIdx >= 0 {
						m.input.SetValue(val[:atIdx] + sel + val[atIdx+len("@"+query):])
					}
					m.completions.Close()
				}
			case "backspace":
				m.completions.Backspace()
			default:
				if len(k) == 1 && k != "@" {
					m.completions.TypeChar(k)
				} else {
					m.completions.Close()
					return false, nil
				}
			}
			return true, nil
		}

		switch k {
		case "esc":
			if !m.busy {
				m.state = stateWelcome
				return true, nil
			}

		case "/":
			// Slash is reserved for skills. Let the textarea receive it directly.
			return false, nil

		case "@":
			// Open completions AND let textarea receive @ to show it in the input.
			// Only trigger in chat/welcome state AND when a model is configured.
			// Never in config panels where @ might be part of a credential or email.
			if (m.state == stateChat || m.state == stateWelcome) && m.workspace.ModelString() != "" {
				m.completions.Open(m.workspace.WorkingDir())
			}
			// Fall through to textarea (consumed=false) so @ appears in input.
			return false, nil

		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" || m.busy {
				return true, nil
			}
			if m.activeSession == "" {
				return true, tea.Batch(m.createSession(), func() tea.Msg {
					return pendingSubmitMsg{prompt: text}
				})
			}
			atts := m.attachments.List()
			_ = atts
			m.attachments.Reset()
			m.input.Reset()
			*m = m.resizeInput()
			m.chat.AddUserMessage(text)
			m.workspace.Submit(m.ctx, text)
			m.syncComposerAssist()
			return true, nil

		case "shift+enter", "alt+enter":
			// crush uses InsertRune('\n') directly — more reliable than Update(msg).
			m.input.InsertRune('\n')
			return true, nil

		case "ctrl+t":
			// Toggle thinking block collapse on the most recent assistant message.
			m.chat.ToggleThinking()
			return true, nil

		case "ctrl+u":
			// Copy last user message to clipboard.
			text := m.chat.GetLastUserText()
			if text != "" {
				return true, m.copyToClipboard(text, "Message copied")
			}
			return true, nil

		case "ctrl+a":
			return true, nil

		case "pgup":
			m.chat.PageUp()
			return true, nil
		case "pgdown":
			m.chat.PageDown()
			return true, nil
		case "home":
			m.chat.GotoTop()
			return true, nil
		case "end":
			m.chat.GotoBottom()
			return true, nil
		}
	}

	// Key was not handled — forward to the textarea.
	return false, nil
}

func (m *Model) activateSettingsSelection() tea.Cmd {
	sel := m.commands.Selected()
	if sel == nil {
		return nil
	}
	switch sel.Kind {
	case components.PaletteSectionKind:
		m.commands.OpenSection(sel.ID)
		return nil
	case components.PaletteRouteKind:
		switch sel.ID {
		case "providers":
			m.returnState = stateCommands
			m.state = stateProviderConfig
			return m.loadProviderConfig()
		case "models":
			m.returnState = stateCommands
			m.state = stateModelSelect
			m.modelSelect.ClearFilter()
			return m.listModels()
		case "search":
			m.returnState = stateCommands
			m.state = stateSearchConfig
			return m.loadSearchConfig()
		}
	case components.PaletteActionKind:
		cmd := m.executeCommand(sel.ID)
		if m.state == stateCommands {
			m.state = m.prevChatState()
		}
		return cmd
	case components.PaletteInfoKind:
		if strings.HasPrefix(sel.Name, "/") {
			return m.insertSkillIntoComposer(sel.Name)
		}
		return nil
	}
	return nil
}

func (m *Model) insertSkillIntoComposer(skill string) tea.Cmd {
	m.state = m.prevChatState()
	m.focus = uiFocusEditor
	m.input.SetValue(skill + " ")
	m.input.CursorEnd()
	*m = m.resizeInput()
	return m.input.Focus()
}

func (m *Model) executeCommand(id string) tea.Cmd {
	switch id {
	case "new-session":
		return m.createSession()
	case "sessions":
		m.state = stateSessions
		return m.loadSessions()
	case "model":
		m.returnState = stateCommands
		m.state = stateModelSelect
		m.modelSelect.ClearFilter()
		return m.listModels()
	case "thinking":
		m.chat.ToggleThinking()
		return nil
	case "copy-msg":
		text := m.chat.GetLastUserText()
		if text != "" {
			return m.copyToClipboard(text, "Message copied")
		}
		return nil
	case "toggle-verbose-steps":
		m.chat.SetVerboseInterim(!m.chat.VerboseInterim())
		m.refreshSettingsHubData()
		return nil
	case "provider-config":
		m.state = stateProviderConfig
		return m.loadProviderConfig()
	case "quit":
		m.cancel()
		return tea.Quit
	default:
		return nil
	}
}

func editorPrompt(styles common.Styles) func(textarea.PromptInfo) string {
	return func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			if info.Focused {
				return styles.InputPrompt.Render("> ")
			}
			return styles.InputHint.Render("> ")
		}
		return "  "
	}
}
