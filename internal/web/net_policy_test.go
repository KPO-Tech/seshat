package web

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"testing"
)

// What the SSRF guard must refuse: a literal local or private address, in every way of writing it, and the names that mean "this
// machine" or "the internal network". The metadata endpoint of the cloud providers (169.254.169.254) is the reason it exists.
func TestRejectLocalNetworkTargetRefusesLocalTargets(t *testing.T) {
	t.Parallel()
	refused := []string{
		"http://localhost/",
		"http://LOCALHOST:8080/",
		"http://localhost./",
		"http://localhost.localdomain/",
		"http://host.docker.internal/",
		"http://printer.local/",
		"http://metadata.google.internal/",
		"http://metadata.google.internal./computeMetadata/v1/",
		"http://127.0.0.1/",
		"http://127.1.2.3:9200/",
		"http://[::1]/",
		"http://10.0.0.5/",
		"http://172.16.0.1/",
		"http://172.31.255.255/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[fe80::1]/",
		"http://[fd00::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://[::ffff:10.0.0.1]/",
		"http://[::ffff:169.254.169.254]/",
		"http://0.0.0.0/",
		"http://[::]/",
		"http://224.0.0.1/",
		"http://100.64.0.1/",
		"http://100.127.255.254/",
		"http://255.255.255.255/",
		"http://[64:ff9b::7f00:1]/",
	}
	for _, raw := range refused {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := RejectLocalNetworkTarget(parsed); err == nil {
			t.Errorf("%s must be refused", raw)
		}
	}
}

func TestRejectLocalNetworkTargetAllowsPublicTargets(t *testing.T) {
	t.Parallel()
	allowed := []string{
		"https://example.com/",
		"https://93.184.216.34/",
		"https://[2606:2800:220:1:248:1893:25c8:1946]/",
		"https://172.15.0.1/",
		"https://172.32.0.1/",
		"https://100.63.255.255/",
		"https://100.128.0.1/",
		"https://localhost.example.com/",
		"https://notlocal/",
	}
	for _, raw := range allowed {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := RejectLocalNetworkTarget(parsed); err != nil {
			t.Errorf("%s must be allowed: %v", raw, err)
		}
	}
	if err := RejectLocalNetworkTarget(nil); err == nil {
		t.Error("a missing URL must be refused")
	}
}

// A public name that resolves to a private address (DNS rebinding, or a record that points inside) is refused, whatever else it
// resolves to: one private answer among public ones is enough.
func TestResolveAndRejectRefusesAnyPrivateAnswer(t *testing.T) {
	t.Parallel()
	parsed, _ := url.Parse("https://example.com/")
	public := netip.MustParseAddr("93.184.216.34")
	for _, private := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fd12::1", "::ffff:192.168.0.1", "100.64.0.9"} {
		resolver := fakeResolver{addrs: []netip.Addr{public, netip.MustParseAddr(private)}}
		if err := ResolveAndRejectLocalNetworkTarget(context.Background(), parsed, resolver); err == nil {
			t.Errorf("an answer set that holds %s must be refused", private)
		}
	}

	if err := ResolveAndRejectLocalNetworkTarget(context.Background(), parsed, fakeResolver{}); err == nil {
		t.Error("a name that resolves to nothing must be refused")
	}
	failing := fakeResolver{err: errors.New("no such host")}
	if err := ResolveAndRejectLocalNetworkTarget(context.Background(), parsed, failing); err == nil {
		t.Error("a name that does not resolve must be refused")
	}
}

// The dial check is the one that holds when the URL looked fine and the name resolved somewhere else later.
func TestRejectLocalDialTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	public := fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	inside := fakeResolver{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.7")}}

	for _, address := range []string{"localhost:80", "127.0.0.1:443", "[::1]:443", "169.254.169.254:80", "10.0.0.1", "service.internal:8080"} {
		if err := RejectLocalDialTarget(ctx, address, public); err == nil {
			t.Errorf("%s must be refused", address)
		}
	}
	if err := RejectLocalDialTarget(ctx, "example.com:443", inside); err == nil {
		t.Error("a public name that resolves inside must be refused at dial time")
	}
	if err := RejectLocalDialTarget(ctx, "example.com:443", public); err != nil {
		t.Errorf("a public name that resolves to a public address must pass: %v", err)
	}
	if err := RejectLocalDialTarget(ctx, "93.184.216.34:443", inside); err != nil {
		t.Errorf("a literal public address does not need the resolver: %v", err)
	}
	if err := RejectLocalDialTarget(ctx, "", public); err == nil {
		t.Error("an empty target must be refused")
	}
}
