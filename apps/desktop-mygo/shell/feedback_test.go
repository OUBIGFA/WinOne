package shell

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

type feedbackWindow struct{ approved, canceled int }

func (*feedbackWindow) Minimize()                  {}
func (*feedbackWindow) ToggleMaximize()            {}
func (*feedbackWindow) IsMaximized() bool          { return false }
func (*feedbackWindow) Close()                     {}
func (w *feedbackWindow) ApproveClose()            { w.approved++ }
func (w *feedbackWindow) CancelClose()             { w.canceled++ }
func (*feedbackWindow) ApplySettings(theme string) {}

func TestClosingShellFeedbackDoesNotRemoveUnsavedCloseProtection(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	i18n.SetLocale(i18n.English)
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		return map[string]any{}, nil
	})
	window := &feedbackWindow{}
	s := New(h.Env, window, Options{Directory: h.Env.Directory})
	s.loaded = true
	s.snapshot.RuntimeWarning = "Current runtime warning"
	s.instance = core.PageFunc(func(c *ui.Context) { ui.Text(c, "Unchanged saved and unsaved content") })
	h.Env.Activity.Report("draft", true, false, nil)
	tt := ui.NewTester(s.View, 1000, 900)
	t.Cleanup(s.Discarded)
	if err := tt.Click(i18n.T("notices.dismiss")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if tt.HasText(s.snapshot.RuntimeWarning) || !tt.HasText("Unchanged saved and unsaved content") {
		t.Fatal("closing feedback changed the page rather than the notice")
	}
	if s.RequestClose() || s.pending == nil || window.approved != 0 {
		t.Fatal("closing feedback removed the unsaved close guard")
	}
	h.Settle(tt)
	if !tt.HasText(i18n.T("navigation.dirtyTitle")) || !tt.HasText(i18n.T("navigation.keepEditing")) {
		t.Fatal("the necessary native close decision was not shown")
	}
	if err := tt.Click(i18n.T("navigation.keepEditing")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if s.pending != nil || window.canceled != 1 {
		t.Fatal("continuing editing did not cancel the close")
	}
	if dirty, _ := s.Activity(); !dirty {
		t.Fatal("the owner's unsaved state was discarded")
	}
}
