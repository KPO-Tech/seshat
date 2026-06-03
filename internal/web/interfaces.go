package web

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

const (
	// RenderModeAuto lets the fetch core choose the cheapest backend.
	RenderModeAuto = "auto"
	// RenderModeHTTP forces the HTTP fast path.
	RenderModeHTTP = "http"
	// RenderModeBrowser forces JS-rendered browser extraction.
	RenderModeBrowser = "browser"
)

// BrowserManager exposes the shared browser runtime without tying callers to a concrete implementation.
type BrowserManager = browsercore.Manager

// ArtifactStore exposes persisted blob storage to reusable web services without coupling them to a provider.
type ArtifactStore = storage.ArtifactStore

// FetchRequest is the common input shape for reusable web fetch services.
type FetchRequest struct {
	URL        string
	RenderMode string
	SessionID  types.SessionID
}

// RedirectInfo signals that a host-changing redirect should be explicitly followed by the caller.
type RedirectInfo struct {
	Type        string `json:"type"`
	OriginalURL string `json:"originalUrl"`
	RedirectURL string `json:"redirectUrl"`
	StatusCode  int    `json:"statusCode"`
}

// FetchResponse is the normalized output shape shared by fetch callers.
type FetchResponse struct {
	Content            string
	Bytes              int
	Code               int
	CodeText           string
	ContentType        string
	FinalURL           string
	PersistedPath      string
	PersistedSize      int
	BrowserRecommended bool
	Mode               string
	Redirect           *RedirectInfo
}

// SearchRequest is the common input shape for reusable web search services.
type SearchRequest struct {
	Query          string
	AllowedDomains []string
	BlockedDomains []string
}

// SearchResult is a provider-agnostic search hit used across wrappers and services.
type SearchResult struct {
	Title       string
	URL         string
	Description string
	Source      string
}

// SearchResponse is the normalized output shape shared by search callers.
type SearchResponse struct {
	Query           string
	Results         []SearchResult
	Provider        string
	DurationSeconds float64
}

// MapRequest describes a bounded website discovery request.
type MapRequest struct {
	URL               string
	MaxURLs           int
	IncludeSubdomains bool
}

// MapEntry is one discovered canonical URL.
type MapEntry struct {
	URL    string
	Source string
}

// MapResponse is the normalized output shape shared by map callers.
type MapResponse struct {
	RootURL string
	URLs    []MapEntry
}

// CrawlRequest describes a bounded multi-page collection request.
type CrawlRequest struct {
	URL               string
	SessionID         types.SessionID
	MaxPages          int
	MaxDepth          int
	RenderMode        string
	IncludeSubdomains bool
	IncludePatterns   []string
	ExcludePatterns   []string
}

// CrawlPage represents one crawled page.
type CrawlPage struct {
	URL           string
	FinalURL      string
	Depth         int
	Mode          string
	Content       string
	PersistedPath string
	PersistedSize int
}

// CrawlResponse is the normalized output shape shared by crawl callers.
type CrawlResponse struct {
	RootURL string
	Pages   []CrawlPage
}

// FetchService represents the reusable fetch core exposed from internal/web.
type FetchService interface {
	Fetch(ctx context.Context, request FetchRequest) (FetchResponse, error)
}

// SearchService represents the reusable search core exposed from internal/web.
type SearchService interface {
	Search(ctx context.Context, request SearchRequest) (SearchResponse, error)
}

// MapService represents the reusable map core exposed from internal/web.
type MapService interface {
	Map(ctx context.Context, request MapRequest) (MapResponse, error)
}

// CrawlService represents the reusable crawl core exposed from internal/web.
type CrawlService interface {
	Crawl(ctx context.Context, request CrawlRequest) (CrawlResponse, error)
}
