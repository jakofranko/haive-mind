// Package inbox provides an unbounded FIFO queue for handing hub messages to
// a TUI model. Push never blocks and never drops; Next blocks until an item
// is available. It replaces fixed-size channels whose senders had to choose
// between blocking the hub and silently dropping messages.
package inbox

import "sync"

type Queue[T any] struct {
	mu     sync.Mutex
	items  []T
	ready  chan struct{}
	done   chan struct{}
	closed bool
}

func New[T any]() *Queue[T] {
	return &Queue[T]{ready: make(chan struct{}, 1), done: make(chan struct{})}
}

// Push appends v. It is safe for concurrent use and never blocks. After Close
// the item is still queued, so it can be drained by Next.
func (q *Queue[T]) Push(v T) {
	q.mu.Lock()
	q.items = append(q.items, v)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// Close marks the queue closed and releases every waiting Next. Items already
// queued are still delivered, in order, before Next reports closed. Closing
// more than once is harmless.
func (q *Queue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.done)
}

// Next removes and returns the oldest item with ok=true, blocking until one
// is available. Once the queue is closed and drained it returns the zero value
// and ok=false, immediately and on every later call.
func (q *Queue[T]) Next() (v T, ok bool) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			v = q.items[0]
			var zero T
			q.items[0] = zero
			q.items = q.items[1:]
			q.mu.Unlock()
			return v, true
		}
		closed := q.closed
		q.mu.Unlock()
		if closed {
			return v, false
		}
		select {
		case <-q.ready:
		case <-q.done:
		}
	}
}
