// Package browser provides the shared local browser runtime used by Nexus tools and sessions.
package browser

import (
	"context"
	"sync"
	"time"

	"github.com/go-rod/rod"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const defaultBlankURL = "about:blank"

var (
	defaultManagerOnce sync.Once
	defaultManager     Manager
)

// DefaultManager returns the process-wide lazy browser manager.
func DefaultManager() Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = NewManager(nil)
	})
	return defaultManager
}

// RodManager manages a shared Chrome instance with per-session incognito contexts.
type RodManager struct {
	config *Config

	mu         sync.Mutex
	root       *rod.Browser
	sessions   map[types.SessionID]*sessionState
	closeOnce  sync.Once
	closeCh    chan struct{}
	reaperDone chan struct{}
}

type sessionState struct {
	mu            sync.Mutex
	id            types.SessionID
	createdAt     time.Time
	lastActivity  time.Time
	actionCount   int
	lastAction    string
	repeatCount   int
	incognito     *rod.Browser
	pages         map[string]*pageState
	pageTargets   map[string]string
	pageOrder     []string
	nextPageSeq   int
	activePageID  string
	networkLog    []NetworkEntry
	nextNetSeq    int64
	downloads     []DownloadEntry
	downloadByID  map[string]int
	downloadDir   string
	networkPolicy NetworkPolicy
	watchCancel   context.CancelFunc
	maxNetLog     int
	maxDownloads  int
}

type pageState struct {
	info                 PageInfo
	page                 *rod.Page
	targetID             string
	domRevision          string
	domRefs              map[string]domRef
	watchCancel          context.CancelFunc
	lastSnapshotAt       time.Time
	lastSnapshotText     string
	lastSnapshotHeadings []HeadingInfo
}

type snapshotPayload struct {
	URL      string        `json:"url"`
	Title    string        `json:"title"`
	Text     string        `json:"text"`
	Elements []ElementInfo `json:"elements"`
	Headings []HeadingInfo `json:"headings"`
}

type domRef struct {
	SelectorHint string
	Role         string
	Editable     bool
}

// NewManager creates a new Rod-backed browser manager.
func NewManager(config *Config) *RodManager {
	if config == nil {
		config = DefaultConfig()
	}
	return &RodManager{
		config:   config,
		sessions: make(map[types.SessionID]*sessionState),
		closeCh:  make(chan struct{}),
	}
}
