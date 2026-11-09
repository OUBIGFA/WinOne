package flashchat

import (
	"github.com/egoist/mygo"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCompleteAppResourcesAndRoutes(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("complete app")}, "streamsaver/sw.js": {Data: []byte("self.onfetch = null")}}
	handler := assetHandler(assets, "127.0.0.1:19000")
	for _, tc := range []struct {
		method, host, path string
		status             int
		body               string
	}{
		{"GET", "127.0.0.1:19000", "/private/test", 200, "complete app"},
		{"GET", "127.0.0.1:19000", "/streamsaver/sw.js", 200, "self.onfetch = null"},
		{"GET", "127.0.0.1:19000", "/missing.js", 404, ""},
		{"GET", "attacker.test", "/", 403, ""},
		{"POST", "127.0.0.1:19000", "/", 403, ""},
		{"GET", "127.0.0.1:19000", "/../secrets", 404, ""},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || (tc.body != "" && w.Body.String() != tc.body) {
			t.Fatalf("%s: %d %q", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestCompleteAppOriginPersistsAndCloseReleasesPort(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("complete app")}}
	dir := t.TempDir()
	first, err := startLocalApp(dir, assets)
	if err != nil {
		t.Fatal(err)
	}
	origin := first.origin
	response, err := http.Get(origin + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "complete app" {
		t.Fatal(string(body))
	}
	if occupied, err := startLocalApp(dir, assets); err == nil {
		occupied.close()
		t.Fatal("attached to occupied port")
	}
	first.close()
	second, err := startLocalApp(dir, assets)
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
	if second.origin != origin {
		t.Fatal("settings origin changed")
	}
	if !strings.HasPrefix(origin, "http://127.0.0.1:") {
		t.Fatal("listener exposed outside loopback")
	}
}

func TestCompleteAppPermissionBoundary(t *testing.T) {
	origin := "http://127.0.0.1:19000"
	for _, tc := range []struct {
		origin  string
		kind    mygo.Permission
		allowed bool
	}{
		{origin, mygo.PermissionCamera, true}, {origin, mygo.PermissionMicrophone, true}, {origin, mygo.PermissionNotifications, true},
		{origin, mygo.PermissionGeolocation, false}, {"https://example.com", mygo.PermissionCamera, false},
		{"http://127.0.0.1:19001", mygo.PermissionCamera, false},
	} {
		if appPermission(origin, mygo.PermissionRequest{Origin: tc.origin, Permissions: []mygo.Permission{tc.kind}}) != tc.allowed {
			t.Fatal(tc)
		}
	}
}
