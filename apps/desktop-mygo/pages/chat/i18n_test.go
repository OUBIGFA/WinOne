package chat

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"winone/desktopmygo/i18n"
)

// TestEveryKeyIsInBothLanguages checks the keys the page names, written
// out or composed from what the views offer, against both tables.
func TestEveryKeyIsInBothLanguages(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Keys written out, and chat keys in tables of labels; Host method
	// names and composed prefixes are not keys.
	literal := regexp.MustCompile(`i18n\.T\("([a-zA-Z0-9_.-]+)"\s*[,)]|"(chat\.[a-zA-Z0-9_.]+[a-zA-Z0-9])"`)
	// chat.messageActionBar.reply is a Cindy key read through botT, not a Chat key.
	methods := map[string]bool{"chat.export": true, "chat.messageActionBar.reply": true}
	keys := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range literal.FindAllStringSubmatch(string(data), -1) {
			if key := m[1] + m[2]; !methods[key] {
				keys[key] = true
			}
		}
	}
	for _, level := range thinkingLevels[1:] {
		keys["chat.thinking_"+level] = true
	}
	for _, status := range []string{"running", "completed", "failed", "stopped", "allowed", "denied"} {
		keys["chat.operationStatus_"+status] = true
	}
	// Tool rows name a file tool's action, its dialog the snapshot's source.
	for _, action := range []string{"read", "edit", "create"} {
		keys["chat.toolVerb_"+action] = true
	}
	for _, source := range []string{"read", "write", "edit"} {
		keys["chat.toolSnapshot_"+source] = true
	}
	for key := range keys {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			if got := i18n.Translate(locale, key); got == key {
				t.Errorf("%s has no %s text", key, locale)
			}
		}
	}
}
