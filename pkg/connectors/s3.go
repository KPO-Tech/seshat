// S3 connector: any S3-compatible object store (AWS S3, MinIO, Cloudflare
// R2, Backblaze B2 - anything speaking the S3 REST API against a
// configurable endpoint).
//
// Unlike Drive/SharePoint, S3 has no "changes" API to page through - Sync's
// cursor is instead an RFC3339Nano timestamp: a full ListObjectsV2 pass
// every time, filtered to objects whose LastModified is after the cursor.
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// allowedS3Extensions is a business-document allowlist: no source/notebook
// code, no spreadsheets/tabular exports - structured data is a database/BI
// concern, not RAG's.
var allowedS3Extensions = map[string]bool{
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".ppt":  true,
	".pptx": true,
	".txt":  true,
	".md":   true,
}

func isAllowedS3Key(key string) bool {
	if strings.HasSuffix(key, "/") {
		return false // a folder marker, not an object with content
	}
	return allowedS3Extensions[strings.ToLower(path.Ext(key))]
}

// maxS3ObjectBytes bounds a single object's downloaded content.
const maxS3ObjectBytes = 20 * 1024 * 1024 // 20MB

// S3AccountConfig is the non-secret connection config for one connected
// account - bucket/region/endpoint/prefix aren't secrets, so they don't
// belong in Secret. Secret.AccessToken holds the S3 access key ID,
// RefreshToken the secret access key - a deliberate reinterpretation of
// those two generically-named fields, not a naming mistake.
type S3AccountConfig struct {
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
	Prefix   string `json:"prefix"`
}

// ParseS3AccountConfig decodes a config string into its S3 config - used by
// a caller's service layer to validate a connect request's config shape
// using the same rules Discover/Sync will apply.
func ParseS3AccountConfig(config string) (S3AccountConfig, error) {
	var cfg S3AccountConfig
	if strings.TrimSpace(config) == "" {
		return cfg, fmt.Errorf("s3 account config is missing (bucket/region/endpoint)")
	}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid s3 account config: %w", err)
	}
	if cfg.Bucket == "" || cfg.Endpoint == "" {
		return cfg, fmt.Errorf("s3 account config requires at least bucket and endpoint")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1" // AWS SDK requires a non-empty region even for non-AWS endpoints
	}
	return cfg, nil
}

// EncodeS3AccountConfig is ParseS3AccountConfig's inverse, for a caller's
// service layer to build the config string value it stores at connect time.
func EncodeS3AccountConfig(bucket, region, endpoint, prefix string) (string, error) {
	data, err := json.Marshal(S3AccountConfig{Bucket: bucket, Region: region, Endpoint: endpoint, Prefix: prefix})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// S3Connector implements S3-compatible Discover/Sync.
type S3Connector struct {
	// extractText is wired via WithTextExtractor, normally to the caller's
	// own docling-backed extraction, so connector-synced content goes
	// through the exact same extraction path as an uploaded file.
	extractText TextExtractorFunc
}

func NewS3Connector() *S3Connector {
	return &S3Connector{}
}

func (c *S3Connector) WithTextExtractor(fn TextExtractorFunc) *S3Connector {
	c.extractText = fn
	return c
}

// client builds an S3 client for one call, using the connected account's own
// static credentials and endpoint - not a shared/cached client, since each
// account can point at a different bucket/region/endpoint entirely.
func (c *S3Connector) client(ctx context.Context, secret Secret, cfg S3AccountConfig) (*s3.Client, error) {
	if secret.AccessToken == "" || secret.RefreshToken == "" {
		return nil, fmt.Errorf("s3 account is missing access key credentials")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			secret.AccessToken, secret.RefreshToken, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	}), nil
}

func s3PrefixArg(cfg S3AccountConfig) *string {
	if cfg.Prefix == "" {
		return nil
	}
	return aws.String(cfg.Prefix)
}

