package web

import (
	"context"
	"net/netip"
	"net/url"
	"testing"
)

// --- from net_policy_test.go ---

type fakeResolver struct {
	addrs []netip.Addr
	err   error
}

func (f fakeResolver) LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error) {
	return f.addrs, f.err
}

func TestResolveAndRejectLocalNetworkTargetRejectsPrivateResolution(t *testing.T) {
	parsed, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	err = ResolveAndRejectLocalNetworkTarget(context.Background(), parsed, fakeResolver{
		addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
	})
	if err == nil {
		t.Fatal("expected private resolution to be rejected")
	}
}

func TestResolveAndRejectLocalNetworkTargetAllowsPublicResolution(t *testing.T) {
	parsed, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	err = ResolveAndRejectLocalNetworkTarget(context.Background(), parsed, fakeResolver{
		addrs: []netip.Addr{netip.MustParseAddr("93.184.216.34")},
	})
	if err != nil {
		t.Fatalf("expected public resolution to pass, got: %v", err)
	}
}

// --- from policy_test.go ---

func TestEvaluatePermissionAllowsPreapprovedFetch(t *testing.T) {
	input := EnrichFetchPermissionInput(map[string]any{
		"url": "https://go.dev/doc/",
	})

	result := EvaluatePermission(input)
	if !result.IsAllowed() {
		t.Fatalf("expected preapproved fetch to be auto-allowed, got %s", result.Behavior)
	}
}

func TestEvaluatePermissionAllowsLocalBrowserSessionActions(t *testing.T) {
	input := EnrichBrowserPermissionInput(map[string]any{
		"page_id": "page-1",
	}, ActionWait, "https://example.com")

	result := EvaluatePermission(input)
	if !result.IsAllowed() {
		t.Fatalf("expected browser wait to be auto-allowed, got %s", result.Behavior)
	}
}

func TestEvaluatePermissionAllowsSearchRestrictedToPreapprovedDomains(t *testing.T) {
	input := EnrichSearchPermissionInput(map[string]any{
		"query":           "site docs",
		"allowed_domains": []any{"go.dev", "developer.mozilla.org"},
	})

	result := EvaluatePermission(input)
	if !result.IsAllowed() {
		t.Fatalf("expected constrained preapproved search to be auto-allowed, got %s", result.Behavior)
	}
}

func TestPermissionMatcherMatchesSharedFields(t *testing.T) {
	input := EnrichBrowserPermissionInput(map[string]any{
		"url": "https://go.dev/doc/",
	}, ActionNavigate, "")

	matcher := PermissionMatcher(input)
	if !matcher("action:navigate") {
		t.Fatal("expected action matcher to match")
	}
	if !matcher("host:go.dev") {
		t.Fatal("expected host matcher to match")
	}
	if !matcher("url:https://go.dev/*") {
		t.Fatal("expected url matcher to match wildcard")
	}
}

func TestCategoryForAction(t *testing.T) {
	if got := categoryForAction(ActionClick); got != CategoryInteract {
		t.Fatalf("expected click category %q, got %q", CategoryInteract, got)
	}
	if got := categoryForAction(ActionFetch); got != CategoryRead {
		t.Fatalf("expected fetch category %q, got %q", CategoryRead, got)
	}
}

func TestEvaluatePermissionAllowsBlankBrowserOpen(t *testing.T) {
	input := EnrichBrowserPermissionInput(map[string]any{}, ActionOpen, "")

	result := EvaluatePermission(input)
	if !result.IsAllowed() {
		t.Fatalf("expected blank browser open to be auto-allowed, got %s", result.Behavior)
	}
}
