package mailbox_test

import (
	"context"
	"errors"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	mbtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/mailbox"
)

// ─── fake dispatcher ──────────────────────────────────────────────────────────

type fakeDispatcher struct {
	lastSendFrom    string
	lastSendTo      string
	lastAssignRole  string
	lastAssignTeam  string
	lastBroadcastTo string
	lastSubject     string
	lastBody        string
	lastReplyTo     string
	err             error
}

func (f *fakeDispatcher) Send(_ context.Context, from, to, subject, body string) error {
	f.lastSendFrom, f.lastSendTo = from, to
	f.lastSubject, f.lastBody = subject, body
	return f.err
}
func (f *fakeDispatcher) Assign(_ context.Context, from, role, teamID, subject, body string) error {
	f.lastSendFrom = from
	f.lastAssignRole, f.lastAssignTeam = role, teamID
	f.lastSubject, f.lastBody = subject, body
	return f.err
}
func (f *fakeDispatcher) Broadcast(_ context.Context, from, teamID, subject, body string) error {
	f.lastSendFrom, f.lastBroadcastTo = from, teamID
	f.lastSubject, f.lastBody = subject, body
	return f.err
}
func (f *fakeDispatcher) Reply(_ context.Context, from, to, replyTo, subject, body string) error {
	f.lastSendFrom, f.lastSendTo = from, to
	f.lastReplyTo = replyTo
	f.lastSubject, f.lastBody = subject, body
	return f.err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func callInput(parsed map[string]any) tool.CallInput {
	return tool.CallInput{Parsed: parsed}
}

// ─── SendTool ─────────────────────────────────────────────────────────────────

func TestSendTool_DirectByAgentID(t *testing.T) {
	d := &fakeDispatcher{}
	st := mbtool.NewSendTool(d, "agent-from")

	_, err := st.Call(context.Background(), callInput(map[string]any{
		"to_agent_id": "agent-to",
		"subject":     "Fix the bug",
		"body":        "See issue #42.",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.lastSendFrom != "agent-from" {
		t.Errorf("expected from=agent-from, got %q", d.lastSendFrom)
	}
	if d.lastSendTo != "agent-to" {
		t.Errorf("expected to=agent-to, got %q", d.lastSendTo)
	}
	if d.lastSubject != "Fix the bug" {
		t.Errorf("expected subject=%q, got %q", "Fix the bug", d.lastSubject)
	}
}

func TestSendTool_ByRole(t *testing.T) {
	d := &fakeDispatcher{}
	st := mbtool.NewSendTool(d, "manager-1")

	_, err := st.Call(context.Background(), callInput(map[string]any{
		"to_agent_id": "",
		"to_role":     "engineer",
		"to_team":     "alpha",
		"subject":     "Build the feature",
		"body":        "Details here.",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.lastAssignRole != "engineer" {
		t.Errorf("expected role=engineer, got %q", d.lastAssignRole)
	}
	if d.lastAssignTeam != "alpha" {
		t.Errorf("expected team=alpha, got %q", d.lastAssignTeam)
	}
}

func TestSendTool_Reply(t *testing.T) {
	d := &fakeDispatcher{}
	st := mbtool.NewSendTool(d, "agent-from")

	_, err := st.Call(context.Background(), callInput(map[string]any{
		"to_agent_id": "agent-to",
		"reply_to_id": "msg-999",
		"subject":     "Re: Fix the bug",
		"body":        "Done.",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.lastReplyTo != "msg-999" {
		t.Errorf("expected reply_to=msg-999, got %q", d.lastReplyTo)
	}
}

func TestSendTool_ValidationErrors(t *testing.T) {
	d := &fakeDispatcher{}
	st := mbtool.NewSendTool(d, "agent-from")

	cases := []struct {
		name   string
		parsed map[string]any
	}{
		{"missing subject", map[string]any{"to_agent_id": "x", "subject": "", "body": "b"}},
		{"missing body", map[string]any{"to_agent_id": "x", "subject": "s", "body": ""}},
		{"no address", map[string]any{"to_agent_id": "", "to_role": "", "subject": "s", "body": "b"}},
		{"both address", map[string]any{"to_agent_id": "x", "to_role": "eng", "subject": "s", "body": "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, valErr := st.ValidateInput(context.Background(), tc.parsed)
			if valErr == nil {
				t.Errorf("expected validation error for %q", tc.name)
			}
		})
	}
}

func TestSendTool_DispatcherError(t *testing.T) {
	d := &fakeDispatcher{err: errors.New("network failure")}
	st := mbtool.NewSendTool(d, "agent-from")

	result, err := st.Call(context.Background(), callInput(map[string]any{
		"to_agent_id": "agent-to",
		"subject":     "s",
		"body":        "b",
	}), nil)
	if err != nil {
		t.Fatalf("Call itself should not return error: %v", err)
	}
	if !result.IsError() {
		t.Error("expected IsError()=true when dispatcher returns error")
	}
}

// ─── BroadcastTool ───────────────────────────────────────────────────────────

func TestBroadcastTool_FansOut(t *testing.T) {
	d := &fakeDispatcher{}
	bt := mbtool.NewBroadcastTool(d, "manager-1")

	_, err := bt.Call(context.Background(), callInput(map[string]any{
		"team_id": "product",
		"subject": "Sprint kick-off",
		"body":    "Let's go!",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.lastBroadcastTo != "product" {
		t.Errorf("expected team=product, got %q", d.lastBroadcastTo)
	}
	if d.lastSendFrom != "manager-1" {
		t.Errorf("expected from=manager-1, got %q", d.lastSendFrom)
	}
}

func TestBroadcastTool_ValidationErrors(t *testing.T) {
	d := &fakeDispatcher{}
	bt := mbtool.NewBroadcastTool(d, "x")

	cases := []struct {
		name   string
		parsed map[string]any
	}{
		{"missing team_id", map[string]any{"team_id": "", "subject": "s", "body": "b"}},
		{"missing subject", map[string]any{"team_id": "t", "subject": "", "body": "b"}},
		{"missing body", map[string]any{"team_id": "t", "subject": "s", "body": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, valErr := bt.ValidateInput(context.Background(), tc.parsed)
			if valErr == nil {
				t.Errorf("expected validation error for %q", tc.name)
			}
		})
	}
}
