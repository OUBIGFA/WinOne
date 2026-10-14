// Copied into the pinned provider package only for its integration tests.
package provider

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWinOneOAuthKeepsAccountInPrivateProfile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData/Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData/Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	token := func(claims map[string]any) string {
		value, _ := json.Marshal(claims)
		return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(value) + ".test-only"
	}
	idToken := token(map[string]any{"email": "winone-test@example.invalid", "exp": time.Now().Add(time.Hour).Unix(), "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "fixture-account", "chatgpt_plan_type": "plus"}})
	access := token(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code") != "winone-test-code" || r.Form.Get("code_verifier") == "" {
			t.Error("OAuth did not carry its code and PKCE verifier")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": idToken, "access_token": access, "refresh_token": "test-only-refresh"})
	}))
	defer endpoint.Close()
	previousToken, previousCallback := codexTokenURL, codexCallbackAddr
	codexTokenURL = endpoint.URL
	codexCallbackAddr = "127.0.0.1:0"
	t.Cleanup(func() { codexTokenURL = previousToken; codexCallbackAddr = previousCallback })
	state, err := StartSignIn("codex")
	if err != nil {
		t.Fatal(err)
	}
	defer CancelSignIn(state.ID)
	location, err := url.Parse(state.URL)
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("code_challenge_method") != "S256" || location.Query().Get("state") == "" {
		t.Fatal("OAuth request lost its security parameters")
	}
	callback := location.Query().Get("redirect_uri") + "?code=winone-test-code&state=" + url.QueryEscape(location.Query().Get("state"))
	if err = SubmitSignInCallback(state.ID, callback); err != nil {
		t.Fatal(err)
	}
	completed, ok := SignInStatus(state.ID)
	if !ok || completed.State != "done" || completed.User != "winone-test@example.invalid" {
		t.Fatalf("sign in failed: %+v", completed)
	}
	if _, err = os.Stat(filepath.Join(home, ".codex", "auth.json")); err != nil {
		t.Fatal("account was not stored in the private agent home", err)
	}
	if accounts := Logins("codex"); len(accounts) != 1 || accounts[0].User != "winone-test@example.invalid" {
		t.Fatalf("account list mismatch: %+v", accounts)
	}
}
