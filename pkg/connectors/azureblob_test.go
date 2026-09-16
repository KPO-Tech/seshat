package connectors

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// --- pure unit tests, no Azurite needed ---

func TestParseAzureBlobAccountConfigRequiresAccountNameAndContainer(t *testing.T) {
	if _, err := ParseAzureBlobAccountConfig(""); err == nil {
		t.Fatal("expected empty config to be rejected")
	}
	if _, err := ParseAzureBlobAccountConfig(`{"prefix":"docs/"}`); err == nil {
		t.Fatal("expected config missing account_name/container to be rejected")
	}
	cfg, err := ParseAzureBlobAccountConfig(`{"account_name":"acct","container":"docs"}`)
	if err != nil {
		t.Fatalf("expected minimal valid config to parse: %v", err)
	}
	if cfg.AccountName != "acct" || cfg.Container != "docs" {
		t.Fatalf("unexpected parsed config: %+v", cfg)
	}
}

func TestEncodeAzureBlobAccountConfigRoundTrips(t *testing.T) {
	encoded, err := EncodeAzureBlobAccountConfig("myaccount", "mycontainer", "docs/")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	cfg, err := ParseAzureBlobAccountConfig(encoded)
	if err != nil {
		t.Fatalf("parse encoded config: %v", err)
	}
	if cfg.AccountName != "myaccount" || cfg.Container != "mycontainer" || cfg.Prefix != "docs/" {
		t.Fatalf("round trip mismatch: %+v", cfg)
	}
}

func TestBlobEntryToResourceRef(t *testing.T) {
	modified := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	ref := blobEntryToResourceRef(blobEntry{Name: "docs/report.pdf", LastModified: modified, Size: 1234})
	if ref.ID != "docs/report.pdf" || ref.Name != "report.pdf" || ref.Kind != "file" || !ref.Modified.Equal(modified) {
		t.Fatalf("unexpected resource ref: %+v", ref)
	}
}

// --- integration tests against a real, disposable Azurite emulator ---
//
// Azurite (mcr.microsoft.com/azure-storage/azurite) is Microsoft's own
// local Azure Storage emulator, speaking the real Blob REST API wire
// protocol - the same rigor as s3_test.go's MinIO integration tests, not a
// hand-rolled stub. Its account name/key are fixed, publicly documented
// defaults (not a real credential), the same as MinIO's well-known
// minioadmin/minioadmin123 used elsewhere in this file.

const (
	azuriteAccountName = "devstoreaccount1"
	azuriteAccountKey  = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
)

type testAzurite struct {
	endpoint string
}

// startTestAzurite runs a real Azurite container (docker run -d --rm,
// random host port). Skips (not fails) if Docker isn't available, so the
// pure unit tests above still run in environments without it.
func startTestAzurite(t *testing.T) testAzurite {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH - skipping azure blob Azurite integration tests")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable - skipping azure blob Azurite integration tests")
	}

	containerName := fmt.Sprintf("core-connectors-azureblob-test-%d", time.Now().UnixNano())
	runCmd := exec.Command("docker", "run", "-d", "--rm",
		"-p", "127.0.0.1:0:10000",
		"--name", containerName,
		"mcr.microsoft.com/azure-storage/azurite", "azurite-blob", "--blobHost", "0.0.0.0",
		// The azblob SDK sends whatever API version it was built against,
		// which can be newer than the Azurite image's own compatibility
		// table - Azurite's own error message for this points at this flag.
		"--skipApiVersionCheck",
	)
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("docker run azurite: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	portOut, err := exec.Command("docker", "port", containerName, "10000/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	mapping := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	parts := strings.Split(mapping, ":")
	hostPort := parts[len(parts)-1]
	endpoint := "http://127.0.0.1:" + hostPort

	waitForAzuriteReady(t, endpoint)
	return testAzurite{endpoint: endpoint}
}

func waitForAzuriteReady(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// Azurite has no dedicated health endpoint - a request against the
		// account root returns a real (if error) HTTP response once the
		// server is actually listening, which is all readiness needs here.
		resp, err := http.Get(endpoint + "/" + azuriteAccountName)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("azurite container did not become ready in time")
}

