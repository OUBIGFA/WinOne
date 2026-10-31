package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"winone/desktopmygo/core"
)

// The bridge takes a page's message only with its surface's secret and
// only of the types the surface accepts; heights size the surface and the
// rest reach Receive on the UI thread.
func TestServeCarriesOnlyAcceptedMessagesOfItsSurface(t *testing.T) {
	var posted []func()
	env := &core.Env{Post: func(fn func()) { posted = append(posted, fn) }}
	var received []Message
	s := New(env, Options{URL: "http://127.0.0.1:9/page", Accept: []string{"winone-plugin-height", "winone-plugin-native"},
		HeightType: "winone-plugin-height", Height: 60, Receive: func(m Message) { received = append(received, m) }})
	s.secret = "secret"
	surfaces.Lock()
	surfaces.bySecret["secret"] = entry{surface: s}
	surfaces.Unlock()
	t.Cleanup(func() { surfaces.Lock(); delete(surfaces.bySecret, "secret"); surfaces.Unlock() })

	post := func(path, body string) int {
		w := httptest.NewRecorder()
		serve(w, httptest.NewRequest(http.MethodPost, "http://winone-web.localhost"+path, strings.NewReader(body)))
		return w.Code
	}
	if code := post("/post/other", `{"type":"winone-plugin-native"}`); code != http.StatusNotFound {
		t.Fatalf("another secret: %d", code)
	}
	if code := post("/post/secret", `{"type":"winone-routing-masking"}`); code != http.StatusBadRequest {
		t.Fatalf("an unaccepted type: %d", code)
	}
	if code := post("/post/secret", `{"type":"winone-plugin-height","height":420}`); code != http.StatusNoContent {
		t.Fatalf("a height: %d", code)
	}
	if code := post("/post/secret", `{"type":"winone-plugin-native","id":"1","method":"choose"}`); code != http.StatusNoContent {
		t.Fatalf("a request: %d", code)
	}
	for _, fn := range posted {
		fn()
	}
	if s.height != 420 || len(received) != 1 || received[0].String("method") != "choose" {
		t.Fatalf("height %v, received %v", s.height, received)
	}
}

// The script posts to its own secret's address, from the page's origin
// only, and ignores what frames inside the page post.
func TestScriptNamesItsOriginAndSecret(t *testing.T) {
	s := New(&core.Env{}, Options{URL: "http://127.0.0.1:9/page", Accept: []string{"winone-plugin-height"}})
	s.origin, s.secret = "http://127.0.0.1:9", "abc"
	script := s.script()
	for _, want := range []string{`"http://127.0.0.1:9"`, `"http://winone-web.localhost/post/abc"`, "event.source !== window", `["winone-plugin-height"]`} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %s:\n%s", want, script)
		}
	}
}
