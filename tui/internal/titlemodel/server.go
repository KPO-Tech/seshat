package titlemodel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// readyTimeout is how long a server may take to load the model: half a gigabyte read from disk.
const readyTimeout = 90 * time.Second

// Server is a running llama-server bound to the loopback address.
type Server struct {
	cmd     *exec.Cmd
	baseURL string
	done    chan struct{}
	once    sync.Once
}

// BaseURL is the address of the server's HTTP API.
func (s *Server) BaseURL() string { return s.baseURL }

// freePort asks the system for a port nobody uses on the loopback address.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// serverArgs are the arguments of llama-server: one slot, a small context (a title is made from at most 500
// characters), the loopback address only, no web interface.
func serverArgs(modelPath string, port int, threads int) []string {
	return []string{
		"-m", modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"-c", "2048",
		"-np", "1",
		"-t", strconv.Itoa(threads),
		"--no-webui",
	}
}

// StartServer launches llama-server on a free loopback port and returns once the model is loaded and the server
// answers. log, when not nil, receives what the server prints.
func StartServer(ctx context.Context, serverPath, modelPath string, log io.Writer) (*Server, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find a free port: %w", err)
	}
	threads := runtime.NumCPU() / 2
	if threads < 1 {
		threads = 1
	}
	if threads > 8 {
		threads = 8
	}
	cmd := exec.Command(serverPath, serverArgs(modelPath, port, threads)...) // #nosec G204 -- the path is the llama-server this package installed and checked
	cmd.Dir = filepath.Dir(serverPath)
	if log != nil {
		cmd.Stdout, cmd.Stderr = log, log
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start llama-server: %w", err)
	}
	s := &Server{cmd: cmd, baseURL: "http://127.0.0.1:" + strconv.Itoa(port), done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()

	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if err := s.waitReady(readyCtx); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

func (s *Server) waitReady(ctx context.Context) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return errors.New("llama-server stopped before it was ready")
		case <-ctx.Done():
			return fmt.Errorf("llama-server was not ready in time: %w", ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/health", nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

// Alive reports whether the process is still running.
func (s *Server) Alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Stop ends the server. It is safe to call more than once.
func (s *Server) Stop() {
	s.once.Do(func() {
		if s.cmd == nil || s.cmd.Process == nil {
			return
		}
		_ = s.cmd.Process.Kill()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	})
}

// LogFile opens the file the server output goes to, next to the installation.
func LogFile(root string) (*os.File, error) {
	dir := filepath.Join(root, "title-model")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}
