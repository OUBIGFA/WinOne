package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"tailscale.com/tstest/integration"
	"testing"
	"time"
)

func TestLiveShareRoundTrip(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"WinOne.Interconnect.Lan.exe", "WinOne.Interconnect.Transfer.exe"} {
		if _, err = os.Stat(filepath.Join(filepath.Dir(exe), name)); err != nil {
			t.Skip("compile this test executable into the interconnect build bundle to exercise child processes")
		}
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "_temp", "interconnect-peer-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "live-share-")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "small.txt")
	if err = os.WriteFile(source, []byte("cached bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	dm := integration.RunDERPAndSTUN(t, func(string, ...any) {}, "127.0.0.1")
	maps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(dm) }))
	defer maps.Close()
	spec := specification{Mode: "share_server", Paths: []string{source}, DataDir: filepath.Join(dir, "server"), DERPMapURL: maps.URL}
	server, err := start(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer server.stop()
	published := waitReady(t, server)
	for _, listen := range []string{"0.0.0.0:0", "[::1]:0", "localhost:0"} {
		if unexpected, err := start(specification{Mode: "share_client", Code: published.Code, Listen: listen}); err == nil {
			unexpected.stop()
			t.Fatalf("share gateway accepted non-CSP origin: %s", listen)
		}
	}
	client, err := start(specification{Mode: "share_client", Code: published.Code, DERPMapURL: maps.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.stop()
	gateway := waitReady(t, client)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	request := func(path, method string, body []byte, headers map[string]string, status int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, "http://"+gateway.Address+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range headers {
			if key == "Host" {
				req.Host = value
			} else {
				req.Header.Set(key, value)
			}
		}
		response, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("%s: status=%d error=%v body=%s", path, response.StatusCode, err, data)
		}
		return data
	}
	var files struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
	}
	readItems := func() {
		t.Helper()
		if err := json.Unmarshal(request("/api/items", "GET", nil, nil, 200), &files.Items); err != nil {
			t.Fatal(err)
		}
	}
	readItems()
	if len(files.Items) != 1 {
		t.Fatalf("unexpected list: %+v", files)
	}
	download := "/api/items/" + files.Items[0].ID + "/download"
	if err = os.WriteFile(source, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := request(download, "GET", nil, nil, 200); string(got) != "cached bytes" {
		t.Fatal("small file cache was not immutable")
	}
	if got := request(download, "GET", nil, map[string]string{"Range": "bytes=1-3"}, 206); string(got) != "ach" {
		t.Fatal("range bytes changed")
	}
	request("/api/text", "POST", []byte(`{"content":"blocked"}`), map[string]string{"Content-Type": "application/json"}, 403)
	request("/api/items", "GET", nil, map[string]string{"Host": "other.invalid"}, 403)
	request("/api/items", "GET", nil, map[string]string{"Origin": "https://other.invalid"}, 403)
	added := filepath.Join(dir, "append.txt")
	if err = os.WriteFile(added, []byte("appended"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, _ := json.Marshal([]string{added})
	if err = server.changeShare("add", paths); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for server.read().Updating && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	readItems()
	if len(files.Items) != 2 {
		t.Fatal("append did not publish")
	}
	item, _ := json.Marshal(files.Items[0].ID)
	if err = server.changeShare("remove", item); err != nil {
		t.Fatal(err)
	}
	for server.read().Updating && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	readItems()
	if len(files.Items) != 1 {
		t.Fatal("remove did not update pool")
	}
	if err = client.stop(); err != nil {
		t.Fatal(err)
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}
	spec.Resume = true
	spec.AllowWrites = true
	resumed, err := start(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.stop()
	if state := waitReady(t, resumed); state.Code != published.Code {
		t.Fatal("resumed address changed")
	}
	connected, err := start(specification{Mode: "share_client", Code: published.Code, DERPMapURL: maps.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer connected.stop()
	gateway = waitReady(t, connected)
	request("/api/text", "POST", []byte(`{"content":"received text"}`), map[string]string{"Content-Type": "application/json", "Origin": "http://" + gateway.Address}, 200)
	readItems()
	if len(files.Items) != 2 {
		t.Fatal("resumed pool or write was lost")
	}
	if err = resumed.lan.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-resumed.lan.done
	if state := resumed.read(); state.Ready || state.Status != "failed" {
		t.Fatalf("dead source advertised ready: %+v", state)
	}
}
