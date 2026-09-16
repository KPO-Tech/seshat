// SharePoint connector: SharePoint Online (read-only) via Microsoft Graph.
// Structurally mirrors gdrive.go, with two differences both because
// SharePoint/Graph works differently, not because this connector cuts
// corners:
//   - No native-format export step: Office files (.docx/.pptx/.xlsx) are
//     real binaries and download directly — there's no SharePoint
//     equivalent of a Google Doc with no native binary form.
//   - Delta (change tracking) is per-drive, not account-wide — a SharePoint
//     account can see many drives (one per site's document library), each
//     with its own delta link, so the sync cursor is a small JSON map
//     (driveID -> deltaLink) instead of Drive's single startPageToken. See
//     SPDriveCursor.
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// SharePointScopes requested from Microsoft - read-only Sites/Files access
// plus openid/profile/offline_access (the last is required to receive a
// refresh token, Microsoft's equivalent of Google's access_type=offline).
// Read-only by design: this is a Knowledge connector, never writes back to
// SharePoint.
var SharePointScopes = []string{
	"openid",
	"profile",
	"offline_access",
	"https://graph.microsoft.com/Files.Read.All",
	"https://graph.microsoft.com/Sites.Read.All",
}

// spAllowedExtensions is a business-document allowlist, not a code
// denylist - mirrors gdriveAllowedExtensions exactly, same rationale.
var spAllowedExtensions = map[string]bool{
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".ppt":  true,
	".pptx": true,
	".txt":  true,
	".md":   true,
}

// maxSPFileBytes bounds a single file's downloaded content - same cap and
// same rationale as Drive's.
const maxSPFileBytes = 20 * 1024 * 1024 // 20MB

// spIsAllowedFile reports whether item should be discovered/synced at all.
// Folders, "package" items (OneNote notebooks and similar - no directly
// downloadable content), and delta-feed deletions are never allowed
// regardless of name.
func spIsAllowedFile(item spGraphDriveItem) bool {
	if item.Deleted != nil || item.Folder != nil || item.Package != nil || item.File == nil {
		return false
	}
	return spAllowedExtensions[strings.ToLower(filepath.Ext(item.Name))]
}

// SharePointConnector implements SharePoint Discover/Sync.
type SharePointConnector struct {
	oauthConfig *oauth2.Config
	extractText TextExtractorFunc
}

func NewSharePointConnector(oauthConfig *oauth2.Config) *SharePointConnector {
	return &SharePointConnector{oauthConfig: oauthConfig}
}

func (c *SharePointConnector) WithTextExtractor(fn TextExtractorFunc) *SharePointConnector {
	c.extractText = fn
	return c
}

