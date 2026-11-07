package overview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

// A runtime error is visible without opening a disclosure or another page.
func TestHomeShowsTheActualProblemAndDoesNotInventAnUnknownCause(t *testing.T) {
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, reason := range []string{"context deadline exceeded", "reader.exe: access denied", ""} {
			s := catalog(true)
			s.Modules[0].Runtime.State = "faulted"
			s.Modules[0].Runtime.Detail = reason
			h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) { return s, nil })
			p := newPage(h.Env)
			tt := ui.NewTester(p.View, 560, 900)
			h.Settle(tt)
			text := strings.Join(tt.Texts(), "\n")
			if reason != "" && !strings.Contains(text, reason) {
				t.Fatalf("actual diagnostic hidden: %q", text)
			}
			if reason == "" && !strings.Contains(text, "faulted") {
				t.Fatalf("unknown state was hidden: %q", text)
			}
			if len(h.Called("modules.setEnabled")) != 0 {
				t.Fatal("displaying a problem changed the module")
			}
		}
	}
}
