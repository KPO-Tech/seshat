package titlemodel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperServer is not a test: when HELPER_TITLE_SERVER_PORT is set, the test binary runs as a stand-in for a
// llama-server that has loaded HELPER_TITLE_MODEL (it answers /health and /props), until it is killed.
func TestHelperServer(t *testing.T) {
	port := os.Getenv("HELPER_TITLE_SERVER_PORT")
	if port == "" {
		t.Skip("helper process only")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	mux.HandleFunc("/props", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"model_path":%q}`, os.Getenv("HELPER_TITLE_MODEL"))
	})
	_ = http.ListenAndServe("127.0.0.1:"+port, mux)
}

// httpGet is a GET with a deadline.
func httpGet(url string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// startHelper starts the stand-in server and waits until it answers.
func startHelper(t *testing.T, model string) (*exec.Cmd, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperServer$")
	cmd.Env = append(os.Environ(), "HELPER_TITLE_SERVER_PORT="+strconv.Itoa(port), "HELPER_TITLE_MODEL="+model)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := httpGet("http://127.0.0.1:" + strconv.Itoa(port) + "/health"); err == nil {
			_ = resp.Body.Close()
			return cmd, port
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the helper server did not start")
	return nil, 0
}

func processGone(cmd *exec.Cmd) bool {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func TestCleanupOrphanStopsAServerThatHoldsOurModel(t *testing.T) {
	root := t.TempDir()
	model := "C:/models/title.gguf"
	cmd, port := startHelper(t, model)
	writeRecord(root, serverRecord{PID: cmd.Process.Pid, Port: port, ModelPath: model})

	if !CleanupOrphan(root) {
		t.Fatal("expected the orphan to be stopped")
	}
	if !processGone(cmd) {
		t.Fatal("the orphan process is still running")
	}
	if _, ok := readRecord(root); ok {
		t.Error("the record must be removed")
	}
}

func TestCleanupOrphanLeavesAnotherProgramAlone(t *testing.T) {
	root := t.TempDir()
	// The pid and port are alive, but what answers is not our model: pids are reused, so it must not be killed.
	cmd, port := startHelper(t, "/somewhere/else.gguf")
	writeRecord(root, serverRecord{PID: cmd.Process.Pid, Port: port, ModelPath: "/models/title.gguf"})

	if CleanupOrphan(root) {
		t.Fatal("a server that holds another model must not be stopped")
	}
	if _, ok := readRecord(root); ok {
		t.Error("the stale record must still be removed")
	}
	if resp, err := httpGet("http://127.0.0.1:" + strconv.Itoa(port) + "/health"); err != nil {
		t.Errorf("the other program must still run: %v", err)
	} else {
		_ = resp.Body.Close()
	}
}

func TestCleanupOrphanWithNothingToDo(t *testing.T) {
	root := t.TempDir()
	if CleanupOrphan(root) {
		t.Fatal("no record, nothing to stop")
	}
	// A record whose server is gone.
	writeRecord(root, serverRecord{PID: 999999, Port: 1, ModelPath: "/m.gguf"})
	if CleanupOrphan(root) {
		t.Fatal("a dead server is not stopped")
	}
	if _, ok := readRecord(root); ok {
		t.Error("the record of a dead server must be removed")
	}
	// A damaged record.
	if err := os.WriteFile(recordPath(root), []byte("not json"), 0o600); err == nil {
		if CleanupOrphan(root) {
			t.Fatal("a damaged record stops nothing")
		}
	}
}

func TestGeneratorWritesAndRemovesTheRecord(t *testing.T) {
	root := t.TempDir()
	model := "/models/title.gguf"
	cmd, port := startHelper(t, model)
	// The generator is given a server that wraps the helper process, as StartServer would return.
	g := NewGenerator(root, State{ModelPath: model})
	g.start = func(context.Context, State) (*Server, error) {
		s := &Server{cmd: cmd, baseURL: "http://127.0.0.1:" + strconv.Itoa(port), port: port, done: make(chan struct{})}
		go func() { _ = cmd.Wait(); close(s.done) }()
		writeRecord(root, serverRecord{PID: s.PID(), Port: port, ModelPath: model})
		return s, nil
	}
	if _, err := g.ensureServer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec, ok := readRecord(root); !ok || rec.PID != cmd.Process.Pid {
		t.Fatalf("expected a record for the running server, got %+v %v", rec, ok)
	}
	g.Close()
	if _, ok := readRecord(root); ok {
		t.Error("closing the generator must remove the record")
	}
}

func TestSamePath(t *testing.T) {
	if !samePath(`C:\a\b.gguf`, "C:/a/b.gguf") && strings.Contains(os.Getenv("OS"), "Windows") {
		t.Error("backslashes and slashes are the same path on Windows")
	}
	if samePath("/a/b.gguf", "/a/c.gguf") {
		t.Error("different files")
	}
}
