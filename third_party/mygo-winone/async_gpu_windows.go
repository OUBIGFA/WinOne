//go:build windows

package ui

import (
	"github.com/egoist/mygo/internal/gpu/d3d11"
	"github.com/egoist/mygo/internal/platform"
)

func init() {
	prepareGPU = func(n platform.SurfaceNative) *gpuPreparation {
		// Composed surfaces need a D3D device even to show CPU pixels. Keep
		// their existing path; opaque native windows can paint through GDI.
		if n.HWND == 0 || n.Composed {
			return nil
		}
		ready, construct, cancel := d3d11.PrepareDevice()
		return &gpuPreparation{ready: ready, cancel: cancel, finish: func(current platform.SurfaceNative) (gpuRenderer, error) {
			return construct(current.HWND, current.Composed)
		}}
	}
}
