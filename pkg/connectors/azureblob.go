// Azure Blob Storage connector: shared-key auth (an account name + account
// key, an admin pastes in once), the same static-credential shape as S3 -
// not SAS tokens or Entra ID/OAuth, deliberately (see this connector's
// entry in ROADMAP.md for why: this is an admin-configured infrastructure
// credential, not a per-employee identity, so the OAuth machinery
// SharePoint needs doesn't apply here).
//
// Like S3, Azure's flat blob listing has no "changes" API to page through -
// Sync's cursor is an RFC3339Nano timestamp, same full-list-then-filter
// approach as S3Connector.Sync.
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// maxAzureBlobBytes bounds a single blob's downloaded content - same cap as
// S3's maxS3ObjectBytes.
const maxAzureBlobBytes = 20 * 1024 * 1024 // 20MB

// AzureBlobAccountConfig is the non-secret connection config for one
// connected account - account name/container/prefix aren't secrets, so
// they don't belong in Secret. Secret.AccessToken holds the storage
// account's shared key; Secret.RefreshToken stays unused - shared-key auth
// needs only one secret, unlike S3's access-key-ID/secret-access-key pair.
type AzureBlobAccountConfig struct {
	AccountName string `json:"account_name"`
	Container   string `json:"container"`
	Prefix      string `json:"prefix"`
}

// ParseAzureBlobAccountConfig decodes a config string into its Azure Blob
// config - used by a caller's service layer to validate a connect request's
// config shape using the same rules Discover/Sync will apply.
func ParseAzureBlobAccountConfig(config string) (AzureBlobAccountConfig, error) {
	var cfg AzureBlobAccountConfig
	if strings.TrimSpace(config) == "" {
		return cfg, fmt.Errorf("azure blob account config is missing (account_name/container)")
	}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid azure blob account config: %w", err)
	}
	if cfg.AccountName == "" || cfg.Container == "" {
		return cfg, fmt.Errorf("azure blob account config requires at least account_name and container")
	}
	return cfg, nil
}

// EncodeAzureBlobAccountConfig is ParseAzureBlobAccountConfig's inverse, for
// a caller's service layer to build the config string value it stores at
// connect time.
func EncodeAzureBlobAccountConfig(accountName, container, prefix string) (string, error) {
	data, err := json.Marshal(AzureBlobAccountConfig{AccountName: accountName, Container: container, Prefix: prefix})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AzureBlobConnector implements Azure Blob Storage Discover/Sync.
type AzureBlobConnector struct {
	// extractText is wired via WithTextExtractor, normally to the caller's
	// own document-reader extraction, so connector-synced content goes
	// through the exact same extraction path as an uploaded file.
	extractText TextExtractorFunc
}

func NewAzureBlobConnector() *AzureBlobConnector {
	return &AzureBlobConnector{}
}

func (c *AzureBlobConnector) WithTextExtractor(fn TextExtractorFunc) *AzureBlobConnector {
	c.extractText = fn
	return c
}

// azureBlobServiceURL builds a storage account's service URL - a func var,
// not a fixed format string, so tests can point it at a local Azurite
// emulator (a different URL shape entirely: the account name is a path
// segment, not a subdomain) instead of the real Azure endpoint. See
// SetServiceURLForTesting.
var azureBlobServiceURL = func(accountName string) string {
	return fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
}

// SetServiceURLForTesting points this package's Azure Blob service URL
// builder at a test server (e.g. a local Azurite emulator), returning a
// function that restores the real value. Never call this outside a test.
func SetServiceURLForTesting(build func(accountName string) string) (restore func()) {
	original := azureBlobServiceURL
	azureBlobServiceURL = build
	return func() { azureBlobServiceURL = original }
}

// client builds an Azure Blob client for one call, using the connected
// account's own shared key - not a shared/cached client, since each account
// can point at a different storage account/container entirely.
func (c *AzureBlobConnector) client(secret Secret, cfg AzureBlobAccountConfig) (*azblob.Client, error) {
	if secret.AccessToken == "" {
		return nil, fmt.Errorf("azure blob account is missing an account key")
	}
	cred, err := azblob.NewSharedKeyCredential(cfg.AccountName, secret.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("build azure shared key credential: %w", err)
	}
	return azblob.NewClientWithSharedKeyCredential(azureBlobServiceURL(cfg.AccountName), cred, nil)
}

// blobEntry is a plain-data view of one listed blob - keeps the SDK's own
// generated response types out of this connector's exported surface.
type blobEntry struct {
	Name         string
	LastModified time.Time
	Size         int64
	ContentType  string
}

func azureBlobPrefixArg(cfg AzureBlobAccountConfig) *string {
	if cfg.Prefix == "" {
		return nil
	}
	return &cfg.Prefix
}

// listBlobs pages through every blob in the account's configured container
// (optionally scoped to a prefix), invoking visit for each one - shared by
// Discover and Sync so the pagination logic lives in one place.
func (c *AzureBlobConnector) listBlobs(ctx context.Context, cl *azblob.Client, cfg AzureBlobAccountConfig, visit func(blobEntry)) error {
	pager := cl.NewListBlobsFlatPager(cfg.Container, &azblob.ListBlobsFlatOptions{
		Prefix: azureBlobPrefixArg(cfg),
	})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list azure blobs: %w", err)
		}
		if page.Segment == nil {
			continue
		}
		for _, item := range page.Segment.BlobItems {
			if item == nil || item.Name == nil {
				continue
			}
			entry := blobEntry{Name: *item.Name}
			if item.Properties != nil {
				if item.Properties.LastModified != nil {
					entry.LastModified = *item.Properties.LastModified
				}
				if item.Properties.ContentLength != nil {
					entry.Size = *item.Properties.ContentLength
				}
				if item.Properties.ContentType != nil {
					entry.ContentType = *item.Properties.ContentType
				}
			}
			visit(entry)
		}
	}
	return nil
}

