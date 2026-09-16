package connectors

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// --- pure unit tests, no MinIO needed ---

func TestIsAllowedS3Key(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"report.pdf", true},
		{"notes.docx", true},
		{"deck.pptx", true},
		{"readme.txt", true},
		{"notes.md", true},
		{"script.py", false},
		{"data.json", false},
		{"export.csv", false},
		{"sheet.xlsx", false},
		{"folder/", false},
		{"noextension", false},
	}
	for _, tc := range cases {
		if got := isAllowedS3Key(tc.key); got != tc.want {
			t.Errorf("isAllowedS3Key(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestParseS3AccountConfigRequiresBucketAndEndpoint(t *testing.T) {
	if _, err := ParseS3AccountConfig(""); err == nil {
		t.Fatal("expected empty config to be rejected")
	}
	if _, err := ParseS3AccountConfig(`{"region":"us-east-1"}`); err == nil {
		t.Fatal("expected config missing bucket/endpoint to be rejected")
	}
	cfg, err := ParseS3AccountConfig(`{"bucket":"b","endpoint":"http://localhost:9000"}`)
	if err != nil {
		t.Fatalf("expected minimal valid config to parse: %v", err)
	}
	if cfg.Region != "us-east-1" {
		t.Fatalf("expected default region to be filled in, got %q", cfg.Region)
	}
}

func TestEncodeS3AccountConfigRoundTrips(t *testing.T) {
	encoded, err := EncodeS3AccountConfig("my-bucket", "eu-west-1", "https://s3.example.com", "docs/")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	cfg, err := ParseS3AccountConfig(encoded)
	if err != nil {
		t.Fatalf("parse encoded config: %v", err)
	}
	if cfg.Bucket != "my-bucket" || cfg.Region != "eu-west-1" || cfg.Endpoint != "https://s3.example.com" || cfg.Prefix != "docs/" {
		t.Fatalf("round trip mismatch: %+v", cfg)
	}
}

// --- integration tests against a real, disposable MinIO container ---
//
// Ported from seshat-backend/internal/knowledge/s3/connector_test.go - the
// pure unit tests above already lived on both sides identically, but this
// live-container coverage only existed in seshat-backend. Since the actual
// Discover/Sync behavior under test now lives here, this is the more
// direct place for it to run.

type testMinIO struct {
	endpoint  string
	accessKey string
	secretKey string
}

// startTestMinIO runs a real MinIO container (docker run -d --rm, random
// host port) rather than a hand-rolled S3 stub. Skips (not fails) if Docker
// isn't available, so the pure unit tests above still run in environments
// without it.
func startTestMinIO(t *testing.T) testMinIO {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH - skipping s3 MinIO integration tests")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable - skipping s3 MinIO integration tests")
	}

	const accessKey = "minioadmin"
	const secretKey = "minioadmin123"
	containerName := fmt.Sprintf("core-connectors-s3-test-%d", time.Now().UnixNano())

	runCmd := exec.Command("docker", "run", "-d", "--rm",
		"-p", "127.0.0.1:0:9000",
		"-e", "MINIO_ROOT_USER="+accessKey,
		"-e", "MINIO_ROOT_PASSWORD="+secretKey,
		"--name", containerName,
		"minio/minio", "server", "/data",
	)
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("docker run minio: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	portOut, err := exec.Command("docker", "port", containerName, "9000/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	// Output looks like "127.0.0.1:54321" (possibly with a trailing newline
	// and, rarely, more than one mapping line - take the first).
	mapping := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	parts := strings.Split(mapping, ":")
	hostPort := parts[len(parts)-1]
	endpoint := "http://127.0.0.1:" + hostPort

	waitForMinIOReady(t, endpoint)
	return testMinIO{endpoint: endpoint, accessKey: accessKey, secretKey: secretKey}
}

func waitForMinIOReady(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/minio/health/live")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("minio container did not become ready in time")
}

func (m testMinIO) client(t *testing.T) *s3.Client {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(m.accessKey, m.secretKey, "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(m.endpoint)
		o.UsePathStyle = true
	})
}

