package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestLimitsShareMinuteAcrossCompletedWorkers(t *testing.T) {
	s := requestScheduler{window: 80 * time.Millisecond}
	l := RequestLimits{MaxRPM: 1}
	release, err := s.take(context.Background(), "same-key", l)
	if err != nil {
		t.Fatal(err)
	}
	release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if release, err := s.take(ctx, "same-key", l); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("worker reset RPM history: %v", err)
	}
	release, err = s.take(context.Background(), "other-key", l)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release, err = s.take(context.Background(), "same-key", l)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRequestLimitsQueueBoundAndCancellation(t *testing.T) {
	s := requestScheduler{}
	l := RequestLimits{MaxConcurrency: 1, QueueLimit: 1}
	release, err := s.take(context.Background(), "key", l)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		next, err := s.take(ctx, "key", l)
		if next != nil {
			next()
		}
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		queued := len(s.lanes["key"].queue)
		s.mu.Unlock()
		if queued == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter never queued")
		}
		time.Sleep(time.Millisecond)
	}
	if next, err := s.take(context.Background(), "key", l); err == nil {
		next()
		t.Fatal("queue admitted beyond limit")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
	next, err := s.take(context.Background(), "key", l)
	if err != nil {
		t.Fatal("cancellation leaked queue slot", err)
	}
	next()
}

func TestRequestLimitsHonorQueueWaitAndReleasePosition(t *testing.T) {
	s := requestScheduler{}
	limits := RequestLimits{MaxConcurrency: 1, QueueLimit: 1, QueueWait: 1}
	release, err := s.take(context.Background(), "key", limits)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if next, err := s.take(context.Background(), "key", limits); !errors.Is(err, context.DeadlineExceeded) {
		if next != nil {
			next()
		}
		t.Fatalf("queue wait did not expire: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("queue wait was %s, want one second", elapsed)
	}
	release()
	next, err := s.take(context.Background(), "key", limits)
	if err != nil {
		t.Fatal("timed-out waiter leaked its position", err)
	}
	next()
}
