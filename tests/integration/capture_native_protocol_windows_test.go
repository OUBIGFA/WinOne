//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

func testNativeCaptureProtocol(t *testing.T, executable string) {
	t.Helper()
	directory := t.TempDir()
	settings, _ := json.Marshal(map[string]any{"schemaVersion": 1, "revision": 0, "outputDirectory": filepath.Join(directory, "images")})
	if err := os.WriteFile(filepath.Join(directory, "managed-settings.json"), settings, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-native-protocol-%d`, time.Now().UnixNano())
	command := exec.Command(executable, "--pipe", pipe, "--data-dir", directory)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			<-done
		}
	}()
	call := func(method string) (host.Response, error) {
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "native-protocol", Method: method})
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		reply, err := call("snapshot")
		if err == nil && reply.Error == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("native worker not ready: %v %+v", err, reply)
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, method := range []string{"prepare", "snapshot", "draft.get"} {
		reply, err := call(method)
		if err != nil || reply.Error != nil {
			t.Fatalf("native %s: %v %+v", method, err, reply)
		}
	}
	for _, method := range []string{"settings.save", "copy.last", "unknown.operation"} {
		reply, err := call(method)
		if err != nil || reply.Error == nil {
			t.Fatalf("invalid native operation accepted %s: %v %+v", method, err, reply)
		}
	}
	reply, err := call("shutdown")
	if err != nil || reply.Error != nil {
		t.Fatalf("native shutdown acknowledgment: %v %+v", err, reply)
	}
	reply, err = call("capture.activate")
	if err == nil && reply.Error == nil {
		t.Fatal("native worker admitted another operation after shutdown")
	}
	select {
	case err = <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("native worker did not exit")
	}
}
