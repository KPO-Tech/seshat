package dialog

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/config"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/util"
	searchproviders "github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/exp/charmtone"
)

const WebSearchConfigID = "web_search_config"

type webSearchConfigState int

const (
	webSearchConfigStateInitial webSearchConfigState = iota
	webSearchConfigStateVerifying
	webSearchConfigStateVerified
	webSearchConfigStateError
)

type webSearchConfigStateMsg struct {
	state webSearchConfigState
	err   error
}

type WebSearchConfig struct {
	com *common.Common

	providerID   string
	providerName string
	fieldKey     string
	fieldLabel   string
	existing     string
	width        int
	state        webSearchConfigState
	err          error

	keyMap struct {
		Submit key.Binding
		Close  key.Binding
	}
	input   textinput.Model
	spinner spinner.Model
	help    help.Model
}

var _ Dialog = (*WebSearchConfig)(nil)

func NewWebSearchConfig(com *common.Common, providerID string) (*WebSearchConfig, error) {
	m := &WebSearchConfig{
		com:        com,
		providerID: strings.ToLower(strings.TrimSpace(providerID)),
		width:      68,
	}

	switch m.providerID {
	case "tavily":
		m.providerName, m.fieldKey, m.fieldLabel, m.existing = "Tavily", "api_key", "API key", strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	case "exa":
		m.providerName, m.fieldKey, m.fieldLabel, m.existing = "Exa", "api_key", "API key", strings.TrimSpace(os.Getenv("EXA_API_KEY"))
	case "jina":
		m.providerName, m.fieldKey, m.fieldLabel, m.existing = "Jina", "api_key", "API key", strings.TrimSpace(os.Getenv("JINA_API_KEY"))
	case "langsearch":
		m.providerName, m.fieldKey, m.fieldLabel, m.existing = "LangSearch", "api_key", "API key", strings.TrimSpace(os.Getenv("LANGSEARCH_API_KEY"))
	case "searxng":
		m.providerName, m.fieldKey, m.fieldLabel, m.existing = "SearXNG", "base_url", "Base URL", strings.TrimSpace(os.Getenv("SEARXNG_BASE_URL"))
		if m.existing == "" {
			m.existing = "http://localhost:8080"
		}
	default:
		return nil, fmt.Errorf("unsupported web search provider %q", providerID)
	}

	innerWidth := m.width - com.Styles.Dialog.View.GetHorizontalFrameSize() - 2
	inputWidth := max(0, innerWidth-com.Styles.Dialog.InputPrompt.GetHorizontalFrameSize()-1)
	m.input = textinput.New()
	m.input.SetVirtualCursor(false)
	m.input.SetStyles(com.Styles.TextInput)
	m.input.SetWidth(inputWidth)
	if m.fieldKey == "api_key" {
		m.input.Placeholder = "Enter your API key..."
		if m.existing != "" {
			m.input.Placeholder = "Leave blank to keep your saved API key"
		}
	} else {
		m.input.Placeholder = "Enter the service base URL"
		m.input.SetValue(m.existing)
	}
	m.input.Focus()

	m.spinner = spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(com.Styles.Dialog.APIKey.Spinner),
	)
	m.help = help.New()
	m.help.Styles = com.Styles.DialogHelpStyles()
	m.keyMap.Submit = key.NewBinding(key.WithKeys("enter", "ctrl+y"), key.WithHelp("enter", "submit"))
	m.keyMap.Close = CloseKey
	return m, nil
}

func (m *WebSearchConfig) ID() string { return WebSearchConfigID }

func (m *WebSearchConfig) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case webSearchConfigStateMsg:
		m.state = msg.state
		m.err = msg.err
		if m.state == webSearchConfigStateInitial || m.state == webSearchConfigStateError {
			m.input.Focus()
		}
		if m.state == webSearchConfigStateVerifying {
			return ActionCmd{tea.Batch(m.spinner.Tick, m.verify)}
		}
	case spinner.TickMsg:
		if m.state == webSearchConfigStateVerifying {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			if cmd != nil {
				return ActionCmd{cmd}
			}
		}
	case tea.KeyPressMsg:
		switch {
		case m.state == webSearchConfigStateVerifying:
			return nil
		case key.Matches(msg, m.keyMap.Close):
			if m.state == webSearchConfigStateVerified {
				return m.save()
			}
			return ActionClose{}
		case key.Matches(msg, m.keyMap.Submit):
			if m.state == webSearchConfigStateVerified {
				return m.save()
			}
			return webSearchConfigStateMsg{state: webSearchConfigStateVerifying}
		default:
			m.state = webSearchConfigStateInitial
			m.err = nil
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			if cmd != nil {
				return ActionCmd{cmd}
			}
		}
	case tea.PasteMsg:
		m.state = webSearchConfigStateInitial
		m.err = nil
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if cmd != nil {
			return ActionCmd{cmd}
		}
	}
	return nil
}

func (m *WebSearchConfig) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	textStyle := t.Dialog.SecondaryText
	helpStyle := t.Dialog.HelpView.Width(m.width - t.Dialog.View.Width(m.width).GetHorizontalFrameSize())
	inputStyle := t.Dialog.InputPrompt
	contentParts := []string{
		m.headerView(),
		textStyle.Render("  " + m.fieldLabel),
		inputStyle.Render(m.inputView()),
	}
	if m.fieldKey == "api_key" && m.existing != "" {
		contentParts = append(contentParts, textStyle.Render("  Leave the field blank to keep the saved credential."))
	}
	if m.fieldKey == "base_url" && m.existing != "" {
		contentParts = append(contentParts, textStyle.Render("  Default endpoint: "+m.existing))
	}
	if m.state == webSearchConfigStateError && m.err != nil {
		contentParts = append(contentParts, t.Dialog.TitleError.Render(m.err.Error()))
	}
	contentParts = append(contentParts, "", helpStyle.Render(m.help.View(m)))
	view := t.Dialog.View.Width(m.width).Render(strings.Join(contentParts, "\n"))
	cur := m.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

