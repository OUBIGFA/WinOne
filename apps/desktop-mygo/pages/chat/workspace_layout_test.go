package chat

import (
	"fmt"
	"testing"

	"winone/desktopmygo/i18n"
)

// A saved, wide workspace must not squeeze the conversation to an unusable
// sliver when the actual content area narrows. Resizing is a viewing choice:
// it must preserve both the preferred dock/size and the message draft.
func TestWorkspaceKeepsConversationReadableAcrossResizeAndSavedWidth(t *testing.T) {
	_, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.updateText("Keep this message draft", nil, "c1")
	p.prefs.Right, p.prefs.Dock = 2000, "right"
	p.openTools("resources")
	defer i18n.SetLocale(i18n.Chinese)
	defer func() { dark = false }()
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, theme := range []bool{false, true} {
			dark = theme
			for _, width := range []int{1100, 784, 560, 1100} {
				tt.SetSize(width, 760)
				settle(h, tt)
				reading, ok := tt.Find(i18n.T("chat.readingArea"))
				if !ok || reading.W < 300 || reading.H < 80 {
					t.Fatalf("%s dark=%v width=%d: unreadable transcript %+v", locale, theme, width, reading)
				}
				input, ok := tt.Find(i18n.T("chat.message"))
				if !ok || input.W < 250 || input.X+input.W > float32(width)+1 {
					t.Fatalf("%s dark=%v width=%d: unusable composer %+v", locale, theme, width, input)
				}
				handle, ok := tt.Find(i18n.T("chat.resizeDetails"))
				if !ok || handle.X < 0 || handle.Y < 0 || handle.X+handle.W > float32(width)+1 || handle.Y+handle.H > 761 {
					t.Fatalf("workspace divider escaped the window: %+v", handle)
				}
				if width <= 784 && handle.W <= handle.H {
					t.Fatalf("narrow content did not move the workspace below the conversation: %+v", handle)
				}
				if width == 1100 && handle.W >= handle.H {
					t.Fatalf("wide content did not restore the preferred right dock: %+v", handle)
				}
				if p.prefs.Right != 2000 || p.prefs.Dock != "right" || p.text != "Keep this message draft" || !p.ui.details {
					t.Fatal("responsive layout changed a preference, closed the workspace or lost the draft")
				}
				shot(t, tt, fmt.Sprintf("workspace-layout-%s-%v-%d", locale, theme, width))
			}
		}
	}
}
