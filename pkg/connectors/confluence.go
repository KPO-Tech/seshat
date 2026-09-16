// Confluence connector: Confluence Cloud (read-only, pages only) via
// Atlassian's REST APIs. Structurally different from every other OAuth
// connector in this package in two real ways, not cosmetic ones:
//
//   - Atlassian rotates refresh tokens on every use (the old one is
//     invalidated the instant a new one is issued) - unlike Google/
//     Microsoft's long-lived, reusable refresh tokens, which is why every
//     other connector here can get away with never persisting a refreshed
//     token back to the caller's store. This connector proactively
//     refreshes once per call and records the result on lastRefreshed - a
//     deliberate, contained stateful exception to this package's otherwise
//     stateless-per-call connector shape - so a caller can persist the
//     rotated pair via RefreshedToken() before the next call needs it.
//   - There is no fixed API base URL: every call is scoped under a
//     per-account cloud ID, resolved once per call via Atlassian's
//     accessible-resources endpoint (see confluence_atlassian.go).
//
// No native delta/changes API exists (unlike SharePoint/OneDrive's Graph
// delta) - Sync uses Confluence's CQL search sorted by lastmodified desc,
// stopping once it reaches the cursor boundary, the same "poll and refilter
// by modified-since" shape S3 already uses for the same reason.
package connectors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// ConfluenceScopes requested from Atlassian - read-only page/space access,
// identity (to resolve the connected account's email), and offline_access
// for a refresh token. Verified live against Atlassian's granular-scopes
// docs and the User Identity API docs before implementing.
var ConfluenceScopes = []string{
	"read:page:confluence",
	"read:space:confluence",
	"read:me",
	"offline_access",
}

// maxConfluenceResponseBytes bounds any single API response this connector
// reads (search results or one page's content) - same defensive cap used
// throughout this package.
const maxConfluenceResponseBytes = 20 * 1024 * 1024 // 20MB

// confluencePageCQL matches every page, newest-modified first - Sync relies
// on the descending order to stop early once it reaches its cursor
// boundary, rather than embedding a date literal in the CQL string itself.
const confluencePageCQL = "type=page order by lastmodified desc"

// ConfluenceConnector implements Confluence Cloud Discover/Sync.
type ConfluenceConnector struct {
	oauthConfig *oauth2.Config
	extractText TextExtractorFunc
	// lastRefreshed is set by client() when a call actually rotated the
	// refresh token - nil otherwise. See the package doc comment above for
	// why this one field is a deliberate exception to this package's
	// stateless-per-call shape. Read via RefreshedToken() after a
	// Discover/Sync call returns.
	lastRefreshed *oauth2.Token
}

func NewConfluenceConnector(oauthConfig *oauth2.Config) *ConfluenceConnector {
	return &ConfluenceConnector{oauthConfig: oauthConfig}
}

func (c *ConfluenceConnector) WithTextExtractor(fn TextExtractorFunc) *ConfluenceConnector {
	c.extractText = fn
	return c
}

// RefreshedToken reports the rotated credential pair from the most recent
// Discover/Sync call, if that call actually triggered a token refresh -
// ok=false means the stored refresh token is still the current one and
// nothing needs persisting.
func (c *ConfluenceConnector) RefreshedToken() (accessToken, refreshToken string, expiry time.Time, ok bool) {
	if c.lastRefreshed == nil {
		return "", "", time.Time{}, false
	}
	return c.lastRefreshed.AccessToken, c.lastRefreshed.RefreshToken, c.lastRefreshed.Expiry, true
}

// client eagerly resolves a usable token for this call (refreshing right
// away if secret's access token is expired, rather than relying on lazy
// refresh-on-first-request like every other connector) so a rotation is
// caught and recorded before any API call is made, then resolves this
// account's cloud ID and returns a client scoped to it.
func (c *ConfluenceConnector) client(ctx context.Context, secret Secret) (*confluenceClient, error) {
	if c.oauthConfig == nil {
		return nil, fmt.Errorf("atlassian oauth is not configured")
	}
	token := &oauth2.Token{
		AccessToken:  secret.AccessToken,
		RefreshToken: secret.RefreshToken,
		Expiry:       secret.ExpiresAt,
	}
	fresh, err := c.oauthConfig.TokenSource(ctx, token).Token()
	if err != nil {
		return nil, fmt.Errorf("refresh atlassian token: %w", err)
	}
	if fresh.RefreshToken != "" && fresh.RefreshToken != secret.RefreshToken {
		c.lastRefreshed = fresh
	}
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(fresh))
	cloudID, err := resolveConfluenceCloudID(ctx, httpClient)
	if err != nil {
		return nil, err
	}
	return &confluenceClient{http: httpClient, baseURL: "https://api.atlassian.com/ex/confluence/" + cloudID}, nil
}

