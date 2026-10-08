package netdiag

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func stepByName(steps []Step, name string) (Step, bool) {
	for _, s := range steps {
		if s.Name == name {
			return s, true
		}
	}
	return Step{}, false
}

func TestProbeHTTPSuccessAgainstRealServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := ProbeHTTP(context.Background(), "test target", server.URL, "/")
	if !result.OK {
		t.Fatalf("expected overall success, got %+v", result)
	}
	for _, name := range []string{"dns", "tcp", "http"} {
		step, ok := stepByName(result.Steps, name)
		if !ok || step.Status != StatusPass {
			t.Fatalf("expected step %q to pass, got %+v", name, step)
		}
	}
	tlsStep, ok := stepByName(result.Steps, "tls")
	if !ok || tlsStep.Status != StatusSkipped {
		t.Fatalf("expected tls to be skipped for a plain HTTP target, got %+v", tlsStep)
	}
}

// TestProbeHTTPSucceedsOnNon2xxStatus covers the deliberate design choice:
// the HTTP step's success criterion is "no transport error", never a
// specific status code - a 404/405 still proves the network path works.
func TestProbeHTTPSucceedsOnNon2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	result := ProbeHTTP(context.Background(), "test target", server.URL, "/")
	httpStep, ok := stepByName(result.Steps, "http")
	if !ok || httpStep.Status != StatusPass {
		t.Fatalf("expected the http step to still pass on a non-2xx response, got %+v", httpStep)
	}
	if httpStep.Detail != "HTTP 405" {
		t.Fatalf("expected the status code to be reported in detail, got %q", httpStep.Detail)
	}
}

func TestProbeHTTPFailsDNSForInvalidHost(t *testing.T) {
	result := ProbeHTTP(context.Background(), "test target", "http://this-host-does-not-exist.invalid", "/")
	if result.OK {
		t.Fatal("expected overall failure for an unresolvable host")
	}
	dnsStep, ok := stepByName(result.Steps, "dns")
	if !ok || dnsStep.Status != StatusFail {
		t.Fatalf("expected dns to fail, got %+v", dnsStep)
	}
	if dnsStep.ActOn != ActOnNetworkAdmin {
		t.Fatalf("expected a network_admin suggestion on DNS failure, got %+v", dnsStep)
	}
	for _, name := range []string{"tcp", "tls", "http"} {
		step, ok := stepByName(result.Steps, name)
		if !ok || step.Status != StatusSkipped {
			t.Fatalf("expected step %q to be skipped after a DNS failure, got %+v", name, step)
		}
	}
}

func TestProbeHTTPFailsTCPForClosedPort(t *testing.T) {
	// Bind and immediately close a local port so it's very likely nothing is
	// listening there for the duration of this test.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	result := ProbeHTTP(context.Background(), "test target", "http://"+addr, "/")
	if result.OK {
		t.Fatal("expected overall failure for a closed port")
	}
	tcpStep, ok := stepByName(result.Steps, "tcp")
	if !ok || tcpStep.Status != StatusFail {
		t.Fatalf("expected tcp to fail, got %+v", tcpStep)
	}
	if tcpStep.ActOn != ActOnNetworkAdmin {
		t.Fatalf("expected a network_admin suggestion on TCP failure, got %+v", tcpStep)
	}
	dnsStep, ok := stepByName(result.Steps, "dns")
	if !ok || dnsStep.Status != StatusPass {
		t.Fatalf("expected dns to have passed before tcp was attempted, got %+v", dnsStep)
	}
	httpStep, ok := stepByName(result.Steps, "http")
	if !ok || httpStep.Status != StatusSkipped {
		t.Fatalf("expected http to be skipped after a TCP failure, got %+v", httpStep)
	}
}

func TestProbeHTTPFailsTLSForPlaintextPort(t *testing.T) {
	// A real TCP listener that never speaks TLS - dialing it with an https
	// target must fail the TLS step specifically (TCP already succeeded).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				time.Sleep(50 * time.Millisecond)
				conn.Close()
			}()
		}
	}()

	result := ProbeHTTP(context.Background(), "test target", "https://"+listener.Addr().String(), "/")
	if result.OK {
		t.Fatal("expected overall failure for a plaintext port under an https target")
	}
	tcpStep, ok := stepByName(result.Steps, "tcp")
	if !ok || tcpStep.Status != StatusPass {
		t.Fatalf("expected tcp to pass, got %+v", tcpStep)
	}
	tlsStep, ok := stepByName(result.Steps, "tls")
	if !ok || tlsStep.Status != StatusFail {
		t.Fatalf("expected tls to fail against a plaintext listener, got %+v", tlsStep)
	}
	if tlsStep.ActOn != ActOnITSecurity {
		t.Fatalf("expected an it_security suggestion on TLS failure, got %+v", tlsStep)
	}
	httpStep, ok := stepByName(result.Steps, "http")
	if !ok || httpStep.Status != StatusSkipped {
		t.Fatalf("expected http to be skipped after a TLS failure, got %+v", httpStep)
	}
}

func TestProbeHTTPFailsForMalformedURL(t *testing.T) {
	result := ProbeHTTP(context.Background(), "test target", "://not-a-url", "/")
	if result.OK {
		t.Fatal("expected overall failure for a malformed URL")
	}
	dnsStep, ok := stepByName(result.Steps, "dns")
	if !ok || dnsStep.Status != StatusFail {
		t.Fatalf("expected dns (the first step) to report the malformed URL, got %+v", dnsStep)
	}
	if dnsStep.ActOn != ActOnSupport {
		t.Fatalf("expected a support suggestion for a malformed URL (a config bug, not a network issue), got %+v", dnsStep)
	}
}

func TestProbeTCPSuccessAgainstRealListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	result := ProbeTCP(context.Background(), "SMTP relay", host, port)
	if !result.OK {
		t.Fatalf("expected overall success, got %+v", result)
	}
	if _, ok := stepByName(result.Steps, "tls"); ok {
		t.Fatal("expected no tls step for a TCP-only probe")
	}
	if _, ok := stepByName(result.Steps, "http"); ok {
		t.Fatal("expected no http step for a TCP-only probe")
	}
}

func TestProbeTCPFailsForClosedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	result := ProbeTCP(context.Background(), "SMTP relay", host, port)
	if result.OK {
		t.Fatal("expected overall failure for a closed port")
	}
	tcpStep, ok := stepByName(result.Steps, "tcp")
	if !ok || tcpStep.Status != StatusFail || tcpStep.ActOn != ActOnNetworkAdmin {
		t.Fatalf("expected tcp to fail with a network_admin suggestion, got %+v", tcpStep)
	}
}
