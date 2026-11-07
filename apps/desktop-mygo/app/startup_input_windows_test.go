//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"winone/desktopmygo/core"
	"winone/internal/transport"
)

type startupInputState struct {
	Button, Field ui.Rect
	GPU           bool
	Text          string
	Clicks        int
}

var postStartupInput = user32.NewProc("PostMessageW")

// This page uses ordinary production controls in the real production shell.
// Draw receipts are written after presentation through the window callback,
// so the parent does not confuse layout or handle creation with a live frame.
type startupInputPage struct {
	directory string
	env       *core.Env
	text      string
	clicks    int
	prior     startupInputState
}

func (p *startupInputPage) View(c *ui.Context) {
	var state startupInputState
	ui.Column(c).FillWidth().Children(func() {
		if ui.Button(c, "Startup input check").Draw(func(_ *ui.Painter, r ui.Rect) { state.Button = r }).Clicked() {
			p.clicks++
		}
		ui.TextInput(c, &p.text).Width(320).Draw(func(_ *ui.Painter, r ui.Rect) { state.Field = r })
	}).Draw(func(painter *ui.Painter, _ ui.Rect) {
		state.GPU, state.Text, state.Clicks = painter.Accelerated(), p.text, p.clicks
		if p.prior.Field.W > 0 && state.GPU == p.prior.GPU && state.Text == p.prior.Text && state.Clicks == p.prior.Clicks {
			return // Do not create an update/paint loop merely to emit receipts.
		}
		p.env.Post(func() {
			if state.Button.W == 0 || state.Field.W == 0 || state == p.prior {
				return
			}
			p.prior = state
			// No polling loop: every receipt corresponds to a changed drawn frame.
			data, err := json.Marshal(state)
			if err != nil {
				panic(err)
			}
			if err = os.WriteFile(filepath.Join(p.directory, "state.json"), data, 0600); err != nil {
				panic(err)
			}
		})
	})
}

func TestWindowStartupInputAndGPUHandoff(t *testing.T) {
	root := os.Getenv("WINONE_MYGO_CLOSE_CHECK_DIR")
	if root == "" || os.Getenv("WINONE_MYGO_CLOSE_CHECK_GPU") != "1" {
		t.Skip("build/test-desktop-mygo-close.ps1 -UseGPU selects the owned private desktop")
	}
	directory := filepath.Join(root, "startup-input")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-Startup-Input-%d`, os.Getpid())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	go transport.Serve(ctx, listener, newNativeProbeHost())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, self, "--close-window-helper", pipe, directory, "startup-input")
	cmd.Env = append(os.Environ(), "APPDATA="+directory, "LOCALAPPDATA="+directory, "MYGO_GPU=1", "MYGO_FRAME_STATS=all")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	var exitErr error
	go func() { exitErr = cmd.Wait(); close(exited) }()
	defer func() {
		cancel()
		<-exited
		_ = os.WriteFile(filepath.Join(directory, "frames.log"), output.Bytes(), 0600)
	}()
	await := func(want func(startupInputState) bool) startupInputState {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-exited:
				t.Fatalf("input helper exited: %v; %s", exitErr, output.String())
			default:
			}
			data, _ := os.ReadFile(filepath.Join(directory, "state.json"))
			var state startupInputState
			if json.Unmarshal(data, &state) == nil && want(state) {
				return state
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("native input/handoff receipt timed out")
		return startupInputState{}
	}
	state := await(func(s startupInputState) bool { return s.Button.W > 0 && s.Field.W > 0 })
	window := lifecycleWindow(pipe, uint32(cmd.Process.Pid))
	class, _ := windows.UTF16PtrFromString("MyGoSurface")
	surface, _, _ := procFindWindowExW.Call(window, 0, uintptr(unsafe.Pointer(class)), 0)
	if window == 0 || surface == 0 {
		t.Fatal("owned native input surface missing")
	}
	click := func(r ui.Rect) {
		t.Helper()
		point := uintptr(uint32(int(r.X+r.W/2))&0xffff | uint32(int(r.Y+r.H/2))<<16)
		for _, message := range []uintptr{0x201, 0x202} { // WM_LBUTTONDOWN/UP on our private surface.
			if ok, _, e := postStartupInput.Call(surface, message, 0, point); ok == 0 {
				t.Fatal(e)
			}
		}
	}
	click(state.Button)
	state = await(func(s startupInputState) bool { return s.Clicks == 1 })
	clickedBeforeGPU := !state.GPU
	click(state.Field)
	for _, r := range "before" {
		if ok, _, e := postStartupInput.Call(surface, 0x102, uintptr(r), 0); ok == 0 {
			t.Fatal(e)
		}
	}
	state = await(func(s startupInputState) bool { return s.Text == "before" })
	typedBeforeGPU := !state.GPU
	state = await(func(s startupInputState) bool { return s.GPU })
	for _, r := range " after" {
		if ok, _, e := postStartupInput.Call(surface, 0x102, uintptr(r), 0); ok == 0 {
			t.Fatal(e)
		}
	}
	state = await(func(s startupInputState) bool { return s.Text == "before after" })
	if state.Clicks != 1 {
		t.Fatal("handoff replayed a button action")
	}
	// A fast driver can win before input arrives. Keep this observable rather
	// than failing a faster machine; the controlled-device regression proves
	// the pending path regardless of hardware timing.
	t.Logf("input received before GPU handoff: click=%t text=%t", clickedBeforeGPU, typedBeforeGPU)
	if ok, _, e := postStartupInput.Call(window, 0x10, 0, 0); ok == 0 {
		t.Fatal(e)
	}
	select {
	case <-exited:
		if exitErr != nil {
			t.Fatal(exitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("input helper stayed resident")
	}
	if !bytes.Contains(output.Bytes(), []byte("drawn on the GPU")) {
		t.Fatal("actual GPU rendering was not observed")
	}
	if (clickedBeforeGPU || typedBeforeGPU) && !bytes.Contains(output.Bytes(), []byte("drawn in memory")) {
		t.Fatal("pre-GPU input did not have an actual CPU-rendered frame")
	}
}