// Discover lists every page the connected account can see, without
// downloading content - a lightweight preview, mirroring
// SharePointConnector.Discover.
func (c *ConfluenceConnector) Discover(ctx context.Context, secret Secret) ([]ResourceRef, error) {
	cl, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}
	var refs []ResourceRef
	err = cl.searchPages(ctx, confluencePageCQL, func(item confluenceSearchResult) bool {
		if item.Content.Type != "page" {
			return true
		}
		modified, _ := parseConfluenceTime(item.LastModified)
		refs = append(refs, ResourceRef{ID: item.Content.ID, Name: item.Title, Kind: "file", Modified: modified})
		return true
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") or continues (cursor is an RFC3339Nano
// timestamp, same shape as S3's) - no change-feed API exists, so
// incrementality here means "search sorted newest-first, stop once past the
// cursor boundary" rather than resuming a change stream. The returned
// cursor is captured before the search starts, so a page modified mid-sync
// isn't missed by the next incremental run.
func (c *ConfluenceConnector) Sync(ctx context.Context, secret Secret, cursor string) ([]SyncItem, string, error) {
	cl, err := c.client(ctx, secret)
	if err != nil {
		return nil, "", err
	}

	since, err := DecodeTimeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	nextCursor := NewTimeCursorNow()

	var items []SyncItem
	err = cl.searchPages(ctx, confluencePageCQL, func(item confluenceSearchResult) bool {
		if item.Content.Type != "page" {
			return true
		}
		modified, ok := parseConfluenceTime(item.LastModified)
		if ok && !since.Since.IsZero() && !modified.After(since.Since) {
			// Sorted newest-modified-first - everything from here on is also
			// at or before the cursor, so the whole rest of the search can be
			// skipped rather than walked and filtered out one by one.
			return false
		}
		if syncedItem, ok := c.syncOnePage(ctx, cl, item, modified); ok {
			items = append(items, syncedItem)
		}
		return true
	})
	if err != nil {
		return nil, "", err
	}
	return items, nextCursor.Encode(), nil
}

// syncOnePage downloads one page's rendered content and extracts its text.
// ok=false is a normal skip (download failure, empty extracted text), not
// an error.
func (c *ConfluenceConnector) syncOnePage(ctx context.Context, cl *confluenceClient, item confluenceSearchResult, modified time.Time) (SyncItem, bool) {
	title, html, err := cl.getPageContent(ctx, item.Content.ID)
	if err != nil || strings.TrimSpace(html) == "" {
		return SyncItem{}, false
	}
	text := html
	if c.extractText != nil {
		// Unlike every other connector here (which pass an empty content
		// type since they're handing over opaque downloaded bytes),
		// export_view content is genuinely known to be HTML - a real hint
		// for the extractor, not a guess.
		text = c.extractText(ctx, []byte(html), "text/html", title)
		if strings.TrimSpace(text) == "" {
			return SyncItem{}, false
		}
	}
	return SyncItem{
		ResourceRef: ResourceRef{ID: item.Content.ID, Name: title, Kind: "file", Modified: modified},
		Data:        []byte(html),
		ContentType: "text/html",
		Text:        text,
		// No per-page ACL: Confluence's space+page permission model isn't
		// portably mappable the way Drive/SharePoint's is - nil falls back
		// to the caller's own coarser scope, same accepted precedent as
		// S3/Azure Blob.
		AccessControl: nil,
	}, true
}

// parseConfluenceTime tries the timestamp layouts Confluence's search API
// is documented to return - ok=false on a page whose timestamp doesn't
// parse is a normal skip from the cursor-boundary check's perspective, not
// a fatal error.
func parseConfluenceTime(raw string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
