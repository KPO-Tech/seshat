package providers

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// defaultStreamIdleTimeout is how long a response body may stay silent before the
// request is given up on. A provider streams a token, a keep-alive comment or a
// ping every few seconds; Z.ai pauses up to about twenty seconds between bursts.
// Three minutes is far past any of these and still short of a spinner that runs
// for ever on a connection that stopped without closing.
const defaultStreamIdleTimeout = 3 * time.Minute

// streamIdleTimeoutEnv overrides the idle timeout, in seconds. 0 turns it off.
const streamIdleTimeoutEnv = "SESHAT_STREAM_IDLE_TIMEOUT"

// streamIdleTimeout reads the idle timeout from the environment.
func streamIdleTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv(streamIdleTimeoutEnv))
	if raw == "" {
		return defaultStreamIdleTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return defaultStreamIdleTimeout
	}
	return time.Duration(seconds) * time.Second
}

// streamIdleError is what a read returns when the body stayed silent for too long.
// It reports itself as a timeout, so that the error classifier treats it as a
// transient network failure that is worth retrying.
type streamIdleError struct{ idle time.Duration }

func (e streamIdleError) Error() string {
	return fmt.Sprintf("no data received from the provider for %s", e.idle)
}

func (e streamIdleError) Timeout() bool   { return true }
func (e streamIdleError) Temporary() bool { return true }

// idleBody is a response body that closes itself when no byte arrives for idle.
// A read that is blocked on a connection that stopped without closing never
// returns by itself: the request had no deadline on the body, only the context,
// so the turn waited until the user pressed Esc.
type idleBody struct {
	rc    io.ReadCloser
	idle  time.Duration
	timer *time.Timer
	fired atomic.Bool
}

// newIdleBody wraps rc. An idle of zero or less returns rc unchanged.
func newIdleBody(rc io.ReadCloser, idle time.Duration) io.ReadCloser {
	if idle <= 0 {
		return rc
	}
	b := &idleBody{rc: rc, idle: idle}
	b.timer = time.AfterFunc(idle, func() {
		b.fired.Store(true)
		_ = rc.Close()
	})
	return b
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.timer.Reset(b.idle)
	}
	if err != nil && b.fired.Load() {
		return n, streamIdleError{idle: b.idle}
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	return b.rc.Close()
}
