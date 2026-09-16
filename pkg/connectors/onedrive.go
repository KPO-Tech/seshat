// OneDrive for Business connector: a single employee's own personal drive
// via Microsoft Graph, reusing sharepoint_graph.go's spGraphClient (its
// delta/download/permissions primitives are keyed only by an opaque
// driveID, with no SharePoint-specific coupling) and sharepoint.go's shared
// graphSyncOneItem/spIsAllowedFile/spDriveItemToResourceRef. The only real
// difference from SharePointConnector: this connector always has exactly
// one drive (GET /me/drive), never a site to enumerate, so its cursor is a
// plain delta-link string rather than SPDriveCursor's per-drive map.
package connectors

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

// OneDriveScopes requested from Microsoft - read-only Files access plus
// openid/profile/offline_access (the last is required to receive a refresh
// token). No Sites.Read.All - this connector never touches a SharePoint
// site, only the connected user's own personal drive.
var OneDriveScopes = []string{
	"openid",
	"profile",
	"offline_access",
	"https://graph.microsoft.com/Files.Read.All",
}

// OneDriveConnector implements OneDrive for Business Discover/Sync.
type OneDriveConnector struct {
	oauthConfig *oauth2.Config
	extractText TextExtractorFunc
}

func NewOneDriveConnector(oauthConfig *oauth2.Config) *OneDriveConnector {
	return &OneDriveConnector{oauthConfig: oauthConfig}
}

func (c *OneDriveConnector) WithTextExtractor(fn TextExtractorFunc) *OneDriveConnector {
	c.extractText = fn
	return c
}

func (c *OneDriveConnector) client(ctx context.Context, secret Secret) (*spGraphClient, error) {
	if c.oauthConfig == nil {
		return nil, fmt.Errorf("microsoft graph oauth is not configured")
	}
	token := &oauth2.Token{
		AccessToken:  secret.AccessToken,
		RefreshToken: secret.RefreshToken,
		Expiry:       secret.ExpiresAt,
	}
	return &spGraphClient{http: c.oauthConfig.Client(ctx, token)}, nil
}

// Discover lists files in the connected account's own OneDrive, without
// downloading content - a lightweight preview, mirroring
// SharePointConnector.Discover. Always a fresh full enumeration (deltaPage
// with no stored link), never touching a stored sync cursor.
func (c *OneDriveConnector) Discover(ctx context.Context, secret Secret) ([]ResourceRef, error) {
	graph, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}
	driveID, err := graph.myDriveID(ctx)
	if err != nil {
		return nil, err
	}
	items, _, err := graph.deltaPage(ctx, driveID, "")
	if err != nil {
		return nil, err
	}
	var refs []ResourceRef
	for _, item := range items {
		if !spIsAllowedFile(item) {
			continue
		}
		refs = append(refs, spDriveItemToResourceRef(driveID, item))
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") or continues (cursor holds the drive's own
// delta link) - unlike SharePoint there is only ever one drive, so the
// cursor is that drive's plain delta-link string, not a per-drive map.
func (c *OneDriveConnector) Sync(ctx context.Context, secret Secret, cursor string) ([]SyncItem, string, error) {
	graph, err := c.client(ctx, secret)
	if err != nil {
		return nil, "", err
	}
	driveID, err := graph.myDriveID(ctx)
	if err != nil {
		return nil, "", err
	}
	items, deltaLink, err := graph.deltaPage(ctx, driveID, cursor)
	if err != nil {
		return nil, "", err
	}
	var out []SyncItem
	for _, item := range items {
		if !spIsAllowedFile(item) {
			continue
		}
		if syncedItem, ok := graphSyncOneItem(ctx, graph, driveID, item, c.extractText); ok {
			out = append(out, syncedItem)
		}
	}
	return out, deltaLink, nil
}