// Discover lists blobs in the account's container without downloading
// content - a lightweight preview of what a Sync would pull in. config is
// the JSON string ParseAzureBlobAccountConfig/EncodeAzureBlobAccountConfig
// round-trip.
func (c *AzureBlobConnector) Discover(ctx context.Context, config string, secret Secret) ([]ResourceRef, error) {
	cfg, err := ParseAzureBlobAccountConfig(config)
	if err != nil {
		return nil, err
	}
	cl, err := c.client(secret, cfg)
	if err != nil {
		return nil, err
	}

	var refs []ResourceRef
	err = c.listBlobs(ctx, cl, cfg, func(entry blobEntry) {
		if !isAllowedS3Key(entry.Name) {
			// Reuses S3's extension allowlist - same "no source/spreadsheet
			// files, RAG-relevant business documents only" reasoning
			// applies to any object-store connector.
			return
		}
		refs = append(refs, blobEntryToResourceRef(entry))
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") by listing and downloading every allowed
// blob, or (cursor != "", an RFC3339Nano timestamp) re-lists everything but
// only downloads blobs modified after the cursor - Azure's flat blob list
// has no change-feed API either, matching S3Connector.Sync's exact
// reasoning. The returned cursor is captured before listing starts so a
// blob modified mid-sync isn't missed by the next incremental run.
func (c *AzureBlobConnector) Sync(ctx context.Context, config string, secret Secret, cursor string) ([]SyncItem, string, error) {
	cfg, err := ParseAzureBlobAccountConfig(config)
	if err != nil {
		return nil, "", err
	}
	cl, err := c.client(secret, cfg)
	if err != nil {
		return nil, "", err
	}

	since, err := DecodeTimeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	nextCursor := NewTimeCursorNow()

	var items []SyncItem
	err = c.listBlobs(ctx, cl, cfg, func(entry blobEntry) {
		if !isAllowedS3Key(entry.Name) {
			return
		}
		if !since.Since.IsZero() && !entry.LastModified.After(since.Since) {
			return
		}
		if item, ok := c.syncOneBlob(ctx, cl, cfg, entry); ok {
			items = append(items, item)
		}
	})
	if err != nil {
		return nil, "", err
	}
	return items, nextCursor.Encode(), nil
}

// syncOneBlob downloads and extracts one blob's content. ok=false is a
// normal skip (over maxAzureBlobBytes, download failure, empty extracted
// text), not an error.
func (c *AzureBlobConnector) syncOneBlob(ctx context.Context, cl *azblob.Client, cfg AzureBlobAccountConfig, entry blobEntry) (SyncItem, bool) {
	if entry.Size > maxAzureBlobBytes {
		return SyncItem{}, false
	}
	resp, err := cl.DownloadStream(ctx, cfg.Container, entry.Name, nil)
	if err != nil {
		return SyncItem{}, false
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAzureBlobBytes))
	if err != nil || len(data) == 0 {
		return SyncItem{}, false
	}

	contentType := entry.ContentType
	text := string(data)
	if c.extractText != nil {
		text = c.extractText(ctx, data, contentType, path.Base(entry.Name))
	}
	if strings.TrimSpace(text) == "" && len(data) == 0 {
		return SyncItem{}, false
	}

	return SyncItem{
		ResourceRef: blobEntryToResourceRef(entry),
		Data:        data,
		ContentType: contentType,
		Text:        text,
		// No per-blob ACL: Azure Blob's container-level access model isn't
		// a portable per-object permission system - nil falls back to the
		// caller's own coarser scope, same as S3.
		AccessControl: nil,
	}, true
}

func blobEntryToResourceRef(entry blobEntry) ResourceRef {
	return ResourceRef{
		ID:       entry.Name,
		Name:     path.Base(entry.Name),
		Kind:     "file",
		Modified: entry.LastModified,
	}
}