// Discover lists objects in the account's bucket without downloading
// content - a lightweight preview of what a Sync would pull in. config is
// the JSON string ParseS3AccountConfig/EncodeS3AccountConfig round-trip.
func (c *S3Connector) Discover(ctx context.Context, config string, secret Secret) ([]ResourceRef, error) {
	cfg, err := ParseS3AccountConfig(config)
	if err != nil {
		return nil, err
	}
	cl, err := c.client(ctx, secret, cfg)
	if err != nil {
		return nil, err
	}

	var refs []ResourceRef
	err = c.listObjects(ctx, cl, cfg, func(obj types.Object) {
		key := aws.ToString(obj.Key)
		if !isAllowedS3Key(key) {
			return
		}
		refs = append(refs, objectToS3ResourceRef(obj))
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") by listing and downloading every allowed
// object, or (cursor != "", an RFC3339Nano timestamp) re-lists everything
// but only downloads objects modified after the cursor - S3 has no
// change-feed API, so incrementality here means "re-list, filter by
// LastModified" rather than "resume a change stream". The returned cursor
// is captured before listing starts so an object modified mid-sync isn't
// missed by the next incremental run.
func (c *S3Connector) Sync(ctx context.Context, config string, secret Secret, cursor string) ([]SyncItem, string, error) {
	cfg, err := ParseS3AccountConfig(config)
	if err != nil {
		return nil, "", err
	}
	cl, err := c.client(ctx, secret, cfg)
	if err != nil {
		return nil, "", err
	}

	since, err := DecodeTimeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	nextCursor := NewTimeCursorNow()

	var items []SyncItem
	err = c.listObjects(ctx, cl, cfg, func(obj types.Object) {
		key := aws.ToString(obj.Key)
		if !isAllowedS3Key(key) {
			return
		}
		modified := aws.ToTime(obj.LastModified)
		if !since.Since.IsZero() && !modified.After(since.Since) {
			return
		}
		if item, ok := c.syncOneObject(ctx, cl, cfg, obj); ok {
			items = append(items, item)
		}
	})
	if err != nil {
		return nil, "", err
	}
	return items, nextCursor.Encode(), nil
}

// listObjects pages through every object under the account's configured
// prefix, invoking visit for each one - shared by Discover and Sync so the
// pagination logic lives in one place.
func (c *S3Connector) listObjects(ctx context.Context, cl *s3.Client, cfg S3AccountConfig, visit func(types.Object)) error {
	var continuationToken *string
	for {
		out, err := cl.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(cfg.Bucket),
			Prefix:            s3PrefixArg(cfg),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return fmt.Errorf("list s3 objects: %w", err)
		}
		for _, obj := range out.Contents {
			visit(obj)
		}
		if out.IsTruncated == nil || !*out.IsTruncated || out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}
	return nil
}

// syncOneObject downloads and extracts one object's content. ok=false is a
// normal skip (over maxS3ObjectBytes, download failure, empty extracted
// text), not an error.
func (c *S3Connector) syncOneObject(ctx context.Context, cl *s3.Client, cfg S3AccountConfig, obj types.Object) (SyncItem, bool) {
	if obj.Size != nil && *obj.Size > maxS3ObjectBytes {
		return SyncItem{}, false
	}
	key := aws.ToString(obj.Key)
	out, err := cl.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(cfg.Bucket), Key: aws.String(key)})
	if err != nil || out == nil || out.Body == nil {
		return SyncItem{}, false
	}
	defer out.Body.Close()
	data, err := io.ReadAll(io.LimitReader(out.Body, maxS3ObjectBytes))
	if err != nil || len(data) == 0 {
		return SyncItem{}, false
	}

	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	text := string(data)
	if c.extractText != nil {
		text = c.extractText(ctx, data, contentType, path.Base(key))
	}
	if strings.TrimSpace(text) == "" && len(data) == 0 {
		return SyncItem{}, false
	}

	return SyncItem{
		ResourceRef: objectToS3ResourceRef(obj),
		Data:        data,
		ContentType: contentType,
		Text:        text,
		// No per-object ACL: S3-compatible providers don't have a portable,
		// reliable per-object permission model across implementations - nil
		// falls back to the caller's own coarser scope.
		AccessControl: nil,
	}, true
}

func objectToS3ResourceRef(obj types.Object) ResourceRef {
	key := aws.ToString(obj.Key)
	return ResourceRef{
		ID:       key,
		Name:     path.Base(key),
		Kind:     "file",
		Modified: aws.ToTime(obj.LastModified),
	}
}
