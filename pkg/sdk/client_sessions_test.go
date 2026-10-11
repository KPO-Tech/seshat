package sdk

import (
	"context"
	"testing"
)

// These guard H4 in TODO.md: RegisterHook used to compile and return a bare
// id for ANY event name, including ones the runtime never fires (e.g.
// HookEventWorktreeCreate), so a registered handler silently never ran with
// nothing to warn about it.

func TestRegisterHook_RejectsUnimplementedEvent(t *testing.T) {
	client, err := NewClient(&ClientConfig{PersistSessions: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	id, err := client.RegisterHook(HookEventWorktreeCreate, func(ctx context.Context, p HookProgress) (*HookResult, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected RegisterHook to reject an event the runtime never fires")
	}
	if id != "" {
		t.Fatalf("expected an empty id on rejection, got %q", id)
	}
}

func TestRegisterHook_AcceptsImplementedEvent(t *testing.T) {
	client, err := NewClient(&ClientConfig{PersistSessions: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	id, err := client.RegisterHook(HookEventPreToolUse, func(ctx context.Context, p HookProgress) (*HookResult, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("expected RegisterHook to accept an implemented event, got error: %v", err)
	}
	if id == "" {
		t.Error("expected a non-empty hook id")
	}
}
