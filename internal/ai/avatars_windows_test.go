//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPackagedAvatarUsesPrivateProfileBesidePortableMarker(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	directory := t.TempDir()
	bin := filepath.Join(directory, "bin")
	if err := os.MkdirAll(filepath.Join(bin, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	copyExe := filepath.Join(bin, "WinOne.AI.exe")
	if err := os.WriteFile(copyExe, program, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, ".portable"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	translator := Translator{Executable: copyExe, DataDirectory: filepath.Join(directory, "private")}
	picture, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aA9sAAAAASUVORK5CYII=")
	picture = append(picture, bytes.Repeat([]byte{0}, MaxAvatarBytes-len(picture))...)
	icon, err := translator.ImportAvatar(context.Background(), "icons.store", AvatarInput{Data: picture})
	if err != nil {
		t.Fatal(err)
	}
	var received []byte
	for len(received) < len(picture) {
		chunk, err := translator.ReadAvatar(icon, len(received))
		if err != nil {
			t.Fatal(err)
		}
		received = append(received, chunk.Data...)
	}
	if !bytes.Equal(received, picture) {
		t.Fatal("stored picture bytes changed")
	}
	entries, err := os.ReadDir(filepath.Join(bin, "data"))
	if err != nil || len(entries) != 0 {
		t.Fatal("Magpie escaped the isolated data directory", err)
	}
}

// A selected picture must survive a settings round trip without replacing the
// preset's default artwork; otherwise resetting it cannot restore that artwork.
func TestProviderAvatarSurvivesReopenAndCanReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	var provider Provider
	if err := json.Unmarshal([]byte(`{"id":"custom","name":"Custom","authMode":"api-key","protocol":"chat","baseUrl":"https://example.invalid/v1","preset":"custom-preset","icon":"deepseek-color","avatar":"file:0123456789abcdef.png","models":["model"]}`), &provider); err != nil {
		t.Fatal(err)
	}
	configuration.Providers = []Provider{provider}
	if _, err := store.SaveConfiguration(configuration, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs, _, err := reopened.ForTaskKeys("chat", &Selection{Provider: "custom", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if prefs.ProviderIcon != "file:0123456789abcdef.png" {
		t.Fatalf("reply lost custom avatar: %q", prefs.ProviderIcon)
	}
	saved := reopened.Configuration()
	if saved.Providers[0].Icon != "deepseek-color" {
		t.Fatal("avatar overwrote default artwork")
	}
	// JSON models both old clients omitting the new field and an explicit reset.
	if err := json.Unmarshal([]byte(`{"avatar":""}`), &saved.Providers[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.SaveConfiguration(saved, nil); err != nil {
		t.Fatal(err)
	}
	prefs, _, err = reopened.ForTaskKeys("chat", &Selection{Provider: "custom", Model: "model"})
	if err != nil || prefs.ProviderIcon != "deepseek-color" {
		t.Fatalf("default artwork did not return: %q, %v", prefs.ProviderIcon, err)
	}
}

func TestProviderAvatarRejectsPathsAndRemoteReferences(t *testing.T) {
	for _, avatar := range []string{"https://example.invalid/avatar.png", "file:../../image.png", "file:0123456789abcdef.exe", "file:0123456789ABCDEf.png"} {
		t.Run(avatar, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
			if err != nil {
				t.Fatal(err)
			}
			c := store.Configuration()
			p := Provider{ID: "custom", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}}
			data, _ := json.Marshal(map[string]string{"avatar": avatar})
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			c.Providers = []Provider{p}
			if _, err := store.SaveConfiguration(c, nil); err == nil {
				t.Fatal("unvalidated avatar reference was saved")
			}
		})
	}
}
