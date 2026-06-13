package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/modes"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/workspace"
)

// nexusBrailleLogo is the braille-art rendering of the Nexus logo.
const nexusBrailleLogo = `` +
	`⠀⠀⠀⠀⠀⠀⠀⢀⢠⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⢸⣸⡜⡜⡜⣜⢸⠠⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⢀⣸⡞⡇⡇⡇⡇⡇⡗⡜⢼⢸⢠⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣐⢼⢨⢠⣸⡞⠇⠁⠀⠀⠀⠀⠀⠂⣟⠨⠀⠀⠀⠀` + "\n" +
	`⠀⠀⢀⣿⠁⠀⠀⠀⠀⢰⣸⡜⡜⢼⢺⣣⣷⣼⢸⡼⡜⡟⡇⡇⡇⡇⡟⡜⣜⣿⠫⡇⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⡒⣿⠀⠀⠀⠀` + "\n" +
	`⠀⠀⡒⣿⠀⠀⠀⠀⣒⢯⣰⢼⠀⢰⡺⡏⠇⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠂⣟⠨⠀⠀⠀⡐⣟⣯⡧⣝⠨⠀⣒⡿⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⣗⢬⠀⠀⠀⠀⡗⠇⣿⡎⠇⠀⠀⠀⠀⠀⠀⠀⢀⢸⣸⢼⢸⢸⢸⠠⠀⡂⡝⡞⡏⡇⡗⣿⡧⠇⣒⣿⠀⣾⠍⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⡗⢼⠠⠀⠀⠀⠀⣿⠨⠀⠀⠀⠀⠀⠀⢀⣸⡏⠁⠀⠀⣿⢨⣺⠭⠀⠀⠀⠀⠀⠀⠀⠂⣟⢨⣺⠍⣺⠏⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠂⣿⠌⠀⢠⢰⣻⠍⠀⢰⣸⡜⡜⡜⣾⣝⢸⢸⢠⢠⢸⢻⡿⣽⡜⡜⡜⢼⢨⠀⠀⠀⠀⡗⢯⣺⠏⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⣺⡿⣾⡯⠇⠁⠀⠀⣾⠇⠀⠀⠀⣲⢯⢰⣸⡞⡇⡇⡝⢼⢨⣳⢭⠀⠀⠀⡃⣽⠀⠀⠀⠀⣗⢯⠀⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⢀⣿⠁⠀⠀⠀⠀⣿⠠⠀⢀⢰⣿⡇⠃⠀⠀⠀⠀⠀⠀⠃⡇⣿⢨⠠⠀⢀⣿⠀⠀⠀⠀⠂⣿⠠⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⣒⡿⠀⠀⠀⠀⠀⡂⣝⣸⡏⢓⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠫⡗⢼⡾⠅⠀⠀⠀⠀⠀⣟⠭⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⣒⣽⠀⠀⠀⠀⢰⣸⣾⡷⢼⣢⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⢥⣸⡏⣝⠨⠀⠀⠀⠀⠀⣾⠭⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⠂⣿⠠⠀⠀⠀⣟⢸⣺⠍⠂⡃⣿⢸⢠⠀⠀⠀⠀⠀⠀⢠⢸⣿⠇⠁⠀⠂⣿⠀⠀⠀⠀⢀⣿⠁⠀⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⠀⣳⢽⠀⠀⠀⠀⣟⢩⠀⠀⠀⣓⢯⡃⡗⣜⢸⢸⡼⡏⠇⣳⠯⢰⢸⢨⢰⡿⠀⠀⢀⢰⣺⡿⣾⡯⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠀⣰⡯⣳⢼⠀⠀⠀⠀⡃⡗⡜⡜⡜⣟⡾⡇⡇⠃⠃⡇⡇⣝⡿⡞⢿⢠⣻⠏⠀⣐⣯⠇⠃⠀⡐⣿⠠⠀⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⣰⡯⣐⡯⡃⣽⠠⠀⠀⠀⠀⠀⠀⠀⠂⡗⢼⠠⠀⠀⢀⣸⡏⠁⠀⠂⠃⠁⠀⠀⡂⣿⠀⠀⠀⠀⠂⡗⢼⠀⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⣐⡿⠀⣿⠭⢰⢺⣿⢼⢸⣸⡼⣜⠨⠀⠀⠀⠂⡇⡗⡏⡇⠁⠀⠀⠀⠀⠀⠀⠀⢰⡸⣿⢰⢼⠀⠀⠀⠀⡓⢽⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⣾⠭⠀⡂⣝⢺⣻⣽⠌⠀⠀⠀⡂⣽⠠⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⢰⣸⡮⠇⠀⡗⠏⣳⠭⠀⠀⠀⠀⣿⠬⠀⠀` + "\n" +
	`⠀⠀⠀⠀⣿⠬⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⢸⣢⣿⡝⡜⣼⢸⢸⢸⢸⣼⡜⡞⡇⡟⢿⢫⡧⡗⡜⡜⡏⠇⠀⠀⠀⠀⢀⣿⠁⠀⠀` + "\n" +
	`⠀⠀⠀⠀⡂⣽⠠⠀⠀⠀⠀⠀⢀⢰⡼⡏⠃⡃⡗⠍⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠃⡇⡗⡜⢼⢸⢸⢸⢸⢸⡼⡏⠁⠀⠀⠀` + "\n" +
	`⠀⠀⠀⠀⠀⠂⡇⡝⡜⡜⡜⡏⡇⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠃⠁⠀⠀⠀⠀⠀⠀⠀`

