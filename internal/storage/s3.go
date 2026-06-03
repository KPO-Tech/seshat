package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type S3Provider struct {
	client    *s3.Client
	bucket    string
	endpoint  string
	keyPrefix string
	region    string
}

func NewS3Provider() (*S3Provider, error) {
	return NewS3ProviderWithConfig(GetConfigFromEnv())
}

func NewS3ProviderWithConfig(cfg Config) (*S3Provider, error) {
	if cfg.S3Endpoint == "" {
		return nil, fmt.Errorf("S3 endpoint is required")
	}
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	if cfg.S3AccessKeyID == "" {
		return nil, fmt.Errorf("S3 access key ID is required")
	}
	if cfg.S3SecretAccessKey == "" {
		return nil, fmt.Errorf("S3 secret access key is required")
	}
	if cfg.S3Region == "" {
		cfg.S3Region = DefaultS3Region
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.S3Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID,
			cfg.S3SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		}
		o.UsePathStyle = true
	})

	return &S3Provider{
		client:    client,
		bucket:    cfg.S3Bucket,
		endpoint:  cfg.S3Endpoint,
		keyPrefix: cfg.S3KeyPrefix,
		region:    cfg.S3Region,
	}, nil
}

func (p *S3Provider) fullKey(key string) string {
	if p.keyPrefix == "" {
		return key
	}
	return p.keyPrefix + "/" + key
}

func (p *S3Provider) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	fullKey := p.fullKey(key)

	input := &s3.PutObjectInput{
		Bucket:      aws.String(p.bucket),
		Key:         aws.String(fullKey),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := p.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

func (p *S3Provider) Download(ctx context.Context, key string) ([]byte, error) {
	reader, _, err := p.OpenReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 response: %w", err)
	}

	return data, nil
}

func (p *S3Provider) Delete(ctx context.Context, key string) error {
	fullKey := p.fullKey(key)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullKey),
	}

	_, err := p.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}

	return nil
}

func (p *S3Provider) GetURL(ctx context.Context, key string) (string, error) {
	if p.endpoint != "" {
		return fmt.Sprintf("%s/%s/%s", p.endpoint, p.bucket, p.fullKey(key)), nil
	}
	return fmt.Sprintf("s3://%s/%s", p.bucket, p.fullKey(key)), nil
}

func (p *S3Provider) Exists(ctx context.Context, key string) (bool, error) {
	_, err := p.Stat(ctx, key)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "NotFound", "NoSuchKey", "NoSuchBucket":
				return false, nil
			}
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *S3Provider) OpenReader(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	fullKey := p.fullKey(key)

	input := &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullKey),
	}

	result, err := p.client.GetObject(ctx, input)
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("failed to open object from S3: %w", err)
	}
	info := ObjectInfo{
		Key:         key,
		ContentType: aws.ToString(result.ContentType),
		Size:        aws.ToInt64(result.ContentLength),
	}
	if result.LastModified != nil {
		info.ModifiedAt = result.LastModified.UTC()
	}
	if result.ETag != nil {
		info.ETag = strings.TrimSpace(aws.ToString(result.ETag))
	}
	info.URL, _ = p.GetURL(ctx, key)
	return result.Body, info, nil
}

func (p *S3Provider) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	fullKey := p.fullKey(key)
	input := &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullKey),
	}
	result, err := p.client.HeadObject(ctx, input)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("failed to stat object in S3: %w", err)
	}
	info := ObjectInfo{
		Key:         key,
		ContentType: aws.ToString(result.ContentType),
		Size:        aws.ToInt64(result.ContentLength),
	}
	if result.LastModified != nil {
		info.ModifiedAt = result.LastModified.UTC()
	}
	if result.ETag != nil {
		info.ETag = strings.TrimSpace(aws.ToString(result.ETag))
	}
	info.URL, _ = p.GetURL(ctx, key)
	return info, nil
}

func (p *S3Provider) List(ctx context.Context, options ListOptions) ([]ObjectInfo, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(p.bucket),
		Prefix: aws.String(p.fullKey(options.Prefix)),
	}
	if options.Limit > 0 {
		input.MaxKeys = aws.Int32(int32(options.Limit))
	}

	results := make([]ObjectInfo, 0)
	pager := s3.NewListObjectsV2Paginator(p.client, input)
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in S3: %w", err)
		}
		for _, object := range page.Contents {
			key := strings.TrimPrefix(aws.ToString(object.Key), strings.TrimSuffix(p.keyPrefix, "/")+"/")
			if p.keyPrefix == "" {
				key = aws.ToString(object.Key)
			}
			info := ObjectInfo{
				Key:        key,
				Size:       aws.ToInt64(object.Size),
				ModifiedAt: aws.ToTime(object.LastModified).UTC(),
			}
			info.URL, _ = p.GetURL(ctx, key)
			results = append(results, info)
			if options.Limit > 0 && len(results) >= options.Limit {
				return results, nil
			}
		}
	}
	return results, nil
}

var _ StorageProvider = (*S3Provider)(nil)
