package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EngineerProjects/seshat/internal/types"
)

// Config holds the runtime configuration for the automation tools.
type Config struct {
	ServiceURL string // base URL of seshat-automation daemon, e.g. "http://localhost:8090"
	APIKey     string // API key for daemon auth (AUTOMATION_API_KEY)
}

// available reports whether the automation daemon is configured.
func (c Config) available() bool {
	return strings.TrimSpace(c.ServiceURL) != ""
}

type daemonClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func newDaemonClient(cfg Config) *daemonClient {
	return &daemonClient{
		baseURL: strings.TrimRight(cfg.ServiceURL, "/"),
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do performs an authenticated request to the daemon and returns the raw body + status code.
func (c *daemonClient) do(ctx context.Context, method, path string, body any, userID string) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if userID != "" {
		req.Header.Set("X-Seshat-User-ID", userID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("automation daemon unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// errNotConfigured is returned when the automation service URL is not set.
const errNotConfigured = "automation service not configured (AUTOMATION_SERVICE_URL not set)"

func userIDFromCtx(ctx context.Context) string {
	return types.AgentUserIDFromContext(ctx)
}

// parseResponse unmarshals a successful daemon response into dst.
// Returns an error string on non-2xx status codes.
func parseResponse(data []byte, statusCode int, dst any) error {
	if statusCode < 200 || statusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(data, &errBody); jsonErr == nil && errBody.Error != "" {
			return fmt.Errorf("daemon error %d: %s", statusCode, errBody.Error)
		}
		return fmt.Errorf("daemon returned status %d", statusCode)
	}
	if dst != nil && len(data) > 0 {
		return json.Unmarshal(data, dst)
	}
	return nil
}
