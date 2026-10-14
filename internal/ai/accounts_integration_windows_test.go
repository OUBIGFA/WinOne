//go:build windows

package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic credentials stay in two owned profile homes. This exercises the
// real magpie account discovery without signing in or contacting a vendor.
func TestSeparateProfilesDiscoverOnlyTheirOwnAccount(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	runtime := Translator{Executable: exe, DataDirectory: t.TempDir()}
	jwt := func(value any) string {
		data, _ := json.Marshal(value)
		return "e30." + base64.RawURLEncoding.EncodeToString(data) + ".fixture"
	}
	for _, id := range []string{"personal", "work"} {
		scoped := runtime.ForProvider(id)
		path := filepath.Join(scoped.DataDirectory, "home", ".codex", "auth.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"auth_mode": "chatgpt", "tokens": map[string]any{"id_token": jwt(map[string]any{"email": id + "@example.invalid", "https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "plus"}}), "access_token": jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}), "refresh_token": "fixture-" + id, "account_id": "fixture-" + id}})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"personal", "work", "personal"} {
		raw, err := runtime.ForProvider(id).Accounts(context.Background(), "accounts.list", "codex")
		if err != nil {
			t.Fatal(err)
		}
		var value struct {
			Accounts []struct {
				User string `json:"user"`
			} `json:"accounts"`
		}
		if err = json.Unmarshal(raw, &value); err != nil || len(value.Accounts) != 1 || value.Accounts[0].User != id+"@example.invalid" {
			t.Fatalf("account isolation failed for %s: %v", id, err)
		}
	}
}

func TestAccountSignInDoesNotCancelAnotherLogin(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		t.Skip("another login already owns the callback port")
	}
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	server.Listener = listener
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runtime := Translator{Executable: exe, DataDirectory: t.TempDir()}
	err = runtime.SignIn(ctx, "codex", make(chan string), func(json.RawMessage) {})
	if err == nil {
		t.Fatal("occupied callback port accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("starting login contacted/canceled the other application's login")
	}
}

func TestAccountSignInCancelReleasesCallbackAndWorker(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		t.Skip("another login owns the callback port")
	}
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	runtime := Translator{Executable: exe, DataDirectory: t.TempDir()}
	seen := false
	err = runtime.SignIn(ctx, "codex", make(chan string), func(raw json.RawMessage) {
		var event struct {
			SignIn *struct{ State, URL string } `json:"signin"`
		}
		if json.Unmarshal(raw, &event) != nil || event.SignIn == nil || event.SignIn.State != "waiting" {
			return
		}
		parsed, parseErr := url.Parse(event.SignIn.URL)
		if parseErr != nil || parsed.Query().Get("code_challenge_method") != "S256" {
			t.Error("OAuth URL lost PKCE")
		}
		seen = true
		cancel()
	})
	if !seen || err == nil {
		t.Fatal("sign-in cancellation was not exercised", err)
	}
	listener, err = net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		t.Fatal("OAuth callback listener survived cancellation", err)
	}
	listener.Close()
}
