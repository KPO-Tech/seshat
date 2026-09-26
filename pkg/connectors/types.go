// Package connectors is the tenant-agnostic core of SeshatOS's Knowledge
// connectors: the Google Drive, SharePoint, and S3-compatible Discover/Sync
// logic, extracted out of both seshat-backend (internal/knowledge/gdrive,
// internal/knowledge/sharepoint, internal/knowledge/s3) and seshat-server
// (internal/server/connectors) after both had independently ported the same
// ~1,700 lines of API-client/permission-mapping/extension-allowlist code -
// see ROADMAP.md's "core/connectors" entry for the extraction and why it
// mirrors core/prospecting's shape (a small sibling Go module shared via
// go.work, not imported by either backend directly into the other).
//
// Deliberately does not export an Account type or a shared connector
// interface: neither is needed. Grepping both original sides confirmed
// Discover/Sync never used anything from the caller's account struct except
// S3's own Config string (bucket/region/endpoint/prefix) - so each
// connector's methods take exactly what they use (Secret, plus an explicit
// config string for S3 only), not a generic Account shape. Each side keeps
// its own connector-selection interface (connector.KnowledgeConnector in
// seshat-backend, knowledgeConnector in seshat-server) and its own thin
// wrapper adapting that interface to these concrete types - the two
// interfaces' shapes already differed slightly before this extraction, and
// unifying them isn't this package's job.
//
// OAuth flow orchestration (state/PKCE/token exchange/app credentials) also
// stays on each side, since it's genuinely different per side (seshat-backend:
// one shared global client; seshat-server: a per-organization app registry) -
// only the scope lists a connector actually needs (GDriveScopes/
// SharePointScopes) are shared here, since both sides were duplicating the
// same string literals.
package connectors

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// TextExtractorFunc converts a downloaded binary file's raw bytes into
// ingestable text (e.g. PDF/DOCX/PPTX via a document reader) - nil means "no
// extractor wired", and each connector falls back to treating raw bytes as
// text, which is wrong for binary formats. Wired via each connector type's
// WithTextExtractor, normally to the caller's own document-reader extraction
// so connector-synced content goes through the same path as a manual upload.
type TextExtractorFunc func(ctx context.Context, data []byte, contentType, filename string) string

// Secret is the decrypted connection material a connector needs to call its
// external API. For gdrive/sharepoint these are OAuth access/refresh
// tokens; for S3, AccessToken/RefreshToken are reinterpreted as the access
// key ID and secret access key (a deliberate reuse of the generic field
// names, not a naming mistake - see S3Connector's doc comment).
type Secret struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// AccessEntry names one identity or group allowed to see a resource
// ("user:alice@example.com", "group:eng-team", "domain:example.com", or
// "public"), mirroring Elastic Connectors' prefixed-identity ACL model. A
// nil or empty AccessControl on a SyncItem means "inherit the caller's own
// coarser scope" - neither connector over-shares when it can't resolve a
// permission to a concrete identity.
type AccessEntry string

// ResourceRef is what Discover returns per resource found - enough to
// decide whether/how to sync it without fetching its content yet.
type ResourceRef struct {
	ID       string
	Name     string
	Kind     string // "file", "folder", "record", ...
	Modified time.Time
}

// SyncItem is what Sync yields per resource - content ready for ingestion,
// plus the access control list it should carry.
type SyncItem struct {
	ResourceRef
	Data          []byte
	ContentType   string
	Text          string
	AccessControl []AccessEntry
}

// TimeCursor is the shared sync-cursor shape for a connector with no native
// change-feed API (s3.go, azureblob.go, confluence.go, today) - each of
// those instead resyncs by listing everything and filtering to whatever
// changed after a timestamp, so "the cursor" is just "when the previous
// sync started." Was three independent, byte-for-byte identical
// RFC3339Nano parse/format implementations before being extracted here;
// mirrors SPDriveCursor's existing Decode.../Encode() shape (sharepoint.go)
// for a connector kind that does have real structure to a cursor.
type TimeCursor struct {
	Since time.Time
}

// DecodeTimeCursor parses cursor as RFC3339Nano - not RFC3339, which
// truncates to whole seconds and can lose real ordering between a
// resource's own modified time and this cursor landing in the same second.
// Empty means "no cursor yet" (Since stays the zero time.Time, so
// (TimeCursor{}).Since.IsZero() is how a caller detects "bootstrap sync").
func DecodeTimeCursor(cursor string) (TimeCursor, error) {
	if cursor == "" {
		return TimeCursor{}, nil
	}
	since, err := time.Parse(time.RFC3339Nano, cursor)
	if err != nil {
		return TimeCursor{}, fmt.Errorf("invalid sync cursor: %w", err)
	}
	return TimeCursor{Since: since}, nil
}

// Encode formats c back into the string form ConnectorAccount.SyncCursor
// stores.
func (c TimeCursor) Encode() string {
	return c.Since.UTC().Format(time.RFC3339Nano)
}

// NewTimeCursorNow is the "next cursor" a bootstrap or incremental sync
// returns - capture it once, before listing starts, so a resource modified
// mid-sync isn't missed by the following incremental run.
func NewTimeCursorNow() TimeCursor {
	return TimeCursor{Since: time.Now().UTC()}
}

// RateLimited is implemented by an error a connector returns to represent a
// rate-limit/backpressure response (HTTP 429, or a provider-specific
// quota-exceeded signal) rather than a genuine failure - a caller decides
// how long to back off by checking for this via errors.As instead of
// applying a flat retry delay regardless of cause. ok=false from
// RetryAfter means "this was rate-limiting, but no specific delay was
// suggested" - the caller should fall back to its own default, not treat
// it as "not rate-limited."
type RateLimited interface {
	error
	RetryAfter() (time.Duration, bool)
}

// parseRetryAfter reads a standard Retry-After response header (RFC 7231,
// section 7.1.3): either a whole number of seconds, or an HTTP-date. Returns
// (0, false) when the header is absent, unparseable, or names a time
// already in the past.
func parseRetryAfter(h http.Header) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if when, err := http.ParseTime(v); err == nil {
		if d := time.Until(when); d > 0 {
			return d, true
		}
	}
	return 0, false
}

// rateLimitWrap is the shared RateLimited implementation for a connector
// (gdrive) whose errors don't already have one dedicated error type to
// extend directly, unlike spGraphError/confluenceError.
type rateLimitWrap struct {
	err        error
	retryAfter time.Duration
	hasDelay   bool
}

func (w *rateLimitWrap) Error() string                     { return w.err.Error() }
func (w *rateLimitWrap) Unwrap() error                     { return w.err }
func (w *rateLimitWrap) RetryAfter() (time.Duration, bool) { return w.retryAfter, w.hasDelay }

var _ RateLimited = (*rateLimitWrap)(nil)
