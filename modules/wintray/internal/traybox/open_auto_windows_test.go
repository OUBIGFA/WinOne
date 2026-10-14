//go:build windows

package traybox

import (
	"context"
	"errors"
	"fmt"
	"github.com/lxn/win"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutoOpenNativeSingleAndDoublePrograms(t *testing.T) {
	for _, doubleOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(doubleOnly), func(t *testing.T) {
			f := startFakeShell(t)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			var doubles atomic.Int32
			f.mu.Lock()
			f.onEvent = func(hwnd win.HWND, _, event uintptr) {
				if uint32(event) == win.WM_LBUTTONDBLCLK {
					doubles.Add(1)
					if doubleOnly {
						win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)
					}
				}
				if !doubleOnly && uint32(event) == win.WM_LBUTTONUP {
					if win.IsWindowVisible(hwnd) {
						win.ShowWindow(hwnd, win.SW_HIDE)
					} else {
						win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)
					}
				}
			}
			f.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := OpenAutomatically(ctx, Icon{owner: uint32(f.program), ExePath: executable, callback: testCallback, uid: 7}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "automatic open shows target", func() bool { return win.IsWindowVisible(f.program) })
			want := int32(0)
			if doubleOnly {
				want = 1
			}
			if doubles.Load() != want {
				t.Fatal("incorrect fallback count", doubles.Load())
			}
		})
	}
}

func TestAutoOpenUsesOnlyOneFallback(t *testing.T) {
	for _, opened := range []bool{false, true} {
		var actions []Action
		err := autoOpen(func(a Action) error { actions = append(actions, a); return nil }, func() error {
			if opened {
				return nil
			}
			return context.DeadlineExceeded
		})
		want := 1
		if !opened {
			want = 2
		}
		if err != nil || len(actions) != want || actions[0] != ActionClick {
			t.Fatal(actions, err)
		}
		if !opened && actions[1] != ActionDoubleClick {
			t.Fatal(actions)
		}
	}
}
func TestAutoOpenDoesNotRetryReadOrSendFailure(t *testing.T) {
	failure := errors.New("unavailable")
	calls := 0
	err := autoOpen(func(Action) error { calls++; return nil }, func() error { return failure })
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatal(calls, err)
	}
	err = autoOpen(func(Action) error { return failure }, func() error { t.Fatal("observed after failed send"); return nil })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
