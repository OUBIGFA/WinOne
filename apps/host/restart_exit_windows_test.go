//go:build windows

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestartExitWaitRequiresWindowExitAndHelperAcknowledgement(t *testing.T) {
	probes := 0
	err := waitRestartExit(context.Background(), func() (bool, error) { probes++; return probes >= 2, nil }, func() (bool, error) { return false, nil }, func() bool { return probes >= 3 }, time.Millisecond)
	if err != nil || probes != 3 {
		t.Fatal(probes, err)
	}
	for _, ownerGone := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		err = waitRestartExit(ctx, func() (bool, error) { return ownerGone, nil }, func() (bool, error) { return false, nil }, func() bool { return !ownerGone }, time.Millisecond)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(ownerGone, err)
		}
	}
}
func TestRestartExitWaitRejectsHelperDeath(t *testing.T) {
	if err := waitRestartExit(context.Background(), func() (bool, error) { return true, nil }, func() (bool, error) { return true, nil }, func() bool { return true }, time.Millisecond); err == nil {
		t.Fatal("dead helper allowed old host to exit")
	}
}
func TestRestartCommitOnlyExitsAfterAcknowledgedWindowClosure(t *testing.T) {
	var ownerGone atomic.Bool
	var watches, closed atomic.Int32
	exited := make(chan struct{})
	r := &restartPreparation{timeout: time.Second, authorizeHelper: func(uint32) error { return nil }, prepare: func() (func(), error) { return func() {}, nil }, exit: func() { close(exited) }, watchExit: func(owner, helper uint32) (func(func() bool) error, func(), error) {
		watches.Add(1)
		if owner != 10 || helper != 20 {
			t.Fatal(owner, helper)
		}
		return func(ack func() bool) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			return waitRestartExit(ctx, func() (bool, error) { return ownerGone.Load(), nil }, func() (bool, error) { return false, nil }, ack, time.Millisecond)
		}, func() { closed.Add(1) }, nil
	}}
	job, _ := r.beginFor(10)
	waitRestartState(t, r, job.Token, "prepared")
	if _, err := r.bindHelper(20); err != nil {
		t.Fatal(err)
	}
	if _, err := r.commit(job.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := r.commit(job.Token); err != nil {
		t.Fatal(err)
	}
	if watches.Load() != 1 {
		t.Fatal("duplicate commit opened another watcher")
	}
	ownerGone.Store(true)
	select {
	case <-exited:
		t.Fatal("exited without helper acknowledgement")
	default:
	}
	if _, err := r.acknowledgeHelper(20, job.Token); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("old host did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for closed.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("process handles not closed")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestFailedRestartCommitRestoresAdmissionAndReportsFailure(t *testing.T) {
	var restored atomic.Int32
	failed := make(chan error, 1)
	r := &restartPreparation{timeout: time.Second, authorizeHelper: func(uint32) error { return nil }, prepare: func() (func(), error) { return func() { restored.Add(1) }, nil }, exit: func() { t.Error("failed restart exited host") }, failed: func(err error) { failed <- err }, watchExit: func(uint32, uint32) (func(func() bool) error, func(), error) {
		return func(func() bool) error { return errors.New("helper failed") }, func() {}, nil
	}}
	job, _ := r.beginFor(10)
	waitRestartState(t, r, job.Token, "prepared")
	_, _ = r.bindHelper(20)
	if _, err := r.commit(job.Token); err != nil {
		t.Fatal(err)
	}
	select {
	case <-failed:
	case <-time.After(2 * time.Second):
		t.Fatal("failure not reported")
	}
	if restored.Load() != 1 || r.latestError() == "" {
		t.Fatal(restored.Load(), r.latestError())
	}
	waitRestartState(t, r, job.Token, "cancelled")
}
