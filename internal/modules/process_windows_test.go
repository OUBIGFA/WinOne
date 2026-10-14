//go:build windows

package modules

import (
	"context"
	"testing"
	"winone/internal/host"
)

func TestCancelledStartDoesNotLaunchOrRequestElevation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, elevated := range []bool{false, true} {
		p := start(ctx, "always-on-top", "窗口置顶", "unused.exe", "unused", t.TempDir(), false, elevated)
		if p.Status().State != "faulted" || p.pid != 0 {
			t.Fatal("cancelled start attempted a launch")
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConfigurationRefreshSkipsClosedLazyWorker(t *testing.T) {
	lazy := NewLazy(context.Background(), "always-on-top", "置顶", "not-installed.exe", "unused", t.TempDir())
	if err := lazy.Close(); err != nil {
		t.Fatal(err)
	}
	if response, called := lazy.HandleRunning(host.Request{Version: 1, ID: "refresh", Method: "settings.reload"}); called {
		t.Fatal("closed worker blocked preference persistence", response)
	}
}

func TestFailedStartKeepsOriginalCauseInsteadOfProbingAnAbsentWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process := start(ctx, "example.tool", "Example", "unused.exe", "unused-pipe", t.TempDir(), false, false)
	reply := process.Handle(host.Request{Version: 1, ID: "failed", Method: "snapshot"})
	if reply.Error == nil || reply.Error.Code != "module_unavailable" || reply.Error.Message != context.Canceled.Error() {
		t.Fatal("startup cause was replaced by a pipe error", reply.Error)
	}
}
