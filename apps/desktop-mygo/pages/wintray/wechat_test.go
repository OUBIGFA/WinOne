package wintray

import (
	"testing"
)

// The option follows the executable in every startup mode, not the display name.
func TestWeChatAutoLoginOptionOnlyAppearsForWeChat(t *testing.T) {
	for _, name := range []string{"Weixin.exe", "WECHAT.EXE", "player.exe", "WeixinHelper.exe"} {
		for _, startupMode := range []string{"tray", "normal", "task", "hidden"} {
			t.Run(name+"/"+startupMode, func(t *testing.T) {
				document := programDocument("a", "微信", `C:\Apps\`+name)
				document["launchViaLogonTask"] = startupMode == "task"
				document["launchHiddenInBackground"] = startupMode == "hidden"
				document["trayBehavior"].(map[string]any)["autoMinimizeAndHideOnLaunch"] = startupMode == "tray"
				f := setup(t, "running", document)
				want := name == "Weixin.exe" || name == "WECHAT.EXE"
				if f.tt.HasText("微信自动登录") != want {
					t.Fatalf("auto-login visibility for %s/%s: %q", name, startupMode, f.tt.Texts())
				}
			})
		}
	}
}

// Tray, normal and task launches save the same opt-in field with the whole draft.
func TestWeChatAutoLoginIsSaved(t *testing.T) {
	for _, startupMode := range []string{"tray", "normal", "task"} {
		t.Run(startupMode, func(t *testing.T) {
			document := programDocument("a", "微信", `C:\Apps\Weixin.exe`)
			document["launchViaLogonTask"] = startupMode == "task"
			document["trayBehavior"].(map[string]any)["autoMinimizeAndHideOnLaunch"] = startupMode == "tray"
			f := setup(t, "running", document)
			f.click(t, "微信自动登录")
			if !f.page.active().WeChatAutoLogin {
				t.Fatal("option not turned on")
			}
			f.h.Env.Rail.Save.Run()
			f.h.Settle(f.tt)
			if len(f.fake.saved) != 1 {
				t.Fatalf("saved %v", f.fake.saved)
			}
			app := f.fake.saved[0]["managedApps"].([]any)[0].(map[string]any)
			if app["wechatAutoLogin"] != true || mode(*f.page.active()) != startupMode {
				t.Fatalf("saved program %v", app)
			}
		})
	}
}

// Background mode cannot click login UI, but must not erase the user's choice.
func TestWeChatAutoLoginKeepsPreferenceAcrossLaunchModes(t *testing.T) {
	document := programDocument("a", "微信", `C:\Apps\Weixin.exe`)
	document["wechatAutoLogin"] = true
	f := setup(t, "running", document)
	f.click(t, "后台启动")
	if !f.tt.HasText("微信自动登录") || !f.page.active().WeChatAutoLogin {
		t.Fatal("background mode erased or hid the saved choice")
	}
	f.click(t, "微信自动登录")
	if !f.page.active().WeChatAutoLogin {
		t.Fatal("background mode allowed editing auto-login")
	}
	f.click(t, "正常启动")
	f.click(t, "微信自动登录")
	if f.page.active().WeChatAutoLogin {
		t.Fatal("normal mode did not allow changing auto-login")
	}
}
