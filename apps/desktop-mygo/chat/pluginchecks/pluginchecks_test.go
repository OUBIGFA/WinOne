package pluginchecks

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestPickerKeepsMissingSavedCheckAndUsesTheRealCandidateIDs(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, width := range []int{560, 1000} {
			h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
				if method != "chat.ghosts.candidates" {
					t.Fatalf("unexpected %s", method)
				}
				var input struct{ Point string }
				if err := json.Unmarshal(params, &input); err != nil {
					t.Fatal(err)
				}
				if input.Point != "will-schedule-run" {
					t.Fatalf("wrong point: %q", input.Point)
				}
				return []Candidate{{PluginID: "installed-instance", Name: "Local check"}}, nil
			})
			p := &Picker{}
			value := &Check{PluginID: "missing-instance", OnFailure: "skip"}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(kit.Theme(c.Preferences(), false))
				ui.Column(c).Fill().Padding(16).Gap(8).Children(func() { p.View(c, h.Env, "will-schedule-run", &value) })
				p.Dialogs(c)
			}, width, 600)
			for range 3 {
				h.Settle(tt)
			}
			if value.PluginID != "missing-instance" || value.OnFailure != "skip" || !tt.HasText(text("missing", "id", "missing-instance")) {
				t.Fatalf("saved check changed or disappeared: %+v %q", value, tt.Texts())
			}
			if err := tt.Click(text("missing", "id", "missing-instance")); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Local check"); err != nil {
				t.Fatal(err)
			}
			h.Settle(tt)
			if value.PluginID != "installed-instance" || value.OnFailure != "skip" {
				t.Fatalf("selection: %+v", value)
			}
		}
	}
}

func TestConfigurationCloseUsesTheSharedUnsavedGuard(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if method == "chat.ghosts.list" {
			return []plugin{}, nil
		}
		t.Fatalf("unexpected %s", method)
		return nil, nil
	})
	m := NewManager(h.Env)
	m.configuration = &plugin{PluginID: "installed-instance", Name: "Local check"}
	m.baseline, m.parameters = "{}", "{}"
	m.secretText, m.secretClear = map[string]string{}, map[string]bool{}
	var pending func()
	h.Env.RequestChange = func(action func()) { pending = action }
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); m.View(c) }, 1000, 760)
	for range 3 {
		h.Settle(tt)
	}
	if err := tt.Click(text("parameters")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type(`{"enabled":true}`)
	h.Settle(tt)
	if !m.Dirty() {
		t.Fatal("native parameter edit was not protected")
	}
	if err := tt.Click(text("close")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if pending == nil || !m.open || !m.Dirty() {
		t.Fatal("close lost the configuration before the shared confirmation")
	}
	m.discard()
	pending()
	if m.open || m.Dirty() {
		t.Fatal("confirmed discard did not close the manager")
	}
}

func TestEveryDeclaredPermissionLabelIsLocalized(t *testing.T) {
	for _, key := range []string{"subscribeScheduleRun", "subscribeRoutineRun", "networkHost", "networkSecret"} {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			if got := i18n.Translate(locale, "ghostPermission."+key); got == "ghostPermission."+key {
				t.Errorf("missing %s %s", locale, key)
			}
		}
	}
}
