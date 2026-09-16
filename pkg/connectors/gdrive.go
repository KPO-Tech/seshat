// Google Drive connector: Google Drive (read-only) via the Drive API v3.
package connectors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// gdriveDefaultRateLimitBackoff is used when Google's response doesn't
// include a Retry-After header - Drive's quota errors often don't, unlike
// some other Google APIs. 30s matches the same conservative default used
// for Microsoft Graph/Confluence (sharepoint_graph.go, confluence_atlassian.go).
const gdriveDefaultRateLimitBackoff = 30 * time.Second

// wrapGDriveRateLimit checks whether err is (or wraps, via errors.As) a
// *googleapi.Error indicating rate-limiting - a 429, or Drive's own way of
// signaling quota exhaustion via 403 with a "rateLimitExceeded"/
// "userRateLimitExceeded" reason - and if so wraps it as a RateLimited
// error (types.go) using the response's own Retry-After header when
// present, or gdriveDefaultRateLimitBackoff otherwise. Returns err
// unchanged (same value, safe to compare with ==) for anything else,
// including a nil err.
func wrapGDriveRateLimit(err error) error {
	if err == nil {
		return nil
	}
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return err
	}
	if gerr.Code != http.StatusTooManyRequests && !isGDriveQuotaReason(gerr) {
		return err
	}
	delay, ok := parseRetryAfter(gerr.Header)
	if !ok {
		delay = gdriveDefaultRateLimitBackoff
	}
	return &rateLimitWrap{err: err, retryAfter: delay, hasDelay: true}
}

// isGDriveQuotaReason reports whether a 403 googleapi.Error is Drive's way
// of signaling quota exhaustion rather than a genuine permission problem -
// both share HTTP 403, distinguished only by the structured reason code.
func isGDriveQuotaReason(gerr *googleapi.Error) bool {
	if gerr.Code != http.StatusForbidden {
		return false
	}
	for _, item := range gerr.Errors {
		if item.Reason == "rateLimitExceeded" || item.Reason == "userRateLimitExceeded" {
			return true
		}
	}
	return false
}

// GDriveScopes requested from Google - read-only Drive access plus the
// account's email (used by callers as the connector account's external
// identity). Read-only by design: this is a Knowledge connector, never
// writes back to Drive.
var GDriveScopes = []string{
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/userinfo.email",
}

// gdriveFileListFields/gdriveChangeListFields/gdrivePermissionFields limit
// each Drive API response to only the fields this connector reads - Drive
// returns a large default field set otherwise.
const (
	gdriveFileListFields   = "nextPageToken, files(id, name, mimeType, modifiedTime, trashed, size)"
	gdriveChangeListFields = "nextPageToken, newStartPageToken, changes(fileId, removed, file(id, name, mimeType, modifiedTime, trashed, size))"
	gdrivePermissionFields = "permissions(type, emailAddress, domain, role)"
)

// gdriveExportMimeTypes maps a Google-native document's mimeType to the
// mimeType requested from Files.Export - only Docs/Slides need this; every
// other allowed file downloads its raw bytes directly via
// Files.Get(...).Download(). Google Sheets is deliberately absent - same
// spreadsheet reasoning as gdriveAllowedExtensions's doc comment, and this
// map doubles as gdriveIsAllowedFile's "is this a native Google format we
// accept" check.
var gdriveExportMimeTypes = map[string]string{
	"application/vnd.google-apps.document":     "text/plain",
	"application/vnd.google-apps.presentation": "text/plain",
}

// gdriveFolderMimeType identifies a Drive folder, which Discover/Sync both
// skip - there's no content to ingest, only files.
const gdriveFolderMimeType = "application/vnd.google-apps.folder"

