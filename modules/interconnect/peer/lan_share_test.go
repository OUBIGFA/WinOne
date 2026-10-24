package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A failed public relay must leave the real credential-protected LAN download
// usable, including browser cookies, range requests and live file management.
func TestLANShareSurvivesRelayFailure(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable", true: "timeout"}[timeout], func(t *testing.T) { testLANShareRelayFailure(t, timeout) })
	}
}

func testLANShareRelayFailure(t *testing.T, timeout bool) {
	exe, _ := os.Executable()
	for _, name := range []string{"WinOne.Interconnect.Lan.exe", "WinOne.Interconnect.Transfer.exe"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), name)); err != nil {
			t.Skip("run in the isolated interconnect test bundle")
		}
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", "..", "_temp", "interconnect-peer-tests"))
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "lan-relay-failure-")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "手机下载.txt")
	if err = os.WriteFile(source, []byte("phone download bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if timeout {
			<-r.Context().Done()
			return
		}
		http.Error(w, "relay unavailable", 503)
	}))
	defer relay.Close()
	server, err := start(specification{Mode: "share_server", Paths: []string{source}, DataDir: filepath.Join(dir, "server"), DERPMapURL: relay.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer server.stop()
	var published snapshot
	deadlineReady := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadlineReady) {
		published = server.read()
		if published.Ready || published.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !published.Ready {
		t.Fatalf("LAN share not ready: %+v", published)
	}
	var share struct {
		Info  struct{ URL string }
		Items []struct{ ID string }
	}
	if err = json.Unmarshal(published.Share, &share); err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(share.Info.URL)
	if err != nil || address.Scheme != "http" || address.Query().Get("key") == "" {
		t.Fatalf("not a browser capability URL: %s", share.Info.URL)
	}
	if len(share.Items) != 1 {
		t.Fatal("shared file missing")
	}
	// A machine without LAN adapters may advertise loopback, but the listener
	// must still be reachable through every advertised interface.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	request := func(client *http.Client, path, method string, headers map[string]string, want int) []byte {
		t.Helper()
		req, _ := http.NewRequest(method, path, nil)
		for k, v := range headers {
			if k == "Host" {
				req.Host = v
			} else {
				req.Header.Set(k, v)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s: got %d want %d: %s (%v)", path, response.StatusCode, want, body, err)
		}
		return body
	}
	base := address.Scheme + "://" + address.Host
	anonymous := &http.Client{Timeout: 5 * time.Second}
	request(anonymous, base+"/api/items", "GET", nil, 401)
	body := request(browser, share.Info.URL, "GET", nil, 200)
	if !strings.Contains(string(body), "<html") {
		t.Fatal("QR destination is not a download page")
	}
	download := base + "/api/items/" + share.Items[0].ID + "/download"
	if got := request(browser, download, "GET", nil, 200); string(got) != "phone download bytes" {
		t.Fatalf("wrong file: %s", got)
	}
	if got := request(browser, download, "GET", map[string]string{"Range": "bytes=0-4"}, 206); string(got) != "phone" {
		t.Fatal("range lost")
	}
	request(browser, base+"/api/text", "POST", nil, 403)
	request(browser, base+"/api/items", "GET", map[string]string{"Origin": "https://other.invalid"}, 403)
	request(browser, base+"/api/items", "GET", map[string]string{"Host": "other.invalid"}, 403)
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		state := server.read()
		raw, _ := json.Marshal(state)
		var status struct{ RelayError string }
		json.Unmarshal(raw, &status)
		if status.RelayError != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	state := server.read()
	raw, _ := json.Marshal(state)
	var status struct{ RelayError string }
	json.Unmarshal(raw, &status)
	if !state.Ready || state.Status != "running" || status.RelayError == "" {
		t.Fatalf("relay failure disabled LAN or was hidden: %s", raw)
	}
	request(browser, download, "GET", nil, 200)
	if timeout && !strings.Contains(status.RelayError, "context deadline exceeded") {
		t.Fatalf("missing timeout diagnostic: %s", status.RelayError)
	}
	item, _ := json.Marshal(share.Items[0].ID)
	if err = server.changeShare("remove", item); err != nil {
		t.Fatal(err)
	}
	for server.read().Updating {
		time.Sleep(20 * time.Millisecond)
	}
	request(browser, download, "GET", nil, 404)
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}
	if conn, err := net.DialTimeout("tcp", address.Host, time.Second); err == nil {
		conn.Close()
		t.Fatal("LAN listener survived stop")
	}
}
