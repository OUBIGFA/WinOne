package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A share exposes only the FileShare client. Its internal capability must never
// escape into the browser or permit writes when the sender chose read-only.
func TestShareProxyBoundary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-key" {
			t.Error("missing upstream capability")
		}
		if r.Header.Get("Origin") != "http://"+r.Host {
			t.Error("incorrect upstream origin")
		}
		if r.URL.Path == "/api/items/1/download" && r.Header.Get("Range") != "bytes=1-3" {
			t.Error("Range lost")
		}
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	proxy, err := shareHandler(context.Background(), upstream.URL+"/?key=private-key", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "http://peer/api/upload", nil)
		proxy.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("write allowed: %s %d", method, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://peer/api/items/1/download", nil)
	req.Header.Set("Range", "bytes=1-3")
	req.Header.Set("Origin", "http://peer")
	proxy.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	if rec.Code != 200 || string(body) != "ok" {
		t.Fatalf("read failed: %d %s", rec.Code, body)
	}
	for _, bad := range []string{"https://example.com/?key=x", "http://192.168.1.1/?key=x", "http://127.0.0.1/"} {
		if _, err := shareHandler(context.Background(), bad, false); err == nil {
			t.Fatalf("accepted invalid target: %s", bad)
		}
	}
}
