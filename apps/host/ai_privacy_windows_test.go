//go:build windows

package main

import (
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
	"winone/internal/ai"
	"winone/internal/host"
)

// No model or account is downloaded. The packaged process and Host own their
// real pipes, capability keys, profiles and shutdown paths in this fixture.
func privacyTestService(t *testing.T) *aiService {
	t.Helper()
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	dir := t.TempDir()
	s := newAIService(ctx, dir, filepath.Join(dir, "WinOne.Host.exe"))
	s.translator.Executable = exe
	s.gatewaySlots = make(chan struct{}, 4)
	t.Cleanup(func() {
		s.closeRouting()
		s.privacy.mu.Lock()
		defer s.privacy.mu.Unlock()
		if s.privacy.process != nil {
			s.privacy.cancel()
			s.privacy.process.Close()
		}
	})
	return s
}

func privacyCall(t *testing.T, s *aiService, method, viewer, action string) ai.PrivacyStatus {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"viewer": viewer, "action": action})
	r := s.handle(host.Request{ID: "privacy-check", Method: "magpie.privacy." + method, Params: body})
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	var state ai.PrivacyStatus
	if err := json.Unmarshal(r.Result, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestPrivacyModelViewingDoesNotDownloadAndLeasesAreIndependent(t *testing.T) {
	s := privacyTestService(t)
	url, key, err := s.privacyEndpoint(s.ctx)
	if err != nil || url != "" || key != "" || s.privacy.process != nil {
		t.Fatalf("off preference started the model: %q %v", url, err)
	}
	state := privacyCall(t, s, "open", "first-viewer", "")
	if state.On || state.Downloading || state.Installed || state.Running || state.Size <= 0 {
		t.Fatalf("opening the page changed the model: %+v", state)
	}
	process := s.privacy.process
	privacyCall(t, s, "open", "second-viewer", "")
	if process != s.privacy.process {
		t.Fatal("each viewer started its own model process")
	}
	// Replacing routing after a settings save must not close the model page.
	if _, err := s.startRouting(); err != nil {
		t.Fatal(err)
	}
	s.closeRouting()
	if !process.Alive() {
		t.Fatal("routing restart stopped the independent privacy service")
	}
	privacyCall(t, s, "close", "first-viewer", "")
	if !process.Alive() {
		t.Fatal("one viewer released another viewer's process")
	}
	privacyCall(t, s, "close", "second-viewer", "")
	if process.Alive() || s.privacy.process != nil {
		t.Fatal("unused off-state privacy process was retained")
	}
	if enabled, err := s.translator.PrivacyEnabled(); err != nil || enabled {
		t.Fatalf("viewing saved an opt-in: %v %v", enabled, err)
	}
}

func TestPrivacyModelRejectsInvalidActionsBeforeStarting(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), "missing-host.exe")
	for _, input := range []struct{ method, body string }{
		{"open", `{"viewer":"../escape"}`},
		{"state", `{"viewer":"ok","action":"on"}`},
		{"action", `{"viewer":"ok","action":"download-anything"}`},
		{"action", `{"viewer":"ok","action":""}`},
		{"unknown", `{"viewer":"ok"}`},
		{"open", `{"viewer":"ok","url":"https://example.invalid"}`},
	} {
		r := s.handle(host.Request{ID: "invalid", Method: "magpie.privacy." + input.method, Params: json.RawMessage(input.body)})
		if r.Error == nil || s.privacy.process != nil || len(s.privacy.viewers) != 0 {
			t.Fatalf("invalid operation acquired a process/lease: %+v %s", r, input.body)
		}
	}
}

// The preference file simulates a completed, user-requested installation. No
// weights are present: opting in must then fail closed before the vendor sees
// another request, including when the router was already running while off.
func TestPrivacyModelToggleReachesAnAlreadyRunningRouter(t *testing.T) {
	s := privacyTestService(t)
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	store, err := s.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.RoutingEnabled = true
	configuration.MagpieSettings = &ai.MagpieSettings{Redact: true, RedactPersonal: true, NoStats: true, NoUsageStats: true}
	configuration.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}}}
	credential := "fixture-key"
	if _, err := store.SaveConfiguration(configuration, map[string]*string{"fixture": &credential}); err != nil {
		t.Fatal(err)
	}
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture/model","messages":[{"role":"user","content":"Meet Ada Cedar tomorrow"}]}`)).WithContext(s.ctx)
		s.serveChat(w, r)
		return w
	}
	if result := call(); result.Code != 200 || calls.Load() != 1 {
		t.Fatalf("off-state fixture failed: %d %s", result.Code, result.Body.String())
	}
	router := s.routing.process
	path := filepath.Join(s.translator.DataDirectory, "privacy", "config", "magpie", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"redactModel":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if result := call(); result.Code < 500 || calls.Load() != 1 {
		t.Fatalf("enabled model was bypassed by the cached router: status=%d calls=%d body=%s", result.Code, calls.Load(), result.Body.String())
	}
	privacyCall(t, s, "action", "toggle-viewer", "off")
	if result := call(); result.Code != 200 || calls.Load() != 2 {
		t.Fatalf("turning the model off left routing blocked: %d %s", result.Code, result.Body.String())
	}
	if s.routing.process != router || !router.Alive() {
		t.Fatal("privacy toggle restarted routing and invalidated its page/draft")
	}
	privacyCall(t, s, "close", "toggle-viewer", "")
}
