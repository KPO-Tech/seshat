package nodes

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// allowLocal lets the node reach an httptest server, which listens on 127.0.0.1: the URL check accepts every address and the client does
// not refuse the connection to a loopback address. The redirects are still followed through the client's own policy.
func allowLocal(node *HTTPRequest) {
	node.checkSSRF = func(string) error { return nil }
	node.client = node.guardedClient(false)
}

// A public server can answer with a redirect to an internal address: each hop goes through the guard again, so the second server is
// never reached.
func TestHTTPRequestChecksEveryRedirect(t *testing.T) {
	t.Parallel()
	var internalHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		_, _ = w.Write([]byte("secret"))
	}))
	t.Cleanup(internal.Close)
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	t.Cleanup(public.Close)

	node := NewHTTPRequest()
	node.client = node.guardedClient(false)
	node.checkSSRF = func(rawURL string) error {
		if strings.Contains(rawURL, internal.Listener.Addr().String()) {
			return errors.New("blocked: internal address")
		}
		return nil
	}

	_, err := node.Execute(context.Background(), nil, nil, map[string]any{"url": public.URL})
	if err == nil || !strings.Contains(err.Error(), "blocked: internal address") {
		t.Fatalf("a redirect to a blocked address must fail the call, got %v", err)
	}
	if internalHits.Load() != 0 {
		t.Errorf("the blocked address was reached %d time(s)", internalHits.Load())
	}
}

// The check of the URL happens before the name is resolved; the dialer checks the address the connection is really made to, so a name
// that resolved somewhere else in between (DNS rebinding) cannot reach a loopback address.
func TestHTTPRequestRefusesAConnectionToAnInternalAddress(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("reached"))
	}))
	t.Cleanup(server.Close)

	node := NewHTTPRequest()
	node.checkSSRF = func(string) error { return nil } // the URL looked fine when it was checked

	_, err := node.Execute(context.Background(), nil, nil, map[string]any{"url": server.URL})
	if err == nil || !strings.Contains(err.Error(), "is blocked") {
		t.Fatalf("a connection to 127.0.0.1 must be refused by the dialer, got %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("the loopback server was reached %d time(s)", hits.Load())
	}
}

func TestHTTPRequestStopsAfterTooManyRedirects(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL, http.StatusFound)
	}))
	t.Cleanup(server.Close)

	node := NewHTTPRequest()
	allowLocal(node)

	_, err := node.Execute(context.Background(), nil, nil, map[string]any{"url": server.URL})
	if err == nil || !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("a redirect loop must stop, got %v", err)
	}
}

// NAT64 reaches an IPv4 address through a gateway: 64:ff9b::7f00:1 is 127.0.0.1.
func TestDialerRefusesNAT64AndKeepsPublicAddresses(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe", "127.0.0.1", "::1", "169.254.169.254", "10.1.2.3", "100.100.100.200"} {
		if err := refuseBlockedAddress("tcp", net.JoinHostPort(host, "80"), nil); err == nil {
			t.Errorf("%s must be refused by the dialer", host)
		}
	}
	for _, host := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if err := refuseBlockedAddress("tcp", net.JoinHostPort(host, "443"), nil); err != nil {
			t.Errorf("%s must be allowed: %v", host, err)
		}
	}
}
