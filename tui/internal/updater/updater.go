// Package updater lets the seshat binary find out that a newer release exists and replace itself with it.
//
// A release is a GitHub release of KPO-Tech/seshat-runtime that holds one binary per platform (seshat-linux-amd64,
// seshat-windows-amd64.exe, ...) and a SHA256SUMS.txt, the same files scripts/install.sh and scripts/install.ps1
// download. The binary is checked against the sums of the same release before it replaces the running one. That
// proves the download is intact; it does not prove who published it, which is the trust a user already places in
// the GitHub release when running the installer.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is the repository the releases are read from.
const DefaultRepo = "KPO-Tech/seshat-runtime"

// maxBinaryBytes bounds a download: a release binary is about 120 MB.
const maxBinaryBytes = 600 << 20

// Release is the part of a GitHub release the updater needs.
type Release struct {
	Tag    string
	Assets map[string]string // asset name -> download URL
}

// Client reads releases and downloads their files.
type Client struct {
	// APIBase is the GitHub API root; tests point it at a local server.
	APIBase string
	Repo    string
	HTTP    *http.Client
}

// NewClient returns a client for the official repository.
func NewClient() *Client {
	return &Client{APIBase: "https://api.github.com", Repo: DefaultRepo, HTTP: &http.Client{Timeout: 30 * time.Minute}}
}

func (c *Client) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "seshat-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return resp, nil
}

// Latest returns the latest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	resp, err := c.get(ctx, strings.TrimRight(c.APIBase, "/")+"/repos/"+c.Repo+"/releases/latest")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("read the release: %w", err)
	}
	if payload.Tag == "" {
		return Release{}, errors.New("the latest release has no tag")
	}
	release := Release{Tag: payload.Tag, Assets: make(map[string]string, len(payload.Assets))}
	for _, asset := range payload.Assets {
		release.Assets[asset.Name] = asset.URL
	}
	return release, nil
}

// AssetName is the name of the binary of a release for a platform, as the release workflow and the installers
// name it.
func AssetName(goos, goarch string) string {
	name := "seshat-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Newer reports whether latest is a higher release than current. Both are "vMAJOR.MINOR.PATCH" with an optional
// "-prerelease" and "+build"; anything else (a development build is "dev") is never updated, because there is
// nothing to compare it with. A prerelease is lower than its release.
func Newer(current, latest string) bool {
	c, okC := parseVersion(current)
	l, okL := parseVersion(latest)
	if !okC || !okL {
		return false
	}
	for i := range c.nums {
		if l.nums[i] != c.nums[i] {
			return l.nums[i] > c.nums[i]
		}
	}
	switch {
	case c.pre == l.pre:
		return false
	case c.pre == "":
		return false // current is the release, latest a prerelease of it
	case l.pre == "":
		return true
	default:
		return l.pre > c.pre
	}
}

// IsRelease reports whether raw is a release version (vMAJOR.MINOR.PATCH...), as opposed to a development build.
func IsRelease(raw string) bool {
	_, ok := parseVersion(raw)
	return ok
}

type version struct {
	nums [3]int
	pre  string
}

func parseVersion(raw string) (version, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	if i := strings.IndexByte(raw, '+'); i >= 0 {
		raw = raw[:i]
	}
	var v version
	if i := strings.IndexByte(raw, '-'); i >= 0 {
		v.pre = raw[i+1:]
		raw = raw[:i]
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, false
		}
		v.nums[i] = n
	}
	return v, true
}

// ChecksumFor finds the sum of file in the text of a SHA256SUMS.txt ("<hex>  <name>" or "<hex> *<name>").
func ChecksumFor(sums, file string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == file {
			sum := strings.ToLower(fields[0])
			if _, err := hex.DecodeString(sum); err != nil || len(sum) != sha256.Size*2 {
				return "", fmt.Errorf("the sum of %s is not a SHA-256", file)
			}
			return sum, nil
		}
	}
	return "", fmt.Errorf("%s is not listed in SHA256SUMS.txt", file)
}

// Download fetches the binary of release for this platform into a new file in dir, checks it against the sums of
// the release and returns its path. Nothing is left behind on failure.
func (c *Client) Download(ctx context.Context, release Release, goos, goarch, dir string) (string, error) {
	asset := AssetName(goos, goarch)
	binaryURL, ok := release.Assets[asset]
	if !ok {
		return "", fmt.Errorf("release %s has no %s: it does not include a build for this platform", release.Tag, asset)
	}
	sumsURL, ok := release.Assets["SHA256SUMS.txt"]
	if !ok {
		return "", fmt.Errorf("release %s has no SHA256SUMS.txt, so the download cannot be checked", release.Tag)
	}

	sumsResp, err := c.get(ctx, sumsURL)
	if err != nil {
		return "", fmt.Errorf("download the checksums: %w", err)
	}
	sumsText, err := io.ReadAll(io.LimitReader(sumsResp.Body, 1<<20))
	_ = sumsResp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("read the checksums: %w", err)
	}
	want, err := ChecksumFor(string(sumsText), asset)
	if err != nil {
		return "", err
	}

	resp, err := c.get(ctx, binaryURL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	defer resp.Body.Close()

	file, err := os.CreateTemp(dir, ".seshat-update-*")
	if err != nil {
		return "", fmt.Errorf("create a file next to the binary: %w", err)
	}
	path := file.Name()
	fail := func(err error) (string, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxBinaryBytes+1))
	if err != nil {
		return fail(fmt.Errorf("download %s: %w", asset, err))
	}
	if written > maxBinaryBytes {
		return fail(fmt.Errorf("%s is larger than %d MB", asset, maxBinaryBytes>>20))
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return fail(errors.New("the checksum of the download does not match SHA256SUMS.txt: nothing was installed"))
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// OldSuffix marks the previous binary kept aside during a replacement.
const OldSuffix = ".old"

// Replace puts the file at newPath in the place of the binary at exePath. The old binary is renamed aside first
// (a running executable can be renamed on every system, but only Unix lets it be overwritten), the new one is
// renamed into place, and the old one is put back if that fails. The renamed-aside file is removed by
// CleanupOld at the next start, or at once where the system allows it.
func Replace(exePath, newPath string) error {
	old := exePath + OldSuffix
	_ = os.Remove(old)
	if err := os.Rename(exePath, old); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("cannot replace %s (is the folder writable? re-run the installer if it is not): %w", exePath, err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		if rbErr := os.Rename(old, exePath); rbErr != nil {
			return fmt.Errorf("the new binary could not be put in place (%w) and the old one could not be restored (%w): it is at %s", err, rbErr, old)
		}
		_ = os.Remove(newPath)
		return fmt.Errorf("put the new binary in place: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(old)
	}
	return nil
}

// CleanupOld removes the previous binary a replacement left aside. It never fails.
func CleanupOld(exePath string) {
	_ = os.Remove(exePath + OldSuffix)
}

// CurrentExecutable is the path of the running binary with symbolic links resolved, so that an update replaces
// the file and not a link to it.
func CurrentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// Update downloads the release and replaces the binary at exePath with it.
func (c *Client) Update(ctx context.Context, release Release, exePath string) error {
	newPath, err := c.Download(ctx, release, runtime.GOOS, runtime.GOARCH, filepath.Dir(exePath))
	if err != nil {
		return err
	}
	return Replace(exePath, newPath)
}
