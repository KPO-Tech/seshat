package titlemodel

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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
	"sync/atomic"
	"testing"
	"time"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarGzOf(t *testing.T, files map[string]string, links map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(content))
	}
	for name, target := range links {
		if err := tw.WriteHeader(&tar.Header{Name: name, Linkname: target, Typeflag: tar.TypeSymlink, Mode: 0o777}); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// fixture serves a fake llama.cpp archive and a fake model, and an Installer wired to them.
type fixture struct {
	in     *Installer
	hits   atomic.Int32
	server *httptest.Server
}

func newFixture(t *testing.T, archiveBytes []byte, archiveName string, modelBytes []byte, tamperModel bool) *fixture {
	t.Helper()
	f := &fixture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/llama/"+archiveName, func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/hf/org/repo/resolve/main/m.gguf", func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		if tamperModel {
			_, _ = w.Write([]byte("something else"))
			return
		}
		_, _ = w.Write(modelBytes)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	goos, goarch := runtime.GOOS, runtime.GOARCH
	f.in = &Installer{
		Root:      t.TempDir(),
		LlamaBase: f.server.URL + "/llama",
		HFBase:    f.server.URL + "/hf",
		HTTP:      f.server.Client(),
		Archives:  map[string]Archive{goos + "-" + goarch: {Name: archiveName, SHA256: sum(archiveBytes), Size: int64(len(archiveBytes))}},
		Models:    []Model{{ID: "tiny", Label: "Tiny", Repo: "org/repo", File: "m.gguf", SHA256: sum(modelBytes), Size: int64(len(modelBytes))}},
	}
	return f
}

func serverFileName() string { return serverBinaryName(runtime.GOOS) }

func archiveForHost(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return zipOf(t, files), "llama-test.zip"
	}
	return tarGzOf(t, files, nil), "llama-test.tar.gz"
}

func TestInstallDownloadsChecksAndRecords(t *testing.T) {
	archive, name := archiveForHost(t, map[string]string{"build/bin/" + serverFileName(): "fake server", "build/bin/libx.dll": "lib"})
	f := newFixture(t, archive, name, []byte("fake model weights"), false)

	st, err := f.in.Install(context.Background(), "tiny")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !st.Enabled || !st.Ready() || st.ModelID != "tiny" || st.LlamaTag != LlamaTag {
		t.Fatalf("unexpected state: %+v", st)
	}
	if got, _ := os.ReadFile(st.ModelPath); string(got) != "fake model weights" {
		t.Errorf("model = %q", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(st.ServerPath), "libx.dll")); err != nil {
		t.Errorf("the files next to the server must be unpacked too: %v", err)
	}
	if loaded := LoadState(f.in.Root); loaded != st {
		t.Errorf("the state must be recorded: %+v vs %+v", loaded, st)
	}

	// A second install finds both files intact and downloads nothing.
	before := f.hits.Load()
	if _, err := f.in.Install(context.Background(), "tiny"); err != nil {
		t.Fatal(err)
	}
	if f.hits.Load() != before {
		t.Errorf("an intact installation must not be downloaded again (%d hits more)", f.hits.Load()-before)
	}
}

func TestInstallRefusesAFileThatDoesNotMatchItsChecksum(t *testing.T) {
	archive, name := archiveForHost(t, map[string]string{serverFileName(): "fake server"})
	f := newFixture(t, archive, name, []byte("fake model weights"), true)

	_, err := f.in.Install(context.Background(), "tiny")
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if st := LoadState(f.in.Root); st.Enabled {
		t.Error("nothing may be recorded after a failed install")
	}
	models := filepath.Join(f.in.Root, "title-model", "models")
	if entries, _ := os.ReadDir(models); len(entries) != 0 {
		t.Errorf("the rejected download must not be kept, got %d files", len(entries))
	}
}

func TestInstallNeedsABuildForThePlatform(t *testing.T) {
	archive, name := archiveForHost(t, map[string]string{serverFileName(): "x"})
	f := newFixture(t, archive, name, []byte("m"), false)
	f.in.GOOS, f.in.GOARCH = "plan9", "mips"
	if _, err := f.in.Install(context.Background(), "tiny"); err == nil || !strings.Contains(err.Error(), "no llama.cpp build") {
		t.Fatalf("expected a clear error, got %v", err)
	}
	f.in.GOOS, f.in.GOARCH = "", ""
	if _, err := f.in.Install(context.Background(), "nope"); err == nil {
		t.Fatal("expected an error for an unknown model")
	}
}

