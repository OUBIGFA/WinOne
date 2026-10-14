//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Opt-in because this links an isolated worker from the pinned, prepared
// source. Only CodexBase changes, to this test's HTTP server. No real account,
// installed application, credential store or external endpoint is used.
func TestAccountQuotaWithRealWorkerAndLocalVendor(t *testing.T) {
	source, compiler := os.Getenv("WINONE_AI_QUOTA_SOURCE"), os.Getenv("WINONE_GO_EXE")
	if source == "" || compiler == "" {
		t.Skip("explicit prepared Magpie source and Go compiler required")
	}
	var failing atomic.Bool
	var calls atomic.Int32
	tokenClaims, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
	token := "e30." + base64.RawURLEncoding.EncodeToString(tokenClaims) + ".fixture"
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("private fixture credential did not reach the configured vendor")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/models" {
			fmt.Fprint(w, `{"models":[]}`)
			return
		}
		if r.URL.Path != "/wham/usage" {
			t.Errorf("unexpected quota route: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		if failing.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"private vendor failure"}`)
			return
		}
		fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_after_seconds":18000}}}`)
	}))
	defer vendor.Close()
	executable := filepath.Join(t.TempDir(), "WinOne.AI.Quota.Test.exe")
	command := exec.Command(compiler, "build", "-trimpath", "-ldflags", "-s -w -X github.com/yetone/magpie/internal/provider.CodexBase="+vendor.URL, "-o", executable, "./cmd/winone-ai")
	command.Dir = source
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("link private quota worker: %v\n%s", err, output)
	}
	s := queuedAccountService(t, context.Background())
	s.translator.Executable = executable
	profile := s.translator.ForProvider("account")
	path := filepath.Join(profile.DataDirectory, "home", ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal(map[string]any{"email": "quota@example.invalid", "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "fixture-account", "chatgpt_plan_type": "plus"}})
	auth, _ := json.Marshal(map[string]any{"auth_mode": "chatgpt", "tokens": map[string]string{"access_token": token, "id_token": "e30." + base64.RawURLEncoding.EncodeToString(identity) + ".fixture", "account_id": "fixture-account"}})
	if err := os.WriteFile(path, auth, 0600); err != nil {
		t.Fatal(err)
	}
	gate := s.accountGateFor("account")
	gate <- struct{}{}
	release := time.AfterFunc(50*time.Millisecond, func() { <-gate })
	defer release.Stop()
	response := s.handleAccounts(quotaRequest())
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var result struct {
		Quotas []struct {
			Error   string
			AsOf    string
			Windows []struct{ Used float64 }
		}
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(result.Quotas) != 1 || len(result.Quotas[0].Windows) == 0 || result.Quotas[0].Windows[0].Used != 25 {
		t.Fatalf("real quota response lost: %s", response.Result)
	}
	failing.Store(true)
	response = s.handleAccounts(quotaRequest())
	if response.Error != nil {
		return
	} // An actual worker failure is also explicit.
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Quotas) != 1 || result.Quotas[0].Error == "" || len(result.Quotas[0].Windows) != 0 && result.Quotas[0].AsOf == "" {
		t.Fatalf("failed vendor quota presented as a usable balance: %s", response.Result)
	}
}
