//go:build windows

package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservabilityHeadersAreEncryptedWriteOnlyAndClearable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	c.MagpieSettings.OTel.Headers = map[string]string{"Authorization": "bad\r\nX-Injected: value"}
	if _, err = s.SaveConfiguration(c, nil); err == nil {
		t.Fatal("invalid private header accepted before sealing")
	}
	c.MagpieSettings.OTel.Headers = map[string]string{"Authorization": "fixture-collector-token"}
	saved, err := s.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	public, _ := json.Marshal(saved)
	if strings.Contains(string(data), "fixture-collector-token") || strings.Contains(string(public), "fixture-collector-token") {
		t.Fatal("export token is not write-only")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveConfiguration(s.Configuration(), nil); err != nil {
		t.Fatal(err)
	}
	runtime, err := s.MagpieRuntime()
	if err != nil || runtime.OTel.Headers["Authorization"] != "fixture-collector-token" {
		t.Fatal("header lost during save/restart", err)
	}
	c = s.Configuration()
	delete(c.MagpieSettings.OTel.Headers, "Authorization")
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	runtime, err = s.MagpieRuntime()
	if err != nil || len(runtime.OTel.Headers) != 0 {
		t.Fatal("deleted header remains active", err)
	}
}
