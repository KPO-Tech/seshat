package browser

import (
	"context"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Manager owns browser lifecycle, per-session isolation, and page operations.
type Manager interface {
	EnsureSession(ctx context.Context, sessionID types.SessionID) (SessionState, error)
	OpenPage(ctx context.Context, sessionID types.SessionID, url string) (PageInfo, error)
	Navigate(ctx context.Context, sessionID types.SessionID, pageID string, url string) (PageInfo, error)
	Snapshot(ctx context.Context, sessionID types.SessionID, pageID string, options SnapshotOptions) (Snapshot, error)
	Screenshot(ctx context.Context, sessionID types.SessionID, pageID string, options ScreenshotOptions) (Screenshot, error)
	Click(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (PageInfo, error)
	Type(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (PageInfo, error)
	Press(ctx context.Context, sessionID types.SessionID, pageID string, key string) (PageInfo, error)
	Scroll(ctx context.Context, sessionID types.SessionID, pageID string, options ScrollOptions) (PageInfo, error)
	Wait(ctx context.Context, sessionID types.SessionID, pageID string, options WaitOptions) (PageInfo, error)
	ListPages(ctx context.Context, sessionID types.SessionID) ([]PageInfo, error)
	ListNetwork(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]NetworkEntry, error)
	ListDownloads(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]DownloadEntry, error)
	SearchSnapshots(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]SnapshotSearchHit, error)
	GetNetworkPolicy(ctx context.Context, sessionID types.SessionID) (NetworkPolicy, error)
	SetNetworkPolicy(ctx context.Context, sessionID types.SessionID, policy NetworkPolicy) (NetworkPolicy, error)
	SelectPage(ctx context.Context, sessionID types.SessionID, pageID string) (PageInfo, error)
	ClosePage(ctx context.Context, sessionID types.SessionID, pageID string) (SessionState, error)
	CloseSession(ctx context.Context, sessionID types.SessionID) error
	Close() error
}

// Config controls the local browser runtime.
type Config struct {
	Headless              bool
	ExecutablePath        string
	RemoteControlURL      string
	NavigationTimeout     time.Duration
	ActionTimeout         time.Duration
	MaxPagesPerSession    int
	MaxSnapshotText       int
	MaxSnapshotElements   int
	MaxActionsPerSession  int
	MaxRepeatedAction     int
	MaxSessionAge         time.Duration
	WaitAfterNavigation   time.Duration
	LauncherLeakless      bool
	DisableDefaultBrowser bool
	ScreenshotDir         string
	MaxIdleSession        time.Duration
	SessionReaperInterval time.Duration
	ArtifactStore         storage.ArtifactStore
	MaxNetworkEntries     int
	MaxDownloadEntries    int
	DownloadDir           string
}

// DefaultConfig returns a pragmatic V1 browser configuration.
func DefaultConfig() *Config {
	return &Config{
		Headless:              true,
		NavigationTimeout:     20 * time.Second,
		ActionTimeout:         10 * time.Second,
		MaxPagesPerSession:    5,
		MaxSnapshotText:       4000,
		MaxSnapshotElements:   40,
		MaxActionsPerSession:  64,
		MaxRepeatedAction:     6,
		MaxSessionAge:         30 * time.Minute,
		MaxIdleSession:        10 * time.Minute,
		WaitAfterNavigation:   1500 * time.Millisecond,
		LauncherLeakless:      true,
		SessionReaperInterval: time.Minute,
		MaxNetworkEntries:     256,
		MaxDownloadEntries:    64,
	}
}

// SessionState describes the browser state attached to one Nexus session.
type SessionState struct {
	SessionID    types.SessionID `json:"session_id"`
	ActivePageID string          `json:"active_page_id,omitempty"`
	PageCount    int             `json:"page_count"`
	ActionCount  int             `json:"action_count"`
	CreatedAt    time.Time       `json:"created_at"`
	LastActivity time.Time       `json:"last_activity,omitempty"`
}

