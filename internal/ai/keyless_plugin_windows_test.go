//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A real plugin signs in twice without an input channel. Its single-identity
// contract must replace credentials, not accumulate hidden accounts.
func TestKeylessSingleIdentityPluginSignIn(t *testing.T) {
	executable := os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated AI bundle")
	}
	root := t.TempDir()
	fixture := filepath.Join(root, "keyless.mjs")
	source := `export default async()=>({config(c){c.provider??={};c.provider.publicfixture={name:"Public fixture",models:{m1:{name:"One"}}}},auth:{provider:"publicfixture",single:true,checkin:async()=>({outcome:"claimed",credit:12,streak:2}),methods:[{type:"api",keyless:true,label:"Use public service",authorize:async()=>({type:"success",key:"local-fixture-key"})}]}})`
	if err := os.WriteFile(fixture, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(root, "third-party", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"plugins": []any{map[string]string{"spec": fixture}}})
	if err := os.WriteFile(registry, data, 0600); err != nil {
		t.Fatal(err)
	}
	runtime := Translator{Executable: executable, DataDirectory: root, AccountBackend: "plugin"}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for range 2 {
		done := false
		err := runtime.SignInMethod(ctx, "publicfixture", nil, make(chan string), func(raw json.RawMessage) {
			var event struct{ SignIn struct{ State string } }
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			if event.SignIn.State == "input" {
				t.Error("keyless method asked for a key")
				cancel()
			}
			if event.SignIn.State == "done" {
				done = true
			}
		})
		if err != nil || !done {
			t.Fatalf("keyless sign-in did not complete: %v", err)
		}
	}

	listing, err := runtime.Accounts(ctx, "accounts.list", "publicfixture")
	var state struct{ CheckinAvailable bool }
	if err != nil || json.Unmarshal(listing, &state) != nil || !state.CheckinAvailable {
		t.Fatalf("check-in capability missing: %s %v", listing, err)
	}
	result, err := runtime.Accounts(ctx, "accounts.checkin", "publicfixture")
	var status struct {
		Checkins []struct {
			Outcome string
			Credit  float64
			Asked   bool
		}
	}
	if err != nil || json.Unmarshal(result, &status) != nil || len(status.Checkins) != 1 || status.Checkins[0].Outcome != "claimed" || status.Checkins[0].Credit != 12 || !status.Checkins[0].Asked {
		t.Fatalf("check-in result lost: %s %v", result, err)
	}
	auth, err := os.ReadFile(filepath.Join(root, "config", "magpie", "plugin-auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var accounts map[string]json.RawMessage
	if err := json.Unmarshal(auth, &accounts); err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("single identity saved %d accounts", len(accounts))
	}
}
