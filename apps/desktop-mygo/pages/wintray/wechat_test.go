package wintray

import (
	"testing"
)

// The auto-login option belongs to the WeChat clients only.
func TestWeChatAutoLoginOptionOnlyAppearsForWeChat(t *testing.T) {
	f := setup(t, "running", programDocument("a", "微信", `C:\Apps\Weixin.exe`))
	if !f.tt.HasText("微信自动登录") {
		t.Fatalf("missing option %q", f.tt.Texts())
	}
	g := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	if g.tt.HasText("微信自动登录") {
		t.Fatalf("option shown for another program %q", g.tt.Texts())
	}
}

// The option is saved with the rest of the draft.
func TestWeChatAutoLoginIsSaved(t *testing.T) {
	f := setup(t, "running", programDocument("a", "微信", `C:\Apps\Weixin.exe`))
	_ = f.tt.Click("微信自动登录")
	f.h.Settle(f.tt)
	if !f.page.active().WeChatAutoLogin {
		t.Fatal("option not turned on")
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 {
		t.Fatalf("saved %v", f.fake.saved)
	}
	app := f.fake.saved[0]["managedApps"].([]any)[0].(map[string]any)
	if app["wechatAutoLogin"] != true {
		t.Fatalf("saved program %v", app)
	}
}
