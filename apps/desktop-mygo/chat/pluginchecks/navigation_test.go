package pluginchecks

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// Real-window regression: the shared guard immediately applies navigation on a
// clean form, in the same frame as the click, rather than always deferring it.
func TestBackFromCleanPluginConfigurationDoesNotRenderReleasedState(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) { return []plugin{}, nil })
	m := NewManager(h.Env)
	m.configuration = &plugin{PluginID: "fixture", Name: "Fixture"}
	m.parameters, m.baseline = "{}", "{}"
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); m.View(c) }, 900, 760)
	h.Settle(tt)
	if err := tt.Click(text("back")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if m.configuration != nil || !tt.HasText(text("install")) {
		t.Fatal("clean configuration did not return to the plugin list")
	}
}
