package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

type asyncTestSurface struct {
	*testSurface
	requests int
}

func (s *asyncTestSurface) RequestFrame() { s.requests++ }

func TestWinOneAsyncGPUWakesIdleWindowAndIgnoresClosedWindow(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "closed"}[closed], func(t *testing.T) {
			h, surface, frame := gpuHost(t, func() (gpuRenderer, error) { t.Fatal("blocking constructor used"); return nil, nil })
			s := &asyncTestSurface{testSurface: surface}
			h.conn.Surface = s
			posted := make(chan func(), 1)
			h.conn.Post = func(fn func()) { posted <- fn }
			ready := make(chan struct{})
			previous := prepareGPU
			prepareGPU = func(platform.SurfaceNative) *gpuPreparation {
				return &gpuPreparation{ready: ready, finish: func(platform.SurfaceNative) (gpuRenderer, error) { return &testGPU{}, nil }, cancel: func() {}}
			}
			defer func() { prepareGPU = previous }()
			h.prepareGraphics()
			frame()
			if closed {
				h.detach()
			} else {
				defer h.detach()
			}
			before := s.requests
			close(ready)
			select {
			case fn := <-posted:
				fn()
			case <-time.After(time.Second):
				t.Fatal("device completion did not reach the UI thread")
			}
			if closed && s.requests != before || !closed && s.requests != before+1 {
				t.Fatal("completion failed to repaint the open window or touched a closed window")
			}
		})
	}
}

// Keep the production scene and input engine alive while the device is slow.
// Only the device/OS boundary is controlled; layout, pixels, hit tests, focus,
// text editing and click delivery use the actual UI implementation.
func TestWinOneAsyncGPUKeepsControlsLive(t *testing.T) {
	h, surface, frame := gpuHost(t, func() (gpuRenderer, error) {
		t.Fatal("startup used the blocking GPU constructor")
		return nil, nil
	})
	ready := make(chan struct{})
	gpu := &testGPU{}
	h.preparing = &gpuPreparation{ready: ready, finish: func(platform.SurfaceNative) (gpuRenderer, error) { return gpu, nil }, cancel: func() {}}
	var button, field *node
	text, clicks := "", 0
	h.rt = newRuntime(func(c *context) {
		coreColumn(c).Children(func() {
			button = coreButton(c, "Open")
			if button.Clicked() {
				clicks++
			}
			field = coreTextInput(c, &text).Width(180)
		})
	}, h)
	defer h.detach()
	frame()
	if surface.pixels != 1 || h.gpu != nil || h.gpuTried || len(h.soft.Image.Pix) == 0 {
		t.Fatal("full CPU frame was not presented while the device was pending")
	}
	click := func(element *node) {
		r := element.Bounds()
		ev := platform.SurfaceEvent{Kind: platform.PointerDown, X: float64(r.X + r.W/2), Y: float64(r.Y + r.H/2)}
		h.event(ev)
		ev.Kind = platform.PointerUp
		h.event(ev)
		frame()
	}
	click(button)
	click(field)
	h.event(platform.SurfaceEvent{Kind: platform.TextInput, Text: "before"})
	frame()
	if clicks != 1 || text != "before" {
		t.Fatalf("controls were not live before GPU readiness: clicks=%d text=%q", clicks, text)
	}
	close(ready)
	frame()
	if h.gpu != gpu || gpu.frames == 0 || h.preparing != nil || len(h.soft.Image.Pix) != 0 {
		t.Fatal("GPU did not take over or the temporary CPU frame was retained")
	}
	h.event(platform.SurfaceEvent{Kind: platform.TextInput, Text: " after"})
	frame()
	if text != "before after" || clicks != 1 {
		t.Fatalf("handoff lost focus, text or repeated actions: clicks=%d text=%q", clicks, text)
	}
}

func TestWinOneAsyncGPUFailureKeepsPaintedControls(t *testing.T) {
	h, surface, frame := gpuHost(t, func() (gpuRenderer, error) { t.Fatal("unexpected synchronous retry"); return nil, nil })
	ready := make(chan struct{})
	h.preparing = &gpuPreparation{ready: ready, finish: func(platform.SurfaceNative) (gpuRenderer, error) {
		return nil, errors.New("test driver unavailable")
	}, cancel: func() {}}
	defer h.detach()
	frame()
	close(ready)
	frame()
	if surface.pixels != 2 || !h.gpuTried || h.gpu != nil || h.preparing != nil {
		t.Fatal("driver failure lost the existing CPU-rendered interface")
	}
}

func TestWinOneAsyncGPUDetachCancelsPendingDevice(t *testing.T) {
	h, _, frame := gpuHost(t, func() (gpuRenderer, error) { t.Fatal("unexpected constructor"); return nil, nil })
	ready := make(chan struct{})
	cancelled := 0
	h.preparing = &gpuPreparation{ready: ready, finish: func(platform.SurfaceNative) (gpuRenderer, error) {
		t.Fatal("closed window claimed a device")
		return nil, nil
	}, cancel: func() { cancelled++ }}
	frame()
	h.detach()
	close(ready)
	frame()
	if cancelled != 1 || h.preparing != nil {
		t.Fatal("close did not cancel the pending device exactly once")
	}
}
