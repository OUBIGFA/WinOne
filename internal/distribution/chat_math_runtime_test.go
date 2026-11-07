package distribution

import (
	"slices"
	"strings"
	"testing"
)

// A native renderer must not be packageable without the font's actual license
// and complete source. Older Chat bundles did not have math and remain valid.
func TestMathPayloadRequiresOriginalNoticesAndSource(t *testing.T) {
	for _, marker := range chatMathFiles {
		if !allowed(marker) {
			t.Fatalf("reviewed math resource rejected: %s", marker)
		}
		required := requiredForPayload(map[string]bool{strings.ToLower(marker): true})
		for _, name := range chatMathFiles {
			if !slices.Contains(required, name) {
				t.Errorf("payload with %s can omit %s", marker, name)
			}
		}
	}
	legacy := requiredForPayload(map[string]bool{"shared/chat/licenses/chat-share-notices.json": true})
	for _, name := range chatMathFiles {
		if slices.Contains(legacy, name) {
			t.Errorf("legacy share bundle now requires %s", name)
		}
	}
	for _, name := range []string{"shared/chat/sources/local.tgz", "shared/chat/licenses/unreviewed-font.txt"} {
		if allowed(name) {
			t.Errorf("unreviewed math file allowed: %s", name)
		}
	}
}