func (m *WebSearchConfig) headerView() string {
	t := m.com.Styles
	titleStyle := t.Dialog.Title
	dialogStyle := t.Dialog.View.Width(m.width)
	headerOffset := titleStyle.GetHorizontalFrameSize() + dialogStyle.GetHorizontalFrameSize()
	return common.DialogTitle(t, titleStyle.Render(m.dialogTitle()), m.width-headerOffset, t.Dialog.TitleGradFromColor, t.Dialog.TitleGradToColor)
}

func (m *WebSearchConfig) dialogTitle() string {
	t := m.com.Styles
	textStyle := t.Dialog.TitleText
	errorStyle := t.Dialog.TitleError
	accentStyle := t.Dialog.TitleAccent
	switch m.state {
	case webSearchConfigStateInitial:
		return textStyle.Render("Configure ") + accentStyle.Render(m.providerName) + textStyle.Render(" web search")
	case webSearchConfigStateVerifying:
		return textStyle.Render("Validating ") + accentStyle.Render(m.providerName) + textStyle.Render("...")
	case webSearchConfigStateVerified:
		return accentStyle.Render(m.providerName) + textStyle.Render(" configuration validated.")
	case webSearchConfigStateError:
		return errorStyle.Render("Invalid ") + accentStyle.Render(m.providerName) + errorStyle.Render(" configuration.")
	default:
		return ""
	}
}

func (m *WebSearchConfig) inputView() string {
	t := m.com.Styles
	switch m.state {
	case webSearchConfigStateInitial:
		m.input.Prompt = "> "
		m.input.SetStyles(t.TextInput)
		m.input.Focus()
	case webSearchConfigStateVerifying:
		ts := t.TextInput
		ts.Blurred.Prompt = ts.Focused.Prompt
		m.input.Prompt = m.spinner.View()
		m.input.SetStyles(ts)
		m.input.Blur()
	case webSearchConfigStateVerified:
		ts := t.TextInput
		ts.Blurred.Prompt = ts.Focused.Prompt
		m.input.Prompt = styles.CheckIcon + " "
		m.input.SetStyles(ts)
		m.input.Blur()
	case webSearchConfigStateError:
		ts := t.TextInput
		ts.Focused.Prompt = ts.Focused.Prompt.Foreground(charmtone.Cherry)
		m.input.Prompt = styles.LSPErrorIcon + " "
		m.input.SetStyles(ts)
		m.input.Focus()
	}
	return m.input.View()
}

func (m *WebSearchConfig) Cursor() *tea.Cursor {
	if m.state == webSearchConfigStateVerifying || m.state == webSearchConfigStateVerified {
		return nil
	}
	cur := InputCursor(m.com.Styles, m.input.Cursor())
	if cur != nil {
		cur.Y++ // account for the field label line rendered above the input
	}
	return cur
}

func (m *WebSearchConfig) ShortHelp() []key.Binding {
	return []key.Binding{m.keyMap.Submit, m.keyMap.Close}
}

func (m *WebSearchConfig) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}

func (m *WebSearchConfig) verify() tea.Msg {
	start := time.Now()
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		value = strings.TrimSpace(m.existing)
	}

	var err error
	if value == "" {
		err = fmt.Errorf("%s is required", strings.ToLower(m.fieldLabel))
	} else {
		input := searchproviders.SearchInput{Query: "nexus"}
		switch m.providerID {
		case "tavily":
			_, err = searchproviders.NewTavilyProviderWithAPIKey(value).Search(input)
		case "exa":
			_, err = searchproviders.NewExaProviderWithAPIKey(value).Search(input)
		case "jina":
			_, err = searchproviders.NewJinaProviderWithAPIKey(value).Search(input)
		case "langsearch":
			_, err = searchproviders.NewLangSearchProviderWithAPIKey(value).Search(input)
		case "searxng":
			_, err = searchproviders.NewSearXNGProviderWithBaseURL(value).Search(input)
		default:
			err = fmt.Errorf("unsupported provider %s", m.providerID)
		}
	}

	elapsed := time.Since(start)
	if elapsed < 750*time.Millisecond {
		time.Sleep(750*time.Millisecond - elapsed)
	}
	if err == nil {
		return webSearchConfigStateMsg{state: webSearchConfigStateVerified}
	}
	return webSearchConfigStateMsg{state: webSearchConfigStateError, err: err}
}

func (m *WebSearchConfig) save() Action {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		value = strings.TrimSpace(m.existing)
	}

	var err error
	switch m.fieldKey {
	case "api_key":
		if strings.TrimSpace(m.input.Value()) != "" {
			err = m.com.Workspace.SetConfigField(config.ScopeGlobal, fmt.Sprintf("web_search.%s.api_key", m.providerID), value)
		}
	case "base_url":
		err = m.com.Workspace.SetConfigField(config.ScopeGlobal, fmt.Sprintf("web_search.%s.base_url", m.providerID), value)
	}
	if err != nil {
		return ActionCmd{util.ReportError(err)}
	}
	return ActionSelectWebSearchProvider{ProviderID: m.providerID}
}
