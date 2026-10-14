//go:build windows

// Stability probe performs a full enabled -> disabled -> enabled transition
// against one dedicated private-desktop Host. It never handles user profiles.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
)

func main() {
	pipe := flag.String("pipe", "", "dedicated stability pipe")
	exitPID := flag.Uint("exit-pid", 0, "normal tray exit for the held fixture Host on this private desktop")
	cycle := flag.Bool("cycle", false, "cycle the six bundled modules")
	disable := flag.Bool("disable", false, "finish by disabling all modules before normal Host exit")
	flag.Parse()
	if *exitPID != 0 {
		// Pin ownership to the actual server of this dedicated pipe before
		// targeting a window. A CLI PID is not a process identity by itself.
		conn, err := windows.CreateFile(windows.StringToUTF16Ptr(*pipe), windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			panic(err)
		}
		var owner uint32
		err = windows.GetNamedPipeServerProcessId(conn, &owner)
		windows.CloseHandle(conn)
		if err != nil || owner != uint32(*exitPID) {
			panic("exit target is not the dedicated pipe server")
		}
		user := windows.NewLazySystemDLL("user32.dll")
		var found uintptr
		callback := windows.NewCallback(func(window, _ uintptr) uintptr {
			var pid uint32
			user.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
			var name [64]uint16
			user.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&name[0])), 64)
			if pid == uint32(*exitPID) && windows.UTF16ToString(name[:]) == "SystrayClass" {
				found = window
			}
			return 1
		})
		if err := windows.EnumWindows(callback, nil); err != nil {
			panic(err)
		}
		if found == 0 {
			panic("owned Host tray not found on the fixture desktop")
		}
		// A private desktop has no Explorer notification area, so NIM_ADD can
		// reject the icon before its menu is built. Modules were explicitly
		// disabled by the caller; close this owned Host window normally to run
		// the existing WM_DESTROY/onExit cancellation and deferred cleanup.
		if ok, _, err := user.NewProc("PostMessageW").Call(found, 0x10, 0, 0); ok == 0 {
			panic(err)
		}
		fmt.Printf("Exit request sent to owned Host PID %d HWND %x\n", owner, found)
		return
	}
	if !strings.HasPrefix(*pipe, `\\.\pipe\WinOne-stability-`) {
		panic("dedicated stability pipe required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	call := func(method string, params any, out any) error {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		r, err := transport.CallCompleting(ctx, *pipe, host.Request{Version: 1, ID: fmt.Sprint(time.Now().UnixNano()), Method: method, Params: raw}, 5*time.Second)
		if err != nil {
			return err
		}
		if r.Error != nil {
			return fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	}
	snapshot := func() modules.ControlSnapshot {
		var value modules.ControlSnapshot
		if err := call("modules.snapshot", nil, &value); err != nil {
			panic(err)
		}
		return value
	}
	verify := func(enabled bool) {
		state := snapshot()
		if len(state.Modules) != 6 {
			panic("expected all six bundled modules")
		}
		for _, item := range state.Modules {
			if item.Enabled != enabled || item.PendingStop || (!enabled && item.Runtime.State != "disabled") || (enabled && item.Runtime.State != "available" && item.Runtime.State != "ready") {
				panic(fmt.Sprintf("incomplete module state: %+v", item))
			}
		}
	}
	verify(true)
	times := map[string]float64{}
	if *cycle || *disable {
		for _, id := range []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke"} {
			if err := call(id+".snapshot", nil, nil); err != nil {
				panic(err)
			}
		}
		states := []bool{false, true}
		if *disable {
			states = []bool{false}
		}
		for _, enabled := range states {
			for _, id := range []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke"} {
				revision := snapshot().Revision
				started := time.Now()
				var job modules.ControlJob
				if err := call("modules.setEnabled", map[string]any{"id": id, "enabled": enabled, "revision": revision}, &job); err != nil {
					panic(err)
				}
				for job.State == "running" && time.Since(started) < 5*time.Second {
					time.Sleep(25 * time.Millisecond)
					if err := call("modules.jobs.get", map[string]string{"id": job.ID}, &job); err != nil {
						panic(err)
					}
				}
				if job.State != "completed" || time.Since(started) > 5*time.Second {
					panic(fmt.Sprintf("transition exceeded 5 second budget: %s %t %+v", id, enabled, job))
				}
				times[fmt.Sprintf("%s:%t", id, enabled)] = float64(time.Since(started)) / float64(time.Millisecond)
			}
			verify(enabled)
		}
	}
	result := map[string]any{"capturedUTC": time.Now().UTC().Format(time.RFC3339Nano), "healthy": true, "cycled": *cycle, "transitionsMs": times, "modules": snapshot().Modules}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}
