package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// blockedNetworks are private/link-local/metadata ranges an HTTP node must
// never be allowed to reach — a workflow's URL parameter can come from an
// LLM-authored definition or from upstream item data, so it's untrusted
// input in the SSRF sense even in a single-tenant deployment. Adapted from
// neul-labs/m9m's internal/nodes/http SSRF guard (see ../NOTICE) — the
// blocklist itself is the valuable, easy-to-get-wrong part; kept intact
// rather than redone from scratch.
var blockedNetworks = mustParseCIDRs(
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
	"169.254.0.0/16", "0.0.0.0/8", "100.64.0.0/10",
	"192.0.0.0/24", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24",
	"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
	"::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
)

var blockedHosts = map[string]bool{
	"localhost": true, "metadata.google.internal": true, "metadata.goog": true,
	"169.254.169.254": true, "169.254.170.2": true, "fd00:ec2::254": true,
	"instance-data": true, "metadata": true,
	"kubernetes.default": true, "kubernetes.default.svc": true, "kubernetes.default.svc.cluster.local": true,
}

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, len(cidrs))
	for i, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("dataflow: invalid CIDR %q: %v", c, err))
		}
		nets[i] = n
	}
	return nets
}

// validateURLForSSRF rejects a URL that would reach a private, loopback, or
// cloud-metadata address instead of a genuine external service.
func validateURLForSSRF(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http/https schemes are allowed, got %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if blockedHosts[strings.ToLower(host)] {
		return fmt.Errorf("access to host %q is blocked", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Resolution failing here just means the request will fail at
		// dial time too — nothing to block on.
		return nil
	}
	for _, ip := range ips {
		for _, blocked := range blockedNetworks {
			if blocked.Contains(ip) {
				return fmt.Errorf("access to %s (resolves inside %s) is blocked", host, blocked.String())
			}
		}
	}
	return nil
}

// HTTPRequest is the "http_request" node type: a single HTTP call per Run
// (not per input item — chain it after a node that produces one item per
// call if per-item requests are needed).
//
// Parameters: method (default GET), url (required), headers
// (map[string]string), body (string, sent as-is with Content-Type left to
// headers).
type HTTPRequest struct {
	client *http.Client
	// checkSSRF defaults to validateURLForSSRF; tests override it to allow
	// pointing at an httptest.Server (which binds to 127.0.0.1, itself a
	// blocked address — the point of the guard) without weakening it for
	// real use.
	checkSSRF func(string) error
}

// NewHTTPRequest creates an HTTP Request node with a 30-second timeout that refuses private, internal and cloud metadata addresses.
func NewHTTPRequest() *HTTPRequest {
	return &HTTPRequest{client: &http.Client{Timeout: 30 * time.Second}, checkSSRF: validateURLForSSRF}
}

// Description returns the catalog entry of the http_request node: it makes a single HTTP call and returns the response as one item.
func (n *HTTPRequest) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "http_request", Name: "HTTP Request", Category: "Network",
		Description: "Makes a single HTTP call and returns the response as one item (fields \"status\", \"headers\", and \"json\" or \"body\"). Private/internal/cloud-metadata addresses are blocked. " +
			"Parameters: url (string, required). method (string, default \"GET\"). headers (object of string->string, optional). body (string, optional, sent as-is).",
		Properties: []dataflow.NodeProperty{
			{Name: "url", DisplayName: "URL", Type: dataflow.PropString, Required: true, Placeholder: "https://api.example.com/resource"},
			{Name: "method", DisplayName: "Method", Type: dataflow.PropOptions, Default: "GET", Options: []dataflow.NodePropertyOption{
				{Label: "GET", Value: "GET"}, {Label: "POST", Value: "POST"}, {Label: "PUT", Value: "PUT"},
				{Label: "PATCH", Value: "PATCH"}, {Label: "DELETE", Value: "DELETE"}, {Label: "HEAD", Value: "HEAD"},
			}},
			{Name: "headers", DisplayName: "Headers", Type: dataflow.PropJSON, Description: "Object of header name -> value."},
			{Name: "body", DisplayName: "Body", Type: dataflow.PropText, Description: "Sent as-is; set a Content-Type header to match."},
		}}
}

// ValidateParameters requires a url and refuses one that points at a private, internal or cloud metadata address.
func (n *HTTPRequest) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "url", "") == "" {
		return errors.New("url is required")
	}
	return n.checkSSRF(dataflow.StringParam(params, "url", ""))
}

// Execute makes the call described by url, method, headers and body, whose values may be expressions resolved against the first input item, and returns one item with the status, the headers and either the parsed JSON ("json") or the text ("body") of the response, which is read up to 10 MB.
func (n *HTTPRequest) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	// http_request makes one call per Run, not per item (see the type's own
	// doc comment) - so expressions resolve against the first input item
	// (or an empty item for a trigger-seeded run with no upstream node
	// output), index 0, matching how the node already treats input as a
	// single batch rather than looping over it.
	item0 := dataflow.Item{}
	if len(input) > 0 {
		item0 = input[0]
	}

	method := strings.ToUpper(dataflow.StringParam(params, "method", "GET"))
	rawURLVal, err := dataflow.ResolveParam(ctx, rt, params, "url", item0, 0)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve url: %w", err)
	}
	rawURL := dataflow.AsString(rawURLVal)
	if err := n.checkSSRF(rawURL); err != nil {
		return dataflow.Output{}, err
	}

	var bodyReader io.Reader
	bodyVal, err := dataflow.ResolveParam(ctx, rt, params, "body", item0, 0)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve body: %w", err)
	}
	if body := dataflow.AsString(bodyVal); body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("build request: %w", err)
	}
	if headers, ok := params["headers"].(map[string]any); ok {
		for k, v := range headers {
			resolved, err := dataflow.ResolveValue(ctx, rt, v, item0, 0)
			if err != nil {
				return dataflow.Output{}, fmt.Errorf("resolve header %q: %w", k, err)
			}
			if s := dataflow.AsString(resolved); s != "" {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MB cap
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("read response body: %w", err)
	}

	item := dataflow.Item{"status": resp.StatusCode, "headers": flattenHeaders(resp.Header)}
	var parsed any
	if json.Unmarshal(respBody, &parsed) == nil {
		item["json"] = parsed
	} else {
		item["body"] = string(respBody)
	}
	return dataflow.Main([]dataflow.Item{item}), nil
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k := range h {
		out[k] = h.Get(k)
	}
	return out
}
