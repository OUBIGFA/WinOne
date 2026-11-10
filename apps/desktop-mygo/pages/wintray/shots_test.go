package wintray

import (
	"fmt"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
)

// TestShots saves every tab of the page in both themes and two widths
// when WINONE_SHOTS names a folder, for looking at.
func TestShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	f := setup(t, "running")
	h, p := f.h, f.page
	for i, key := range []string{"wintray.list", "wintray.advancedStartup"} {
		label := i18n.T(key)
		h.Shots(t, fmt.Sprintf("wintray-%d", i), p.View, 1600, func(tt *ui.Tester) {
			if err := tt.Click(label); err != nil {
				t.Logf("tab %q: %v", label, err)
			}
		})
	}
}

// Auto-login remains available beside the startup modes in both languages,
// including a disabled-but-preserved choice for hidden launches.
func TestWeChatAutoLoginShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	t.Cleanup(func() { i18n.SetLocale(i18n.Chinese) })
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, startupMode := range []string{"tray", "normal", "task", "hidden"} {
			document := programDocument("a", "WeChat", `C:\Apps\Weixin.exe`)
			document["wechatAutoLogin"] = true
			document["launchViaLogonTask"] = startupMode == "task"
			document["launchHiddenInBackground"] = startupMode == "hidden"
			document["trayBehavior"].(map[string]any)["autoMinimizeAndHideOnLaunch"] = startupMode == "tray"
			f := setup(t, "running", document)
			i18n.SetLocale(locale)
			f.h.Shots(t, "wechat-"+locale+"-"+startupMode, f.page.View, 1000, nil)
		}
	}
}

// Use the production page for both new entry points, including long paths.
func TestUpstreamSettingsShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	t.Cleanup(func() { i18n.SetLocale(i18n.Chinese) })
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, tab := range []int{0, 1} {
			document := programDocument("a", "QQ", `C:\Apps\QQ.exe`)
			document["qqWaitForLogin"] = true
			document["workingDirectory"] = `D:\工作资料\A long project folder\QQ data`
			f := setup(t, "running", document)
			i18n.SetLocale(locale)
			f.page.detailTab = tab
			f.h.Shots(t, fmt.Sprintf("upstream-%s-%d", locale, tab), f.page.View, 1000, nil)
		}
	}
}
