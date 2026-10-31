package usage

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"winone/desktopmygo/i18n"
)

// TestEveryKeyIsInBothLanguages checks the keys the tab names, written
// out or composed from what the views offer, against both tables.
func TestEveryKeyIsInBothLanguages(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`(?:\btr|i18n\.T)\("([a-zA-Z0-9_.-]+)"\s*[,)]`)
	keys := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range literal.FindAllStringSubmatch(string(data), -1) {
			keys[m[1]] = true
		}
	}
	composed := map[string][]string{
		"usage.":        {"cost", "calls", "speed", "model", "modelAt", "provider", "agent", "callerKey", "output", "messages", "sessions", "minutes", "autonomy", "tools", "skills"},
		"usage.group":   {"agent", "model", "account", "provider", "project", "hour"},
		"usage.score":   {"cache", "lean", "pace", "reliable"},
		"usage.part":    contextParts,
		"usage.archive": {"request", "response"},
		"usage.speed":   {"Failed", "AnswerFloor", "WholeFloor", "NoDuration", "NoOutput", "NoFirstToken", "ReasoningOnly", "Burst", "TooLittle"},
	}
	for prefix, rests := range composed {
		for _, rest := range rests {
			keys[prefix+rest] = true
		}
	}
	for key := range keys {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			if got := i18n.Translate(locale, key); got == key {
				t.Errorf("%s has no %s text", key, locale)
			}
		}
	}
}
