// Package updates owns checked release results and cancellation across desktop frontends.
package updates

import (
	"context"
	"errors"
	"sync"
)

type Reply struct {
	Result
	ErrorCode string `json:"errorCode,omitempty"`
}

type Session struct {
	mu     sync.Mutex
	id     string
	cancel context.CancelFunc
	url    string
}

func (s *Session) Check(ctx context.Context, id string, run func(context.Context) (Result, error)) Reply {
	if id == "" || len(id) > 128 {
		return Reply{Result: Result{State: "failed"}, ErrorCode: "invalid_request"}
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return Reply{Result: Result{State: "failed"}, ErrorCode: "busy"}
	}
	ctx, cancel := context.WithCancel(ctx)
	s.id = id
	s.cancel = cancel
	s.url = ""
	s.mu.Unlock()
	defer cancel()
	result, err := run(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cancellation wins even when a response arrives at the same time.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	reply := Reply{Result: result}
	if err != nil {
		reply.Result = Result{State: "failed"}
		reply.ErrorCode = "unavailable"
		var failure *CheckError
		if errors.As(err, &failure) {
			reply.ErrorCode = failure.Code
		}
		if errors.Is(err, context.Canceled) {
			reply.ErrorCode = "cancelled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			reply.ErrorCode = "timeout"
		}
	}
	if err == nil && result.State == "available" {
		s.url = result.ReleaseURL
	}
	s.id = ""
	s.cancel = nil
	return reply
}
func (s *Session) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil && s.id == id {
		s.cancel()
	}
}
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *Session) ReleaseURL() string { s.mu.Lock(); defer s.mu.Unlock(); return s.url }
