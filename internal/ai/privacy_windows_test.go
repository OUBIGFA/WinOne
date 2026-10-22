//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivacyOptInUsesRootProfileAndRejectsCorruptState(t *testing.T) {
	root := t.TempDir()
	runtime := Translator{DataDirectory: filepath.Join(root, "providers", "one"), RootDataDirectory: root}
	if on, err := runtime.PrivacyEnabled(); on || err != nil {
		t.Fatalf("missing state is not off: %v %v", on, err)
	}
	path := filepath.Join(root, "privacy", "config", "magpie", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		body      string
		on, valid bool
	}{{`{"redactModel":true}`, true, true}, {`{"redactModel":false}`, false, true}, {`{}`, false, true}, {`null`, false, false}, {`[]`, false, false}, {`{"redactModel":"true"}`, false, false}, {`{`, false, false}} {
		if err := os.WriteFile(path, []byte(c.body), 0600); err != nil {
			t.Fatal(err)
		}
		on, err := runtime.PrivacyEnabled()
		if (err == nil) != c.valid || on != c.on {
			t.Fatalf("%s: %v %v", c.body, on, err)
		}
	}
}

// A controlled local model service exercises the real remote masking adapter.
// This proves transport/restoration and fail-closed behavior, not model quality.
func TestPackagedPrivacyRemoteMasksAndFailsClosed(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	const name = "Ada Cedar"
	const privateKey = "fixture-private-key-at-least-32-characters"
	var mode, masks, calls atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		masks.Add(1)
		if r.URL.Path != "/api/privacy-model/find" || r.Header.Get("Authorization") != "Bearer "+privateKey {
			t.Error("private service authentication was lost")
			w.WriteHeader(401)
			return
		}
		if mode.Load() == 1 {
			w.WriteHeader(503)
			return
		}
		if mode.Load() == 2 {
			io.WriteString(w, `[[{"Start":-1,"End":500,"Label":"private_person"}]]`)
			return
		}
		var body struct{ Texts []string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		spans := make([][]map[string]any, len(body.Texts))
		for i, text := range body.Texts {
			if at := strings.Index(text, name); at >= 0 {
				spans[i] = []map[string]any{{"Start": at, "End": at + len(name), "Label": "private_person"}}
			}
		}
		_ = json.NewEncoder(w).Encode(spans)
	}))
	defer private.Close()
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) == 0 {
			t.Errorf("invalid vendor request: %v", err)
			w.WriteHeader(400)
			return
		}
		text := body.Messages[len(body.Messages)-1].Content
		if strings.Contains(text, name) {
			t.Error("unmasked name reached vendor")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": text}, "finish_reason": "stop"}}})
	}))
	defer vendor.Close()
	runtime := Translator{Executable: exe, DataDirectory: t.TempDir(), PrivacyService: func(context.Context) (string, string, error) {
		return private.URL + "/api/privacy-model/find", privateKey, nil
	}}
	prefs := Preferences{Engine: "llm", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Model: "model", MagpieSettings: &MagpieSettings{Redact: true, RedactPersonal: true, NoStats: true, NoUsageStats: true}}
	body := []byte(`{"model":"model","messages":[{"role":"user","content":"Meet Ada Cedar tomorrow"}]}`)
	for _, state := range []int32{0, 1, 2} {
		mode.Store(state)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		out := httptest.NewRecorder()
		_, err := runtime.GatewayRequest(ctx, prefs, "fixture-key", "/v1/chat/completions", body, nil, out)
		cancel()
		if state == 0 && (err != nil || !strings.Contains(out.Body.String(), name)) {
			t.Fatalf("masked response not restored: %v %s", err, out.Body.String())
		}
		if state != 0 && err == nil {
			t.Fatalf("failed/invalid model response was accepted: mode=%d", state)
		}
		if calls.Load() != 1 {
			t.Fatalf("failed model response fell back to an unmasked call: %d", calls.Load())
		}
	}
	if masks.Load() != 3 {
		t.Fatalf("workers did not share the private endpoint: %d", masks.Load())
	}
}

func TestPackagedPrivacySurfaceRejectsForeignRoutesAndBrowserWrites(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	process, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).StartPrivacy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	for _, c := range []struct {
		method, path, auth, origin, body string
		status                           int
	}{
		{"GET", "/api/privacy-model", "", "", "", 401},
		{"GET", "/api/privacy-model", process.Key, "", "", 200},
		{"POST", "/api/privacy-model", process.Key, "https://example.invalid", `{"do":"off"}`, 403},
		{"POST", "/api/privacy-model/find", process.Key, "", `{"texts":["fixture"]}`, 409},
		{"GET", "/api/groups", process.Key, "", "", 404},
		{"GET", "/api/plugins", process.Key, "", "", 404},
		{"GET", "/v1/models", process.Key, "", "", 404},
		{"GET", "/api/tags", process.Key, "", "", 404},
	} {
		r, _ := http.NewRequestWithContext(ctx, c.method, process.URL+c.path, strings.NewReader(c.body))
		if c.auth != "" {
			r.Header.Set("Authorization", "Bearer "+c.auth)
		}
		r.Header.Set("Origin", c.origin)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != c.status {
			t.Fatalf("%s %s: %d, want %d", c.method, c.path, response.StatusCode, c.status)
		}
	}
}

func TestPackagedPrivacyBindingIsHostOnlyAndRejectsInvalidTargets(t *testing.T) {
	router, ctx := startUpdateRouter(t, nil, nil)
	for _, c := range []struct {
		path, auth, origin, body string
		status                   int
	}{
		{"/api/winone/privacy", router.Key, "", `{"url":"","key":""}`, 204},
		{"/session/" + router.Key + "/api/winone/privacy", "", "", `{"url":"","key":""}`, 401},
		{"/api/winone/privacy", router.Key, "https://example.invalid", `{"url":"","key":""}`, 401},
		{"/api/winone/privacy", router.Key, "", `{"url":"http://example.invalid:80/api/privacy-model/find","key":"fixture-private-key-at-least-32-characters"}`, 400},
		{"/api/winone/privacy", router.Key, "", `{"url":"","key":""}{}`, 400},
	} {
		r, _ := http.NewRequestWithContext(ctx, "POST", router.URL+c.path, bytes.NewBufferString(c.body))
		if c.auth != "" {
			r.Header.Set("Authorization", "Bearer "+c.auth)
		}
		r.Header.Set("Origin", c.origin)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != c.status {
			t.Fatalf("binding %s: %d, want %d", c.path, response.StatusCode, c.status)
		}
	}
}
