package fetch

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
)

// rebindingResolver answers with a different set of addresses each time it is
// asked, the way a DNS record with a zero TTL does for a rebinding attack.
type rebindingResolver struct {
	mu      sync.Mutex
	answers [][]netip.Addr
	calls   int
}

func (r *rebindingResolver) LookupNetIP(_ context.Context, _ string, _ string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.calls
	if i >= len(r.answers) {
		i = len(r.answers) - 1
	}
	r.calls++
	return r.answers[i], nil
}

func addrs(values ...string) []netip.Addr {
	out := make([]netip.Addr, 0, len(values))
	for _, v := range values {
		out = append(out, netip.MustParseAddr(v))
	}
	return out
}

// recordingDial records what was dialed and returns a connection that is not one.
func recordingDial(dialed *[]string, failFirst bool) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _ string, address string) (net.Conn, error) {
		*dialed = append(*dialed, address)
		if failFirst && len(*dialed) == 1 {
			return nil, errors.New("connection refused")
		}
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
}

// Found by an audit: the policy resolved the name, found it public, and then the
// dialer resolved it again and could be given 127.0.0.1.
func TestTheDialGoesToTheAddressThatWasCheckedNotToAFreshResolution(t *testing.T) {
	resolver := &rebindingResolver{answers: [][]netip.Addr{addrs("93.184.216.34"), addrs("127.0.0.1")}}
	var dialed []string
	dial := guardedDialContext(resolver, recordingDial(&dialed, false))

	conn, err := dial(t.Context(), "tcp", "rebind.example.com:443")
	if err != nil {
		t.Fatalf("a public host must be dialed: %v", err)
	}
	_ = conn.Close()

	if len(dialed) != 1 || dialed[0] != "93.184.216.34:443" {
		t.Errorf("the dial must go to the checked address, got %v", dialed)
	}
	if resolver.calls != 1 {
		t.Errorf("the name must be resolved once, it was resolved %d times: a second resolution is what a rebinding record answers differently", resolver.calls)
	}
}

func TestTheDialRefusesAHostWhoseAnswerIsPrivate(t *testing.T) {
	for name, answer := range map[string][]netip.Addr{
		"loopback":          addrs("127.0.0.1"),
		"private":           addrs("10.0.0.5"),
		"metadata":          addrs("169.254.169.254"),
		"one of several":    addrs("93.184.216.34", "192.168.1.10"),
		"mapped loopback":   addrs("::ffff:127.0.0.1"),
		"ipv6 unique local": addrs("fd00::1"),
	} {
		var dialed []string
		dial := guardedDialContext(&rebindingResolver{answers: [][]netip.Addr{answer}}, recordingDial(&dialed, false))
		if _, err := dial(t.Context(), "tcp", "evil.example.com:80"); err == nil {
			t.Errorf("%s: a host that answers a private address must be refused", name)
		}
		if len(dialed) != 0 {
			t.Errorf("%s: nothing may be dialed, got %v", name, dialed)
		}
	}
}

func TestTheDialRefusesAPrivateLiteralAndLocalNames(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "[::1]:80", "10.1.2.3:443", "localhost:3000", "metadata.google.internal:80"} {
		var dialed []string
		dial := guardedDialContext(&rebindingResolver{answers: [][]netip.Addr{addrs("93.184.216.34")}}, recordingDial(&dialed, false))
		if _, err := dial(t.Context(), "tcp", address); err == nil {
			t.Errorf("%s must be refused", address)
		}
		if len(dialed) != 0 {
			t.Errorf("%s: nothing may be dialed, got %v", address, dialed)
		}
	}
}

func TestTheDialTriesTheNextCheckedAddress(t *testing.T) {
	resolver := &rebindingResolver{answers: [][]netip.Addr{addrs("93.184.216.34", "93.184.216.35")}}
	var dialed []string
	dial := guardedDialContext(resolver, recordingDial(&dialed, true))

	conn, err := dial(t.Context(), "tcp", "two.example.com:443")
	if err != nil {
		t.Fatalf("the second address must be tried when the first is refused: %v", err)
	}
	_ = conn.Close()
	if len(dialed) != 2 || dialed[1] != "93.184.216.35:443" {
		t.Errorf("want the two checked addresses in order, got %v", dialed)
	}
}

func TestTheDialOfAPublicLiteralIsAllowed(t *testing.T) {
	var dialed []string
	dial := guardedDialContext(&rebindingResolver{answers: [][]netip.Addr{addrs("127.0.0.1")}}, recordingDial(&dialed, false))
	conn, err := dial(t.Context(), "tcp", "93.184.216.34:443")
	if err != nil {
		t.Fatalf("a public literal needs no resolution: %v", err)
	}
	_ = conn.Close()
	if len(dialed) != 1 || dialed[0] != "93.184.216.34:443" {
		t.Errorf("got %v", dialed)
	}
}
