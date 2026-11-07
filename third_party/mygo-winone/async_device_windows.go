//go:build windows

package d3d11

import (
	"errors"
	"sync"

	"github.com/egoist/mygo/internal/gpu/d3d11/device"
)

type deviceResult struct {
	device, context uintptr
	software        bool
	err             error
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

// PrepareDevice creates only the free-threaded device and context off the UI
// thread. Once ready, the caller constructs the renderer on the UI thread with
// the surface's current HWND; composition, swap chains and painting stay there.
func PrepareDevice() (<-chan struct{}, func(uintptr, bool) (*Renderer, error), func()) {
	ready, take, cancel := prepareDevice(func() deviceResult {
		d, c, software, err := device.New()
		return deviceResult{d, c, software, err}
	}, func(result deviceResult) {
		free(&result.context)
		free(&result.device)
	})
	return ready, func(hwnd uintptr, composed bool) (*Renderer, error) {
		result := take()
		if result.err != nil {
			return nil, result.err
		}
		return initializeRenderer(&Renderer{hwnd: hwnd, composed: composed, images: map[uint64]*texture{},
			device: result.device, ctx: result.context, warp: result.software})
	}, cancel
}

func initializeRenderer(r *Renderer) (*Renderer, error) {
	if err := r.init(); err != nil {
		r.Release()
		return nil, err
	}
	return r, nil
}
