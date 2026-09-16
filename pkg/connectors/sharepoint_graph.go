package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// spGraphDefaultRateLimitBackoff is used when a 429/503 response carries no
// Retry-After header - Microsoft's own throttling guidance for Graph is "a
// short, randomized backoff," 30s is a conservative middle ground that
// avoids hammering a still-throttled endpoint without waiting excessively
// long when the real window is shorter.
const spGraphDefaultRateLimitBackoff = 30 * time.Second

// spGraphBaseURL is the stable v1.0 Microsoft Graph endpoint - deliberately
// not beta, since this connector only needs long-stable surfaces (sites,
// drives, delta, content, permissions).
const spGraphBaseURL = "https://graph.microsoft.com/v1.0"

// spDriveItemFields limits each delta/listing response to only the fields
// this connector reads - Graph, like Drive, returns a larger default field
// set otherwise.
const spDriveItemFields = "id,name,size,lastModifiedDateTime,file,folder,package,deleted"

type spGraphSite struct {
	ID string `json:"id"`
}

type spSitesResponse struct {
	Value    []spGraphSite `json:"value"`
	NextLink string        `json:"@odata.nextLink"`
}

type spGraphDrive struct {
	ID string `json:"id"`
}

type spDrivesResponse struct {
	Value []spGraphDrive `json:"value"`
}

type spGraphMyDrive struct {
	ID string `json:"id"`
}

// spGraphDriveItem is one file/folder/package as returned by a delta page.
// Deleted is non-nil when this item represents a deletion in the delta feed.
type spGraphDriveItem struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Size                 int64     `json:"size"`
	LastModifiedDateTime string    `json:"lastModifiedDateTime"`
	File                 *struct{} `json:"file,omitempty"`
	Folder               *struct{} `json:"folder,omitempty"`
	Package              *struct{} `json:"package,omitempty"`
	Deleted              *struct{} `json:"deleted,omitempty"`
}

type spDeltaResponse struct {
	Value     []spGraphDriveItem `json:"value"`
	NextLink  string             `json:"@odata.nextLink"`
	DeltaLink string             `json:"@odata.deltaLink"`
}

type spGraphSiteUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	LoginName   string `json:"loginName"`
}

type spGraphIdentity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type spGraphIdentitySet struct {
	User     *spGraphIdentity `json:"user,omitempty"`
	SiteUser *spGraphSiteUser `json:"siteUser,omitempty"`
}

type spGraphPermission struct {
	GrantedToV2           *spGraphIdentitySet  `json:"grantedToV2,omitempty"`
	GrantedToIdentitiesV2 []spGraphIdentitySet `json:"grantedToIdentitiesV2,omitempty"`
	Invitation            *struct {
		Email string `json:"email"`
	} `json:"invitation,omitempty"`
	Link *struct {
		Scope string `json:"scope"`
	} `json:"link,omitempty"`
}

type spPermissionsResponse struct {
	Value []spGraphPermission `json:"value"`
}

// spGraphClient is a minimal hand-rolled Microsoft Graph HTTP client - no
// official Go SDK is vendored in this module, and the connector only needs
// five call shapes (sites, drives, delta, content, permissions), not the
// full Graph surface a generated SDK would bring in.
type spGraphClient struct {
	http *http.Client
}

// spGraphError carries the HTTP status so callers can special-case 410 Gone
// (expired delta link, see deltaPage's doc comment) and 429/503 throttling
// (see RetryAfter/RateLimited, types.go).
type spGraphError struct {
	Status      int
	Location    string
	Body        string
	retryAfter  time.Duration
	isRateLimit bool
}

func (e *spGraphError) Error() string {
	return fmt.Sprintf("microsoft graph returned %d: %s", e.Status, e.Body)
}

// RetryAfter implements RateLimited (types.go) - isRateLimit is only true
// for a 429/503 response; every other spGraphError reports (0, false).
func (e *spGraphError) RetryAfter() (time.Duration, bool) {
	if !e.isRateLimit {
		return 0, false
	}
	return e.retryAfter, true
}

var _ RateLimited = (*spGraphError)(nil)

func (c *spGraphClient) get(ctx context.Context, url string, out any) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		gerr := &spGraphError{Status: resp.StatusCode, Location: resp.Header.Get("Location"), Body: string(body)}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			gerr.isRateLimit = true
			if delay, ok := parseRetryAfter(resp.Header); ok {
				gerr.retryAfter = delay
			} else {
				gerr.retryAfter = spGraphDefaultRateLimitBackoff
			}
		}
		return resp, gerr
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode graph response: %w", err)
		}
	}
	return resp, nil
}

