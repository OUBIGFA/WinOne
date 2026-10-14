//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestDraftAccountCanAuthenticateWithoutSavingProvider(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), "unused.exe")
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.store = store
	before := store.Configuration()
	p, err := s.accountProvider("new-account", "cline", &ai.AccountDraft{Agent: "cline", Backend: "plugin"})
	if err != nil || p.Agent != "cline" || p.AgentBackend != "plugin" {
		t.Fatal(p, err)
	}
	if after := store.Configuration(); after.Revision != before.Revision || len(after.Providers) != len(before.Providers) {
		t.Fatal("login draft committed provider settings")
	}
	if s.translator.ForProvider(p.ID).DataDirectory == s.translator.ForProvider("another-account").DataDirectory {
		t.Fatal("draft accounts share credentials")
	}
	for _, id := range []string{"", "../escape", "legacy"} {
		if _, err := s.accountProvider(id, "cline", &ai.AccountDraft{Agent: "cline", Backend: "plugin"}); err == nil {
			t.Fatal("unsafe draft accepted", id)
		}
	}
	if _, err := s.accountProvider("new-account", "codex", &ai.AccountDraft{Agent: "cline", Backend: "plugin"}); err == nil {
		t.Fatal("agent mismatch accepted")
	}
}

func TestDraftAccountShowsPluginLoginStepsWithoutSaving(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("explicit bundled plugin login test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := newAIService(ctx, t.TempDir(), "unused.exe")
	s.translator.Executable = executable
	call := func(method string, params any) host.Response {
		raw, _ := json.Marshal(params)
		return s.handle(host.Request{ID: "draft-test", Method: method, PeerPID: uint32(os.Getpid()), Params: raw})
	}
	draft := ai.AccountDraft{Agent: "cline", Backend: "plugin"}
	response := call("accounts.list", map[string]any{"agent": "cline", "provider": "unsaved-account", "draft": draft})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var listing struct {
		Methods []struct {
			Type string `json:"type"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(response.Result, &listing); err != nil {
		t.Fatal(err)
	}
	method := -1
	for i, m := range listing.Methods {
		if m.Type == "api" {
			method = i
			break
		}
	}
	if method < 0 {
		t.Fatal("plugin API-key login method missing")
	}
	response = call("accounts.signin.start", map[string]any{"agent": "cline", "provider": "unsaved-account", "draft": draft, "method": method})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var job struct {
		ID     string
		Status aiSignInState
	}
	if err := json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	defer call("accounts.signin.cancel", map[string]string{"id": job.ID})
	for !job.Status.PasteKey && job.Status.Prompt == nil {
		if ctx.Err() != nil || job.Status.State == "failed" {
			t.Fatalf("login steps unavailable: %+v", job.Status)
		}
		time.Sleep(20 * time.Millisecond)
		response = call("accounts.signin.get", map[string]string{"id": job.ID})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		if err := json.Unmarshal(response.Result, &job); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.store.Configuration().Providers) != 0 {
		t.Fatal("login step committed unsaved configuration")
	}
}