func (m testMinIO) createBucketAndSeed(t *testing.T, bucket string, objects map[string]string) {
	t.Helper()
	cl := m.client(t)
	ctx := context.Background()
	if _, err := cl.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	for key, body := range objects {
		if _, err := cl.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader([]byte(body)),
		}); err != nil {
			t.Fatalf("put object %q: %v", key, err)
		}
	}
}

func TestDiscoverListsOnlyAllowedExtensions(t *testing.T) {
	minio := startTestMinIO(t)
	bucket := "discover-test"
	minio.createBucketAndSeed(t, bucket, map[string]string{
		"report.pdf": "%PDF-fake-content",
		"notes.txt":  "plain text notes",
		"data.csv":   "a,b,c\n1,2,3",
		"script.py":  "print('hi')",
		"folder/":    "",
	})

	c := NewS3Connector()
	cfg, err := EncodeS3AccountConfig(bucket, "us-east-1", minio.endpoint, "")
	if err != nil {
		t.Fatalf("EncodeS3AccountConfig: %v", err)
	}
	refs, err := c.Discover(context.Background(), cfg,
		Secret{AccessToken: minio.accessKey, RefreshToken: minio.secretKey},
	)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	names := map[string]bool{}
	for _, ref := range refs {
		names[ref.ID] = true
	}
	if !names["report.pdf"] || !names["notes.txt"] {
		t.Fatalf("expected allowed files to be discovered, got %+v", refs)
	}
	if names["data.csv"] || names["script.py"] {
		t.Fatalf("expected disallowed extensions to be filtered out, got %+v", refs)
	}
}

func TestSyncDownloadsAndExtractsAllowedObjects(t *testing.T) {
	minio := startTestMinIO(t)
	bucket := "sync-test"
	minio.createBucketAndSeed(t, bucket, map[string]string{
		"notes.txt": "hello from a real minio object",
		"data.csv":  "a,b,c\n1,2,3",
	})

	c := NewS3Connector()
	cfg, err := EncodeS3AccountConfig(bucket, "us-east-1", minio.endpoint, "")
	if err != nil {
		t.Fatalf("EncodeS3AccountConfig: %v", err)
	}
	items, nextCursor, err := c.Sync(context.Background(), cfg,
		Secret{AccessToken: minio.accessKey, RefreshToken: minio.secretKey},
		"",
	)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one allowed item synced, got %d: %+v", len(items), items)
	}
	if items[0].ID != "notes.txt" || items[0].Text != "hello from a real minio object" {
		t.Fatalf("unexpected synced item: %+v", items[0])
	}
	if nextCursor == "" {
		t.Fatal("expected a non-empty next cursor")
	}

	// An incremental sync using the returned cursor, with nothing new
	// uploaded since, must find nothing to sync.
	items2, _, err := c.Sync(context.Background(), cfg,
		Secret{AccessToken: minio.accessKey, RefreshToken: minio.secretKey},
		nextCursor,
	)
	if err != nil {
		t.Fatalf("incremental Sync: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("expected no items on an incremental sync with nothing new, got %d", len(items2))
	}
}

func TestSyncRejectsWrongCredentials(t *testing.T) {
	minio := startTestMinIO(t)
	bucket := "auth-test"
	minio.createBucketAndSeed(t, bucket, map[string]string{"notes.txt": "secret content"})

	c := NewS3Connector()
	cfg, err := EncodeS3AccountConfig(bucket, "us-east-1", minio.endpoint, "")
	if err != nil {
		t.Fatalf("EncodeS3AccountConfig: %v", err)
	}
	_, _, err = c.Sync(context.Background(), cfg,
		Secret{AccessToken: "wrong-key", RefreshToken: "wrong-secret"},
		"",
	)
	if err == nil {
		t.Fatal("expected wrong credentials to be rejected by the real MinIO server")
	}
}
