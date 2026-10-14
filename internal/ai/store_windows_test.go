//go:build windows

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyIsProtectedAndNeverReturned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Snapshot()
	p.Engine = "llm"
	p.BaseURL = "https://example.com/v1"
	p.Model = "test"
	key := "test-only-secret-123"
	saved, err := s.Save(p, &key)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.HasKey {
		t.Fatal("credential absent")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), key) {
		t.Fatal("plaintext key persisted")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := reopened.ForRequest()
	if err != nil || got != key {
		t.Fatalf("credential round trip failed: %v", err)
	}
	if _, err = s.Save(p, nil); err == nil {
		t.Fatal("stale write accepted")
	}
}

func TestAccountModeDoesNotReceiveStoredAPIKey(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := store.Snapshot()
	p.Engine = "llm"
	p.BaseURL = "https://example.com/v1"
	p.Model = "model"
	secret := "test-only-key"
	p, err = store.Save(p, &secret)
	if err != nil {
		t.Fatal(err)
	}
	p.AuthMode = "account"
	p.Agent = "codex"
	p, err = store.Save(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := store.ForRequest()
	if err != nil || key != "" || !p.HasKey {
		t.Fatal("account mode exposed or erased the API key", err)
	}
	p.AuthMode = "api-key"
	if _, err = store.Save(p, nil); err != nil {
		t.Fatal(err)
	}
	_, key, err = store.ForRequest()
	if err != nil || key != secret {
		t.Fatal("switching login modes lost the API key", err)
	}
}

func TestSharedModelRemainsUsableWithGoogleTranslation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := store.Snapshot()
	p.Model = "shared-model"
	p.BaseURL = "https://example.invalid/v1"
	key := "shared-secret"
	if _, err = store.Save(p, &key); err != nil {
		t.Fatal(err)
	}
	shared, secret, err := store.ForModelRequest()
	if err != nil || shared.Model != "shared-model" || secret != key {
		t.Fatal("shared credentials unavailable", err)
	}
	if store.Snapshot().Engine != "google" {
		t.Fatal("module request changed translation preference")
	}
	p = store.Snapshot()
	p.OCREngine = "ai"
	p.Model = ""
	if _, err = store.Save(p, nil); err == nil {
		t.Fatal("AI OCR accepted without a configured model")
	}
}
