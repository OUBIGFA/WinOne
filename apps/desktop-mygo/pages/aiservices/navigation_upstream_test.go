package aiservices

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/desktopmygo/testhost"
)

// An imported full-version configuration cannot add retired navigation or
// launch its old plugin/routing RPCs. The fake rejects every unhandled call.
func TestRemovedSettingsDoNotAddNavigation(t *testing.T) {
	f := newFakeAI(t)
	f.configuration["routingEnabled"] = true
	f.configuration["magpieSettings"] = map[string]any{"builtinPreset": "all", "builtinPlugins": map[string]any{"routing": map[string]any{"enabled": true}, "usage": map[string]any{"enabled": true}}, "redactModel": true}
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	tt := ui.NewTester(p.upstreamNavigation, 1000, 400)
	settle(h, tt)
	for _, label := range []string{"Sessions", "Library", "Routing", "Routing groups", "Usage", "Plugins", "Masking"} {
		if tt.HasText(upstreamT(label)) {
			t.Errorf("retired service returned: %s", label)
		}
	}
}
