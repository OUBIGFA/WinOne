package main

import (
	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/oidc"
	"github.com/yetone/magpie/internal/oidc/oidctest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A workflow's JWT must pass the authenticated private bridge as an identity
// token, not be checked as an ordinary gateway key or replaced by our token.
func TestOIDCExchangeThroughWinOneBridge(t *testing.T) {
	issuer := oidctest.New(t)
	secret, err := access.Update("add-key", access.Change{Name: "WinOne OIDC fixture"})
	if err != nil {
		t.Fatal(err)
	}
	who, err := access.Check(secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = access.Update("remove-key", access.Change{Key: who.KeyID}) })
	if _, err = access.Update("issuer-key", access.Change{Key: who.KeyID, Issues: true}); err != nil {
		t.Fatal(err)
	}
	rule, err := oidc.Add(oidc.Rule{Key: who.KeyID, Issuer: issuer.URL, Claims: map[string]string{"repository": "fixture/project"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = oidc.Remove(rule.ID) })
	now := time.Now().Unix()
	token := issuer.Sign(t, "k1", map[string]any{"iss": issuer.URL, "aud": rule.Audience, "sub": "fixture", "repository": "fixture/project", "iat": now, "exp": now + 300, "jti": "winone-fixture"})
	handler := gateway.New().Handler()
	call := func(tok string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost"+gateway.OIDCKeysPath, strings.NewReader(`{"ttl":"10m"}`))
		r.Header.Set("Authorization", "Bearer "+gateway.Token)
		r.Header.Set("X-WinOne-Gateway-Credential", tok)
		r.Header.Set("X-WinOne-Remote-Addr", "127.0.0.1:5000")
		w := httptest.NewRecorder()
		r, ok := gateway.WinOneNamedRequest(w, r)
		if ok {
			r = gateway.WinOneMediaRequest(r, []string{"fixture/model"})
			handler.ServeHTTP(w, r)
		}
		return w
	}
	if w := call(token); w.Code != http.StatusCreated {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
	if w := call(token); w.Code != http.StatusUnauthorized {
		t.Fatalf("replayed identity accepted: %d", w.Code)
	}
	if w := call("not.a.jwt"); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid identity accepted: %d", w.Code)
	}
}
