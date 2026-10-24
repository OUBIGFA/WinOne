//go:build windows

package interconnect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These subprocesses deliberately misbehave. They never touch a real service,
// installation, user directory or external network.
func TestWorkerProbe(t *testing.T) {
	mode := ""
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "winone-probe=") {
			mode = strings.TrimPrefix(arg, "winone-probe=")
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "blocked-input":
		time.Sleep(time.Hour)
	case "exit":
		os.Exit(9)
	case "mismatch":
		var request map[string]any
		_ = json.NewDecoder(os.Stdin).Decode(&request)
		fmt.Fprintln(os.Stdout, `{"id":"wrong","result":null}`)
		time.Sleep(time.Hour)
	case "flood":
		for i := 0; i < 5; i++ {
			fmt.Fprintln(os.Stdout, `{"id":"0","result":null}`)
		}
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

func TestWorkerFailureRetiresProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"blocked-input", "exit", "mismatch", "flood"} {
		t.Run(mode, func(t *testing.T) {
			p, err := startWorker(exe, "-test.run=^TestWorkerProbe$", "winone-probe="+mode)
			if err != nil {
				t.Fatal(err)
			}
			defer p.close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = p.call(ctx, "probe", strings.Repeat("x", 1024*1024))
			if err == nil {
				t.Fatal("misbehaving worker returned success")
			}
			if mode == "blocked-input" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if !p.retired() {
				t.Fatal("failed worker was reusable")
			}
			select {
			case <-p.done:
			case <-time.After(3 * time.Second):
				t.Fatal("retired worker did not exit")
			}
		})
	}
}

// Opt-in executable, isolated data only. No real installation or external network.
func TestLANWorkerLifecycle(t *testing.T) {
	exe := os.Getenv("WINONE_INTERCONNECT_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_INTERCONNECT_TEST_EXE to a built adapter")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-manager-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "run-")
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(exe, data)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	before, err := m.Call(ctx, "get", nil)
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ Running bool }
	if err = json.Unmarshal(before, &state); err != nil || state.Running || m.process != nil {
		t.Fatalf("read starts resident worker: %s %v", before, err)
	}
	params, _ := json.Marshal(Preferences{Bind: "127.0.0.1", Port: 0, DownloadDir: filepath.Join(data, "received")})
	reply, err := m.Call(ctx, "start", params)
	if err != nil {
		t.Fatal(err)
	}
	var info struct{ Port int }
	if err = json.Unmarshal(reply, &info); err != nil || info.Port == 0 {
		t.Fatalf("invalid bound port: %s", reply)
	}
	if _, err = m.Call(ctx, "add_text", json.RawMessage("\"文本\"")); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Call(ctx, "get", nil); err != nil {
		t.Fatal(err)
	}
	p := m.process
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("Close returned with a live adapter")
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(info.Port)), time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("port survived Close")
	}
	restored, err := New(exe, data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.preferences.DownloadDir != filepath.Join(data, "received") {
		t.Fatal("receive folder was not restored")
	}
}