// selectedLargeModel returns the currently selected large language model.
func (m *UI) selectedLargeModel() *workspace.AgentModel {
	if m.com.Workspace.AgentIsReady() {
		model := m.com.Workspace.AgentModel()
		return &model
	}
	return nil
}

// landingView renders the landing page: braille logo + tagline + shortcuts, centered.
func (m *UI) landingView() string {
	t := m.com.Styles
	width := m.layout.main.Dx()
	height := m.layout.main.Dy()

	// Braille logo in orange (use Logo.FieldColor which is set to primary)
	orange := lipgloss.NewStyle().Foreground(t.Logo.FieldColor)
	muted := t.Sidebar.WorkingDir
	subtle := t.Section.Title

	logo := orange.Render(nexusBrailleLogo)
	wordmark := orange.Bold(true).Render("NEXUS")
	tagline := muted.Render("One runtime. Any LLM. Any language.")
	cwd := subtle.Render(m.com.Workspace.WorkingDir())

	shortcuts := strings.Join([]string{
		orange.Bold(true).Render("ctrl+n") + " " + muted.Render("new session"),
		orange.Bold(true).Render("ctrl+s") + " " + muted.Render("sessions"),
		orange.Bold(true).Render("ctrl+p") + " " + muted.Render("settings"),
		orange.Bold(true).Render("ctrl+c") + " " + muted.Render("quit"),
	}, "  ")

	block := lipgloss.JoinVertical(lipgloss.Center,
		logo,
		wordmark,
		"",
		tagline,
		"",
		shortcuts,
		"",
		cwd,
	)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(block)
}

// landingHeaderView renders the minimal one-line header for the landing page:
// "NEXUS  provider:model" on the left, "ready" on the right.
func (m *UI) landingHeaderView(width int) string {
	t := m.com.Styles
	orange := lipgloss.NewStyle().Foreground(t.Logo.FieldColor)
	muted := t.Sidebar.WorkingDir

	left := orange.Bold(true).Render("NEXUS")
	model := m.selectedLargeModel()
	if model != nil {
		modelName := model.ModelCfg.Model
		if model.ModelCfg.Provider != "" {
			modelName = model.ModelCfg.Provider + ":" + modelName
		}
		left += "  " + muted.Render(modelName)
	}

	right := lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground()).Render("ready")

	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

// chatHeaderView renders the one-line chat page header:
// "NEXUS  provider:model" on the left, execution mode on the right.
// Mode colors: execute=orange (default), plan=info blue, pair=success green.
// A spinner appears to the left of the mode label while the agent is running.
func (m *UI) chatHeaderView(width int) string {
	t := m.com.Styles
	orange := lipgloss.NewStyle().Foreground(t.Logo.FieldColor)
	muted := t.Sidebar.WorkingDir

	left := orange.Bold(true).Render("NEXUS")
	model := m.selectedLargeModel()
	if model != nil {
		modelName := model.ModelCfg.Model
		if model.ModelCfg.Provider != "" {
			modelName = model.ModelCfg.Provider + ":" + modelName
		}
		left += "  " + muted.Render(modelName)
	}

	mode := m.com.Workspace.ExecutionMode()
	modeLabel := orange.Bold(true).Render("execute")
	switch {
	case modes.IsPlanModeString(mode):
		modeLabel = lipgloss.NewStyle().Foreground(t.LSP.InfoDiagnostic.GetForeground()).Bold(true).Render("plan")
	case modes.IsPairProgrammingModeString(mode):
		modeLabel = lipgloss.NewStyle().Foreground(t.ToolCallSuccess.GetForeground()).Bold(true).Render("pair")
	}

	// Show spinner to the left of the mode label while the agent is running.
	right := modeLabel
	if m.isAgentBusy() {
		spinnerStyle := lipgloss.NewStyle().Foreground(t.Pills.TodoSpinner.GetForeground())
		right = spinnerStyle.Render(m.todoSpinner.View()) + " " + modeLabel
	}

	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return " " + left + strings.Repeat(" ", gap) + right + " "
}
