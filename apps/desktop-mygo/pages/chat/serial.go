package chat

import "winone/desktopmygo/core"

// serial runs asynchronous jobs one after another on the UI thread, as
// the React page chained its draft and queue writes on one promise: a
// job starts once the one before it called done. Waiters run once every
// job queued before them finished.
type serial struct {
	running bool
	queue   []func(done func())
}

// run queues job; it starts work and calls done on the UI thread when it
// finished, whatever the outcome.
func (s *serial) run(job func(done func())) {
	s.queue = append(s.queue, job)
	if !s.running {
		s.next()
	}
}

// after runs fn once every job queued so far finished: the barrier of
// "await draftWrites.current".
func (s *serial) after(fn func()) {
	s.run(func(done func()) { fn(); done() })
}

func (s *serial) next() {
	if len(s.queue) == 0 {
		s.running = false
		return
	}
	job := s.queue[0]
	s.queue = s.queue[1:]
	s.running = true
	finished := false
	job(func() {
		if finished {
			return
		}
		finished = true
		s.next()
	})
}

// idle reports whether no job runs or waits.
func (s *serial) idle() bool { return !s.running && len(s.queue) == 0 }

// call sends a Host request off the UI thread and gives its decoded
// result to done on the UI thread.
func call[T any](env *core.Env, method string, params any, done func(T, error)) {
	core.Go(env, func() (T, error) {
		var out T
		err := env.Host.CallInto(method, params, &out)
		return out, err
	}, done)
}
