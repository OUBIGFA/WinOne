//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/updates"
)

type updateTestOwner struct {
	gone   chan struct{}
	closed atomic.Int32
}

func (o *updateTestOwner) Wait(ctx context.Context) error {
	select {
	case <-o.gone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (o *updateTestOwner) Close() { o.closed.Add(1) }
func updateRequest(t *testing.T, c *updateControl, pid uint32, method, id string) host.Response {
	t.Helper()
	params, _ := json.Marshal(map[string]string{"id": id})
	return c.handle(host.Request{PeerPID: pid, ID: "test", Method: method, Params: params})
}
func updateSnapshot(t *testing.T, response host.Response) updateControlSnapshot {
	t.Helper()
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var result updateControlSnapshot
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func waitUpdate(t *testing.T, c *updateControl, id string) updateControlSnapshot {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		result := updateSnapshot(t, updateRequest(t, c, 10, "status", id))
		if result.State != "checking" {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatal("update did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestUpdatesControlCoalescesLostRepliesAndBindsResultsToTheOwner(t *testing.T) {
	owner := &updateTestOwner{gone: make(chan struct{})}
	ready := make(chan struct{})
	var checks atomic.Int32
	control := newUpdateControl(context.Background(), updateBuildInfo{Version: "1.0.0", Configured: true}, func(ctx context.Context) (updates.Result, error) {
		checks.Add(1)
		select {
		case <-ready:
		case <-ctx.Done():
			return updates.Result{}, ctx.Err()
		}
		return updates.Result{State: "available", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", ReleaseURL: "https://github.com/example/app/releases/tag/v1.1.0"}, nil
	}, func(pid uint32) (updateOwner, error) {
		if pid != 10 {
			return nil, errors.New("wrong owner")
		}
		return owner, nil
	})
	defer control.close()
	updateSnapshot(t, updateRequest(t, control, 10, "check", "one"))
	updateSnapshot(t, updateRequest(t, control, 10, "check", "one"))
	if response := updateRequest(t, control, 10, "check", "two"); response.Error == nil {
		t.Fatal("concurrent check accepted")
	}
	if response := updateRequest(t, control, 11, "cancel", "one"); response.Error == nil {
		t.Fatal("another window cancelled a check")
	}
	if response := updateRequest(t, control, 10, "release", "one"); response.Error == nil {
		t.Fatal("unchecked release exposed")
	}
	close(ready)
	result := waitUpdate(t, control, "one")
	if result.State != "available" || checks.Load() != 1 || owner.closed.Load() != 1 {
		t.Fatal(result, checks.Load(), owner.closed.Load())
	}
	if response := updateRequest(t, control, 10, "release", "one"); response.Error != nil {
		t.Fatal(response.Error)
	}
	updateSnapshot(t, updateRequest(t, control, 10, "cancel", "one"))
	if response := updateRequest(t, control, 10, "release", "one"); response.Error == nil {
		t.Fatal("cancelled check kept its release")
	}
}

func TestUpdatesControlOwnerExitAndCancellationClearTheResult(t *testing.T) {
	for _, ownerExit := range []bool{false, true} {
		owner := &updateTestOwner{gone: make(chan struct{})}
		started := make(chan struct{})
		control := newUpdateControl(context.Background(), updateBuildInfo{Version: "1.0.0", Configured: true}, func(ctx context.Context) (updates.Result, error) {
			close(started)
			<-ctx.Done()
			return updates.Result{}, ctx.Err()
		}, func(uint32) (updateOwner, error) { return owner, nil })
		updateSnapshot(t, updateRequest(t, control, 10, "check", "one"))
		<-started
		if ownerExit {
			close(owner.gone)
		} else {
			updateSnapshot(t, updateRequest(t, control, 10, "cancel", "one"))
		}
		result := waitUpdate(t, control, "one")
		if result.State != "failed" || result.ErrorCode != "cancelled" || owner.closed.Load() != 1 {
			t.Fatal(result, owner.closed.Load())
		}
		control.close()
	}
}

func TestUpdatesControlRejectsCallerSuppliedSource(t *testing.T) {
	control := newUpdateControl(context.Background(), updateBuildInfo{Version: "1.0.0"}, func(context.Context) (updates.Result, error) {
		t.Fatal("unexpected network check")
		return updates.Result{}, nil
	}, func(uint32) (updateOwner, error) { t.Fatal("unexpected owner lookup"); return nil, nil })
	defer control.close()
	response := control.handle(host.Request{PeerPID: 10, ID: "bad", Method: "check", Params: json.RawMessage(`{"id":"one","repository":"other/publisher"}`)})
	if response.Error == nil {
		t.Fatal("caller source accepted")
	}
}

func TestUpdatesControlConcurrentDuplicateCannotStartAgainAfterCompletion(t *testing.T) {
	firstObserved, secondObserved := make(chan struct{}), make(chan struct{})
	releaseFirst, releaseSecond := make(chan struct{}), make(chan struct{})
	var observations, checks atomic.Int32
	control := newUpdateControl(context.Background(), updateBuildInfo{Version: "1.0.0"}, func(context.Context) (updates.Result, error) {
		checks.Add(1)
		return updates.Result{State: "not_configured", CurrentVersion: "1.0.0"}, nil
	}, func(uint32) (updateOwner, error) {
		if observations.Add(1) == 1 {
			close(firstObserved)
			<-releaseFirst
		} else {
			close(secondObserved)
			<-releaseSecond
		}
		return &updateTestOwner{gone: make(chan struct{})}, nil
	})
	defer control.close()
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
		select {
		case <-releaseSecond:
		default:
			close(releaseSecond)
		}
	}()
	first, second := make(chan host.Response, 1), make(chan host.Response, 1)
	go func() { first <- updateRequest(t, control, 10, "check", "same") }()
	<-firstObserved
	go func() { second <- updateRequest(t, control, 10, "check", "same") }()
	<-secondObserved
	close(releaseFirst)
	updateSnapshot(t, <-first)
	waitUpdate(t, control, "same")
	close(releaseSecond)
	updateSnapshot(t, <-second)
	waitUpdate(t, control, "same")
	if checks.Load() != 1 {
		t.Fatal("same request was executed twice", checks.Load())
	}
}
