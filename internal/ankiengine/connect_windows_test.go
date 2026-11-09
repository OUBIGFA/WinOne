package ankiengine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the handler directly: these tests never bind a socket or request
// Windows network permission, including the disallowed-origin path.
func TestConnectRejectsUnknownOriginsAndKeysBeforeDispatch(t *testing.T) {
	c := &Connect{key: "test-key", settings: ConnectSettings{Port: 8765, Origins: []string{"chrome-extension://example"}}}
	for _, test := range []struct {
		origin, body string
		want         int
	}{
		{"https://untrusted.invalid", `{"action":"version","key":"test-key"}`, 403},
		{"chrome-extension://example", `{"action":"version","key":"wrong"}`, 401},
		{"", `{"action":"version"}`, 401},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/", strings.NewReader(test.body))
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		c.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("status=%d want=%d", response.Code, test.want)
		}
	}
	if err := validateConnect(ConnectSettings{Port: 8765, Origins: []string{"*"}}); err == nil {
		t.Fatal("wildcard origins accepted")
	}
}

func TestConnectAuthenticatedHandlersUseActualAnkiWithoutListener(t *testing.T) {
	python, library := os.Getenv("WINONE_TEST_ANKI_PYTHON"), os.Getenv("WINONE_TEST_ANKI_LIBRARY")
	if python == "" || library == "" {
		t.Skip("explicit offline bundled runtime")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	e := New(Options{Python: python, Library: library, Worker: ankiTestPath("WINONE_TEST_ANKI_WORKER", filepath.Join(root, "modules/anki/worker.py")), Source: ankiTestPath("WINONE_TEST_ANKI_SOURCE", filepath.Join(root, "third_party/anki-connect/plugin")), Profile: t.TempDir()})
	defer e.Close()
	c := &Connect{engine: e, key: "test-key", settings: ConnectSettings{Port: 8765, Origins: []string{}}}
	for _, test := range []struct {
		path, body string
		want       string
	}{
		{"/", `{"action":"version","version":6,"key":"test-key"}`, `{"result":6,"error":null}`},
		{"/", `{"action":"version","version":2,"key":"test-key"}`, `6`},
		{"/api/v1", `{"jsonrpc":"2.0","id":1,"method":"collection.get","params":{}}`, ``},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765"+test.path, strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer test-key")
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var got any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if test.want != "" {
			var want any
			_ = json.Unmarshal([]byte(test.want), &want)
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				t.Fatalf("got %s want %s", a, b)
			}
		} else if strings.Contains(w.Body.String(), `"error"`) {
			t.Fatalf("RPC failed: %s", w.Body.String())
		}
	}
}
