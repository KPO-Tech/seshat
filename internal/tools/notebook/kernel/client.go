// Package kernel implements an HTTP + WebSocket client for the Jupyter Server REST API.
// It covers kernel lifecycle (list/start/restart/interrupt/stop) and provides
// a Channel type for executing code via the Jupyter messaging protocol.
package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config holds the connection parameters for a Jupyter server.
type Config struct {
	ServerURL string // e.g. "http://localhost:8888"
	Token     string // Jupyter token
}

// DefaultConfig returns a Config populated from environment variables.
func DefaultConfig() Config {
	return Config{
		ServerURL: strings.TrimRight(envOr("JUPYTER_SERVER_URL", "http://localhost:8888"), "/"),
		Token:     envOr("JUPYTER_TOKEN", ""),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MergeConfig merges explicit values (from tool input) onto a base config,
// keeping env-var defaults when the explicit value is empty.
func MergeConfig(base Config, serverURL, token string) Config {
	c := base
	if serverURL != "" {
		c.ServerURL = strings.TrimRight(serverURL, "/")
	}
	if token != "" {
		c.Token = token
	}
	return c
}

// KernelInfo represents a running Jupyter kernel.
type KernelInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	LastActivity   string `json:"last_activity"`
	ExecutionState string `json:"execution_state"`
	Connections    int    `json:"connections"`
}

// Client is a minimal Jupyter Server REST client.
type Client struct {
	cfg  Config
	http *http.Client
}

// New creates a Jupyter kernel REST client.
func New(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListKernels returns all running kernels.
func (c *Client) ListKernels(ctx context.Context) ([]KernelInfo, error) {
	var kernels []KernelInfo
	if err := c.get(ctx, "/api/kernels", &kernels); err != nil {
		return nil, err
	}
	return kernels, nil
}

// StartKernel starts a new kernel with the given name (e.g. "python3").
func (c *Client) StartKernel(ctx context.Context, name string) (*KernelInfo, error) {
	body := map[string]string{"name": name}
	var info KernelInfo
	if err := c.post(ctx, "/api/kernels", body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetKernel returns info about a specific kernel.
func (c *Client) GetKernel(ctx context.Context, id string) (*KernelInfo, error) {
	var info KernelInfo
	if err := c.get(ctx, "/api/kernels/"+id, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// RestartKernel restarts a kernel, clearing all state.
func (c *Client) RestartKernel(ctx context.Context, id string) error {
	return c.postEmpty(ctx, "/api/kernels/"+id+"/restart")
}

// InterruptKernel sends an interrupt signal to a running kernel.
func (c *Client) InterruptKernel(ctx context.Context, id string) error {
	return c.postEmpty(ctx, "/api/kernels/"+id+"/interrupt")
}

// StopKernel shuts down a kernel.
func (c *Client) StopKernel(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/kernels/"+id)
}

// OpenChannel opens a WebSocket channel to a kernel for code execution.
func (c *Client) OpenChannel(ctx context.Context, kernelID string) (*Channel, error) {
	return dialChannel(ctx, c.cfg, kernelID)
}

// ─── HTTP helpers ────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.ServerURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postEmpty(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "token "+c.cfg.Token)
	}
}
