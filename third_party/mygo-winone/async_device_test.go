//go:build windows

package d3d11

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/gpu/gputest"
)

// A blocked driver must not block window setup or close. A late result has
// exactly one owner, including when close wins the race with initialization.
func TestWinOneDevicePreparationOwnership(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "consume", true: "close-before-ready"}[cancel], func(t *testing.T) {
			started, finish, freed := make(chan struct{}), make(chan struct{}), make(chan struct{}, 2)
			var releases atomic.Int32
			_, take, discard := prepareDevice(func() deviceResult {
				close(started)
				<-finish
				return deviceResult{device: 11, context: 12}
			}, func(result deviceResult) {
				if result.device != 11 || result.context != 12 {
					t.Errorf("wrong device released: %+v", result)
				}
				releases.Add(1)
				freed <- struct{}{}
			})
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("device creation was not started alongside window setup")
			}
			if cancel {
				discard() // Must return while the driver is still blocked.
				discard()
			}
			close(finish)
			result := take()
			if cancel {
				if result.err == nil {
					t.Fatal("closed window claimed the abandoned device")
				}
				select {
				case <-freed:
				case <-time.After(time.Second):
					t.Fatal("late device was not released")
				}
				if releases.Load() != 1 {
					t.Fatal("device was released more than once")
				}
			} else {
				if result.err != nil || result.device != 11 || result.context != 12 {
					t.Fatalf("device transfer failed: %+v", result)
				}
				discard() // Ownership has passed to the renderer.
				if releases.Load() != 0 || take().err == nil {
					t.Fatal("transferred device was freed or claimed twice")
				}
			}
		})
	}
}

func TestWinOneDevicePreparationRetainsDriverError(t *testing.T) {
	want := errors.New("driver initialization failed")
	_, take, discard := prepareDevice(func() deviceResult { return deviceResult{err: want} }, func(deviceResult) {
		t.Error("consumed failure was discarded")
	})
	if got := take(); got.err != want {
		t.Fatalf("driver error lost: %v", got.err)
	}
	discard()
}

func TestWinOnePreparedRendererDrawsOriginalScene(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s := gputest.Scene()
	ready, construct, cancel := PrepareDevice()
	window := hiddenWindow(t, s.Width, s.Height)
	<-ready
	defer cancel()
	r, err := construct(window, false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	// Read back real GPU pixels, including a changed frame, using the same
	// upstream scene and CPU reference as the ordinary renderer regression.
	for range 2 {
		if err := r.draw(s); err != nil {
			t.Fatal(err)
		}
		pixels, stride := r.readBack(t)
		gputest.Compare(t, "prepared-d3d11", pixels, stride, s)
	}
}
