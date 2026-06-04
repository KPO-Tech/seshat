package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
)

// Dispatcher routes messages between agents using the mailbox as transport.
// It sits on top of ProfileRegistry (to resolve agents by role/team) and
// a Mailbox (to deliver messages).
type Dispatcher struct {
	registry *ProfileRegistry
	mailbox  mailbox.Mailbox
}

// NewDispatcher creates a Dispatcher wired to the given registry and mailbox.
func NewDispatcher(registry *ProfileRegistry, mb mailbox.Mailbox) *Dispatcher {
	return &Dispatcher{registry: registry, mailbox: mb}
}

// Send delivers a task message to a specific agent identified by UUID.
func (d *Dispatcher) Send(ctx context.Context, fromID, toID, subject, body string) error {
	if fromID == "" || toID == "" {
		return fmt.Errorf("dispatcher.Send: fromID and toID must not be empty")
	}
	msg := mailbox.NewMessage(mailbox.KindTask, fromID, toID, subject, body)
	return d.mailbox.Send(ctx, msg)
}

// Reply sends a KindReply message in response to an existing message.
func (d *Dispatcher) Reply(ctx context.Context, fromID, toID, replyToID, subject, body string) error {
	if replyToID == "" {
		return fmt.Errorf("dispatcher.Reply: replyToID must not be empty")
	}
	msg := mailbox.NewMessage(mailbox.KindReply, fromID, toID, subject, body)
	msg.ReplyTo = replyToID
	return d.mailbox.Send(ctx, msg)
}

// Broadcast sends a message to all agents in a team.
// The sender (fromID) is excluded from the recipients.
func (d *Dispatcher) Broadcast(ctx context.Context, fromID, teamID, subject, body string) error {
	if teamID == "" {
		return fmt.Errorf("dispatcher.Broadcast: teamID must not be empty")
	}
	msg := mailbox.NewMessage(mailbox.KindBroadcast, fromID, "*", subject, body)
	msg.TeamID = teamID
	return d.mailbox.Send(ctx, msg)
}

// Assign picks the first available agent with the given role (and optionally
// scoped to a team) and delivers a task message to it.
// Returns ErrNoAgentForRole when no matching agent is found.
func (d *Dispatcher) Assign(ctx context.Context, fromID, role, teamID, subject, body string) error {
	role = strings.ToLower(role)

	var (
		profiles []AgentProfile
		err      error
	)
	if teamID != "" {
		profiles, err = d.registry.FindByTeam(ctx, teamID)
		if err != nil {
			return fmt.Errorf("dispatcher.Assign: list team agents: %w", err)
		}
		// Filter by role within the team.
		filtered := profiles[:0]
		for _, p := range profiles {
			if p.Role == role {
				filtered = append(filtered, p)
			}
		}
		profiles = filtered
	} else {
		profiles, err = d.registry.FindByRole(ctx, role)
		if err != nil {
			return fmt.Errorf("dispatcher.Assign: list agents by role: %w", err)
		}
	}

	if len(profiles) == 0 {
		return &ErrNoAgentForRole{Role: role, TeamID: teamID}
	}

	// Simple strategy: pick the first match.
	// Future: load-balance, prefer idle agents, etc.
	target := profiles[0]
	msg := mailbox.NewMessage(mailbox.KindTask, fromID, target.ID, subject, body)
	if teamID != "" {
		msg.TeamID = teamID
	}
	return d.mailbox.Send(ctx, msg)
}

// ErrNoAgentForRole is returned by Assign when no agent matches the criteria.
type ErrNoAgentForRole struct {
	Role   string
	TeamID string
}

func (e *ErrNoAgentForRole) Error() string {
	if e.TeamID != "" {
		return fmt.Sprintf("no agent with role %q in team %q", e.Role, e.TeamID)
	}
	return fmt.Sprintf("no agent with role %q", e.Role)
}
