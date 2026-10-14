//go:build windows

package ai

import (
	"context"
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
	g := NewGateway(ctx, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "served") }))
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
	status, err = g.Save(status.GatewaySettings, true)
	if err != nil {
		t.Fatal(err)
	}
	key, _ = g.Key()
	if previous == key || call(previous, "", "") != 401 || call(key, "", "") != 200 {
		t.Fatal("key reset did not invalidate old key")
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
