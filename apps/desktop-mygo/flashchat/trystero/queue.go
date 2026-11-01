package trystero

import "sync"

// queue runs functions one at a time, in the order they were posted, on a
// goroutine of its own. A room keeps its state on one queue (as Trystero
// keeps it on the browser's event loop) and calls the app on another, so
// that an app's handler may call the room back without deadlock. Posting
// never blocks: network callbacks must not wait for the app.
type queue struct {
	mu      sync.Mutex
	items   []func()
	wake    chan struct{}
	stopped bool
	done    chan struct{}
}

func newQueue() *queue {
	q := &queue{wake: make(chan struct{}, 1), done: make(chan struct{})}
	go q.run()
	return q
}

// post queues fn and reports whether it will run: nothing runs once the
// queue stopped.
func (q *queue) post(fn func()) bool {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return false
	}
	q.items = append(q.items, fn)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return true
}

// stop runs what was posted before it, then ends the goroutine.
func (q *queue) stop() {
	q.post(func() {
		q.mu.Lock()
		q.stopped = true
		q.mu.Unlock()
	})
}

func (q *queue) run() {
	defer close(q.done)
	for {
		q.mu.Lock()
		items := q.items
		q.items = nil
		q.mu.Unlock()
		for _, fn := range items {
			fn()
		}
		q.mu.Lock()
		stopped, empty := q.stopped, len(q.items) == 0
		q.mu.Unlock()
		if stopped && empty {
			return
		}
		if empty {
			<-q.wake
		}
	}
}

// call runs fn on the queue and waits for it; it must not be called from
// the queue itself.
func call[T any](q *queue, fn func() T) T {
	result := make(chan T, 1)
	if !q.post(func() { result <- fn() }) {
		var zero T
		return zero
	}
	return <-result
}
