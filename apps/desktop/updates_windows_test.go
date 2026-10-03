//go:build windows

package main

import (
	"context"
	"os"
	"testing"
	"winone/internal/updates"
)

func TestUpdateSessionCancellationIdentityAndSingleFlight(t *testing.T) {
	var session updateSession
	started := make(chan struct{})
	finished := make(chan UpdateReply, 1)
	go func() {
		finished <- session.check(context.Background(), "first", func(ctx context.Context) (updates.Result, error) {
			close(started)
			<-ctx.Done()
			return updates.Result{}, ctx.Err()
		})
	}()
	<-started
	if reply := session.check(context.Background(), "second", func(context.Context) (updates.Result, error) {
		t.Error("parallel update check started")
		return updates.Result{}, nil
	}); reply.ErrorCode != "busy" {
		t.Fatal(reply)
	}
	session.cancelRequest("old")
	select {
	case <-finished:
		t.Fatal("stale cancellation stopped new check")
	default:
	}
	session.cancelRequest("first")
	if reply := <-finished; reply.ErrorCode != "cancelled" {
		t.Fatal(reply)
	}
	result := updates.Result{State: "available", LatestVersion: "1.2.0", ReleaseURL: "https://github.com/example/WinOne/releases/tag/v1.2.0"}
	if reply := session.check(context.Background(), "next", func(context.Context) (updates.Result, error) { return result, nil }); reply.State != "available" {
		t.Fatal(reply)
	}
	if session.releaseURL() != result.ReleaseURL {
		t.Fatal("verified release not retained")
	}
	session.check(context.Background(), "failed", func(context.Context) (updates.Result, error) { return updates.Result{}, context.DeadlineExceeded })
	if session.releaseURL() != "" {
		t.Fatal("failed check left a stale release link")
	}
}

func TestUpdateBuildInfoDefaultsToNoSource(t *testing.T) {
	info := (&App{}).UpdateInfo()
	if info.Version == "" || info.Configured {
		t.Fatal(info)
	}
}

func TestUpdateBuildInfoUsesInjectedValues(t *testing.T) {
	repository := os.Getenv("WINONE_TEST_UPDATE_REPOSITORY")
	if repository == "" {
		t.Skip("explicit build metadata check")
	}
	info := (&App{}).UpdateInfo()
	if updateRepository != repository || info.Version != "9.8.7" || !info.Configured {
		t.Fatal("build metadata was not applied", info)
	}
}
