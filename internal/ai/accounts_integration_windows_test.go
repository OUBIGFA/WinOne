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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic Claude files use the upstream offline import path. Exported token
// bytes stay in owned files and never appear in the public worker response.
func TestPackagedAccountImportAndBothExportFormats(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	directory := t.TempDir()
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(directory, "runtime")}
	provider := Provider{ID: "account-files", Name: "Fixture", AuthMode: "account", Agent: "claude", AgentBackend: "builtin"}
	const access = "fixture-access-never-a-real-token"
	const refresh = "fixture-refresh-never-a-real-token"
	data := `{"type":"claude","email":"account-files@example.invalid","access_token":"` + access + `","refresh_token":"` + refresh + `"}`
	raw, err := runtime.ImportAccounts(t.Context(), provider, []string{data}, false)
	var imported struct{ Results []struct{ Status string } }
	if err != nil || json.Unmarshal(raw, &imported) != nil || len(imported.Results) != 1 || imported.Results[0].Status != "added" {
		t.Fatal("account import failed", err)
	}
	for _, format := range []string{"sub2api", "cliproxyapi"} {
		path := directory
		if format == "sub2api" {
			path = filepath.Join(directory, "accounts.json")
		}
		raw, err = runtime.ExportAccounts(t.Context(), provider, format, path, []string{"account-files@example.invalid"})
		var exported struct{ Files []string }
		if err != nil || json.Unmarshal(raw, &exported) != nil || len(exported.Files) != 1 {
			t.Fatal("account export failed", format, err)
		}
		if strings.Contains(string(raw), access) || strings.Contains(string(raw), refresh) {
			t.Fatal("exported credentials crossed the public response")
		}
		file, err := os.ReadFile(exported.Files[0])
		if err != nil || !json.Valid(file) || !strings.Contains(string(file), access) || !strings.Contains(string(file), refresh) {
			t.Fatal("exported format lost the account's credentials", format, err)
		}
	}
	other, err := runtime.ForProvider("other-profile").Accounts(t.Context(), "accounts.list", "claude")
	var listing struct{ Accounts []any }
	if err != nil || json.Unmarshal(other, &listing) != nil || len(listing.Accounts) != 0 {
		t.Fatal("import crossed the provider profile", err)
	}
}

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
