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
//	bus := team.NewTeamBus(registry, mb, team.NewSessionHandler(eng, factory), 2*time.Second)
//	bus.Start(ctx)
func NewSessionHandler(eng *engine.Engine, toolFactory AgentToolFactory) MessageHandler {
	return func(ctx context.Context, profile agent.AgentProfile, msg mailbox.Message) {
		agentEngine := eng.Fork(profile.SystemPrompt(), profile.Model)
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
