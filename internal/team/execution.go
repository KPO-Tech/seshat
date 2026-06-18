package team

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
)

// NewSessionHandler returns a MessageHandler that runs a live engine session
// for each incoming mailbox message. The parent engine is forked per message
// so concurrent agent sessions remain fully isolated (each gets its own Loop).
//
// Usage:
//
//	bus := team.NewTeamBus(registry, mb, team.NewSessionHandler(eng), 2*time.Second)
//	bus.Start(ctx)
func NewSessionHandler(eng *engine.Engine) MessageHandler {
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