// gdriveAllowedExtensions is a business-document allowlist, not a code
// denylist: source/notebook files (.py, .ipynb, .rs, .c, ...) are
// deliberately never synced into Knowledge - not useful company knowledge,
// and a large .ipynb's raw JSON produces thousands of syntactically-
// fragmented, low-quality chunks that drown out genuinely relevant results
// from smaller, well-formed documents. Checked by extension rather than
// Drive's own mimeType: Drive commonly reports arbitrary code/text files as
// generic "text/plain", so mimeType alone can't reliably tell a script from
// a real text document.
//
// Deliberately excludes spreadsheet formats (.xls/.xlsx/.csv, and Google
// Sheets) even though they're not code: a spreadsheet is usually either a
// small table authored to be read or a raw structured-data export, and
// there's no reliable way to tell those apart from the file alone.
// Structured/tabular data is what a real database or BI tool answers
// precisely - RAG's fuzzy semantic search is the wrong tool for it
// regardless of chunking quality.
var gdriveAllowedExtensions = map[string]bool{
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".ppt":  true,
	".pptx": true,
	".txt":  true,
	".md":   true,
}

// gdriveIsAllowedFile reports whether f should be discovered/synced at all -
// Google-native files (Docs/Sheets/Slides, identified by mimeType, not a
// filename extension) are always allowed since gdriveExportMimeTypes
// already converts them to plain text; anything else must match
// gdriveAllowedExtensions.
func gdriveIsAllowedFile(f *drive.File) bool {
	if _, native := gdriveExportMimeTypes[f.MimeType]; native {
		return true
	}
	return gdriveAllowedExtensions[strings.ToLower(filepath.Ext(f.Name))]
}

// maxGDriveFileBytes bounds a single file's exported/downloaded content - a
// pragmatic cap so one huge file can't stall or blow up memory during a sync.
const maxGDriveFileBytes = 20 * 1024 * 1024 // 20MB

// GDriveConnector implements Google Drive Discover/Sync.
type GDriveConnector struct {
	oauthConfig *oauth2.Config
	extractText TextExtractorFunc
}

func NewGDriveConnector(oauthConfig *oauth2.Config) *GDriveConnector {
	return &GDriveConnector{oauthConfig: oauthConfig}
}

// WithTextExtractor wires binary-file extraction into downloadText - see
// TextExtractorFunc's doc comment. Returns the receiver so callers can
// chain it onto NewGDriveConnector.
func (c *GDriveConnector) WithTextExtractor(fn TextExtractorFunc) *GDriveConnector {
	c.extractText = fn
	return c
}

func (c *GDriveConnector) client(ctx context.Context, secret Secret) (*drive.Service, error) {
	if c.oauthConfig == nil {
		return nil, fmt.Errorf("google drive oauth is not configured")
	}
	token := &oauth2.Token{
		AccessToken:  secret.AccessToken,
		RefreshToken: secret.RefreshToken,
		Expiry:       secret.ExpiresAt,
	}
	ts := c.oauthConfig.TokenSource(ctx, token)
	return drive.NewService(ctx, option.WithTokenSource(ts))
}

// Discover lists files in the account's Drive without fetching their
// content - a lightweight preview of what a Sync would pull in.
func (c *GDriveConnector) Discover(ctx context.Context, secret Secret) (refs []ResourceRef, err error) {
	defer func() { err = wrapGDriveRateLimit(err) }()

	srv, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}

	pageToken := ""
	for {
		call := srv.Files.List().Q("trashed = false").Fields(googleapi.Field(gdriveFileListFields)).PageSize(200)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		list, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list drive files: %w", err)
		}
		for _, f := range list.Files {
			if f.MimeType == gdriveFolderMimeType || !gdriveIsAllowedFile(f) {
				continue
			}
			refs = append(refs, gdriveFileToResourceRef(f))
		}
		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") by listing and downloading every file, or
// pulls only what changed since cursor via Drive's Changes API.
func (c *GDriveConnector) Sync(ctx context.Context, secret Secret, cursor string) (items []SyncItem, nextCursor string, err error) {
	defer func() { err = wrapGDriveRateLimit(err) }()

	srv, err := c.client(ctx, secret)
	if err != nil {
		return nil, "", err
	}
	if cursor == "" {
		return c.bootstrapSync(ctx, srv)
	}
	return c.incrementalSync(ctx, srv, cursor)
}

