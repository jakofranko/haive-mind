// Package inbox provides an unbounded FIFO queue for handing hub messages to
// a TUI model. Push never blocks and never drops; Pop blocks until an item
// is available. It replaces fixed-size channels whose senders had to choose
// between blocking the hub and silently dropping messages.
package inbox

import "sync"

type Queue[T any] struct {
	mu    sync.Mutex
	items []T
	ready chan struct{}
}

func New[T any]() *Queue[T] {
	return &Queue[T]{ready: make(chan struct{}, 1)}
}

// Push appends v. It is safe for concurrent use.
func (q *Queue[T]) Push(v T) {
	q.mu.Lock()
	q.items = append(q.items, v)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// Pop removes and returns the oldest item, blocking until one is available.
func (q *Queue[T]) Pop() T {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			v := q.items[0]
			var zero T
			q.items[0] = zero
			q.items = q.items[1:]
			q.mu.Unlock()
			return v
		}
		q.mu.Unlock()
		<-q.ready
	}
}
