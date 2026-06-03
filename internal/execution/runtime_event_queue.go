package execution

import (
	"context"
	"sync/atomic"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// DefaultRuntimeEventQueueCapacity is the default buffer size for RuntimeEventQueue.
const DefaultRuntimeEventQueueCapacity = 1000

// RuntimeEventQueue is a non-blocking buffered channel for structured runtime events.
type RuntimeEventQueue struct {
	ch       chan types.RuntimeEvent
	emitted  atomic.Int64
	overflow atomic.Int64
	closed   atomic.Bool
}

// NewRuntimeEventQueue creates a RuntimeEventQueue with the given buffer capacity.
// If capacity <= 0, DefaultRuntimeEventQueueCapacity is used.
func NewRuntimeEventQueue(capacity int) *RuntimeEventQueue {
	if capacity <= 0 {
		capacity = DefaultRuntimeEventQueueCapacity
	}
	return &RuntimeEventQueue{ch: make(chan types.RuntimeEvent, capacity)}
}

// Emit sends an event into the queue. If the queue is full or already closed,
// the event is discarded and the overflow counter is incremented.
func (q *RuntimeEventQueue) Emit(event types.RuntimeEvent) {
	if q.closed.Load() {
		return
	}
	select {
	case q.ch <- event:
		q.emitted.Add(1)
	default:
		q.overflow.Add(1)
	}
}

// Close signals readers that no more events will arrive.
func (q *RuntimeEventQueue) Close() {
	if q.closed.CompareAndSwap(false, true) {
		close(q.ch)
	}
}

// Recv returns the read-only runtime event channel.
func (q *RuntimeEventQueue) Recv() <-chan types.RuntimeEvent {
	return q.ch
}

// EmittedCount returns the total number of events successfully placed in the queue.
func (q *RuntimeEventQueue) EmittedCount() int64 { return q.emitted.Load() }

// OverflowCount returns the number of events dropped because the queue was full.
func (q *RuntimeEventQueue) OverflowCount() int64 { return q.overflow.Load() }

// RuntimeEventQueueStats holds a point-in-time snapshot of RuntimeEventQueue counters.
type RuntimeEventQueueStats struct {
	Emitted  int64
	Overflow int64
	Pending  int
	Closed   bool
}

// Stats returns a point-in-time snapshot of queue counters.
func (q *RuntimeEventQueue) Stats() RuntimeEventQueueStats {
	return RuntimeEventQueueStats{
		Emitted:  q.emitted.Load(),
		Overflow: q.overflow.Load(),
		Pending:  len(q.ch),
		Closed:   q.closed.Load(),
	}
}

// EmitBlocking sends an event into the queue, blocking until the queue has room
// or until ctx is cancelled.
func (q *RuntimeEventQueue) EmitBlocking(ctx context.Context, event types.RuntimeEvent) bool {
	if q.closed.Load() {
		return false
	}
	select {
	case q.ch <- event:
		q.emitted.Add(1)
		return true
	case <-ctx.Done():
		q.overflow.Add(1)
		return false
	}
}

// Drain blocks until the queue's internal buffer is empty or ctx is cancelled.
func (q *RuntimeEventQueue) Drain(ctx context.Context) error {
	for {
		if len(q.ch) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