// PageInfo is browser-visible page metadata.
type PageInfo struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SnapshotOptions controls browser snapshot extraction.
type SnapshotOptions struct {
	MaxText     int `json:"max_text,omitempty"`
	MaxElements int `json:"max_elements,omitempty"`
}

// WaitOptions controls page stabilization and content waits.
type WaitOptions struct {
	TimeoutMs int    `json:"timeout_ms,omitempty"`
	Text      string `json:"text,omitempty"`
}

// ScrollOptions controls page scrolling interactions.
type ScrollOptions struct {
	Direction string `json:"direction,omitempty"`
	Amount    int    `json:"amount,omitempty"`
}

// ScreenshotOptions controls page screenshot capture.
type ScreenshotOptions struct {
	FullPage bool `json:"full_page,omitempty"`
}

// Snapshot is a compact agent-facing view of a page.
type Snapshot struct {
	Page      PageInfo      `json:"page"`
	Revision  string        `json:"revision,omitempty"`
	Text      string        `json:"text"`
	Elements  []ElementInfo `json:"elements,omitempty"`
	Headings  []HeadingInfo `json:"headings,omitempty"`
	TakenAt   time.Time     `json:"taken_at"`
	Truncated bool          `json:"truncated,omitempty"`
}

// ElementInfo is an actionable element extracted from the DOM.
type ElementInfo struct {
	ID           string `json:"id"`
	Role         string `json:"role,omitempty"`
	Name         string `json:"name,omitempty"`
	Text         string `json:"text,omitempty"`
	SelectorHint string `json:"selector_hint,omitempty"`
	Editable     bool   `json:"editable,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

// HeadingInfo is a compact semantic heading extracted from the page.
type HeadingInfo struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Screenshot is a compact image capture payload for the current page.
type Screenshot struct {
	Page          PageInfo  `json:"page"`
	MimeType      string    `json:"mime_type"`
	DataBase64    string    `json:"data_base64"`
	Bytes         int       `json:"bytes"`
	PersistedPath string    `json:"persisted_path,omitempty"`
	PersistedSize int       `json:"persisted_size,omitempty"`
	FullPage      bool      `json:"full_page"`
	TakenAt       time.Time `json:"taken_at"`
}

// NetworkEntry is a compact browser network activity record retained per session.
type NetworkEntry struct {
	Seq          int64     `json:"seq"`
	PageID       string    `json:"page_id,omitempty"`
	URL          string    `json:"url"`
	Method       string    `json:"method,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	StatusCode   int       `json:"status_code,omitempty"`
	MimeType     string    `json:"mime_type,omitempty"`
	Stage        string    `json:"stage"`
	ErrorText    string    `json:"error_text,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// DownloadEntry is a compact browser download record retained per session.
type DownloadEntry struct {
	GUID              string    `json:"guid"`
	PageID            string    `json:"page_id,omitempty"`
	URL               string    `json:"url"`
	SuggestedFilename string    `json:"suggested_filename,omitempty"`
	State             string    `json:"state"`
	BytesReceived     int       `json:"bytes_received,omitempty"`
	TotalBytes        int       `json:"total_bytes,omitempty"`
	PersistedPath     string    `json:"persisted_path,omitempty"`
	PersistedSize     int       `json:"persisted_size,omitempty"`
	ErrorText         string    `json:"error_text,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// NetworkPolicy is a session-scoped browser request policy.
// V2 intentionally uses URL blocking heuristics instead of full request rewriting
// so it stays lightweight and safe inside the mono-runtime.
type NetworkPolicy struct {
	BlockedURLs    []string `json:"blocked_urls,omitempty"`
	ResourcePolicy string   `json:"resource_policy,omitempty"`
}

// SnapshotSearchHit represents a lightweight cross-page search result over
// previously captured browser snapshots inside one session.
type SnapshotSearchHit struct {
	PageID            string    `json:"page_id"`
	URL               string    `json:"url"`
	Title             string    `json:"title,omitempty"`
	Revision          string    `json:"revision,omitempty"`
	Score             int       `json:"score"`
	Snippet           string    `json:"snippet,omitempty"`
	LastSnapshottedAt time.Time `json:"last_snapshotted_at,omitempty"`
}
