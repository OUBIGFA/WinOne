//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/host"
)

func waitRestartState(t *testing.T, r *restartPreparation, token, state string) restartPreparationJob {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		value, err := r.status(token)
		if err != nil {
			t.Fatal(err)
		}
		if value.State == state {
			return value
		}
		select {
		case <-deadline:
			t.Fatalf("wanted %s, got %+v", state, value)
		case <-time.After(time.Millisecond * 5):
		}
	}
}

func TestRestartRPCDistinguishesDesktopOwnerFromHostHelper(t *testing.T) {
	r := &restartPreparation{timeout: time.Second,
		authorize: func(pid uint32) error {
			if pid != 10 {
				return errors.New("not the desktop executable")
			}
			return nil
		},
		authorizeHelper: func(pid uint32) error {
			if pid != 20 {
				return errors.New("not the verified elevated Host helper")
			}
			return nil
		},
		prepare: func() (func(), error) { return func() {}, nil },
	}
	response := r.handle(host.Request{PeerPID: 10, ID: "prepare", Method: "restart.prepare"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var job restartPreparationJob
	if err := json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	waitRestartState(t, r, job.Token, "prepared")
	params, _ := json.Marshal(map[string]string{"token": job.Token})
	for _, method := range []string{"restart.bootstrap", "restart.status"} {
		response = r.handle(host.Request{PeerPID: 20, ID: "helper", Method: method, Params: params})
		if response.Error != nil {
			t.Fatalf("verified Host helper rejected by %s: %v", method, response.Error)
		}
	}
	for _, method := range []string{"restart.prepare", "restart.commit", "restart.cancel"} {
		if response = r.handle(host.Request{PeerPID: 20, ID: "wrong-role", Method: method, Params: params}); response.Error == nil {
			t.Fatal("helper gained owner operation", method)
		}
	}
	undo, err := r.take(job.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer undo()
	response = r.handle(host.Request{PeerPID: 20, ID: "ack", Method: "restart.acknowledge", Params: params})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if err = json.Unmarshal(response.Result, &job); err != nil || !job.HelperAcknowledged {
		t.Fatal(job, err)
	}
	r.authorizeHelper = func(uint32) error { return errors.New("helper no longer valid") }
	if response = r.handle(host.Request{PeerPID: 20, ID: "invalidated", Method: "restart.status", Params: params}); response.Error == nil {
		t.Fatal("helper identity was not rechecked")
	}
}

func TestRestartRPCRequiresVerifiedCallerAndBindsItsTokenToThatCaller(t *testing.T) {
	var preparations atomic.Int32
	r := &restartPreparation{timeout: time.Second, authorize: func(pid uint32) error {
		if pid == 9 {
			return errors.New("wrong image")
		}
		return nil
	}, prepare: func() (func(), error) { preparations.Add(1); return func() {}, nil }}
	for _, pid := range []uint32{0, 9} {
		if response := r.handle(host.Request{PeerPID: pid, ID: "bad", Method: "restart.prepare"}); response.Error == nil {
			t.Fatal("unverified request accepted")
		}
	}
	if preparations.Load() != 0 {
		t.Fatal("unverified request touched modules")
	}
	response := r.handle(host.Request{PeerPID: 10, ID: "owner", Method: "restart.prepare"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var job restartPreparationJob
	if err := json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]string{"token": job.Token})
	for _, method := range []string{"restart.status", "restart.cancel"} {
		if response := r.handle(host.Request{PeerPID: 11, ID: "other", Method: method, Params: params}); response.Error == nil {
			t.Fatal("different caller used the token", method)
		}
	}
	if response := r.handle(host.Request{PeerPID: 10, ID: "cancel", Method: "restart.cancel", Params: params}); response.Error != nil {
		t.Fatal(response.Error)
	}
	waitRestartState(t, r, job.Token, "cancelled")
}
func TestRestartCancelDuringPreparationRestoresWhenPreparationFinishes(t *testing.T) {
	ready := make(chan struct{})
	var resumed atomic.Int32
	r := &restartPreparation{timeout: time.Second, prepare: func() (func(), error) { <-ready; return func() { resumed.Add(1) }, nil }}
	job, err := r.begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.begin(); err == nil {
		t.Fatal("duplicate preparation accepted")
	}
	if _, err = r.cancel(job.Token, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = r.begin(); err == nil {
		t.Fatal("preparation restarted before cancellation finished")
	}
	close(ready)
	waitRestartState(t, r, job.Token, "cancelled")
	if resumed.Load() != 1 {
		t.Fatal(resumed.Load())
	}
}
func TestRestartTimeoutReleasesPreparationAndOldTokenCannotCancelNewRequest(t *testing.T) {
	var resumed atomic.Int32
	r := &restartPreparation{timeout: 20 * time.Millisecond, prepare: func() (func(), error) { return func() { resumed.Add(1) }, nil }}
	old, err := r.begin()
	if err != nil {
		t.Fatal(err)
	}
	cancelled := waitRestartState(t, r, old.Token, "cancelled")
	if cancelled.Error == "" || resumed.Load() != 1 {
		t.Fatal(cancelled, resumed.Load())
	}
	next, err := r.begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.cancel(old.Token, ""); err == nil {
		t.Fatal("stale token cancelled a different restart")
	}
	if _, err = r.cancel(next.Token, ""); err != nil {
		t.Fatal(err)
	}
	waitRestartState(t, r, next.Token, "cancelled")
}
func TestRestartOwnershipRequiresReadyStateAndCanRollbackFailedHandoff(t *testing.T) {
	ready := make(chan struct{})
	var resumed atomic.Int32
	r := &restartPreparation{timeout: time.Second, prepare: func() (func(), error) { <-ready; return func() { resumed.Add(1) }, nil }}
	job, _ := r.begin()
	if _, err := r.take(job.Token); err == nil {
		t.Fatal("took an unfinished preparation")
	}
	close(ready)
	waitRestartState(t, r, job.Token, "prepared")
	if _, err := r.take(job.Token); err == nil {
		t.Fatal("handoff accepted without a helper")
	}
	r.authorizeHelper = func(uint32) error { return nil }
	if _, err := r.bindHelper(20); err != nil {
		t.Fatal(err)
	}
	undo, err := r.take(job.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.cancel(job.Token, ""); err == nil {
		t.Fatal("client revoked committed ownership")
	}
	undo()
	undo()
	if resumed.Load() != 1 {
		t.Fatal(resumed.Load())
	}
	waitRestartState(t, r, job.Token, "cancelled")
}

func TestRestartHelperAcknowledgementIsBoundToVerifiedProcessAndSession(t *testing.T) {
	r := &restartPreparation{timeout: time.Second, authorize: func(uint32) error { return nil }, authorizeHelper: func(pid uint32) error {
		if pid != 20 {
			return errors.New("not the elevated helper")
		}
		return nil
	}, prepare: func() (func(), error) { return func() {}, nil }}
	job, err := r.beginFor(10)
	if err != nil {
		t.Fatal(err)
	}
	waitRestartState(t, r, job.Token, "prepared")
	if _, err = r.bindHelper(21); err == nil {
		t.Fatal("unverified helper accepted")
	}
	bound, err := r.bindHelper(20)
	if err != nil || bound.HelperPID != 20 {
		t.Fatal(bound, err)
	}
	if _, err = r.acknowledgeHelper(20, job.Token); err == nil {
		t.Fatal("uncommitted restart acknowledged")
	}
	undo, err := r.take(job.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer undo()
	if retried, err := r.bindHelper(20); err != nil || retried.Token != job.Token {
		t.Fatal("lost bind reply could not be retried", err)
	}
	if _, err = r.acknowledgeHelper(21, job.Token); err == nil {
		t.Fatal("another process acknowledged")
	}
	if _, err = r.acknowledgeHelper(20, "old"); err == nil {
		t.Fatal("wrong session acknowledged")
	}
	ack, err := r.acknowledgeHelper(20, job.Token)
	if err != nil || !ack.HelperAcknowledged {
		t.Fatal(ack, err)
	}
	params, _ := json.Marshal(map[string]string{"token": job.Token})
	if response := r.handle(host.Request{PeerPID: 20, ID: "read", Method: "restart.status", Params: params}); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := r.handle(host.Request{PeerPID: 20, ID: "cancel", Method: "restart.cancel", Params: params}); response.Error == nil {
		t.Fatal("helper cancelled the owner's request")
	}
}
func TestFailedRestartPreparationDoesNotPretendReady(t *testing.T) {
	r := &restartPreparation{timeout: time.Second, prepare: func() (func(), error) { return nil, errors.New("截图尚未结束") }}
	job, _ := r.begin()
	value := waitRestartState(t, r, job.Token, "failed")
	if value.Error != "截图尚未结束" {
		t.Fatal(value)
	}
	if _, err := r.take(job.Token); err == nil {
		t.Fatal("failed preparation was committed")
	}
}
