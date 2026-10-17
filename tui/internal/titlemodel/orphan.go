package titlemodel

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// serverRecord is written when a server starts and removed when it stops. If seshat is killed or crashes, the
// record outlives it, and the next run finds the server that was left behind (about 600 MB of memory) and stops it.
type serverRecord struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	ModelPath string `json:"model_path"`
}

func recordPath(root string) string {
	return filepath.Join(root, "title-model", "server.json")
}

func writeRecord(root string, rec serverRecord) {
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(recordPath(root)), 0o700)
	_ = os.WriteFile(recordPath(root), data, 0o600)
}

func removeRecord(root string, pid int) {
	if rec, ok := readRecord(root); ok && rec.PID == pid {
		_ = os.Remove(recordPath(root))
	}
}

func readRecord(root string) (serverRecord, bool) {
	data, err := os.ReadFile(recordPath(root))
	if err != nil {
		return serverRecord{}, false
	}
	var rec serverRecord
	if json.Unmarshal(data, &rec) != nil || rec.PID <= 0 || rec.Port <= 0 {
		return serverRecord{}, false
	}
	return rec, true
}

// samePath compares two model paths as llama-server reports them (forward slashes on every system) and as this
// program built them.
func samePath(a, b string) bool {
	a, b = filepath.ToSlash(a), filepath.ToSlash(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// CleanupOrphan stops a title server that an earlier run of seshat left behind, and reports whether it did.
//
// A process id alone is not proof of anything (the system reuses them), so the process is stopped only if the port
// of the record still answers as a llama-server that has loaded this very model file.
func CleanupOrphan(root string) bool {
	rec, ok := readRecord(root)
	if !ok {
		return false
	}
	defer func() { _ = os.Remove(recordPath(root)) }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(rec.Port)+"/props", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false // nothing listens there any more: the server is gone
	}
	defer resp.Body.Close()
	var props struct {
		ModelPath string `json:"model_path"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&props) != nil {
		return false
	}
	if rec.ModelPath == "" || !samePath(props.ModelPath, rec.ModelPath) {
		return false
	}
	process, err := os.FindProcess(rec.PID)
	if err != nil {
		return false
	}
	return process.Kill() == nil
}
