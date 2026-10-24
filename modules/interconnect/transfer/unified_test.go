package main

import (
	"context"
	"github.com/schollz/croc/v11/src/tcp"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// One send publishes the same selection through HTTP and croc. Completing the
// one-off code transfer must leave the persistent pool available to phones.
func TestUnifiedSendLANCodeAndPersistent(t *testing.T) {
	exe := os.Getenv("WINONE_PEER_EXE")
	if exe == "" {
		t.Skip("set WINONE_PEER_EXE to the isolated peer bundle")
	}
	dir, source, out := transferPaths(t)
	file := filepath.Join(source, "same-selection.txt")
	const content = "one selection, LAN and code, persistent after delivery"
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var listeners []net.Listener
	var ports []string
	for i := 0; i < 3; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, ln)
		ports = append(ports, strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
		defer ln.Close()
	}
	for i, ln := range listeners {
		banner := ""
		if i == 0 {
			banner = strings.Join(ports[1:], ",")
		}
		go tcp.RunWithListener(ln, "unified-test", tcp.WithCtx(ctx), tcp.WithLogLevel("error"), tcp.WithBanner(banner))
	}
	options := map[string]any{"relayAddress": listeners[0].Addr().String(), "relayPorts": ports, "relayPassword": "unified-test", "transport": "relay", "disableLocal": true, "noPrompt": true}
	outage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "isolated outage", 503) }))
	defer outage.Close()
	sender := startProbeCommand(t, exec.Command(exe))
	sender.call(t, "start", map[string]any{"mode": "share_server", "paths": []string{file}, "dataDir": filepath.Join(dir, "share"), "derpMapURL": outage.URL, "transfer": map[string]any{"code": "3456-unified-send-test", "options": options}})
	var state map[string]any
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state = sender.call(t, "snapshot", nil)
		if state["ready"] == true && state["transfer"] != nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if state["ready"] != true {
		t.Fatalf("LAN unavailable: %+v", state)
	}
	transfer, ok := state["transfer"].(map[string]any)
	if !ok || transfer["code"] != "3456-unified-send-test" {
		t.Fatalf("same task has no code: %+v", state)
	}
	share := state["share"].(map[string]any)
	link := share["info"].(map[string]any)["url"].(string)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	response, err := client.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	base := strings.Split(link, "/?")[0]
	item := share["items"].([]any)[0].(map[string]any)
	download := func() {
		t.Helper()
		response, err := client.Get(base + "/api/items/" + item["id"].(string) + "/download")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		bytes, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != 200 || string(bytes) != content {
			t.Fatalf("HTTP contents differ: %d %s %v", response.StatusCode, bytes, err)
		}
	}
	download()
	receiver := startProbe(t)
	receiverOptions := map[string]any{}
	for key,value := range options { receiverOptions[key] = value }
	receiverOptions["transport"] = "auto"
	receiver.call(t, "start", map[string]any{"mode": "receive", "code": "3456-unified-send-test", "dataDir": filepath.Join(dir, "receive"), "outputDir": out, "options": receiverOptions})
	receiver.finish(t)
	received, err := os.ReadFile(filepath.Join(out, "same-selection.txt"))
	if err != nil || string(received) != content {
		t.Fatalf("code contents differ: %s %v", received, err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state = sender.call(t, "snapshot", nil)
		transfer = state["transfer"].(map[string]any)
		if transfer["status"] == "completed" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if transfer["status"] != "completed" || state["ready"] != true || state["status"] != "running" {
		t.Fatalf("code delivery stopped persistent share: %+v", state)
	}
	download()
	sender.call(t, "stop", nil)
}
