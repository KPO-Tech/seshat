package team

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

// AgentToolFactory builds per-session tools scoped to a specific agent.
// It is called once per incoming message with the receiving agent's profile ID
// so that tools like mailbox_send can pre-bake the correct sender identity.
type AgentToolFactory func(agentID string) []tool.Tool

// NewSessionHandler returns a MessageHandler that runs a live engine session
// for each incoming mailbox message. The parent engine is forked per message
// so concurrent agent sessions remain fully isolated (each gets its own Loop).
//
// profileReg and teams are used to inject team-awareness into the system prompt
// (agent ID, team name, roster of teammates and their IDs). Either may be nil
// to skip the corresponding enrichment.
//
// toolFactory, when non-nil, is called after session creation to register
// per-agent tools (e.g. mailbox_send, mailbox_broadcast). Pass nil if no
// extra tools are needed.
//
// Usage:
//
//	dispatcher := team.NewDispatcher(reg, mb)
//	factory := func(id string) []tool.Tool {
//	    return []tool.Tool{
//	        mailboxtool.NewSendTool(dispatcher, id),
//	        mailboxtool.NewBroadcastTool(dispatcher, id),
//	    }
//	}
//	handler := team.NewSessionHandler(eng, factory, profileReg, teamReg)
//	bus := team.NewTeamBus(registry, mb, handler, 2*time.Second)
//	bus.Start(ctx)
func NewSessionHandler(eng *engine.Engine, toolFactory AgentToolFactory, profileReg *agent.ProfileRegistry, teams *TeamRegistry) MessageHandler {
	return func(ctx context.Context, profile agent.AgentProfile, msg mailbox.Message) {
		systemPrompt := buildAgentSystemPrompt(ctx, profile, profileReg, teams)
		agentEngine := eng.Fork(systemPrompt, profile.Model)
		session, err := agentEngine.NewSession(ctx)
		if err != nil {
			slog.Error("teambus: create session failed",
				"agent", profile.Nickname,
				"msg_id", msg.ID,
				"err", err,
			)
			return
		}
		defer session.Close()

		if toolFactory != nil {
			for _, t := range toolFactory(profile.ID) {
				if regErr := session.RegisterTool(t); regErr != nil {
					slog.Warn("teambus: register tool failed",
						"agent", profile.Nickname,
						"tool", t.Definition().Name,
						"err", regErr,
					)
				}
			}
		}

		if _, err := session.SubmitMessage(ctx, FormatIncoming(msg)); err != nil {
			slog.Error("teambus: session error",
				"agent", profile.Nickname,
				"msg_id", msg.ID,
				"err", err,
			)
		}
	}
}

// FormatIncoming formats a mailbox message as the opening user turn text
// fed to the agent's session.
func FormatIncoming(msg mailbox.Message) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s] %s", msg.Kind, msg.Subject))
	if body := strings.TrimSpace(msg.Body); body != "" {
		b.WriteString("\n\n")
		b.WriteString(body)
	}
	return b.String()
}

// buildAgentSystemPrompt returns the agent's base system prompt enriched with:
//   - An identity block (name, role, Agent ID / mailbox address)
//   - A team block with the roster of teammates (if the agent has a TeamID and
//     both profileReg and teams are non-nil)
func buildAgentSystemPrompt(ctx context.Context, profile agent.AgentProfile, profileReg *agent.ProfileRegistry, teams *TeamRegistry) string {
	var sb strings.Builder
	sb.WriteString(profile.SystemPrompt())

	sb.WriteString("\n\n## Your identity\n")
	sb.WriteString(fmt.Sprintf("- **Name**: %s\n", profile.Nickname))
	sb.WriteString(fmt.Sprintf("- **Role**: %s\n", profile.Role))
	sb.WriteString(fmt.Sprintf("- **Agent ID** (your mailbox address): `%s`\n", profile.ID))

	if profile.TeamID == "" || profileReg == nil || teams == nil {
		return sb.String()
	}

	t, err := teams.Get(ctx, profile.TeamID)
	if err != nil {
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("\n## Your team: %s (team_id: `%s`)\n", t.Name, t.ID))

	members, err := teams.Members(ctx, profile.TeamID)
	if err != nil || len(members) == 0 {
		return sb.String()
	}

	sb.WriteString("\n| Name | Role | Agent ID |\n")
	sb.WriteString("|------|------|----------|\n")
	for _, m := range members {
		if m.ID == profile.ID {
			continue // omit self from roster
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | `%s` |\n", m.Nickname, m.Role, m.ID))
	}
	sb.WriteString("\nUse these Agent IDs with the `mailbox_send` tool to contact your teammates directly.\n")

	return sb.String()
}
