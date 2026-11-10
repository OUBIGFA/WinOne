package flashchat

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeBridgeRejectsForeignRequestsAndUnselectedFiles(t *testing.T) {
	b := newNativeBridge("http://127.0.0.1:19000")
	path := filepath.Join(t.TempDir(), "selected.txt")
	if err := os.WriteFile(path, []byte("selected content"), 0600); err != nil {
		t.Fatal(err)
	}
	b.files["selected"] = path
	for _, tc := range []struct {
		host, token, origin, path string
		status                    int
	}{
		{"127.0.0.1:19000", "", "", "/native/file/selected", 403},
		{"attacker.test", b.token, "", "/native/file/selected", 403},
		{"127.0.0.1:19000", b.token, "https://attacker.test", "/native/file/selected", 403},
		{"127.0.0.1:19000", b.token, b.origin, "/native/file/missing", 404},
		{"127.0.0.1:19000", b.token, b.origin, "/native/file/selected", 200},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("X-WinOne-Token", tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		b.serve(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
		if tc.status == 200 && w.Body.String() != "selected content" {
			t.Fatal(w.Body.String())
		}
	}
	var received nativeSnapshot
	b.state = func(state nativeSnapshot) { received = state }
	r := httptest.NewRequest("POST", b.origin+"/native/state", strings.NewReader(`{"revision":2,"ready":true,"roomId":"room"}`))
	r.Header.Set("X-WinOne-Token", b.token)
	w := httptest.NewRecorder()
	b.serve(w, r)
	if w.Code != 200 || received.RoomID != "room" || received.Revision != 2 {
		t.Fatal(w.Code, received)
	}
}