func (c *GDriveConnector) bootstrapSync(ctx context.Context, srv *drive.Service) ([]SyncItem, string, error) {
	// Capture the current page token before listing so nothing that changes
	// during this (possibly long) initial sync is missed by the first
	// incremental sync that follows.
	startToken, err := srv.Changes.GetStartPageToken().Context(ctx).Do()
	if err != nil {
		return nil, "", fmt.Errorf("get drive start page token: %w", err)
	}

	var items []SyncItem
	pageToken := ""
	for {
		call := srv.Files.List().Q("trashed = false").Fields(googleapi.Field(gdriveFileListFields)).PageSize(100)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		list, err := call.Context(ctx).Do()
		if err != nil {
			return nil, "", fmt.Errorf("list drive files: %w", err)
		}
		for _, f := range list.Files {
			if f.MimeType == gdriveFolderMimeType || !gdriveIsAllowedFile(f) {
				continue
			}
			if item, ok := c.syncOneFile(ctx, srv, f); ok {
				items = append(items, item)
			}
		}
		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return items, startToken.StartPageToken, nil
}

func (c *GDriveConnector) incrementalSync(ctx context.Context, srv *drive.Service, cursor string) ([]SyncItem, string, error) {
	var items []SyncItem
	pageToken := cursor
	nextCursor := cursor
	for {
		list, err := srv.Changes.List(pageToken).Fields(googleapi.Field(gdriveChangeListFields)).PageSize(100).Context(ctx).Do()
		if err != nil {
			return nil, "", fmt.Errorf("list drive changes: %w", err)
		}
		for _, ch := range list.Changes {
			if ch.Removed || ch.File == nil || ch.File.Trashed || ch.File.MimeType == gdriveFolderMimeType || !gdriveIsAllowedFile(ch.File) {
				continue
			}
			if item, ok := c.syncOneFile(ctx, srv, ch.File); ok {
				items = append(items, item)
			}
		}
		if list.NewStartPageToken != "" {
			nextCursor = list.NewStartPageToken
		}
		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return items, nextCursor, nil
}

// syncOneFile fetches a file's content and permissions. ok=false is a
// normal skip, not an error - an unexportable Google-native type, a file
// over maxGDriveFileBytes, or content that fails to download all fall
// through to "not synced this round" rather than failing the whole batch.
func (c *GDriveConnector) syncOneFile(ctx context.Context, srv *drive.Service, f *drive.File) (SyncItem, bool) {
	if f.Size > maxGDriveFileBytes {
		return SyncItem{}, false
	}
	text, data, contentType, ok := c.downloadText(ctx, srv, f)
	if !ok {
		return SyncItem{}, false
	}
	return SyncItem{
		ResourceRef:   gdriveFileToResourceRef(f),
		Data:          data,
		ContentType:   contentType,
		Text:          text,
		AccessControl: c.fetchAccessControl(ctx, srv, f.Id),
	}, true
}

func (c *GDriveConnector) downloadText(ctx context.Context, srv *drive.Service, f *drive.File) (string, []byte, string, bool) {
	exportMime, exported := gdriveExportMimeTypes[f.MimeType]
	var resp *http.Response
	var err error
	contentType := f.MimeType
	if exported {
		resp, err = srv.Files.Export(f.Id, exportMime).Context(ctx).Download()
		contentType = exportMime
	} else {
		resp, err = srv.Files.Get(f.Id).Context(ctx).Download()
	}
	if err != nil || resp == nil {
		return "", nil, "", false
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGDriveFileBytes))
	if err != nil || len(data) == 0 {
		return "", nil, "", false
	}
	// Google-native exports are already plain text - no extraction needed.
	// Everything else downloaded raw (PDF, DOCX, images, ...) needs the same
	// Docling-backed extraction an uploaded file gets, or its raw bytes get
	// ingested as unsearchable noise.
	if !exported && c.extractText != nil {
		text := c.extractText(ctx, data, f.MimeType, f.Name)
		return text, data, contentType, true
	}
	return string(data), data, contentType, true
}

// fetchAccessControl maps Drive's per-file permissions to prefixed identity
// strings - "user:email", "group:email", "domain:name", or "public" for
// anyone-with-the-link. Errors are swallowed (nil result): the caller falls
// back to its own coarser scope when AccessControl is empty, so a
// permissions-fetch failure degrades to conservative, never to over-sharing.
func (c *GDriveConnector) fetchAccessControl(ctx context.Context, srv *drive.Service, fileID string) []AccessEntry {
	list, err := srv.Permissions.List(fileID).Fields(googleapi.Field(gdrivePermissionFields)).Context(ctx).Do()
	if err != nil {
		return nil
	}
	return gdrivePermissionsToAccessControl(list.Permissions)
}

// RefreshPermissions re-fetches the current Drive ACL for a batch of
// already-known file IDs, without re-fetching content - lets a caller keep
// CorpusFile.ExternalAccess fresh on its own (typically shorter) schedule,
// independent of content re-sync.
//
// Deliberately does NOT reuse fetchAccessControl, whose "swallow the error,
// return nil" contract is right for first-sync (an unknown file defaults to
// corpus-level access, never blocked) but wrong here: on a refresh pass,
// collapsing "API call failed" into the same nil/empty result as "confirmed
// zero restrictions" would let a transient error silently WIDEN access
// instead of just failing to narrow it. A file whose lookup fails here is
// omitted from the returned map entirely - the caller should leave that
// file's last-known ExternalAccess untouched rather than risk widening it.
func (c *GDriveConnector) RefreshPermissions(ctx context.Context, secret Secret, resourceIDs []string) (map[string][]AccessEntry, error) {
	srv, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]AccessEntry, len(resourceIDs))
	for _, id := range resourceIDs {
		list, listErr := srv.Permissions.List(id).Fields(googleapi.Field(gdrivePermissionFields)).Context(ctx).Do()
		if listErr != nil {
			// Rate-limiting almost certainly means every remaining ID in
			// this batch would fail identically - stop and surface it as
			// the call's own error (via the deferred wrap above) instead
			// of silently grinding through the rest and returning an
			// increasingly-empty map with no signal anything went wrong.
			// A non-rate-limit per-ID error (e.g. a deleted file, 404) is
			// still swallowed and simply omitted from the result, per this
			// method's documented contract above.
			if wrapped := wrapGDriveRateLimit(listErr); wrapped != listErr {
				return nil, wrapped
			}
			continue
		}
		out[id] = gdrivePermissionsToAccessControl(list.Permissions)
	}
	return out, nil
}

// gdrivePermissionsToAccessControl is the pure mapping fetchAccessControl
// applies to whatever the Drive API returns - split out so it's
// unit-testable without a live *drive.Service/HTTP mock.
func gdrivePermissionsToAccessControl(permissions []*drive.Permission) []AccessEntry {
	var acl []AccessEntry
	for _, p := range permissions {
		switch p.Type {
		case "user":
			if p.EmailAddress != "" {
				acl = append(acl, AccessEntry("user:"+p.EmailAddress))
			}
		case "group":
			if p.EmailAddress != "" {
				acl = append(acl, AccessEntry("group:"+p.EmailAddress))
			}
		case "domain":
			if p.Domain != "" {
				acl = append(acl, AccessEntry("domain:"+p.Domain))
			}
		case "anyone":
			acl = append(acl, AccessEntry("public"))
		}
	}
	return acl
}

func gdriveFileToResourceRef(f *drive.File) ResourceRef {
	modified, _ := time.Parse(time.RFC3339, f.ModifiedTime)
	return ResourceRef{
		ID:       f.Id,
		Name:     f.Name,
		Kind:     "file",
		Modified: modified,
	}
}
