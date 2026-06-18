// Package team handles multi-agent coordination: routing messages between
// agents (Dispatcher) and polling inboxes to trigger execution (TeamBus).
//
// Dependency chain (no cycles):
//
//	internal/agent  →  (AgentProfile, ProfileRegistry)
//	internal/mailbox →  (Message, Mailbox)
//	internal/team   →  imports both, owns coordination logic
package team

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
)

// Dispatcher routes messages between agents using the mailbox as transport.
type Dispatcher struct {
	registry *agent.ProfileRegistry
	mailbox  mailbox.Mailbox
}

// NewDispatcher creates a Dispatcher wired to the given registry and mailbox.
func NewDispatcher(registry *agent.ProfileRegistry, mb mailbox.Mailbox) *Dispatcher {
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

// Assign delivers a task to the first agent matching the given role,
// optionally scoped to a team. Returns ErrNoAgentForRole when no match found.
func (d *Dispatcher) Assign(ctx context.Context, fromID, role, teamID, subject, body string) error {
	role = strings.ToLower(role)

	var (
		profiles []agent.AgentProfile
		err      error
	)
	if teamID != "" {
		profiles, err = d.registry.FindByTeam(ctx, teamID)
		if err != nil {
			return fmt.Errorf("dispatcher.Assign: list team agents: %w", err)
		}
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

	target := leastLoaded(ctx, profiles, d.mailbox)
	msg := mailbox.NewMessage(mailbox.KindTask, fromID, target.ID, subject, body)
	if teamID != "" {
		msg.TeamID = teamID
	}
	return d.mailbox.Send(ctx, msg)
}

// leastLoaded returns the agent from candidates with the fewest unread messages.
// When multiple agents share the minimum, one is chosen at random for uniform
// spread. Falls back to a random candidate when PendingCounts fails.
func leastLoaded(ctx context.Context, candidates []agent.AgentProfile, mb mailbox.Mailbox) agent.AgentProfile {
	if len(candidates) == 1 {
		return candidates[0]
	}

	ids := make([]string, len(candidates))
	for i, p := range candidates {
		ids[i] = p.ID
	}

	counts, err := mb.PendingCounts(ctx, ids)
	if err != nil {
		// Degrade gracefully: pick at random rather than blocking assignment.
		return candidates[rand.Intn(len(candidates))]
	}

	// Find the minimum pending count across all candidates.
	minCount := -1
	for _, id := range ids {
		c := counts[id] // 0 when absent (no unread messages)
		if minCount < 0 || c < minCount {
			minCount = c
		}
	}

	// Collect all agents tied at the minimum and pick one at random.
	var tied []agent.AgentProfile
	for _, p := range candidates {
		if counts[p.ID] == minCount {
			tied = append(tied, p)
		}
	}
	return tied[rand.Intn(len(tied))]
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