func TestExtractionRefusesPathsOutsideTheFolder(t *testing.T) {
	dir := t.TempDir()
	evil := zipOf(t, map[string]string{"../escape.txt": "x"})
	zipPath := filepath.Join(dir, "evil.zip")
	if err := os.WriteFile(zipPath, evil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(zipPath, filepath.Join(dir, "out")); err == nil {
		t.Error("a zip entry outside the folder must be refused")
	}

	tgz := filepath.Join(dir, "evil.tar.gz")
	if err := os.WriteFile(tgz, tarGzOf(t, map[string]string{"../escape.txt": "x"}, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tgz, filepath.Join(dir, "out2")); err == nil {
		t.Error("a tar entry outside the folder must be refused")
	}

	linkTgz := filepath.Join(dir, "link.tar.gz")
	if err := os.WriteFile(linkTgz, tarGzOf(t, nil, map[string]string{"lib.so": "../../etc/passwd"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(linkTgz, filepath.Join(dir, "out3")); err == nil {
		t.Error("a link that leaves the folder must be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Error("nothing may be written outside the folder")
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Read a CSV file in Go", "Read a CSV file in Go"},
		{"\"Quoted title.\"", "Quoted title"},
		{"Title: Debug a slow query!", "Debug a slow query"},
		{"\n\n  **Bold title**  \nsecond line", "Bold title"},
		{"one two three four five six seven eight nine ten", "one two three four five six seven eight"},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := CleanTitle(tt.in); got != tt.want {
			t.Errorf("CleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// chatServer answers like llama-server's OpenAI-compatible endpoint.
func chatServer(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func generatorOn(srv *httptest.Server, starts *atomic.Int32) *Generator {
	g := NewGenerator(os.TempDir(), State{})
	g.start = func(context.Context, State) (*Server, error) {
		starts.Add(1)
		return &Server{baseURL: srv.URL, done: make(chan struct{})}, nil
	}
	return g
}

func TestGeneratorTitlesThroughTheServerAndStartsItOnce(t *testing.T) {
	var starts atomic.Int32
	g := generatorOn(chatServer(t, "\"Sum a CSV column in Go.\"", http.StatusOK), &starts)
	defer g.Close()

	for i := 0; i < 3; i++ {
		title, err := g.GenerateTitle(context.Background(), "How do I sum a CSV column in Go?")
		if err != nil || title != "Sum a CSV column in Go" {
			t.Fatalf("title = %q, err = %v", title, err)
		}
	}
	if starts.Load() != 1 {
		t.Errorf("the server must start once and be reused, started %d times", starts.Load())
	}
}

func TestGeneratorReportsAFailingModelSoTheEngineCanFallBack(t *testing.T) {
	var starts atomic.Int32
	g := generatorOn(chatServer(t, "x", http.StatusInternalServerError), &starts)
	defer g.Close()
	if _, err := g.GenerateTitle(context.Background(), "hello"); err == nil {
		t.Fatal("a server error must be reported, not turned into a title")
	}

	empty := generatorOn(chatServer(t, "   ", http.StatusOK), &starts)
	defer empty.Close()
	if _, err := empty.GenerateTitle(context.Background(), "hello"); err == nil {
		t.Fatal("an empty answer must be reported")
	}

	var nilGen *Generator
	if _, err := nilGen.GenerateTitle(context.Background(), "hello"); err == nil {
		t.Fatal("a missing generator must be an error, not a panic")
	}
}

func TestGeneratorRefusesTitlesAfterClose(t *testing.T) {
	var starts atomic.Int32
	g := generatorOn(chatServer(t, "A title", http.StatusOK), &starts)
	g.Close()
	if _, err := g.GenerateTitle(context.Background(), "hello"); err == nil {
		t.Fatal("expected an error after Close")
	}
	g.Close() // closing twice is harmless
}

func TestSharedIsNilUntilInstalledAndEnabled(t *testing.T) {
	root := t.TempDir()
	defer CloseShared()
	if Shared(root) != nil {
		t.Fatal("nothing installed: no generator")
	}
	model := filepath.Join(root, "m.gguf")
	server := filepath.Join(root, serverFileName())
	for _, p := range []string{model, server} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	st := State{Enabled: false, ModelPath: model, ServerPath: server}
	if err := SaveState(root, st); err != nil {
		t.Fatal(err)
	}
	if Shared(root) != nil {
		t.Fatal("turned off: no generator")
	}
	st.Enabled = true
	if err := SaveState(root, st); err != nil {
		t.Fatal(err)
	}
	a, b := Shared(root), Shared(root)
	if a == nil || a != b {
		t.Fatal("expected one shared generator")
	}
	if err := os.Remove(model); err != nil {
		t.Fatal(err)
	}
	CloseShared()
	if Shared(root) != nil {
		t.Fatal("a missing file means not ready: no generator")
	}
}

// TestRealModel runs the real server and model of an installation made by `seshat title-model install`. It is
// skipped unless SESHAT_TITLEMODEL_E2E_ROOT names the runtime root that holds it.
func TestRealModel(t *testing.T) {
	root := os.Getenv("SESHAT_TITLEMODEL_E2E_ROOT")
	if root == "" {
		t.Skip("set SESHAT_TITLEMODEL_E2E_ROOT to a runtime root with an installed title model")
	}
	st := LoadState(root)
	if !st.Ready() {
		t.Fatalf("no usable installation in %s", root)
	}
	g := NewGenerator(root, st)
	defer g.Close()
	messages := []string{
		"How do I read a CSV file in Go and sum one column?",
		"Peux-tu m'expliquer la différence entre un mutex et un canal en Go ?",
		"Fix the failing test in internal/engine, it panics with a nil pointer",
	}
	for i, msg := range messages {
		start := time.Now()
		title, err := g.GenerateTitle(context.Background(), msg)
		if err != nil {
			t.Fatalf("GenerateTitle: %v", err)
		}
		t.Logf("%d: %q in %v (first one includes loading the model)", i, title, time.Since(start))
	}
}
