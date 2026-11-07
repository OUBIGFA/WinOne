//go:build windows

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The native OneBot page's fixed method names are the contract with this Host: a
// name missing from oneBotMethod fails only at run time, on the owner's screen.
func TestNativeOneBotPageCallsOnlyMethodsTheHostAllows(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "desktop-mygo", "pages", "onebot", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("native pages not found: %v", err)
	}
	call := regexp.MustCompile(`(?:oneBotCall\([^,\n]+,\s*|\.call\(|\.mutate\()"([a-zA-Z][\w.]*)"`)
	seen := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range call.FindAllSubmatch(data, -1) {
			method := string(match[1])
			if method[len(method)-1] == '.' {
				continue // A composed name; its parts are listed below.
			}
			seen++
			if allowed, _ := oneBotMethod(method); !allowed {
				t.Errorf("%s calls %q, which the Host refuses", filepath.Base(file), method)
			}
		}
	}
	if seen < 50 {
		t.Fatalf("only %d calls found; the pattern no longer matches the page", seen)
	}
	for _, method := range []string{"groups.continue", "groups.stop", "groups.delete", "groups.plan.start", "groups.plan.dismiss", "groups.plan.continue", "groups.plan.retry", "groups.plan.edit",
		"groups.files.open", "groups.files.reveal", "routines.save", "routines.remove", "routines.runNow",
		"routines.nightly.setEnabled", "routines.nightly.setFrequency", "routines.nightly.runNow", "routines.nightly.revert", "routines.nightly.candidate"} {
		if allowed, _ := oneBotMethod(method); !allowed {
			t.Errorf("composed call %q is refused", method)
		}
	}
}
