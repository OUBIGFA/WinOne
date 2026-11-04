package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCloudProxyPrecedenceAndBypass(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://environment.invalid:8080")
	t.Setenv("NO_PROXY", "bypass.example")
	request := func(host string) *http.Request { return &http.Request{URL: &url.URL{Scheme: "https", Host: host}} }
	spec := defaults()
	spec.Socks5, spec.HTTPProxy = "127.0.0.1:1080", "127.0.0.1:8081"
	choose, err := transferProxy(spec)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := choose(request("api.gofile.io"))
	if got.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("SOCKS5 precedence: %v", got)
	}
	spec.Socks5 = ""
	choose, _ = transferProxy(spec)
	got, _ = choose(request("api.gofile.io"))
	if got.String() != "http://127.0.0.1:8081" {
		t.Fatalf("HTTP precedence: %v", got)
	}
	spec.HTTPProxy = ""
	choose, _ = transferProxy(spec)
	got, _ = choose(request("api.gofile.io"))
	if got.String() != "http://environment.invalid:8080" {
		t.Fatalf("environment precedence: %v", got)
	}
	if got, _ = choose(request("bypass.example")); got != nil {
		t.Fatalf("NO_PROXY fell through to system: %v", got)
	}
}

func TestCloudTransportUsesExplicitHTTPProxy(t *testing.T) {
	var destination string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destination = r.URL.String()
		io.WriteString(w, "proxy response")
	}))
	defer server.Close()
	spec := defaults()
	spec.HTTPProxy = server.URL
	client, err := newTransferHTTP(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://cloud.example/upload")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if destination != "http://cloud.example/upload" {
		t.Fatalf("proxy got %q", destination)
	}
}

func TestCloudTransportNeverDisablesCertificateValidation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS reached HTTP") }))
	defer server.Close()
	client, err := newTransferHTTP(specification{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader("must not upload"))
	if _, err = client.Do(request); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
}

func TestCloudTransportDoesNotReplayPOSTAfterServerCloses(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.Copy(io.Discard, r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	client, err := newTransferHTTP(specification{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	_, err = client.Post(server.URL, "application/json", strings.NewReader("{\"create\":true}"))
	if err == nil || calls != 1 {
		t.Fatalf("POST err=%v calls=%d", err, calls)
	}
}