func (a testAzurite) client(t *testing.T) *azblob.Client {
	t.Helper()
	cred, err := azblob.NewSharedKeyCredential(azuriteAccountName, azuriteAccountKey)
	if err != nil {
		t.Fatalf("build shared key credential: %v", err)
	}
	cl, err := azblob.NewClientWithSharedKeyCredential(a.endpoint+"/"+azuriteAccountName, cred, nil)
	if err != nil {
		t.Fatalf("build azurite client: %v", err)
	}
	return cl
}

func (a testAzurite) createContainerAndSeed(t *testing.T, container string, blobs map[string]string) {
	t.Helper()
	cl := a.client(t)
	ctx := context.Background()
	if _, err := cl.CreateContainer(ctx, container, nil); err != nil {
		t.Fatalf("create container: %v", err)
	}
	for name, body := range blobs {
		if _, err := cl.UploadBuffer(ctx, container, name, []byte(body), nil); err != nil {
			t.Fatalf("upload blob %q: %v", name, err)
		}
	}
}

// swapAzureBlobServiceURLForTest points the connector's client construction
// at the Azurite endpoint (a completely different URL shape than real
// Azure - the account name is a path segment, not a subdomain).
func swapAzureBlobServiceURLForTest(t *testing.T, azurite testAzurite) {
	t.Helper()
	restore := SetServiceURLForTesting(func(accountName string) string {
		return azurite.endpoint + "/" + accountName
	})
	t.Cleanup(restore)
}

func TestAzureBlobDiscoverListsOnlyAllowedExtensions(t *testing.T) {
	azurite := startTestAzurite(t)
	swapAzureBlobServiceURLForTest(t, azurite)
	container := "discover-test"
	azurite.createContainerAndSeed(t, container, map[string]string{
		"report.pdf": "%PDF-fake-content",
		"notes.txt":  "plain text notes",
		"data.csv":   "a,b,c\n1,2,3",
		"script.py":  "print('hi')",
	})

	c := NewAzureBlobConnector()
	cfg, err := EncodeAzureBlobAccountConfig(azuriteAccountName, container, "")
	if err != nil {
		t.Fatalf("EncodeAzureBlobAccountConfig: %v", err)
	}
	refs, err := c.Discover(context.Background(), cfg, Secret{AccessToken: azuriteAccountKey})
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

func TestAzureBlobSyncDownloadsAndExtractsAllowedBlobs(t *testing.T) {
	azurite := startTestAzurite(t)
	swapAzureBlobServiceURLForTest(t, azurite)
	container := "sync-test"
	azurite.createContainerAndSeed(t, container, map[string]string{
		"notes.txt": "hello from a real azurite blob",
		"data.csv":  "a,b,c\n1,2,3",
	})

	c := NewAzureBlobConnector()
	cfg, err := EncodeAzureBlobAccountConfig(azuriteAccountName, container, "")
	if err != nil {
		t.Fatalf("EncodeAzureBlobAccountConfig: %v", err)
	}
	items, nextCursor, err := c.Sync(context.Background(), cfg, Secret{AccessToken: azuriteAccountKey}, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one allowed item synced, got %d: %+v", len(items), items)
	}
	if items[0].ID != "notes.txt" || items[0].Text != "hello from a real azurite blob" {
		t.Fatalf("unexpected synced item: %+v", items[0])
	}
	if nextCursor == "" {
		t.Fatal("expected a non-empty next cursor")
	}

	// An incremental sync using the returned cursor, with nothing new
	// uploaded since, must find nothing to sync.
	items2, _, err := c.Sync(context.Background(), cfg, Secret{AccessToken: azuriteAccountKey}, nextCursor)
	if err != nil {
		t.Fatalf("incremental Sync: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("expected no items on an incremental sync with nothing new, got %d", len(items2))
	}
}

func TestAzureBlobSyncRejectsWrongCredentials(t *testing.T) {
	azurite := startTestAzurite(t)
	swapAzureBlobServiceURLForTest(t, azurite)
	container := "auth-test"
	azurite.createContainerAndSeed(t, container, map[string]string{"notes.txt": "secret content"})

	c := NewAzureBlobConnector()
	cfg, err := EncodeAzureBlobAccountConfig(azuriteAccountName, container, "")
	if err != nil {
		t.Fatalf("EncodeAzureBlobAccountConfig: %v", err)
	}
	_, _, err = c.Sync(context.Background(), cfg, Secret{AccessToken: "d3Jvbmcta2V5LWJ1dC12YWxpZC1iYXNlNjQ="}, "")
	if err == nil {
		t.Fatal("expected wrong credentials to be rejected by the real Azurite server")
	}
}
