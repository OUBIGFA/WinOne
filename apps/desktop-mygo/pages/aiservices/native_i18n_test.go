package aiservices

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"winone/desktopmygo/i18n"
)

// Visible strings are a contract. This checks the native port's literal keys;
// data supplied by providers/plugins remains in its original language.
func TestNativeAIPortHasBothLanguages(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`\bnativeT\("([A-Za-z0-9_.-]+)"\)`)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range literal.FindAllStringSubmatch(string(data), -1) {
			key := "aiNative." + match[1]
			for _, locale := range []string{i18n.Chinese, i18n.English} {
				if i18n.Translate(locale, key) == key {
					t.Errorf("%s needs %s: %s", file, locale, key)
				}
			}
		}
	}
}
