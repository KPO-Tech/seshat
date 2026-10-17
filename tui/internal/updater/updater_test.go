package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"v1.9.0", "v1.9.1", true},
		{"v1.9.0", "v1.10.0", true},
		{"v1.9.0", "v2.0.0", true},
		{"v1.9.0", "v1.9.0", false},
		{"v1.10.0", "v1.9.9", false},
		{"v1.9.0-rc1", "v1.9.0", true},
		{"v1.9.0", "v1.9.0-rc1", false},
		{"v1.9.0-rc1", "v1.9.0-rc2", true},
		{"1.9.0", "v1.9.1", true},
		{"v1.9.0+build5", "v1.9.1", true},
		{"dev", "v1.9.1", false},
		{"v1.9.0", "latest", false},
		{"", "v1.9.1", false},
		{"v1.9", "v1.9.1", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.current, tt.latest); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("linux", "amd64"); got != "seshat-linux-amd64" {
		t.Errorf("got %q", got)
	}
	if got := AssetName("windows", "arm64"); got != "seshat-windows-arm64.exe" {
		t.Errorf("got %q", got)
	}
}

func TestChecksumFor(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	sums := sum + "  seshat-linux-amd64\n" + strings.Repeat("cd", 32) + " *seshat-windows-amd64.exe\n"
	if got, err := ChecksumFor(sums, "seshat-linux-amd64"); err != nil || got != sum {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := ChecksumFor(sums, "seshat-windows-amd64.exe"); err != nil {
		t.Errorf("a binary-mode entry should be found: %v", err)
	}
	if _, err := ChecksumFor(sums, "seshat-darwin-arm64"); err == nil {
		t.Error("an unlisted file must be refused")
	}
	if _, err := ChecksumFor("nothex  seshat-linux-amd64\n", "seshat-linux-amd64"); err == nil {
		t.Error("a malformed sum must be refused")
	}
}

// releaseServer serves a fake GitHub: the latest release with a binary and its sums.
func releaseServer(t *testing.T, tag string, binary []byte, sumOf []byte) *httptest.Server {
	t.Helper()
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(sumOf)
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/repos/KPO-Tech/seshat-runtime/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":"SHA256SUMS.txt","browser_download_url":%q}]}`,
			tag, asset, server.URL+"/dl/"+asset, server.URL+"/dl/SHA256SUMS.txt")
	})
	mux.HandleFunc("/dl/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(binary) })
	mux.HandleFunc("/dl/SHA256SUMS.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func clientFor(server *httptest.Server) *Client {
	return &Client{APIBase: server.URL, Repo: DefaultRepo, HTTP: server.Client()}
}

func TestUpdateReplacesTheBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "seshat")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := []byte("new binary v1.10.0")
	client := clientFor(releaseServer(t, "v1.10.0", newBinary, newBinary))

	release, err := client.Latest(context.Background())
	if err != nil || release.Tag != "v1.10.0" {
		t.Fatalf("Latest = %+v, %v", release, err)
	}
	if err := client.Update(context.Background(), release, exe); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(newBinary) {
		t.Errorf("binary = %q, want the new one", got)
	}
	CleanupOld(exe)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only the binary to be left, got %d entries", len(entries))
	}
}

func TestUpdateRefusesABadChecksumAndKeepsTheOldBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "seshat")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The sums describe other bytes than the ones served.
	client := clientFor(releaseServer(t, "v1.10.0", []byte("tampered"), []byte("what was published")))
	release, err := client.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Update(context.Background(), release, exe); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Errorf("the old binary must be untouched, got %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the failed download must not be left behind, got %d entries", len(entries))
	}
}

func TestDownloadNeedsABuildForThePlatform(t *testing.T) {
	client := clientFor(releaseServer(t, "v1.10.0", []byte("x"), []byte("x")))
	release, _ := client.Latest(context.Background())
	if _, err := client.Download(context.Background(), release, "plan9", "mips", t.TempDir()); err == nil {
		t.Fatal("expected an error for a platform with no build")
	}
}

func TestReplaceRestoresTheOldBinaryWhenTheNewOneCannotBePlaced(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "seshat")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, filepath.Join(dir, "does-not-exist")); err == nil {
		t.Fatal("expected an error")
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Errorf("the old binary must be put back, got %q", got)
	}
}

func TestStateOfferAndRefresh(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	path := StatePath(t.TempDir())

	if st := LoadState(path); st.Latest != "" || !st.Due(now) {
		t.Fatalf("an empty state is due and offers nothing: %+v", st)
	}

	newBinary := []byte("n")
	client := clientFor(releaseServer(t, "v1.10.0", newBinary, newBinary))
	st := LoadState(path)
	st.Refresh(context.Background(), client, now)
	if st.Latest != "v1.10.0" || st.Due(now.Add(time.Hour)) {
		t.Fatalf("refresh should record the release and not be due again soon: %+v", st)
	}
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	st = LoadState(path)

	if tag, ok := st.Offer("v1.9.0", now); !ok || tag != "v1.10.0" {
		t.Errorf("expected an offer of v1.10.0, got %q %v", tag, ok)
	}
	if _, ok := st.Offer("v1.10.0", now); ok {
		t.Error("no offer when up to date")
	}
	if _, ok := st.Offer("dev", now); ok {
		t.Error("no offer to a development build")
	}

	st.SnoozeUntil = now.Add(SnoozeFor)
	if _, ok := st.Offer("v1.9.0", now); ok {
		t.Error("no offer while snoozed")
	}
	if _, ok := st.Offer("v1.9.0", now.Add(SnoozeFor+time.Minute)); !ok {
		t.Error("the offer comes back after the snooze")
	}

	st.SnoozeUntil = time.Time{}
	st.SkippedTag = "v1.10.0"
	if _, ok := st.Offer("v1.9.0", now); ok {
		t.Error("no offer for a skipped release")
	}
	st.Latest = "v1.11.0"
	if _, ok := st.Offer("v1.9.0", now); !ok {
		t.Error("a release newer than the skipped one is offered")
	}
}

func TestRefreshFailureRetriesLater(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close() // nothing listens any more
	st := State{}
	st.Refresh(context.Background(), clientFor(down), now)
	if st.Latest != "" {
		t.Fatalf("a failed check records no release: %+v", st)
	}
	if st.Due(now.Add(30 * time.Minute)) {
		t.Error("a failed check must not be retried at every start")
	}
	if !st.Due(now.Add(2 * time.Hour)) {
		t.Error("a failed check is retried after an hour")
	}
}
