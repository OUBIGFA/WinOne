package wintray

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/desktopmygo/i18n"
)

func TestQQLoginPreferenceAndDelay(t *testing.T) {
	f := setup(t, "running", programDocument("a", "QQ", `C:\Apps\QQ.exe`))
	f.click(t, "QQ 登录后收起")
	if !f.page.active().QQWaitForLogin || f.tt.HasText(i18n.T("wintray.closeDelay")) {
		t.Fatal("QQ login wait did not replace the timed delay")
	}
	if f.page.active().TrayBehavior.Value.CloseDelaySeconds != 3 {
		t.Fatal("saved delay was erased")
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 || f.fake.saved[0]["managedApps"].([]any)[0].(map[string]any)["qqWaitForLogin"] != true {
		t.Fatal("QQ choice not saved")
	}
	f.click(t, "正常启动")
	f.click(t, "QQ 登录后收起")
	if !f.page.active().QQWaitForLogin {
		t.Fatal("non-tray mode changed the saved preference")
	}
	f.click(t, "收进托盘")
	f.click(t, "QQ 登录后收起")
	if !f.tt.HasText(i18n.T("wintray.closeDelay")) || f.page.active().TrayBehavior.Value.CloseDelaySeconds != 3 {
		t.Fatal("turning off login wait lost the original delay")
	}
}

func TestQQOptionIsExecutableScoped(t *testing.T) {
	for _, name := range []string{"QQ.exe", "qq.EXE", "QQHelper.exe", "Weixin.exe"} {
		f := setup(t, "running", programDocument("a", "QQ", `C:\Apps\`+name))
		if f.tt.HasText("QQ 登录后收起") != (name == "QQ.exe" || name == "qq.EXE") {
			t.Fatalf("QQ option for %s", name)
		}
	}
}

func TestWorkingDirectoryEditBrowseCancelSaveAndUndo(t *testing.T) {
	f := setup(t, "running", programDocument("a", "工具", `C:\Apps\tool.exe`))
	f.click(t, i18n.T("wintray.moreSettings"))
	f.click(t, "工作目录")
	f.tt.Type(`..\工作 data`)
	f.h.Settle(f.tt)
	if f.page.active().WorkingDirectory != `..\工作 data` {
		t.Fatalf("typed folder lost: %q", f.page.active().WorkingDirectory)
	}
	f.click(t, "浏览")
	if f.page.active().WorkingDirectory != `..\工作 data` {
		t.Fatal("cancelled picker cleared draft")
	}
	f.h.Picked["工作目录"] = `D:\工作 data`
	f.click(t, "浏览")
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 || f.page.active().WorkingDirectory != `D:\工作 data` {
		t.Fatal("chosen folder not saved/reloaded")
	}
	saved := f.fake.saved[0]["managedApps"].([]any)[0].(map[string]any)
	if saved["workingDirectory"] != `D:\工作 data` || saved["future"] != "kept" {
		t.Fatalf("save lost fields: %v", saved)
	}
	f.click(t, "工作目录")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type(`D:\other`)
	f.h.Settle(f.tt)
	f.h.Env.Rail.Undo.Run()
	f.h.Settle(f.tt)
	if f.page.active().WorkingDirectory != `D:\工作 data` {
		t.Fatal("undo did not restore saved folder")
	}
}
