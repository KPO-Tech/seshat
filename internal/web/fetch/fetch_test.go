package fetch

import (
	"context"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTMLToMarkdownPrefersReadableContent(t *testing.T) {
	input := `
<!doctype html>
<html>
  <body>
    <nav>Home Pricing Docs Sign in</nav>
    <article>
      <h1>Shipping V1</h1>
      <p>Nexus Engine now ships a browser-aware fetch pipeline for rendered pages.</p>
      <p>The extractor should keep the main article content and discard obvious chrome.</p>
    </article>
    <footer>newsletter links and social links</footer>
  </body>
</html>`

	output := HTMLToMarkdown(input, "https://example.com/blog/shipping-v1")
	if !strings.Contains(output, "Shipping V1") {
		t.Fatalf("expected article heading in output, got: %s", output)
	}
	if !strings.Contains(output, "browser-aware fetch pipeline") {
		t.Fatalf("expected article body in output, got: %s", output)
	}
	if strings.Contains(strings.ToLower(output), "pricing docs sign in") {
		t.Fatalf("expected navigation chrome to be removed, got: %s", output)
	}
}

func TestHTMLToMarkdownFallsBackToStructuredContent(t *testing.T) {
	input := `
<!doctype html>
<html>
  <body>
    <div class="sidebar">Version switcher</div>
    <div class="documentation">
      <h2>Browser Session</h2>
      <p>Each Nexus session owns an isolated browser context.</p>
      <p>Inactive sessions are reaped automatically.</p>
    </div>
  </body>
</html>`

	output := HTMLToMarkdown(input, "https://example.com/docs/browser-session")
	if !strings.Contains(output, "Browser Session") {
		t.Fatalf("expected structured fallback heading in output, got: %s", output)
	}
	if !strings.Contains(output, "isolated browser context") {
		t.Fatalf("expected structured fallback body in output, got: %s", output)
	}
}

type fakeHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.do(req)
}

type fakeArtifactStore struct {
	put func(ctx context.Context, key string, body []byte, contentType string) (storage.ArtifactRef, error)
}

func (f fakeArtifactStore) Put(ctx context.Context, key string, body []byte, contentType string) (storage.ArtifactRef, error) {
	return f.put(ctx, key, body, contentType)
}

func (f fakeArtifactStore) PutArtifact(ctx context.Context, request storage.ArtifactPutRequest, body []byte) (storage.ArtifactRef, error) {
	return f.put(ctx, storage.BuildArtifactKey(request), body, request.ContentType)
}

func (f fakeArtifactStore) Get(ctx context.Context, key string) ([]byte, error) { return nil, nil }
func (f fakeArtifactStore) OpenReader(ctx context.Context, key string) (io.ReadCloser, storage.ArtifactRef, error) {
	return io.NopCloser(strings.NewReader("")), storage.ArtifactRef{Key: key}, nil
}
func (f fakeArtifactStore) Stat(ctx context.Context, key string) (storage.ArtifactRef, error) {
	return storage.ArtifactRef{Key: key}, nil
}
func (f fakeArtifactStore) List(ctx context.Context, options storage.ListOptions) ([]storage.ArtifactRef, error) {
	return nil, nil
}
func (f fakeArtifactStore) Metadata(ctx context.Context, key string) (storage.ArtifactMetadata, error) {
	return storage.ArtifactMetadata{Key: key}, nil
}
func (f fakeArtifactStore) ListMetadata(ctx context.Context, options storage.ListOptions) ([]storage.ArtifactMetadata, error) {
	return nil, nil
}
func (f fakeArtifactStore) GarbageCollect(ctx context.Context, options storage.GCOptions) (storage.GCReport, error) {
	return storage.GCReport{}, nil
}
func (f fakeArtifactStore) Delete(ctx context.Context, key string) error         { return nil }
func (f fakeArtifactStore) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (f fakeArtifactStore) URL(ctx context.Context, key string) (string, error)  { return "", nil }

func TestFetchPersistsBinaryContent(t *testing.T) {
	storeCalled := false
	service := NewService(&Config{
		HTTPClient: fakeHTTPClient{
			do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/pdf"}},
					Body:       io.NopCloser(strings.NewReader("%PDF-1.7")),
				}, nil
			},
		},
		ArtifactStore: fakeArtifactStore{
			put: func(ctx context.Context, key string, body []byte, contentType string) (storage.ArtifactRef, error) {
				storeCalled = true
				if contentType != "application/pdf" {
					t.Fatalf("unexpected content type: %s", contentType)
				}
				return storage.ArtifactRef{
					Key:  key,
					URL:  "/tmp/fetched/document.pdf",
					Size: int64(len(body)),
				}, nil
			},
		},
	})

	fetched, err := service.Fetch(context.Background(), Request{URL: "https://example.com/report.pdf", RenderMode: RenderModeHTTP})
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if !storeCalled {
		t.Fatal("expected binary fetch to persist artifact")
	}
	if fetched.PersistedPath != "/tmp/fetched/document.pdf" {
		t.Fatalf("unexpected persisted path: %s", fetched.PersistedPath)
	}
	if fetched.PersistedSize == 0 {
		t.Fatal("expected persisted size to be populated")
	}
	if !strings.Contains(fetched.Content, "Stored at /tmp/fetched/document.pdf") {
		t.Fatalf("unexpected content summary: %s", fetched.Content)
	}
}

func TestNormalizeURLRejectsLocalTargets(t *testing.T) {
	tests := []string{
		"http://localhost:8080",
		"https://127.0.0.1/test",
		"https://10.0.0.5",
		"https://host.docker.internal/app",
	}
	for _, raw := range tests {
		if _, _, err := NormalizeURL(raw); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestNormalizeURLUpgradesHTTPToHTTPS(t *testing.T) {
	normalized, _, err := NormalizeURL("http://example.com/docs")
	if err != nil {
		t.Fatalf("NormalizeURL failed: %v", err)
	}
	if normalized != "https://example.com/docs" {
		t.Fatalf("unexpected normalized URL: %s", normalized)
	}
}
