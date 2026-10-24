package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/schollz/croc/v11/src/store"
	"github.com/schollz/croc/v11/src/tcp"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A real adapter subprocess gives each peer its own working directory, stdin,
// stdout, proxy globals and crypto state, exactly as the Windows Host does.
func TestAdapterProcess(t *testing.T) {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "winone-transfer-probe" {
		main()
		os.Exit(0)
	}
}

type probe struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	replies  chan []byte
	sequence int
}

func startProbe(t *testing.T) *probe {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAdapterProcess$", "winone-transfer-probe")
	return startProbeCommand(t, cmd)
}

func startProbeCommand(t *testing.T, cmd *exec.Cmd) *probe {
	t.Helper()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &probe{cmd: cmd, input: in, replies: make(chan []byte, 8)}
	go func() {
		defer close(p.replies)
		s := bufio.NewScanner(out)
		s.Buffer(make([]byte, 4096), 2<<20)
		for s.Scan() {
			p.replies <- append([]byte(nil), s.Bytes()...)
		}
	}()
	t.Cleanup(func() {
		in.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Log(stderr.String())
			}
		case <-time.After(6 * time.Second):
			cmd.Process.Kill()
			<-done
			t.Error("worker survived EOF")
		}
	})
	return p
}
func (p *probe) call(t *testing.T, method string, params any) map[string]any {
	t.Helper()
	p.sequence++
	id := strconv.Itoa(p.sequence)
	if err := json.NewEncoder(p.input).Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-p.replies:
		var response struct {
			ID     string
			Result map[string]any
			Error  string
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if response.ID != id || response.Error != "" {
			t.Fatalf("%s: %s", method, raw)
		}
		return response.Result
	case <-time.After(12 * time.Second):
		t.Fatalf("%s did not respond", method)
	}
	return nil
}
func (p *probe) finish(t *testing.T) map[string]any {
	t.Helper()
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		s := p.call(t, "snapshot", nil)
		if prompt, ok := s["prompt"].(map[string]any); ok {
			p.call(t, "answer", map[string]any{"id": prompt["id"], "answer": "yes"})
		}
		if s["status"] == "completed" {
			return s
		}
		if s["status"] == "failed" || s["status"] == "cancelled" {
			t.Fatalf("transfer: %+v", s)
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("transfer did not finish: %+v", p.call(t, "snapshot", nil))
	return nil
}
func transferPaths(t *testing.T) (string, string, string) {
	t.Helper()
	root := os.Getenv("WINONE_TRANSFER_TEST_ROOT")
	if root == "" {
		t.Skip("set WINONE_TRANSFER_TEST_ROOT to an isolated workspace _temp directory")
	}
	dir, err := os.MkdirTemp(root, "adapter-")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source")
	out := filepath.Join(dir, "received")
	for _, p := range []string{source, out} {
		if err = os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir, source, out
}
func TestStoredTransferRoundTripAndRevoke(t *testing.T) {
	dir, source, out := transferPaths(t)
	service, err := store.New(store.Config{Root: filepath.Join(dir, "server"), MaxTransferBytes: 16 << 20, MaxTotalBytes: 64 << 20, MinFreeBytes: 1, MaxFiles: 10, MaxDownloads: 10, CreatePerHour: 100, MaxActiveUploads: 10, DisableRootLock: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service)
	defer server.Close()
	path := filepath.Join(source, "资料.bin")
	payload := bytes.Repeat([]byte("croc store round trip"), 400000)
	if err = os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sender := startProbe(t)
	sender.call(t, "start", map[string]any{"mode": "store_send", "paths": []string{path}, "dataDir": filepath.Join(dir, "send"), "storeOrigin": server.URL, "downloads": 2, "expirationHours": 1})
	sent := sender.finish(t)["result"].(map[string]any)
	code := sent["code"].(string)
	inspector := startProbe(t)
	inspector.call(t, "start", map[string]any{"mode": "store_inspect", "code": code, "dataDir": filepath.Join(dir, "inspect")})
	inspected := inspector.finish(t)["result"].(map[string]any)
	if len(inspected["manifest"].(map[string]any)["f"].([]any)) != 1 {
		t.Fatalf("bad manifest: %+v", inspected)
	}
	receiver := startProbe(t)
	receiver.call(t, "start", map[string]any{"mode": "store_receive", "code": code, "dataDir": filepath.Join(dir, "receive-task"), "outputDir": out})
	receiver.finish(t)
	got, err := os.ReadFile(filepath.Join(out, "资料.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("stored bytes differ: %v", err)
	}
	revoke := startProbe(t)
	revoke.call(t, "start", map[string]any{"mode": "store_revoke", "code": code, "uploadToken": sent["uploadToken"], "dataDir": filepath.Join(dir, "revoke")})
	revoke.finish(t)
}
func TestCrocRealRelayFilesFoldersAndCancellation(t *testing.T) {
	dir, source, out := transferPaths(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listeners := []net.Listener{}
	ports := []string{}
	for i := 0; i < 3; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, ln)
		ports = append(ports, strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	}
	for i, ln := range listeners {
		t.Cleanup(func() { ln.Close() })
		banner := ""
		if i == 0 {
			banner = strings.Join(ports[1:], ",")
		}
		go tcp.RunWithListener(ln, "probe-password", tcp.WithCtx(ctx), tcp.WithLogLevel("error"), tcp.WithBanner(banner))
	}
	payload := bytes.Repeat([]byte("real croc bytes"), 65536)
	nested := filepath.Join(source, "目录", "nested")
	os.MkdirAll(filepath.Join(nested, "empty"), 0700)
	if err := os.WriteFile(filepath.Join(nested, "payload.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(nested, "zero.txt"), nil, 0600)
	options := map[string]any{"relayAddress": listeners[0].Addr().String(), "relayPorts": ports, "relayPassword": "probe-password", "transport": "relay", "disableLocal": true, "noPrompt": true}
	sender := startProbe(t)
	receiver := startProbe(t)
	sender.call(t, "start", map[string]any{"mode": "send", "paths": []string{filepath.Join(source, "目录")}, "code": "1234-probe-transfer-code", "dataDir": filepath.Join(dir, "send"), "options": options})
	receiverOptions := map[string]any{}
	for k, v := range options {
		receiverOptions[k] = v
	}
	receiverOptions["transport"] = "auto"
	receiverOptions["noPrompt"] = false
	receiver.call(t, "start", map[string]any{"mode": "receive", "code": "1234-probe-transfer-code", "dataDir": filepath.Join(dir, "receive-task"), "outputDir": out, "options": receiverOptions})
	receiver.finish(t)
	sender.finish(t)
	received := filepath.Join(out, "目录", "nested")
	got, err := os.ReadFile(filepath.Join(received, "payload.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("real croc bytes differ: %v", err)
	}
	if stat, err := os.Stat(filepath.Join(received, "empty")); err != nil || !stat.IsDir() {
		t.Fatal("empty folder missing", err)
	}
	// Croc's own rename policy must preserve an existing, different file.
	if err = os.WriteFile(filepath.Join(received, "payload.bin"), []byte("keep original"), 0600); err != nil {
		t.Fatal(err)
	}
	receiverOptions["rename"] = true
	sender2, receiver2 := startProbe(t), startProbe(t)
	sender2.call(t, "start", map[string]any{"mode": "send", "paths": []string{filepath.Join(source, "目录")}, "code": "5678-probe-rename-code", "dataDir": filepath.Join(dir, "send2"), "options": options})
	receiver2.call(t, "start", map[string]any{"mode": "receive", "code": "5678-probe-rename-code", "dataDir": filepath.Join(dir, "receive2"), "outputDir": out, "options": receiverOptions})
	receiver2.finish(t)
	sender2.finish(t)
	kept, err := os.ReadFile(filepath.Join(received, "payload.bin"))
	if err != nil || string(kept) != "keep original" {
		t.Fatal("rename overwrote the existing file", err)
	}
	matched := false
	entries, _ := os.ReadDir(received)
	for _, entry := range entries {
		if !entry.IsDir() {
			data, _ := os.ReadFile(filepath.Join(received, entry.Name()))
			if bytes.Equal(data, payload) {
				matched = true
			}
		}
	}
	if !matched {
		t.Fatal("renamed bytes missing")
	}
	textSender, textReceiver := startProbe(t), startProbe(t)
	textSender.call(t, "start", map[string]any{"mode": "send", "text": "完整文本 / complete text", "code": "9012-probe-text-code", "dataDir": filepath.Join(dir, "text-send"), "options": options})
	textReceiver.call(t, "start", map[string]any{"mode": "receive", "code": "9012-probe-text-code", "dataDir": filepath.Join(dir, "text-receive"), "outputDir": out, "options": receiverOptions})
	textResult := textReceiver.finish(t)["result"].(map[string]any)
	textSender.finish(t)
	message, err := os.ReadFile(textResult["textPath"].(string))
	if err != nil || !strings.Contains(string(message), "完整文本 / complete text") {
		t.Fatal("received text lost", err)
	}
	waiting := startProbe(t)
	waiting.call(t, "start", map[string]any{"mode": "receive", "code": "9999-other-cancel-code", "dataDir": filepath.Join(dir, "cancel"), "outputDir": out, "options": receiverOptions})
	waiting.call(t, "stop", nil)
	fmt.Fprintln(io.Discard, "isolated relay completed")
}