// listSites paginates GET /sites?search=* — a broad search matching every
// site the connected account can access.
func (c *spGraphClient) listSites(ctx context.Context) ([]spGraphSite, error) {
	var sites []spGraphSite
	url := spGraphBaseURL + "/sites?search=*&$select=id"
	for url != "" {
		var page spSitesResponse
		if _, err := c.get(ctx, url, &page); err != nil {
			return nil, fmt.Errorf("list sites: %w", err)
		}
		sites = append(sites, page.Value...)
		url = page.NextLink
	}
	return sites, nil
}

// listDrives returns a site's document libraries (drives).
func (c *spGraphClient) listDrives(ctx context.Context, siteID string) ([]spGraphDrive, error) {
	var page spDrivesResponse
	url := spGraphBaseURL + "/sites/" + siteID + "/drives?$select=id"
	if _, err := c.get(ctx, url, &page); err != nil {
		return nil, fmt.Errorf("list drives for site %s: %w", siteID, err)
	}
	return page.Value, nil
}

// myDriveID returns the connected account's own OneDrive drive ID
// (GET /me/drive) - used by OneDriveConnector, which (unlike
// SharePointConnector) only ever touches exactly one drive: the signed-in
// user's personal one, never a site's document library.
func (c *spGraphClient) myDriveID(ctx context.Context) (string, error) {
	var drive spGraphMyDrive
	if _, err := c.get(ctx, spGraphBaseURL+"/me/drive?$select=id", &drive); err != nil {
		return "", fmt.Errorf("get my drive: %w", err)
	}
	if drive.ID == "" {
		return "", fmt.Errorf("me/drive response missing id")
	}
	return drive.ID, nil
}

// deltaPage returns every item changed since deltaLink (empty = full
// enumeration, matching Discover/a first-ever Sync) for one drive, paging
// through @odata.nextLink internally and returning once @odata.deltaLink is
// reached — the caller only ever sees one logical "page" (potentially
// large) plus the new deltaLink to persist as this drive's cursor.
//
// A 410 Gone response (an expired/invalidated delta link) carries a fresh,
// token-less delta URL in its Location header; that's treated as "start
// this drive over from a full enumeration" rather than an error, since it's
// an expected, if infrequent, real-world occurrence for a long-lived synced
// account.
func (c *spGraphClient) deltaPage(ctx context.Context, driveID, deltaLink string) ([]spGraphDriveItem, string, error) {
	url := deltaLink
	if url == "" {
		url = spGraphBaseURL + "/drives/" + driveID + "/root/delta?$select=" + spDriveItemFields
	}

	var items []spGraphDriveItem
	for {
		var page spDeltaResponse
		_, err := c.get(ctx, url, &page)
		if err != nil {
			var gerr *spGraphError
			if isSPGraphError(err, &gerr) && gerr.Status == http.StatusGone && gerr.Location != "" {
				// Expired delta link - restart this drive's enumeration from
				// the fresh URL Graph handed back, discarding whatever
				// partial page we'd accumulated so far (it may overlap/
				// conflict with the restarted sequence).
				items = nil
				url = gerr.Location
				continue
			}
			return nil, "", fmt.Errorf("delta page for drive %s: %w", driveID, err)
		}
		items = append(items, page.Value...)
		if page.DeltaLink != "" {
			return items, page.DeltaLink, nil
		}
		if page.NextLink == "" {
			// Shouldn't happen (Graph always returns either nextLink or
			// deltaLink), but avoid an infinite loop if it ever does.
			return items, deltaLink, nil
		}
		url = page.NextLink
	}
}

func isSPGraphError(err error, target **spGraphError) bool {
	gerr, ok := err.(*spGraphError)
	if ok {
		*target = gerr
	}
	return ok
}

// downloadContent fetches a DriveItem's raw bytes. Unlike Google Docs/
// Slides, no native-format export step exists for SharePoint content —
// Office files (.docx/.pptx/.xlsx) are real binary files and download
// directly, same as a PDF. Graph responds with a 302 to a pre-authenticated,
// time-limited URL; Go's default http.Client follows redirects
// transparently, so no special handling is needed here.
func (c *spGraphClient) downloadContent(ctx context.Context, driveID, itemID string, maxBytes int64) ([]byte, error) {
	url := spGraphBaseURL + "/drives/" + driveID + "/items/" + itemID + "/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &spGraphError{Status: resp.StatusCode, Body: string(body)}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// listPermissions returns a DriveItem's access list, mapped by the caller
// into an AccessEntry slice.
func (c *spGraphClient) listPermissions(ctx context.Context, driveID, itemID string) ([]spGraphPermission, error) {
	var page spPermissionsResponse
	url := spGraphBaseURL + "/drives/" + driveID + "/items/" + itemID + "/permissions"
	if _, err := c.get(ctx, url, &page); err != nil {
		return nil, fmt.Errorf("list permissions for item %s: %w", itemID, err)
	}
	return page.Value, nil
}
