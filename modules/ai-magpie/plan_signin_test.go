package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanPlatformKindsFollowUpstreamEndpoints(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, test := range []struct{ url, kind string }{
		{"https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1", "qianwen"},
		{"https://qianfan.baidubce.com/v2/tokenplan/personal", "qianfan"},
		{"https://api.stepfun.ai/step_plan/v1", "stepfun"},
		{"https://relay.example/v1", ""},
	} {
		p, err := configuredProvider(request{ProviderID: "plan-fixture", Protocol: "chat", BaseURL: test.url})
		if err != nil {
			t.Fatal(err)
		}
		if got := planSessionOf(p); got.Kind != test.kind {
			t.Errorf("%s: %s", test.url, got.Kind)
		}
	}
}

// This runs the original PKCE/device polling and persistence entirely in
// memory; it opens no HTTP listener and uses no real platform credentials.
func TestQianwenPlanSignInOwnsPollAndNeverReturnsToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/cli/device/code":
			if r.URL.Query().Get("code_challenge") == "" {
				t.Error("upstream PKCE challenge was lost")
			}
			body = `{"Success":true,"Data":{"Token":"device-fixture","VerificationUrl":"https://example.invalid/approve","ExpiresIn":60,"Interval":0.001}}`
		case "/cli/device/token":
			if r.URL.Query().Get("code_verifier") == "" {
				t.Error("PKCE verifier was lost")
			}
			body = `{"Success":true,"Data":{"Status":"complete","Credentials":{"AccessToken":"private-plan-access","User":{"Email":"fixture@example.invalid"}}}}`
		default:
			t.Fatalf("unexpected platform request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var out bytes.Buffer
	in := request{Operation: "plans.signin", ProviderID: "private-plan", Protocol: "chat", BaseURL: "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1"}
	if err := planOperation(ctx, in, input, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"state":"waiting"`) || !strings.Contains(out.String(), `"state":"done"`) || strings.Contains(out.String(), "private-plan-access") {
		t.Fatal("sign-in progress or credential boundary failed", out.String())
	}
	if !provider.QianwenSignedIn(in.ProviderID) || provider.QianwenSignedIn("another-provider") {
		t.Fatal("sign-in was not scoped to its provider")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(provider.Path()), "qianwen-auth.json")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	in.Operation = "plans.get"
	if err := planOperation(ctx, in, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	var state planSession
	if json.Unmarshal(out.Bytes(), &state) != nil || !state.SignedIn || state.Account != "fixture@example.invalid" {
		t.Fatal(out.String())
	}
	out.Reset()
	in.Operation = "plans.signout"
	if err := planOperation(ctx, in, strings.NewReader(""), &out); err != nil || provider.QianwenSignedIn(in.ProviderID) {
		t.Fatal("signout failed", err)
	}
}

func TestKeyBalanceAlsoReadsTokenPlanWindows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous; provider.WinOneProviders = nil })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unsigned-in plan must not contact a vendor: %s", r.URL)
		return nil, context.Canceled
	})}
	var out bytes.Buffer
	if err := readKeyBalances(t.Context(), request{ProviderID: "qwen-plan", Protocol: "chat", BaseURL: "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1", BalanceKeys: []provider.KeyAccount{{Key: "fixture", Name: "slot"}}}, &out); err != nil {
		t.Fatal(err)
	}
	var result struct{ Quotas []provider.SubscriptionQuota }
	if json.Unmarshal(out.Bytes(), &result) != nil || len(result.Quotas) != 1 || !result.Quotas[0].NeedsAccessKey {
		t.Fatal("missing upstream plan/login state", out.String())
	}
}

func TestQianfanPlatformPasteRetriesAndKeepsRenewedPrivateSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "console.bce.baidu.com" || r.Header.Get("Cookie") != "BDUSS=private-original" {
			t.Fatalf("platform cookie/request lost: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"BDUSS=private-renewed; Path=/"}}, Body: io.NopCloser(strings.NewReader(`{"success":true,"result":{}}`))}, nil
	})}
	var controls bytes.Buffer
	encoder := json.NewEncoder(&controls)
	encoder.Encode(signInControl{Callback: "invalid-session"})
	encoder.Encode(signInControl{Callback: "BDUSS=private-original"})
	in := request{Operation: "plans.signin", ProviderID: "qianfan-fixture", Protocol: "chat", BaseURL: "https://qianfan.baidubce.com/v2/tokenplan/personal"}
	var out bytes.Buffer
	if err := planOperation(t.Context(), in, &controls, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"inputError"`) || !strings.Contains(out.String(), `"state":"done"`) || strings.Contains(out.String(), "private-original") || strings.Contains(out.String(), "private-renewed") {
		t.Fatal("paste retry or secret protection failed", out.String())
	}
	p, _ := configuredProvider(in)
	if state := planSessionOf(p); !state.SignedIn || state.Ended || state.Bookmarklet == "" {
		t.Fatalf("missing plan state: %+v", state)
	}
	session, err := os.ReadFile(filepath.Join(filepath.Dir(provider.Path()), "qianfan-auth.json"))
	if err != nil || !bytes.Contains(session, []byte("private-renewed")) {
		t.Fatal("upstream renewed cookie not kept", err)
	}
	in.Operation = "plans.signout"
	if err := planOperation(t.Context(), in, strings.NewReader(""), io.Discard); err != nil || planSessionOf(p).SignedIn {
		t.Fatal("platform signout failed", err)
	}
}

func TestQianwenPlatformCancelEndsUpstreamPolling(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/cli/device/code" {
			t.Fatalf("canceled sign-in still polled: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"Success":true,"Data":{"Token":"device-fixture","VerificationUrl":"https://example.invalid/approve","ExpiresIn":60,"Interval":2}}`))}, nil
	})}
	in := request{Operation: "plans.signin", ProviderID: "canceled-plan", Protocol: "chat", BaseURL: "https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1"}
	if err := planOperation(t.Context(), in, strings.NewReader("{\"cancel\":true}\n"), io.Discard); err != context.Canceled {
		t.Fatal("cancel did not end sign-in", err)
	}
	if state, _ := provider.QianwenSignInState(in.ProviderID); state != "" || provider.QianwenSignedIn(in.ProviderID) {
		t.Fatal("canceled flow retained sign-in")
	}
}
