package workspace

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

// The visible menu and the close button's accessible name both use real
// document labels, not untranslated dynamic keys or unresolved placeholders.
func TestPanelActionsAndCloseButtonHaveLocalizedNames(t *testing.T) {
	_, h, _, _ := textSetup(t, "notes.txt")
	p := NewPanel(h.Env, PanelOptions{Prefix: "chat.", SessionID: "session"})
	t.Cleanup(p.Close)
	p.OpenFile("notes.txt", 0)
	tt := panelTester(p)
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for range 3 {
			h.Settle(tt)
		}
		closeLabel := i18n.T("chat.closeTab", "name", "notes.txt")
		if _, found := tt.Find(closeLabel); !found {
			t.Errorf("%s close button is not named %q", locale, closeLabel)
		}
		if err := tt.Click(i18n.T("chat.panelActions")); err != nil {
			t.Fatal(err)
		}
		h.Settle(tt)
		fileLabel := "文件"
		if locale == i18n.English {
			fileLabel = "File"
		}
		if err := tt.ChooseMenuItem(fileLabel); err != nil {
			t.Errorf("%s file action is not available: %v; menu: %q", locale, err, tt.Menu())
		} else if !p.newFileOpen {
			t.Error("the localized file action did not open file creation")
		}
		tt.Key(0, ui.KeyEscape) // Close without changing document state.
		h.Settle(tt)
	}
}
