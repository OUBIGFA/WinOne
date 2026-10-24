package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tailscale.com/tstest/integration"
)

func waitReady(t *testing.T, task *task) snapshot {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state := task.read()
		if state.Ready && (task.spec.Mode != "share_server" || state.RelayReady) {
			return state
		}
		if state.Status != "running" {
			t.Fatalf("service failed: %+v", state)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("service did not become ready")
	return snapshot{}
}

// Run the same local DERP/STUN fixture as upstream Tailcat tests. All bytes go
// through the real WireGuard stack and both TCP proxies, without public relays.
func TestTailcatTunnelRoundTripAndRelease(t *testing.T) {
	dm := integration.RunDERPAndSTUN(t, func(string, ...any) {}, "127.0.0.1")
	maps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(dm) }))
	defer maps.Close()
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "_temp", "interconnect-peer-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "tunnel-")
	if err != nil {
		t.Fatal(err)
	}
	server, err := start(specification{Mode: "tunnel_server", Target: echo.Addr().String(), DERPMapURL: maps.URL, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	defer server.stop()
	shared := waitReady(t, server)
	client, err := start(specification{Mode: "tunnel_client", Code: shared.Code, Listen: "127.0.0.1:0", DERPMapURL: maps.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.stop()
	mapped := waitReady(t, client)
	conn, err := net.DialTimeout("tcp", mapped.Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	const payload = "WinOne real Tailcat round trip"
	if _, err = io.WriteString(conn, payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err = io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatal("bytes changed")
	}
	if err = client.stop(); err != nil {
		t.Fatal(err)
	}
	if other, err := net.DialTimeout("tcp", mapped.Address, time.Second); err == nil {
		other.Close()
		t.Fatal("listener survived stop")
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}
	resumed, err := start(specification{Mode: "tunnel_server", Target: echo.Addr().String(), DERPMapURL: maps.URL, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.stop()
	if after := waitReady(t, resumed); after.Code != shared.Code {
		t.Fatal("persistent service address changed")
	}
}
