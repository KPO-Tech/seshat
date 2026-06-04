package team

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
)

// MessageHandler is called by TeamBus when a message arrives for an agent.
// The handler is responsible for processing the message and sending any reply
// via the Dispatcher. Session execution is wired here by the caller.
type MessageHandler func(ctx context.Context, profile agent.AgentProfile, msg mailbox.Message)

// TeamBus polls every registered agent's mailbox and dispatches incoming
// messages to the configured handler. It is intentionally minimal —
// session execution is wired by the caller via MessageHandler.
type TeamBus struct {
	registry *agent.ProfileRegistry
	mailbox  mailbox.Mailbox
	handler  MessageHandler

	pollInterval time.Duration

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewTeamBus creates a TeamBus. pollInterval controls how often each agent's
// inbox is checked; 0 defaults to 2 seconds.
func NewTeamBus(
	registry *agent.ProfileRegistry,
	mb mailbox.Mailbox,
	handler MessageHandler,
	pollInterval time.Duration,
) *TeamBus {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	return &TeamBus{
		registry:     registry,
		mailbox:      mb,
		handler:      handler,
		pollInterval: pollInterval,
	}
}

// Start begins polling all registered agent inboxes. Non-blocking — each
// agent gets its own goroutine. Calling Start twice is a no-op.
func (b *TeamBus) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return nil
	}

	profiles, err := b.registry.List(ctx)
	if err != nil {
		return err
	}

	busCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.running = true

	for _, p := range profiles {
		b.wg.Add(1)
		go b.pollAgent(busCtx, p)
	}

	slog.Info("teambus started", "agents", len(profiles), "poll_interval", b.pollInterval)
	return nil
}

// Stop shuts down all polling goroutines and waits for them to exit cleanly.
func (b *TeamBus) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.cancel()
	b.running = false
	b.mu.Unlock()

	b.wg.Wait()
	slog.Info("teambus stopped")
}

func (b *TeamBus) pollAgent(ctx context.Context, profile agent.AgentProfile) {
	defer b.wg.Done()
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.drainInbox(ctx, profile)
		}
	}
}

func (b *TeamBus) drainInbox(ctx context.Context, profile agent.AgentProfile) {
	msgs, err := b.mailbox.Receive(ctx, profile.ID)
	if err != nil {
		slog.Warn("teambus: receive error", "agent", profile.Nickname, "err", err)
		return
	}
	for _, msg := range msgs {
		// Mark read before invoking the handler so a handler panic doesn't
		// cause infinite redelivery on the next poll cycle.
		if err := b.mailbox.MarkRead(ctx, msg.ID); err != nil {
			slog.Warn("teambus: mark-read error", "msg_id", msg.ID, "err", err)
		}
		if b.handler != nil {
			b.handler(ctx, profile, msg)
		}
	}
}
