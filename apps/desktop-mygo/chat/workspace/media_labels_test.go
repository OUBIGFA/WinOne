package workspace

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Only the production controls' labels are asserted here. Real decoding and
// playback remain in the explicit Windows/Host matrix, not in a fake player.
func TestMediaControlsHaveRealChineseAndEnglishNames(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, playing := range []bool{false, true} {
			p := &MediaPane{preview: FilePreview{Kind: "video", Name: "silent.mp4"}, state: mediaState{Playing: playing, Duration: 3, Volume: 1, Rate: 1}}
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 600, 400)
			tt.Frame()
			play := "mediaPlay"
			if playing {
				play = "mediaPause"
			}
			for _, key := range []string{play, "mediaMute", "mediaSeek", "mediaVolume"} {
				label := wt(key)
				if label == "nativeWorkspace."+key || !tt.HasText(label) {
					t.Fatalf("%s media control lacks its translated name: %q", locale, label)
				}
			}
		}
	}
}
