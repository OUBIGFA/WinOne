package modules

import (
	"context"
	"encoding/json"
	"testing"
	"winone/internal/host"
)

type releaseFixture struct {
	methods []string
	closed  bool
}

func (w *releaseFixture) Resume() {}
func (w *releaseFixture) Handle(r host.Request) host.Response {
	w.methods = append(w.methods, r.Method)
	return host.Response{ID: r.ID, Result: json.RawMessage(`{"state":"completed"}`)}
}
func (w *releaseFixture) Close() error { w.closed = true; return nil }

func TestReleaseWinTraySeparatesUpdateAndUninstall(t *testing.T) {
	for _, uninstall := range []bool{false, true} {
		worker := &releaseFixture{}
		if err := ReleaseWinTray(context.Background(), worker, uninstall); err != nil {
			t.Fatal(err)
		}
		want := "deactivate"
		if uninstall {
			want = "uninstall.prepare"
		}
		if len(worker.methods) != 1 || worker.methods[0] != want || !worker.closed {
			t.Fatal(worker)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker := &releaseFixture{}
	if err := ReleaseWinTray(ctx, worker, false); err == nil || len(worker.methods) != 0 {
		t.Fatal("cancelled release started work")
	}
}
