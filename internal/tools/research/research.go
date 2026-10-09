// Package research groups the tools an agent uses to read the outside world for a market study, a product
// comparison or an analysis of customer reviews: app store reviews, Reddit, YouTube, Google Places reviews and
// Trustpilot. Hacker News, dev.to, web search and page fetch live in their own packages.
//
// Each tool reads one official API and returns readable text, with the source, link, author and date of every item,
// so the agent can quote and compare. Nothing is stored here: a workflow that wants to keep what it collected saves
// it as it saves any file.
//
// Keys. Reddit, YouTube, Google Places and Trustpilot need credentials. A host that serves several people at once
// (seshat-server) hands each run its own keys through Keys and the process environment is then never read, so one
// organization's key cannot reach another's run. A single-user host (the desktop, a CLI) gives no keys and the
// environment is used instead. A tool whose keys are missing is not offered to the model at all.
package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
)

// Key names, as they appear in the per-run map. The environment variable of each is the upper-case name.
const (
	KeyRedditClientID     = "reddit_client_id"
	KeyRedditClientSecret = "reddit_client_secret"
	KeyYouTubeAPIKey      = "youtube_api_key"
	KeyGooglePlacesAPIKey = "google_places_api_key"
	KeyTrustpilotAPIKey   = "trustpilot_api_key"
)

// Keys resolves the credentials of the research tools.
type Keys struct {
	explicit map[string]string
}

// NewKeys builds the resolver. With a non-empty map only that map is used; with none, the process environment is.
func NewKeys(explicit map[string]string) *Keys {
	return &Keys{explicit: explicit}
}

// Get returns the value of a key, or "".
func (k *Keys) Get(name string) string {
	if k != nil && len(k.explicit) > 0 {
		return strings.TrimSpace(k.explicit[name])
	}
	return strings.TrimSpace(os.Getenv(strings.ToUpper(name)))
}

func (k *Keys) has(names []string) bool {
	for _, name := range names {
		if k.Get(name) == "" {
			return false
		}
	}
	return true
}

// Tools returns every research tool, ready to register. A tool whose keys are missing reports itself disabled.
func Tools(keys *Keys) []tool.Tool {
	return append(append(append(append(
		appStoreTools(newAppStoreClient(), keys),
		redditTools(newRedditClient(keys), keys)...),
		youTubeTools(newYouTubeClient(keys), keys)...),
		placesTools(newPlacesClient(keys), keys)...),
		trustpilotTools(newTrustpilotClient(keys), keys)...)
}

// ─── the tool every source is built from ──────────────────────────────────────────────────────────

type spec struct {
	name        string
	displayName string
	description string
	properties  map[string]any
	required    []string
	needs       []string // the keys that must be set
}

type runFn func(ctx context.Context, args map[string]any) (string, error)

type researchTool struct {
	spec spec
	keys *Keys
	run  runFn
}

func newTool(s spec, keys *Keys, run runFn) *researchTool {
	return &researchTool{spec: s, keys: keys, run: run}
}

func (t *researchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        t.spec.name,
		DisplayName: t.spec.displayName,
		Description: t.spec.description,
		Category:    "research",
		InputSchema: schema.FromMap(map[string]any{
			"type":       "object",
			"properties": t.spec.properties,
			"required":   t.spec.required,
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *researchTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	for _, name := range t.spec.required {
		if v, ok := input.Parsed[name]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
			return tool.NewErrorResult(fmt.Errorf("%s is required", name)), nil
		}
	}
	text, err := t.run(ctx, input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	return tool.NewTextResult(text), nil
}

func (t *researchTool) Description(_ context.Context) (string, error) { return t.spec.description, nil }
func (t *researchTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *researchTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *researchTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *researchTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *researchTool) IsEnabled() bool                         { return t.keys.has(t.spec.needs) }
func (t *researchTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *researchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── arguments ────────────────────────────────────────────────────────────────────────────────────

func stringArg(args map[string]any, name string) string {
	v, _ := args[name].(string)
	return strings.TrimSpace(v)
}

func intArg(args map[string]any, name string, def, lo, hi int) int {
	n := def
	switch v := args[name].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func oneOf(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────────────────────────

const (
	userAgent    = "seshat-research/1.0 (+https://github.com/KPO-Tech/seshat)"
	maxBodyBytes = 8 << 20
)

// apiError is a refusal or failure of one API call, worded for the model: it says what to do next.
type apiError struct {
	source     string
	status     int
	retryAfter time.Duration
	detail     string
}

func (e *apiError) Error() string {
	switch {
	case e.status == http.StatusUnauthorized || e.status == http.StatusForbidden:
		return fmt.Sprintf("%s refused the request (HTTP %d): the key or the account has no access to this. %s", e.source, e.status, e.detail)
	case e.status == http.StatusTooManyRequests:
		if e.retryAfter > 0 {
			return fmt.Sprintf("%s is rate limiting this key: try again in about %d seconds, or ask for fewer results.", e.source, int(e.retryAfter.Seconds()))
		}
		return fmt.Sprintf("%s is rate limiting this key: try again later, or ask for fewer results.", e.source)
	case e.status == http.StatusNotFound:
		return fmt.Sprintf("%s found nothing at this address (HTTP 404). %s", e.source, e.detail)
	default:
		return fmt.Sprintf("%s answered HTTP %d. %s", e.source, e.status, e.detail)
	}
}

type httpClient struct {
	source string
	client *http.Client
}

func newHTTPClient(source string) *httpClient {
	return &httpClient{source: source, client: &http.Client{Timeout: 25 * time.Second}}
}

// do sends a request and decodes a JSON answer into out. A non-2xx answer becomes an *apiError.
func (c *httpClient) do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", userAgent)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%s could not be reached: %w", c.source, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%s: reading the answer: %w", c.source, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := &apiError{source: c.source, status: resp.StatusCode, detail: shorten(strings.TrimSpace(string(body)), 300)}
		if seconds, perr := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); perr == nil && seconds > 0 {
			e.retryAfter = time.Duration(seconds) * time.Second
		}
		return e
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s answered something that is not JSON: %w", c.source, err)
	}
	return nil
}

func (c *httpClient) getJSON(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.do(req, out)
}

// ─── text ─────────────────────────────────────────────────────────────────────────────────────────

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// oneLine collapses whitespace so a review or a comment reads on a single line of a list.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func stars(rating float64) string {
	if rating <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1f★", rating)
}

// join writes the parts that are not empty, separated by " · ".
func join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// asAPIError reports whether err is an *apiError and stores it in target.
func asAPIError(err error, target **apiError) bool {
	return errors.As(err, target)
}

// repairMojibake repairs text whose UTF-8 bytes were read as Windows-1252 (what Apple's review feed does: "…" arrives
// as "â€¦"). It changes the text only when every character maps back to a byte and the bytes are valid UTF-8, so
// text that was fine is returned untouched.
func repairMojibake(s string) string {
	if !strings.ContainsAny(s, "ÃÂâ") {
		return s
	}
	buf := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x80:
			buf = append(buf, byte(r))
		case r >= 0xA0 && r <= 0xFF:
			buf = append(buf, byte(r))
		default:
			b, ok := windows1252High[r]
			if !ok {
				return s
			}
			buf = append(buf, b)
		}
	}
	if !utf8.Valid(buf) {
		return s
	}
	return string(buf)
}

// windows1252High maps the characters of the 0x80-0x9F range of Windows-1252 back to their byte.
var windows1252High = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B,
	'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99,
	'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}
