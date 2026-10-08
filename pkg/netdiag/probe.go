// Package netdiag runs a small, named sequence of layered connectivity
// checks (DNS, then TCP, then TLS, then HTTP) against one target, so the person
// who runs a deployment can see exactly which layer is broken instead of a
// single opaque "cannot reach it" failure.
//
// ProbeHTTP checks a URL through all four layers, ProbeTCP stops at TCP for a
// service that does not speak HTTP (a mail relay, say). Each layer is a Step
// with its own outcome, and a layer that cannot run because an earlier one failed
// is reported as skipped. The package is generic and has no product logic: SeshatOS
// and SeshatCloud both use it, each deciding which targets to probe.
package netdiag

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Status is a step's outcome.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

// ActOn names who should act on a failed step - purely advisory, surfaced
// directly to whoever is running the diagnostic (typically enterprise IT).
type ActOn string

const (
	ActOnNetworkAdmin ActOn = "network_admin"
	ActOnITSecurity   ActOn = "it_security"
	ActOnSupport      ActOn = "support"
)

// Step is one named check in the sequence.
type Step struct {
	Name       string `json:"name"`
	Status     Status `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	ActOn      ActOn  `json:"act_on,omitempty"`
}

// Result is one target's full check sequence.
type Result struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Steps  []Step `json:"steps"`
	OK     bool   `json:"ok"`
}

const dialTimeout = 5 * time.Second

// ProbeHTTP runs DNS -> TCP -> TLS (only for an https target) -> HTTP GET
// against rawURL+path, timing each step. name labels the result for a caller
// checking several targets at once (e.g. an SSO provider's display name) -
// it has no bearing on the check itself.
//
// The HTTP step's success criterion is deliberately "no transport error",
// never a specific status code: for a target this process doesn't own (a
// customer's IdP), even a 404/405 on a bare GET proves the network path
// works - the status code is kept in Detail for the reader's own judgment,
// not treated as a failure.
func ProbeHTTP(ctx context.Context, name, rawURL, path string) Result {
	result := Result{Name: name, Target: rawURL}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		result.Steps = []Step{{
			Name: "dns", Status: StatusFail,
			Detail:     fmt.Sprintf("invalid target URL: %v", err),
			Suggestion: "The configured URL itself is malformed - this is a configuration issue, not a network one.",
			ActOn:      ActOnSupport,
		}}
		return result
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	useTLS := parsed.Scheme == "https"

	dnsStep, dnsOK := probeDNS(ctx, host)
	result.Steps = append(result.Steps, dnsStep)
	if !dnsOK {
		result.Steps = append(result.Steps,
			skippedStep("tcp"), skippedStep("tls"), skippedStep("http"))
		return result
	}

	tcpStep, tcpOK := probeTCP(ctx, host, port)
	result.Steps = append(result.Steps, tcpStep)
	if !tcpOK {
		result.Steps = append(result.Steps, skippedStep("tls"), skippedStep("http"))
		return result
	}

	if useTLS {
		tlsStep, tlsOK := probeTLS(ctx, host, port)
		result.Steps = append(result.Steps, tlsStep)
		if !tlsOK {
			result.Steps = append(result.Steps, skippedStep("http"))
			return result
		}
	} else {
		result.Steps = append(result.Steps, Step{Name: "tls", Status: StatusSkipped, Detail: "not applicable (plain HTTP target)"})
	}

	httpStep, _ := probeHTTPGet(ctx, rawURL+path)
	result.Steps = append(result.Steps, httpStep)

	result.OK = allPassedOrSkipped(result.Steps)
	return result
}

// ProbeTCP runs only DNS -> TCP against host:port, for non-HTTP targets like
// an SMTP relay. No TLS step: SMTP's STARTTLS negotiates TLS *after* the
// plaintext connection is already open (typically port 587), so an
// immediate TLS handshake would misreport a perfectly healthy relay as
// broken - DNS+TCP already answers the question this tool exists for ("does
// a firewall block this port"), which is as far as a protocol-agnostic probe
// can honestly go without speaking SMTP itself.
func ProbeTCP(ctx context.Context, name, host string, port int) Result {
	result := Result{Name: name, Target: fmt.Sprintf("%s:%d", host, port)}
	portStr := strconv.Itoa(port)

	dnsStep, dnsOK := probeDNS(ctx, host)
	result.Steps = append(result.Steps, dnsStep)
	if !dnsOK {
		result.Steps = append(result.Steps, skippedStep("tcp"))
		return result
	}

	tcpStep, _ := probeTCP(ctx, host, portStr)
	result.Steps = append(result.Steps, tcpStep)

	result.OK = allPassedOrSkipped(result.Steps)
	return result
}

func skippedStep(name string) Step {
	return Step{Name: name, Status: StatusSkipped, Detail: "not attempted - an earlier step failed"}
}

func allPassedOrSkipped(steps []Step) bool {
	for _, s := range steps {
		if s.Status == StatusFail {
			return false
		}
	}
	return true
}

func probeDNS(ctx context.Context, host string) (Step, bool) {
	start := time.Now()
	if ip := net.ParseIP(host); ip != nil {
		return Step{Name: "dns", Status: StatusPass, DurationMS: time.Since(start).Milliseconds(), Detail: "target is a literal IP, no lookup needed"}, true
	}
	reqCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(reqCtx, host)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return Step{
			Name: "dns", Status: StatusFail, DurationMS: duration,
			Detail:     err.Error(),
			Suggestion: fmt.Sprintf("Could not resolve %q - this is usually a DNS or corporate proxy configuration issue on this network.", host),
			ActOn:      ActOnNetworkAdmin,
		}, false
	}
	return Step{Name: "dns", Status: StatusPass, DurationMS: duration, Detail: fmt.Sprintf("resolved to %v", addrs)}, true
}

func probeTCP(ctx context.Context, host, port string) (Step, bool) {
	start := time.Now()
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return Step{
			Name: "tcp", Status: StatusFail, DurationMS: duration,
			Detail:     err.Error(),
			Suggestion: fmt.Sprintf("Could not open a network connection to %s:%s - a firewall is likely blocking outbound access to this host/port; contact your network administrator.", host, port),
			ActOn:      ActOnNetworkAdmin,
		}, false
	}
	_ = conn.Close()
	return Step{Name: "tcp", Status: StatusPass, DurationMS: duration, Detail: fmt.Sprintf("connected to %s:%s", host, port)}, true
}

func probeTLS(ctx context.Context, host, port string) (Step, bool) {
	start := time.Now()
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: dialTimeout},
		Config:    &tls.Config{ServerName: host},
	}
	reqCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(reqCtx, "tcp", net.JoinHostPort(host, port))
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return Step{
			Name: "tls", Status: StatusFail, DurationMS: duration,
			Detail:     err.Error(),
			Suggestion: "TLS handshake failed - this can indicate a corporate TLS-inspection proxy or an outdated system trust store; contact IT/security.",
			ActOn:      ActOnITSecurity,
		}, false
	}
	defer conn.Close()
	tlsConn, _ := conn.(*tls.Conn)
	version := uint16(0)
	if tlsConn != nil {
		version = tlsConn.ConnectionState().Version
	}
	return Step{Name: "tls", Status: StatusPass, DurationMS: duration, Detail: fmt.Sprintf("negotiated %s", tlsVersionName(version))}, true
}

func probeHTTPGet(ctx context.Context, target string) (Step, bool) {
	start := time.Now()
	client := &http.Client{Timeout: dialTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Step{Name: "http", Status: StatusFail, DurationMS: time.Since(start).Milliseconds(), Detail: err.Error(), Suggestion: "The configured URL itself is malformed.", ActOn: ActOnSupport}, false
	}
	resp, err := client.Do(req)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return Step{
			Name: "http", Status: StatusFail, DurationMS: duration,
			Detail:     err.Error(),
			Suggestion: "The connection opened but no HTTP response was received in time - this may be a temporary outage; contact support if it persists.",
			ActOn:      ActOnSupport,
		}, false
	}
	defer resp.Body.Close()
	return Step{Name: "http", Status: StatusPass, DurationMS: duration, Detail: "HTTP " + strconv.Itoa(resp.StatusCode)}, true
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return "unknown TLS version"
	}
}