func (c *SharePointConnector) client(ctx context.Context, secret Secret) (*spGraphClient, error) {
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

// SPDriveCursor is the opaque per-drive delta-link map a caller persists as
// its own sync cursor between calls - see the package doc comment for why
// it's a map rather than one account-wide token. Callers of Sync never need
// to know its shape beyond encoding/decoding it as a plain string.
type SPDriveCursor map[string]string

func DecodeSPDriveCursor(raw string) SPDriveCursor {
	cursor := SPDriveCursor{}
	if raw == "" {
		return cursor
	}
	_ = json.Unmarshal([]byte(raw), &cursor)
	return cursor
}

func (c SPDriveCursor) Encode() string {
	data, err := json.Marshal(c)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// Discover lists files across every site/drive the connected account can
// access, without downloading content - a lightweight preview, mirroring
// GDriveConnector.Discover. Always does a fresh full enumeration per drive
// (deltaPage with no stored link), never touching a stored sync cursor.
func (c *SharePointConnector) Discover(ctx context.Context, secret Secret) ([]ResourceRef, error) {
	graph, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}
	drives, err := c.allDrives(ctx, graph)
	if err != nil {
		return nil, err
	}
	var refs []ResourceRef
	for _, driveID := range drives {
		items, _, err := graph.deltaPage(ctx, driveID, "")
		if err != nil {
			continue // one unreachable drive shouldn't fail the whole discovery
		}
		for _, item := range items {
			if !spIsAllowedFile(item) {
				continue
			}
			refs = append(refs, spDriveItemToResourceRef(driveID, item))
		}
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") or continues (cursor holds a per-drive
// delta-link map, see SPDriveCursor) - every drive the account can currently
// see is visited every call. A drive with no stored delta link yet gets a
// full enumeration just for that one drive, exactly like Discover; a drive
// that already has one only pulls what changed.
func (c *SharePointConnector) Sync(ctx context.Context, secret Secret, cursor string) ([]SyncItem, string, error) {
	graph, err := c.client(ctx, secret)
	if err != nil {
		return nil, "", err
	}
	drives, err := c.allDrives(ctx, graph)
	if err != nil {
		return nil, "", err
	}

	stored := DecodeSPDriveCursor(cursor)
	next := SPDriveCursor{}
	var items []SyncItem
	for _, driveID := range drives {
		pageItems, deltaLink, err := graph.deltaPage(ctx, driveID, stored[driveID])
		if err != nil {
			// One errored/unreachable drive shouldn't abort the whole sync -
			// keep whatever cursor it already had (retried next time) and
			// move on to the rest.
			if existing := stored[driveID]; existing != "" {
				next[driveID] = existing
			}
			continue
		}
		next[driveID] = deltaLink
		for _, item := range pageItems {
			if !spIsAllowedFile(item) {
				continue
			}
			if syncedItem, ok := c.syncOneItem(ctx, graph, driveID, item); ok {
				items = append(items, syncedItem)
			}
		}
	}
	return items, next.Encode(), nil
}

func (c *SharePointConnector) allDrives(ctx context.Context, graph *spGraphClient) ([]string, error) {
	sites, err := graph.listSites(ctx)
	if err != nil {
		return nil, err
	}
	var driveIDs []string
	for _, site := range sites {
		drives, err := graph.listDrives(ctx, site.ID)
		if err != nil {
			continue // one inaccessible site shouldn't fail the whole crawl
		}
		for _, d := range drives {
			driveIDs = append(driveIDs, d.ID)
		}
	}
	return driveIDs, nil
}

// syncOneItem downloads one file's content and permissions. ok=false is a
// normal skip, not an error, mirroring GDriveConnector.syncOneFile.
func (c *SharePointConnector) syncOneItem(ctx context.Context, graph *spGraphClient, driveID string, item spGraphDriveItem) (SyncItem, bool) {
	return graphSyncOneItem(ctx, graph, driveID, item, c.extractText)
}

// graphSyncOneItem downloads one Graph DriveItem's content and permissions,
// extracting its text - shared by SharePointConnector and OneDriveConnector,
// since both sync plain Graph drives the same way and only differ in which
// drive(s) they enumerate (every site's document library vs. exactly one
// personal drive). ok=false is a normal skip, not an error.
func graphSyncOneItem(ctx context.Context, graph *spGraphClient, driveID string, item spGraphDriveItem, extract TextExtractorFunc) (SyncItem, bool) {
	if item.Size > maxSPFileBytes {
		return SyncItem{}, false
	}
	data, err := graph.downloadContent(ctx, driveID, item.ID, maxSPFileBytes)
	if err != nil || len(data) == 0 {
		return SyncItem{}, false
	}
	text := graphExtractedText(ctx, data, item.Name, extract)
	if strings.TrimSpace(text) == "" && len(data) == 0 {
		return SyncItem{}, false
	}
	return SyncItem{
		ResourceRef:   spDriveItemToResourceRef(driveID, item),
		Data:          data,
		Text:          text,
		AccessControl: graphFetchAccessControl(ctx, graph, driveID, item.ID),
	}, true
}

// graphExtractedText mirrors GDriveConnector.downloadText's non-native
// branch: every Graph-drive file needs the same Docling-backed extraction
// an uploaded file gets, or its raw bytes get ingested as unsearchable
// noise. Plain-text/markdown pass through unchanged when no extractor is
// wired.
func graphExtractedText(ctx context.Context, data []byte, filename string, extract TextExtractorFunc) string {
	if extract == nil {
		return string(data)
	}
	return extract(ctx, data, "", filename)
}

// graphFetchAccessControl maps a DriveItem's permissions to prefixed
// identity strings - see spPermissionsToAccessControl for the actual
// mapping rules. Errors are swallowed (nil result), same discipline as
// GDriveConnector.fetchAccessControl.
func graphFetchAccessControl(ctx context.Context, graph *spGraphClient, driveID, itemID string) []AccessEntry {
	permissions, err := graph.listPermissions(ctx, driveID, itemID)
	if err != nil {
		return nil
	}
	return spPermissionsToAccessControl(permissions)
}

// spPermissionsToAccessControl is the pure mapping fetchAccessControl
// applies to whatever Graph returns - split out so it's unit-testable
// without a live *spGraphClient/HTTP mock, mirroring
// gdrivePermissionsToAccessControl.
//
// Deliberately conservative where Graph's identity data is ambiguous or
// incomplete:
//   - A granted user identity only becomes a "user:" entry when
//     siteUser.loginName is present (≈ the UPN/email on a work/school
//     tenant) - the bare user.id GUID+displayName pair Graph always returns
//     isn't enough to build a principal string that means anything to the
//     rest of this product's ACL model.
//   - An unredeemed email invitation becomes a "user:" entry directly
//     (invitation.email is reliably present, unlike grantedToV2.user).
//   - An "anonymous" sharing link becomes "public" (Drive's exact "anyone"
//     equivalent).
//   - An "organization" sharing link (anyone in the tenant) is NOT mapped
//     to "public" - that would broaden the item's visibility beyond what it
//     actually has, so it's simply skipped.
func spPermissionsToAccessControl(permissions []spGraphPermission) []AccessEntry {
	var acl []AccessEntry
	seen := map[AccessEntry]bool{}
	add := func(entry AccessEntry) {
		if entry == "" || seen[entry] {
			return
		}
		seen[entry] = true
		acl = append(acl, entry)
	}

	for _, p := range permissions {
		if p.GrantedToV2 != nil {
			spAddIdentitySetEntry(add, *p.GrantedToV2)
		}
		for _, ident := range p.GrantedToIdentitiesV2 {
			spAddIdentitySetEntry(add, ident)
		}
		if p.Invitation != nil && p.Invitation.Email != "" {
			add(AccessEntry("user:" + p.Invitation.Email))
		}
		if p.Link != nil && p.Link.Scope == "anonymous" {
			add(AccessEntry("public"))
		}
	}
	return acl
}

func spAddIdentitySetEntry(add func(AccessEntry), identity spGraphIdentitySet) {
	if identity.SiteUser != nil && identity.SiteUser.LoginName != "" {
		add(AccessEntry("user:" + identity.SiteUser.LoginName))
	}
	// No usable UPN/email otherwise (identity.User only ever carries an
	// opaque GUID + display name on this response) - skip rather than
	// guess, same restrictive-by-default handling as an unexploitable
	// permission entry.
}

// spDriveItemToResourceRef composites driveID into ResourceRef.ID (a plain
// Graph item ID is only unique within its own drive, not account-wide, and
// this connector spans many drives).
func spDriveItemToResourceRef(driveID string, item spGraphDriveItem) ResourceRef {
	modified, _ := time.Parse(time.RFC3339, item.LastModifiedDateTime)
	return ResourceRef{
		ID:       driveID + ":" + item.ID,
		Name:     item.Name,
		Kind:     "file",
		Modified: modified,
	}
}
