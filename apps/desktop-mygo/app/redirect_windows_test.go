//go:build windows

package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Exercise the real cross-process window identity on the private desktop. The
// delayed child models scheduler/cold-start work before MyGo creates its HWND.
func TestWindowRedirectedStartup(t *testing.T) {
	root := os.Getenv("WINONE_MYGO_CLOSE_CHECK_DIR")
	if root == "" {
		t.Skip("requires private-desktop runner")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"redirect-delayed", "redirect-hidden"} {
		t.Run(mode, func(t *testing.T) {
			directory := filepath.Join(root, mode)
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-Redirect-%d-%d`, os.Getpid(), time.Now().UnixNano())
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, self, "--redirect-window-helper", pipe, directory, mode)
			cmd.Env = append(os.Environ(), "APPDATA="+directory, "LOCALAPPDATA="+directory, "MYGO_GPU=0")
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); _ = cmd.Wait() }()
			if mode == "redirect-hidden" {
				deadline := time.Now().Add(5 * time.Second)
				for !fileExists(filepath.Join(directory, "attached")) && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if !fileExists(filepath.Join(directory, "attached")) {
					t.Fatal("notification was not attached")
				}
			}
			if err := activateRedirected(pipe); err != nil {
				t.Fatal(err)
			}
			if window := lifecycleWindow(pipe, uint32(cmd.Process.Pid)); window == 0 {
				t.Fatal("activation reported success before the verified settings window was visible")
			}
		})
	}
}

func runRedirectWindowHelper() error {
	pipe, directory, mode := os.Args[2], os.Args[3], os.Args[4]
	if mode == "redirect-delayed" {
		time.Sleep(9 * time.Second)
	}
	inst, err := claimInstance(pipe, false)
	if err != nil {
		return err
	}
	defer inst.close()
	mygo.App.SetName("WinOne Redirect Check")
	mygo.App.SetPath(mygo.PathUserData, directory)
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{Title: "WinOne redirect fixture", Width: 480, Height: 320, Hidden: true, Content: ui.View(func(c *ui.Context) { ui.Text(c, "Startup readiness") })})
		if err := inst.attach(w.NativeHandle(), func(bool) { w.Show() }); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "attached"), nil, 0600); err != nil {
			panic(err)
		}
		if mode == "redirect-hidden" {
			go func() { time.Sleep(750 * time.Millisecond); mygo.RunOnMain(w.Show) }()
		} else {
			w.Show()
		}
	})
	return mygo.App.Run()
}
