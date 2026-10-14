//go:build windows

package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentEnvironmentIsPrivateAndDoesNotChangeParent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "parent-test-key")
	original := os.Getenv("USERPROFILE")
	root := t.TempDir()
	env := map[string]string{}
	for _, entry := range isolatedEnvironment(root) {
		key, value, _ := strings.Cut(entry, "=")
		env[strings.ToUpper(key)] = value
	}
	if env["OPENAI_API_KEY"] != "" || env["USERPROFILE"] != filepath.Join(root, "home") || env["APPDATA"] != filepath.Join(root, "home/AppData/Roaming") {
		t.Fatal("child inherited another client's account environment")
	}
	if os.Getenv("USERPROFILE") != original || os.Getenv("OPENAI_API_KEY") != "parent-test-key" {
		t.Fatal("parent environment was changed")
	}
}
