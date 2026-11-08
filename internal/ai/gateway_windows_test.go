//go:build windows

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGatewayLifecycleAuthenticationAndPersistence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "gateway.json")
	var callerIDs []string
	g := NewGateway(ctx, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callerIDs = append(callerIDs, GatewayCallerKeyID(r.Context()))
		_, _ = io.WriteString(w, "served")
	}))
	defer g.Close()
	if g.Status().Running || g.Status().Enabled {
		t.Fatal("new gateway started")
	}
	available, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := available.Addr().(*net.TCPAddr).Port
	available.Close()
	status, err := g.Save(GatewaySettings{Enabled: true, Port: port}, false)
	if err != nil {
		t.Fatal(err)
	}
	key, err := g.Key()
	if err != nil || len(key) < 64 {
		t.Fatalf("key unavailable: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), key) {
		t.Fatal("key stored in plaintext")
	}
	call := func(token, origin, host string) int {
		t.Helper()
		r, _ := http.NewRequest("GET", status.BaseURL+"/models", nil)
		if host != "" {
			r.Host = host
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", origin)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if call("bad", "", "") != 401 || call(key, "", "") != 200 || call(key, "http://example.com", "") != 403 || call(key, "", "example.com:1234") != 403 {
		t.Fatal("gateway access boundary failed")
	}
	previous := key
	wantID := sha256.Sum256([]byte(key))
	if len(callerIDs) != 1 || callerIDs[0] != hex.EncodeToString(wantID[:]) {
		t.Fatalf("authenticated key identity missing: %v", callerIDs)
	}
	status, err = g.Save(status.GatewaySettings, true)
	if err != nil {
		t.Fatal(err)
	}
	key, _ = g.Key()
	if previous == key || call(previous, "", "") != 401 || call(key, "", "") != 200 {
		t.Fatal("key reset did not invalidate old key")
	}
	if len(callerIDs) != 2 || callerIDs[0] == callerIDs[1] {
		t.Fatal("rotated key kept previous usage identity")
	}
	g.Close()
	reopened := NewGateway(ctx, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer reopened.Close()
	if !reopened.Status().Running {
		t.Fatalf("saved gateway did not restart: %+v", reopened.Status())
	}
	if restored, _ := reopened.Key(); restored != key {
		t.Fatal("key changed after restart")
	}
	value := reopened.Status().GatewaySettings
	value.Enabled = false
	if _, err = reopened.Save(value, false); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal("port not released", err)
	}
	listener.Close()
}

func TestGatewayBusyPortDoesNotCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := NewGateway(ctx, filepath.Join(t.TempDir(), "gateway.json"), http.NotFoundHandler())
	defer g.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, err = g.Save(GatewaySettings{Enabled: true, Port: listener.Addr().(*net.TCPAddr).Port}, false)
	if err == nil || g.Status().Enabled || g.Status().Revision != 0 {
		t.Fatal("busy port committed")
	}
}

func TestGatewayBrowserAuthenticationAndNetworkRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	g := NewGateway(ctx, filepath.Join(t.TempDir(), "gateway.json"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Origin") != "" {
			t.Error("public origin leaked to private worker")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" || r.Header.Get("X-Goog-API-Key") != "" || r.URL.Query().Has("key") {
			t.Error("public credential leaked to private worker")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer g.Close()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	status, err := g.Save(GatewaySettings{Enabled: true, Port: port, GatewayNetwork: GatewayNetwork{CORSOrigins: []string{"https://chat.example"}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	key, err := g.Key()
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, origin, token string, want int) {
		t.Helper()
		r, _ := http.NewRequest(method, status.BaseURL+"/models", nil)
		r.Header.Set("Origin", origin)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if method == "OPTIONS" {
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s: status %d, want %d", method, response.StatusCode, want)
		}
		if origin == "https://chat.example" && response.Header.Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("allowed browser cannot read response")
		}
	}
	call("OPTIONS", "https://chat.example", "", 204)
	call("GET", "https://chat.example", "", 401)
	call("GET", "https://chat.example.evil", key, 403)
	call("GET", "https://chat.example", key, 200)
	if calls != 1 {
		t.Fatalf("unauthenticated request reached handler: %d calls", calls)
	}
	value := status.GatewaySettings
	value.LAN = true
	status, err = g.Save(value, false)
	if err != nil || !status.Running || !status.LAN {
		t.Fatalf("network rebind: %v %+v", err, status)
	}
	call("GET", "https://chat.example", key, 200)
	// A directory cannot become the settings file. Force a real persistence
	// failure after rebinding and verify the last saved listener is restored.
	g.path = t.TempDir()
	value = status.GatewaySettings
	value.LAN = false
	failed, err := g.Save(value, false)
	if err == nil || failed.Revision != status.Revision || !failed.Running || !failed.LAN {
		t.Fatalf("failed save lost active listener: %v %+v", err, failed)
	}
	call("GET", "https://chat.example", key, 200)
}
