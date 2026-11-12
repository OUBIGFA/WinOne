//go:build windows

package d3d11

import (
	"errors"
	"sync"

	"github.com/egoist/mygo/internal/pacing"
)

type deviceResult struct {
	device *sharedDevice
	err    error
}

// prepareDevice hands the result to either the renderer or cancellation,
// exactly once. Cancellation never waits for a driver still initializing.
func prepareDevice(create func() deviceResult, release func(deviceResult)) (<-chan struct{}, func() deviceResult, func()) {
	ready := make(chan struct{})
	done := make(chan deviceResult, 1)
	go func() { done <- create(); close(ready) }()
	var claimed sync.Once
	return ready, func() deviceResult {
			result := deviceResult{err: errors.New("d3d11: prepared device already claimed")}
			claimed.Do(func() { result = <-done })
			return result
		}, func() {
			claimed.Do(func() { go func() { release(<-done) }() })
		}
}

// PrepareDevice acquires MyGo's shared device, shaders and DXGI factory off the
// UI thread. Since v0.9.0, a raw device/context pair no longer initializes a
// renderer. The UI thread still owns the surface, swap chain and painting.
func PrepareDevice() (<-chan struct{}, func(uintptr, bool) (*Renderer, error), func()) {
	ready, take, cancel := prepareDevice(func() deviceResult {
		d, err := acquireDevice()
		return deviceResult{device: d, err: err}
	}, func(result deviceResult) {
		if result.device != nil {
			releaseDevice(result.device)
		}
	})
	return ready, func(hwnd uintptr, composed bool) (*Renderer, error) {
		result := take()
		if result.err != nil {
			return nil, result.err
		}
		if hwnd == 0 {
			releaseDevice(result.device)
			return nil, errors.New("d3d11: no window")
		}
		r := &Renderer{hwnd: hwnd, composed: composed, uncapped: pacing.Uncapped, images: map[uint64]*texture{}}
		r.useDevice(result.device)
		return initializeRenderer(r)
	}, cancel
}

func initializeRenderer(r *Renderer) (*Renderer, error) {
	if err := r.init(); err != nil {
		r.Release()
		return nil, err
	}
	return r, nil
}
