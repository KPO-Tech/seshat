package execution

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// DefaultEventQueueCapacity is the default buffer size for EventQueue.
const DefaultEventQueueCapacity = 1000

// EventQueue is a non-blocking buffered channel for streaming APIResponseChunks.
//
// Emitters call Emit; readers consume Recv(). When the queue is full, Emit
// drops the chunk and increments OverflowCount so callers can detect backpressure.
// Close must be called once to signal readers that no more chunks are coming.
type EventQueue struct {
	ch       chan types.APIResponseChunk
	emitted  atomic.Int64
	overflow atomic.Int64
	closed   atomic.Bool
	// mu serialises Emit and Close to prevent a send-on-closed-channel panic.
	// Emit is non-blocking (default: branch) so holding mu there never deadlocks.
	mu sync.Mutex
}

// NewEventQueue creates an EventQueue with the given buffer capacity.
// If capacity <= 0, DefaultEventQueueCapacity is used.
func NewEventQueue(capacity int) *EventQueue {
	if capacity <= 0 {
		capacity = DefaultEventQueueCapacity
	}
	return &EventQueue{ch: make(chan types.APIResponseChunk, capacity)}
}

// Emit sends chunk into the queue. If the queue is full or already closed,
// the chunk is discarded and the overflow counter is incremented.
func (q *EventQueue) Emit(chunk types.APIResponseChunk) {
	q.mu.Lock()
	if q.closed.Load() {
		q.mu.Unlock()
		return
	}
	// The select is non-blocking (default branch), so holding mu here is safe.
	select {
	case q.ch <- chunk:
		q.mu.Unlock()
		q.emitted.Add(1)
	default:
		q.mu.Unlock()
		q.overflow.Add(1)
	}
}

// Close signals readers that no more chunks will arrive by closing the channel.
// It is safe to call Close multiple times; only the first call has effect.
func (q *EventQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed.CompareAndSwap(false, true) {
		close(q.ch)
	}
}

// Recv returns the read-only channel. Readers range over it or select from it.
// The channel is closed when Close is called.
func (q *EventQueue) Recv() <-chan types.APIResponseChunk {
	return q.ch
}

// EmittedCount returns the total number of chunks successfully placed in the queue.
func (q *EventQueue) EmittedCount() int64 { return q.emitted.Load() }

// OverflowCount returns the number of chunks dropped because the queue was full.
func (q *EventQueue) OverflowCount() int64 { return q.overflow.Load() }

// EventQueueStats holds a point-in-time snapshot of EventQueue counters.
type EventQueueStats struct {
	Emitted  int64
	Overflow int64
	Pending  int // chunks currently buffered (not yet consumed)
	Closed   bool
}

// Stats returns a point-in-time snapshot of queue counters.
// Pending is the number of chunks buffered but not yet read by the consumer.
func (q *EventQueue) Stats() EventQueueStats {
	return EventQueueStats{
		Emitted:  q.emitted.Load(),
		Overflow: q.overflow.Load(),
		Pending:  len(q.ch),
		Closed:   q.closed.Load(),
	}
}

// EmitBlocking sends chunk into the queue, blocking until the queue has room
// or until ctx is cancelled. Returns true if the chunk was enqueued, false if
// ctx was cancelled before the chunk could be sent (chunk is dropped in that case).
//
// Use EmitBlocking instead of Emit when losing chunks is not acceptable, for
// example when a caller requires complete transcript delivery. Be aware that
// blocking here stalls the engine loop — the consumer must drain promptly.
func (q *EventQueue) EmitBlocking(ctx context.Context, chunk types.APIResponseChunk) (ok bool) {
	if q.closed.Load() {
		return false
	}
	// recover handles the rare race where Close() fires between the closed check
	// above and the blocking send below.
	defer func() {
		if r := recover(); r != nil {
			q.overflow.Add(1)
			ok = false
		}
	}()
	select {
	case q.ch <- chunk:
		q.emitted.Add(1)
		return true
	case <-ctx.Done():
		q.overflow.Add(1)
		return false
	}
}

// Drain blocks until the queue's internal buffer is empty or ctx is cancelled.
// It does NOT consume chunks — the caller's reader goroutine must still drain
// Recv(). Drain is useful in tests and shutdown sequences where the caller wants
// to wait until the consumer has caught up before asserting on results.
//
// Returns ctx.Err() if the context expires before the buffer empties, nil otherwise.
func (q *EventQueue) Drain(ctx context.Context) error {
	for {
		if len(q.ch) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
		}
	}
}
