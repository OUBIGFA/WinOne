package ui

import (
	"os"

	"github.com/egoist/mygo/internal/platform"
)

// Only Windows supplies this device preparation. Other backends retain their
// existing GPU lifecycle and threading requirements.
var prepareGPU func(platform.SurfaceNative) *gpuPreparation

type gpuPreparation struct {
	ready  <-chan struct{}
	finish func(platform.SurfaceNative) (gpuRenderer, error)
	cancel func()
}

func (h *windowHost) prepareGraphics() {
	if prepareGPU == nil || h.conn.Post == nil || os.Getenv("MYGO_GPU") == "0" {
		return
	}
	p := prepareGPU(h.conn.Surface.Native())
	if p == nil {
		return
	}
	h.preparing = p
	go func() {
		<-p.ready
		h.conn.Post(func() {
			if !h.detached && h.preparing == p {
				// The existing scene and event state survive the handoff. No
				// placeholder, hidden first frame, timer or resident UI is used.
				h.requestFrame()
			}
		})
	}()
}

func (h *windowHost) cancelGraphics() {
	if h.preparing != nil {
		h.preparing.cancel()
		h.preparing = nil
	}
}
