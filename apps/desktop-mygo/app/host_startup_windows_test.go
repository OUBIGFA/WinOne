//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"winone/desktopmygo/host"
	"winone/internal/transport"
)

// These checks use the production Host beside the test executable, real pipe
// reservations and isolated profiles. The build runner selects a private
// desktop; the ordinary test suite never starts a tray or user modules.
func TestNativeHostStartup(t *testing.T) {
	root := os.Getenv("WINONE_MYGO_HOST_STARTUP_CHECK_DIR")
	if root == "" {
		t.Skip("build/test-host-startup.ps1 selects isolated Host startup checks")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(filepath.Dir(self), "WinOne.Host.exe")
	if _, err := os.Stat(hostPath); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "reservation-released"} {
		t.Run(mode, func(t *testing.T) {
			directory := filepath.Join(root, mode)
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			// Disable system modules before the real Host reads this profile.
			configuration := `{"committed":{"schemaVersion":1,"revision":0,"builtinVersion":5,"values":{"host":{"schemaVersion":1,"revision":0,"theme":"light","language":"zh-CN","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}},"moduleEnabled":{"always-on-top":false,"capture":false,"quick-assistant":false,"selection":false,"stroke":false,"timed-power":false,"wintray":false}}}`
			if err := os.WriteFile(filepath.Join(directory, "configuration.json"), []byte(configuration), 0600); err != nil {
				t.Fatal(err)
			}
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-host-startup-%d-%d`, os.Getpid(), time.Now().UnixNano())
			releasePresence, err := markPresence(pipe)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(releasePresence)
			client, err := host.NewClient(pipe)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Cancel)
			if mode == "reservation-released" {
				// A maintenance/previous-start reservation has no request server.
				// Release it after the initial five-second RPC budget and pipe
				// availability check, while ensureHost is waiting for readiness.
				reservation, err := transport.Listen(pipe)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = reservation.Close() })
				release := time.AfterFunc(6*time.Second, func() { _ = reservation.Close() })
				t.Cleanup(func() { release.Stop() })
			}
			started := time.Now()
			snapshot, err := ensureHost(client, pipe, directory, plan{standard: true})
			if err != nil {
				t.Fatalf("Host was not started after endpoint became available (%s): %v", time.Since(started), err)
			}
			server := client.Server()
			if server == nil || snapshot.Locale != "zh-CN" {
				t.Fatalf("startup did not return a real Host snapshot: server=%v locale=%q", server, snapshot.Locale)
			}
			process, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, server.PID())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				// This is the process created from the test-owned Host executable.
				state, err := windows.WaitForSingleObject(process, 0)
				if err != nil {
					t.Error(err)
				} else if state == uint32(windows.WAIT_TIMEOUT) {
					if err := windows.TerminateProcess(process, 0); err != nil {
						t.Error(err)
					}
				}
				if state, err := windows.WaitForSingleObject(process, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
					t.Errorf("test-owned Host did not exit: state=%d error=%v", state, err)
				}
				if err := windows.CloseHandle(process); err != nil {
					t.Error(err)
				}
			})
			// Reopening a window must connect to the same running Host.
			reopened, err := host.NewClient(pipe)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(reopened.Cancel)
			if _, err := ensureHost(reopened, pipe, directory, plan{standard: true}); err != nil {
				t.Fatal(err)
			}
			if reopened.Server() == nil || reopened.Server().PID() != server.PID() {
				t.Fatal("reopening replaced an already running Host")
			}
			t.Logf("real Host ready in %s; reopening reused PID %d", time.Since(started), server.PID())
			// A zero exit status represents an intentional Quit. A refresh of
			// either existing client must fail without creating another Host.
			if err := windows.TerminateProcess(process, 0); err != nil {
				t.Fatal(err)
			}
			if state, err := windows.WaitForSingleObject(process, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
				t.Fatalf("normal Host exit was not observed: state=%d error=%v", state, err)
			}
			for _, existing := range []*host.Client{client, reopened} {
				if _, err := ensureHost(existing, pipe, directory, plan{standard: true}); err == nil {
					t.Fatal("a refresh resurrected a Host that exited normally")
				}
			}
		})
	}
}
