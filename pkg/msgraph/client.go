// Package msgraph is a minimal, hand-rolled Microsoft Graph client for Mail
// and Teams chat messaging - no official Go SDK is vendored in this module,
// same reasoning as core/connectors' sharepoint_graph.go: only a handful of
// call shapes are needed, not the full Graph surface a generated SDK would
// bring in.
//
// This is shared, tenant-agnostic logic used two ways: seshat-backend wraps
// it in real inbox.Connector implementations (inbox/outlook/inbox/teams) for
// the local Inbox workflow, and seshat-server wraps it in a small
// self-hosted MCP server (internal/server/msgraphmcp) so the workspace chat
// agent can call it as a tool, bridged through a per-employee OAuth-
// connected account the same way every other agent_action connector kind
// works. Neither side stores or manages OAuth tokens here - callers pass an
// already-token-bound *http.Client, mirroring sharepoint_graph.go's
// spGraphClient.
package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// graphBaseURL is the stable v1.0 Microsoft Graph endpoint. A var, not a
// const, so tests can point it at an httptest.Server instead of the real
// Microsoft Graph host - see swapGraphBaseURLForTest in the test files and
// SetBaseURLForTesting below (the same mechanism, exported for consumer
// packages' own tests).
var graphBaseURL = "https://graph.microsoft.com/v1.0"

// SetBaseURLForTesting points this package's Microsoft Graph base URL at a
// test server, returning a function that restores the real value - for use
// by other packages' tests (e.g. seshat-server/internal/server/msgraphmcp)
// that need to verify a request/Bearer token reaches this package's HTTP
// calls without hitting the real Microsoft Graph host. Never call this
// outside a test.
func SetBaseURLForTesting(url string) (restore func()) {
	original := graphBaseURL
	graphBaseURL = url
	return func() { graphBaseURL = original }
}

// Client is a minimal hand-rolled Microsoft Graph HTTP client.
type Client struct {
	http *http.Client
}

// New builds a Client from an already-token-bound *http.Client (e.g.
// oauthConfig.Client(ctx, token)) - this package never sees a raw token or
// does its own OAuth.
func New(httpClient *http.Client) *Client {
	return &Client{http: httpClient}
}

// GraphError carries the HTTP status so callers can special-case specific
// responses (e.g. a 404 meaning "no such chat/message").
type GraphError struct {
	Status int
	Body   string
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("microsoft graph returned %d: %s", e.Status, e.Body)
}

func (c *Client) get(ctx context.Context, url string, out any) error {
	return c.do(ctx, http.MethodGet, url, nil, out)
}

func (c *Client) post(ctx context.Context, url string, body, out any) error {
	return c.do(ctx, http.MethodPost, url, body, out)
}

func (c *Client) patch(ctx context.Context, url string, body any) error {
	return c.do(ctx, http.MethodPatch, url, body, nil)
}

func (c *Client) delete(ctx context.Context, url string) error {
	return c.do(ctx, http.MethodDelete, url, nil, nil)
}

func (c *Client) do(ctx context.Context, method, url string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode graph request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &GraphError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode graph response: %w", err)
		}
	}
	return nil
}
