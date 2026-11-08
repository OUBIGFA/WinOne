//go:build windows

package orchestrator

import (
	"context"
	"testing"
	"time"
)

// A task-launched GUI can have a live process long before it has a window.
// Exceeding the configured window wait must not become "already running,
// skipped": that falsely reports an automatic close-to-tray operation as done.
func TestTaskStartedLateWindowReportsUnfinishedDismissal(t *testing.T) {
	entry, file := startupFixtureEntry(t, "late-2500")
	entry.TrayBehavior.CloseDelaySeconds = 0
	cmd, err := startProcess(entry.ExePath, entry.Args, launchVisible)
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Release()
	svc := NewService(NewWin32WindowEnumerator(), NewWin32WindowManager(), &startupRenderingLogger{t: t, start: time.Now()})
	got := svc.StartAndManage(context.Background(), entry, 1)
	if got.Managed || got.Code != ResultNoWindowManaged {
		t.Errorf("window not ready within configured wait was reported as successful: %+v", got)
	}
	state := awaitStartupFixture(t, file)
	for _, hwnd := range state.Forms {
		if !isWindowVisible(hwnd) {
			t.Fatal("late window should remain usable after the first request ended")
		}
	}
	// Once ready, the original process and windows must still use native close.
	got = svc.StartAndManage(context.Background(), entry, 5)
	if !got.Managed || got.Code != ResultAlreadyRunningManaged || got.Hidden != nil {
		t.Fatalf("retry did not dismiss the existing native windows: %+v", got)
	}
	for _, hwnd := range state.Forms {
		if isWindowVisible(hwnd) || visibilityOwner(hwnd) != 0 {
			t.Fatal("retry left a visible or shielded window")
		}
	}
}

func TestLateWindowWithinConfiguredWaitClosesNatively(t *testing.T) {
	entry, file := startupFixtureEntry(t, "late-1500")
	entry.TrayBehavior.CloseDelaySeconds = 0
	svc := NewService(NewWin32WindowEnumerator(), NewWin32WindowManager(), &startupRenderingLogger{t: t, start: time.Now()})
	got := svc.StartAndManage(context.Background(), entry, 5)
	if !got.Managed || got.Code != ResultManaged || got.Hidden != nil {
		t.Fatalf("late native window was not dismissed: %+v", got)
	}
	for _, hwnd := range awaitStartupFixture(t, file).Forms {
		if isWindowVisible(hwnd) || visibilityOwner(hwnd) != 0 {
			t.Fatal("late native window remained visible or shielded")
		}
	}
}

func TestAlreadyHiddenNativeWindowIsNotADismissalFailure(t *testing.T) {
	entry, file := startupFixtureEntry(t, "silent")
	entry.TrayBehavior.CloseDelaySeconds = 0
	cmd, err := startProcess(entry.ExePath, entry.Args, launchVisible)
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Release()
	state := awaitStartupFixture(t, file)
	svc := NewService(NewWin32WindowEnumerator(), NewWin32WindowManager(), &testLogger{})
	got := svc.StartAndManage(context.Background(), entry, 0)
	if !got.Managed || got.Code != ResultAlreadyRunningSkipped {
		t.Fatalf("already-native-hidden UI reported as a failed dismissal: %+v", got)
	}
	for _, hwnd := range state.Forms {
		if isWindowVisible(hwnd) || visibilityOwner(hwnd) != 0 {
			t.Fatal("already-hidden UI was shown or left shielded")
		}
	}
}

func TestAlreadyRunningPartialDismissalIsNotHiddenSuccess(t *testing.T) {
	entry, file := startupFixtureEntry(t, "partial")
	entry.TrayBehavior.CloseDelaySeconds = 0
	cmd, err := startProcess(entry.ExePath, entry.Args, launchVisible)
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Release()
	state := awaitStartupFixture(t, file)
	svc := NewService(NewWin32WindowEnumerator(), NewWin32WindowManager(), &testLogger{})
	got := svc.StartAndManage(context.Background(), entry, 3)
	if got.Managed || got.Code != ResultNoWindowManaged {
		t.Fatalf("visible secondary window was concealed by a hidden primary: %+v", got)
	}
	if !isWindowVisible(state.Forms[1]) || visibilityOwner(state.Forms[1]) != 0 {
		t.Fatal("refusing window was not preserved and released")
	}
}
