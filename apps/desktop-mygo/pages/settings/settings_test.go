package settings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/desktop"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
	"winone/internal/previewtransfer"
)

// fakeHost answers the settings page's requests. The Host's settings
// carry a field this window does not know, which every save must keep.
type fakeHost struct {
	t        *testing.T
	mu       sync.Mutex
	settings map[string]any
	saved    []map[string]any
	failSave error
	failRead error
	// handlers answers other methods, by name.
	handlers map[string]func(json.RawMessage) (any, error)
}

func newFakeHost(t *testing.T) *fakeHost {
	return &fakeHost{t: t, handlers: map[string]func(json.RawMessage) (any, error){}, settings: map[string]any{
		"schemaVersion": 4, "revision": 7, "theme": "light", "language": "zh-CN", "defaultPage": "overview",
		"captureShortcut": "Shift+A", "pinShortcut": "", "openShortcut": "Alt+A",
		"startup": map[string]any{"runAtLogon": false, "startMinimized": true}, "future": "kept",
	}}
}

func (f *fakeHost) snapshot() map[string]any {
	return map[string]any{"settings": f.settings, "administrator": false, "modules": []any{}}
}

func (f *fakeHost) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	handler := f.handlers[method]
	f.mu.Unlock()
	if handler != nil {
		return handler(params)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case "snapshot":
		if f.failRead != nil {
			return nil, f.failRead
		}
		return f.snapshot(), nil
	case "settings.save":
		if f.failSave != nil {
			return nil, f.failSave
		}
		var document map[string]any
		if err := json.Unmarshal(params, &document); err != nil {
			f.t.Errorf("save %s", params)
		}
		f.saved = append(f.saved, document)
		next := map[string]any{}
		for key, value := range document {
			next[key] = value
		}
		next["revision"] = document["revision"].(float64) + 1
		f.settings = next
		return next, nil
	case "shortcuts.snapshot":
		return map[string]any{"bindings": []map[string]any{{"id": "open-winone", "shortcut": "Alt+A", "registered": true}}, "lastError": ""}, nil
	case "updates.info":
		return map[string]any{"version": "1.2.3", "configured": false}, nil
	}
	f.t.Errorf("unexpected %s %s", method, params)
	return nil, errors.New("unexpected")
}

func (f *fakeHost) lastSaved() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saved) == 0 {
		return nil
	}
	return f.saved[len(f.saved)-1]
}

func (f *fakeHost) on(method string, handler func(json.RawMessage) (any, error)) {
	f.mu.Lock()
	f.handlers[method] = handler
	f.mu.Unlock()
}

type fixture struct {
	fake      *fakeHost
	h         *testhost.Harness
	page      *page
	tt        *ui.Tester
	refreshed int
}

func setup(t *testing.T) *fixture {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	f := &fixture{fake: newFakeHost(t)}
	f.h = testhost.New(t, f.fake.handle)
	f.h.Env.Snapshot = func() internalhost.Snapshot {
		var snapshot internalhost.Snapshot
		f.fake.mu.Lock()
		data, _ := json.Marshal(f.fake.snapshot())
		f.fake.mu.Unlock()
		_ = json.Unmarshal(data, &snapshot)
		return snapshot
	}
	f.h.Env.Refresh = func() { f.refreshed++ }
	f.page = newPage(f.h.Env)
	f.tt = ui.NewTester(f.page.View, 1000, 900)
	f.h.Settle(f.tt)
	return f
}

func (f *fixture) click(t *testing.T, label string) {
	t.Helper()
	if err := f.tt.Click(label); err != nil {
		t.Fatalf("click %q: %v; texts %q", label, err, f.tt.Texts())
	}
	f.h.Settle(f.tt)
}

func (f *fixture) tab(t *testing.T, label string) { t.Helper(); f.click(t, label) }

func TestTabsGoThroughTheUnsavedChangeGuard(t *testing.T) {
	f := setup(t)
	for _, label := range []string{"常规", "启动与快捷键", "运行权限", "配置迁移", "关于与诊断"} {
		if !f.tt.HasText(label) {
			t.Fatalf("tab %q missing: %q", label, f.tt.Texts())
		}
	}
	requested := 0
	f.h.Env.RequestChange = func(apply func()) { requested++ }
	f.click(t, "运行权限")
	if requested != 1 || f.page.tab != tabGeneral {
		t.Fatalf("the tab switched without the guard: %d %d", requested, f.page.tab)
	}
	f.h.Env.RequestChange = func(apply func()) { apply() }
	f.click(t, "运行权限")
	if f.page.tab != tabPermissions || !f.tt.HasText("当前以普通权限运行") {
		t.Fatalf("permissions tab %q", f.tt.Texts())
	}
}

func TestThemeSavesOnlyItsFieldOverTheCurrentSettings(t *testing.T) {
	f := setup(t)
	if !f.tt.HasText("外观") || !f.tt.HasText("简体中文") || !f.tt.HasText("起始页面") {
		t.Fatalf("general tab %q", f.tt.Texts())
	}
	// Another page saved meanwhile: the save starts from the Host's value.
	f.fake.mu.Lock()
	f.fake.settings["revision"] = 8.0
	f.fake.settings["openShortcut"] = "Ctrl+Alt+W"
	f.fake.mu.Unlock()
	f.click(t, "深色")
	saved := f.fake.lastSaved()
	if saved == nil || saved["theme"] != "dark" || saved["revision"] != 8.0 || saved["openShortcut"] != "Ctrl+Alt+W" || saved["future"] != "kept" {
		t.Fatalf("saved %v", saved)
	}
	if toasts := f.h.Toasts(); len(toasts) != 1 || toasts[0] != "已保存" || f.refreshed == 0 {
		t.Fatalf("toasts %q, refreshed %d", toasts, f.refreshed)
	}
	if f.page.settings.Theme != "dark" {
		t.Fatalf("page shows %q", f.page.settings.Theme)
	}
}

func TestLanguageChangeIsConfirmedInTheNewLanguage(t *testing.T) {
	f := setup(t)
	f.click(t, "English")
	if saved := f.fake.lastSaved(); saved == nil || saved["language"] != "en-US" {
		t.Fatalf("saved %v", saved)
	}
	if toasts := f.h.Toasts(); len(toasts) != 1 || toasts[0] != "Saved" {
		t.Fatalf("toasts %q", toasts)
	}
}

func TestSavingDisablesThePreferencesAndKeepsThePageBusy(t *testing.T) {
	f := setup(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.fake.on("settings.save", func(params json.RawMessage) (any, error) {
		close(entered)
		<-release
		var document map[string]any
		_ = json.Unmarshal(params, &document)
		return document, nil
	})
	if err := f.tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	<-entered
	f.tt.Frame()
	if !f.tt.HasText("正在保存…") {
		t.Fatalf("saving %q", f.tt.Texts())
	}
	if _, busy := f.h.Env.Activity.State(); !busy {
		t.Fatal("leaving the page would not wait for the save")
	}
	// The other preferences wait for it: a second choice sends nothing.
	if err := f.tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	close(release)
	f.h.Settle(f.tt)
	if n := len(f.h.Called("settings.save")); n != 1 {
		t.Fatalf("saves %d", n)
	}
	if _, busy := f.h.Env.Activity.State(); busy || f.tt.HasText("正在保存…") {
		t.Fatalf("after saving %q", f.tt.Texts())
	}
}

func TestFailedPreferenceOffersReconnectAndMarksTheConnectionLost(t *testing.T) {
	f := setup(t)
	f.fake.failSave = testhost.Error{Code: "conflict", Message: "设置已被其他页面修改"}
	f.click(t, "English")
	if !f.tt.HasText("操作未完成") || !f.tt.HasText("设置已被其他页面修改 (conflict)") || !f.tt.HasText("重试连接") {
		t.Fatalf("failure %q", f.tt.Texts())
	}
	f.tab(t, "关于与诊断")
	if !f.tt.HasText("未连接") {
		t.Fatalf("connection %q", f.tt.Texts())
	}
	f.fake.failSave = nil
	f.click(t, "重试连接")
	if f.tt.HasText("操作未完成") || !f.tt.HasText("已连接") {
		t.Fatalf("after reconnect %q", f.tt.Texts())
	}
}

func TestSidebarStartupFollowsTheShellAndReportsAFailedWrite(t *testing.T) {
	f := setup(t)
	fail := errors.New("disk full")
	stored := "collapsed"
	f.h.Env.SidebarStartup = func() string { return stored }
	f.h.Env.SetSidebarStartup = func(value string) error {
		if fail != nil {
			return fail
		}
		stored = value
		return nil
	}
	f.click(t, "显示图标和名称")
	if stored != "collapsed" || !f.tt.HasText("侧栏设置保存失败，请重试。") {
		t.Fatalf("failed write: %q %q", stored, f.tt.Texts())
	}
	fail = nil
	f.click(t, "显示图标和名称")
	if stored != "expanded" || f.tt.HasText("侧栏设置保存失败，请重试。") {
		t.Fatalf("retry: %q %q", stored, f.tt.Texts())
	}
	if len(f.fake.saved) != 0 {
		t.Fatal("the sidebar is the window's own preference, not the Host's")
	}
}

func TestStartPageListsTheSidebarAndSavesTheChoice(t *testing.T) {
	f := setup(t)
	f.click(t, "首页")
	for _, page := range []string{"截图助手", "程序管理", "Chat", "AI 服务", "设置"} {
		if !f.tt.HasText(page) {
			t.Fatalf("start page choices miss %q: %q", page, f.tt.Texts())
		}
	}
	f.click(t, "程序管理")
	if saved := f.fake.lastSaved(); saved == nil || saved["defaultPage"] != "wintray" {
		t.Fatalf("saved %v", saved)
	}
}

func TestStartupSwitchesMergeIntoTheCurrentPreferences(t *testing.T) {
	f := setup(t)
	f.tab(t, "启动与快捷键")
	if !f.tt.HasText("打开 WinOne") || !f.tt.HasText("Alt+A") {
		t.Fatalf("startup tab %q", f.tt.Texts())
	}
	f.click(t, "登录 Windows 时启动")
	startup, _ := f.fake.lastSaved()["startup"].(map[string]any)
	if startup["runAtLogon"] != true || startup["startMinimized"] != true {
		t.Fatalf("startup %v", startup)
	}
	if _, set := startup["keepUIRunning"]; set {
		t.Fatalf("an untouched default was written: %v", startup)
	}
	// Keeping the interface is on when never set; turning it off stores it.
	f.click(t, "保持界面后台运行")
	startup, _ = f.fake.lastSaved()["startup"].(map[string]any)
	if startup["keepUIRunning"] != false || startup["runAtLogon"] != true {
		t.Fatalf("startup %v", startup)
	}
}

func TestAdministratorPreferenceRunsTheHelperAndKeepsItsResult(t *testing.T) {
	f := setup(t)
	var actions []desktop.PermissionAction
	result := desktop.PermissionResult{}
	var failure error
	runPermission = func(_ *core.Env, action desktop.PermissionAction, guard func() bool) (desktop.PermissionResult, error) {
		actions = append(actions, action)
		return result, failure
	}
	t.Cleanup(func() {
		runPermission = func(env *core.Env, action desktop.PermissionAction, guard func() bool) (desktop.PermissionResult, error) {
			return desktop.RunPermission(env.Host, env.Directory, action, guard)
		}
	})
	f.tab(t, "运行权限")
	settings := map[string]any{"schemaVersion": 4, "revision": 9, "theme": "light", "captureShortcut": "", "pinShortcut": "",
		"startup": map[string]any{"runAtLogon": false, "startMinimized": true, "preferAdministrator": true}}
	result.Settings, _ = json.Marshal(settings)
	f.click(t, "始终以管理员身份运行")
	if len(actions) != 1 || actions[0].Restart || !actions[0].Enabled {
		t.Fatalf("actions %+v", actions)
	}
	if !f.tt.HasText("重新启动 WinOne 后生效。") || f.h.Toasts()[0] != "已保存" {
		t.Fatalf("after remembering %q", f.tt.Texts())
	}
	failure = &host.Error{Code: "permission", Message: "此配置是独立测试配置，不能修改管理员启动"}
	f.click(t, "以管理员身份重新启动")
	if !f.tt.HasText("此配置是独立测试配置，不能修改管理员启动 (permission)") || f.page.restarting {
		t.Fatalf("restart failure %q", f.tt.Texts())
	}
}

func TestPermissionTextsFollowTheCurrentRights(t *testing.T) {
	f := setup(t)
	read := f.h.Env.Snapshot
	// Before the Host's settings are read, the rights are unknown.
	f.h.Env.Snapshot = func() internalhost.Snapshot { return internalhost.Snapshot{} }
	f.page = newPage(f.h.Env)
	f.tt = ui.NewTester(f.page.View, 1000, 900)
	f.h.Settle(f.tt)
	f.tab(t, "运行权限")
	if !f.tt.HasText("正在读取权限") {
		t.Fatalf("loading %q", f.tt.Texts())
	}
	// An administrator who does not keep the preference runs with
	// standard rights after exiting, and has nothing to restart.
	f.h.Env.Snapshot = func() internalhost.Snapshot { s := read(); s.Administrator = true; return s }
	f.tt.Frame()
	help := strings.Join(f.tt.Texts(), "\n")
	if !f.tt.HasText("当前以管理员身份运行") || !strings.Contains(help, "从托盘退出后，下次启动将使用普通权限。") {
		t.Fatalf("administrator %q", f.tt.Texts())
	}
	runPermission = func(*core.Env, desktop.PermissionAction, func() bool) (desktop.PermissionResult, error) {
		t.Error("a restart ran as administrator")
		return desktop.PermissionResult{}, nil
	}
	t.Cleanup(func() {
		runPermission = func(env *core.Env, action desktop.PermissionAction, guard func() bool) (desktop.PermissionResult, error) {
			return desktop.RunPermission(env.Host, env.Directory, action, guard)
		}
	})
	f.click(t, "以管理员身份重新启动")
}

func TestRestartClosesTheWindowOnlyAfterTheHandoff(t *testing.T) {
	f := setup(t)
	closing := false
	runPermission = func(*core.Env, desktop.PermissionAction, func() bool) (desktop.PermissionResult, error) {
		return desktop.PermissionResult{Closing: closing, Token: strings.Repeat("a", 32)}, nil
	}
	exited := 0
	exitWindow = func() error { exited++; return nil }
	t.Cleanup(func() { exitWindow = desktop.ExitWindow })
	f.tab(t, "运行权限")
	f.click(t, "以管理员身份重新启动")
	if exited != 0 || f.page.restarting || len(f.h.Called("snapshot")) < 2 {
		t.Fatalf("a restart that was not needed closed the window: %d, calls %+v", exited, f.h.Calls())
	}
	closing = true
	f.click(t, "以管理员身份重新启动")
	if exited != 1 || !f.page.closing {
		t.Fatalf("exited %d", exited)
	}
	if _, busy := f.h.Env.Activity.State(); busy {
		t.Fatal("a closing window must not wait for itself")
	}
	if !f.tt.HasText("正在重新启动") {
		t.Fatalf("restarting %q", f.tt.Texts())
	}
}

func TestAdministratorActionsWaitForUnsavedShortcuts(t *testing.T) {
	f := setup(t)
	f.page.shortcuts.Report()
	f.h.Env.Activity.Report("shortcuts:application", true, false, nil)
	f.tab(t, "运行权限")
	called := false
	runPermission = func(*core.Env, desktop.PermissionAction, func() bool) (desktop.PermissionResult, error) {
		called = true
		return desktop.PermissionResult{}, nil
	}
	f.page.restart()
	f.page.setAdministrator(true)
	if called {
		t.Fatal("an administrator action ran over an unsaved draft")
	}
}

func TestExportAddsTheExtensionAndReportsTheFile(t *testing.T) {
	f := setup(t)
	directory := t.TempDir()
	var exported map[string]string
	f.fake.on("profiles.export", func(params json.RawMessage) (any, error) {
		_ = json.Unmarshal(params, &exported)
		return map[string]any{"path": exported["path"], "bytes": 10}, nil
	})
	f.tab(t, "配置迁移")
	f.click(t, "导出到文件")
	if len(f.h.Called("profiles.export")) != 0 || f.tt.HasText("已导出") {
		t.Fatal("a cancelled dialog exported")
	}
	f.h.Picked["导出配置"] = filepath.Join(directory, "家里")
	f.click(t, "导出到文件")
	want := filepath.Join(directory, "家里.winone")
	if exported["path"] != want || exported["name"] != "我的配置" || !f.tt.HasText("已导出："+want) {
		t.Fatalf("exported %v, texts %q", exported, f.tt.Texts())
	}
	// A name typed without the extension never replaces a file unasked.
	if err := os.WriteFile(want, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.click(t, "导出到文件")
	if len(f.h.Called("profiles.export")) != 1 || !f.tt.HasText(want+" 已存在。请重新导出，选择该文件并确认替换。") {
		t.Fatalf("calls %d, texts %q", len(f.h.Called("profiles.export")), f.tt.Texts())
	}
}

// preview is a reviewed archive: capture with a folder to map, and a
// shortcut that conflicts with one kept on this computer.
func preview(mapped bool) map[string]any {
	state, value := "needs_mapping", ""
	if mapped {
		state, value = "ready", `D:\Images`
	}
	return map[string]any{
		"name": "家用配置", "sourceHash": "reviewed-hash", "configurationRevision": 4, "moduleRevision": 2,
		"preferences": map[string]any{"theme": "dark", "language": "zh-CN", "runAtLogon": false, "startMinimized": true,
			"enabled": map[string]any{"capture": false}, "shortcuts": map[string]any{"capture.region": "Ctrl+Alt+T"}},
		"currentShortcuts": map[string]any{"capture.region": "Ctrl+Alt+S", "always-on-top.toggle": "alt+ctrl+t"},
		"plan": map[string]any{"modules": []any{map[string]any{"id": "capture", "state": state, "requiredPaths": []string{"capture.output"},
			"settings": map[string]any{"openEditor": true, "scrollDelay": 300, "outputDirectoryRef": "capture.output"}}}, "shortcutIssues": []any{}},
		"paths": []any{map[string]any{"id": "capture.output", "label": "截图保存目录", "kind": "directory", "state": state, "value": value}},
	}
}

// serveImport answers the chunked preview transfer with documents chosen
// by the requested mappings, and records the apply request.
func serveImport(t *testing.T, f *fixture, fail map[string]error) *map[string]any {
	var pending []byte
	var applied map[string]any
	released := 0
	f.fake.on("profiles.import.preview.start", func(params json.RawMessage) (any, error) {
		var request struct {
			Path     string            `json:"path"`
			Bindings map[string]string `json:"bindings"`
		}
		_ = json.Unmarshal(params, &request)
		if err := fail[request.Path]; err != nil {
			return nil, err
		}
		pending, _ = json.Marshal(preview(request.Bindings["capture.output"] != ""))
		return descriptor(pending), nil
	})
	f.fake.on("profiles.import.preview.read", func(params json.RawMessage) (any, error) {
		var request struct {
			Token  string `json:"token"`
			Offset int    `json:"offset"`
		}
		_ = json.Unmarshal(params, &request)
		end := min(request.Offset+512, len(pending))
		return map[string]any{"token": request.Token, "offset": request.Offset, "data": pending[request.Offset:end]}, nil
	})
	f.fake.on("profiles.import.preview.release", func(json.RawMessage) (any, error) { released++; return map[string]any{}, nil })
	f.fake.on("profiles.import.apply", func(params json.RawMessage) (any, error) {
		_ = json.Unmarshal(params, &applied)
		return map[string]any{"revision": 5}, nil
	})
	t.Cleanup(func() {
		if released != len(f.h.Called("profiles.import.preview.read")) && released == 0 {
			t.Error("a preview transfer was never released")
		}
	})
	return &applied
}

func TestImportReviewsMapsAndAppliesTheArchive(t *testing.T) {
	f := setup(t)
	applied := serveImport(t, f, nil)
	f.tab(t, "配置迁移")
	f.h.Picked["选择配置文件"] = `C:\Backup\Home.winone`
	f.click(t, "选择配置文件")
	for _, text := range []string{"家用配置", `C:\Backup\Home.winone`, "全局偏好", "截图助手", "需要选择位置 · 导入后关闭", "多个操作使用了相同快捷键"} {
		if !f.tt.HasText(text) {
			t.Fatalf("review misses %q: %q", text, f.tt.Texts())
		}
	}
	if len(f.h.Called("profiles.import.preview.read")) < 2 || len(f.h.Called("profiles.import.preview.release")) != 1 {
		t.Fatalf("transfer calls %+v", f.h.Calls())
	}
	// The conflict involves the imported capture shortcut: leaving
	// capture out removes it, and the import applies the preferences alone.
	f.click(t, "截图助手")
	if f.tt.HasText("多个操作使用了相同快捷键") {
		t.Fatalf("conflict kept %q", f.tt.Texts())
	}
	f.click(t, "截图助手")
	f.click(t, "设置本机路径")
	f.h.Picked["选择文件夹"] = `D:\Images`
	f.click(t, "选择截图保存目录")
	if !f.tt.HasText(`D:\Images`) {
		t.Fatalf("mapping %q", f.tt.Texts())
	}
	// The review and its locations stay while another tab is shown.
	f.tab(t, "常规")
	f.tab(t, "配置迁移")
	if !f.tt.HasText(`D:\Images`) || !f.tt.HasText("家用配置") {
		t.Fatalf("after switching tabs %q", f.tt.Texts())
	}
	f.click(t, "导入内容")
	f.click(t, "截图助手")
	f.click(t, "确认导入")
	if *applied == nil || (*applied)["sourceHash"] != "reviewed-hash" || (*applied)["global"] != true || len((*applied)["modules"].([]any)) != 0 {
		t.Fatalf("applied %v", *applied)
	}
	if !f.tt.HasText("配置已导入") || f.page.importer.preview != nil || f.refreshed == 0 {
		t.Fatalf("after import %q", f.tt.Texts())
	}
}

func TestDroppedArchiveIsReviewedAndFoldersAreRefused(t *testing.T) {
	f := setup(t)
	serveImport(t, f, nil)
	f.tab(t, "配置迁移")
	directory := t.TempDir()
	archive := filepath.Join(directory, "Home.winone")
	if err := os.WriteFile(archive, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	at, _ := f.tt.Find("选择配置文件")
	f.tt.DropFiles(at.X+at.W/2, at.Y+at.H/2, directory)
	f.h.Settle(f.tt)
	if len(f.h.Called("profiles.import.preview.start")) != 0 || !f.tt.HasText("此处只接收文件，原内容已保留。") {
		t.Fatalf("folder %q", f.tt.Texts())
	}
	f.tt.DropFiles(at.X+at.W/2, at.Y+at.H/2, archive)
	f.h.Settle(f.tt)
	starts := f.h.Called("profiles.import.preview.start")
	if len(starts) != 1 || !strings.Contains(string(starts[0].Params), "Home.winone") || !f.tt.HasText("家用配置") {
		t.Fatalf("starts %+v, texts %q", starts, f.tt.Texts())
	}
}

func TestReplacementFailureKeepsTheReviewUntilCheckedAgain(t *testing.T) {
	f := setup(t)
	serveImport(t, f, map[string]error{`C:\Broken.winone`: testhost.Error{Code: "profiles", Message: "无法读取配置文件"}})
	f.tab(t, "配置迁移")
	f.h.Picked["选择配置文件"] = `C:\Home.winone`
	f.click(t, "选择配置文件")
	f.click(t, "全局偏好")
	f.h.Picked["选择配置文件"] = `C:\Broken.winone`
	f.click(t, "选择配置文件")
	if !f.tt.HasText("无法读取配置文件 (profiles)") || !f.tt.HasText("家用配置") || f.page.importer.global || !f.page.importer.needsReview {
		t.Fatalf("after a failed replacement %q", f.tt.Texts())
	}
	if dirty, _ := f.h.Env.Activity.State(); !dirty {
		t.Fatal("a pending review is an unsaved change")
	}
	f.click(t, "重新预览")
	if f.page.importer.needsReview || f.page.importer.source != `C:\Home.winone` {
		t.Fatalf("review again %+v", f.page.importer)
	}
	f.click(t, "取消")
	if dirty, _ := f.h.Env.Activity.State(); dirty || f.tt.HasText("家用配置") {
		t.Fatal("cancelling kept the review")
	}
}

func TestDetailsShowTheModuleSettingsInTheArchiveOrder(t *testing.T) {
	f := setup(t)
	serveImport(t, f, nil)
	f.tab(t, "配置迁移")
	f.h.Picked["选择配置文件"] = `C:\Home.winone`
	f.click(t, "选择配置文件")
	f.click(t, "详细设置")
	for _, text := range []string{"模块", "完成截图后", "截图后编辑", "300", "待选择位置", "快捷键：Ctrl+Alt+T"} {
		if !f.tt.HasText(text) {
			t.Fatalf("details miss %q: %q", text, f.tt.Texts())
		}
	}
	editor, _ := f.tt.Find("完成截图后")
	delay, _ := f.tt.Find("300")
	if editor.Y > delay.Y {
		t.Fatal("details lost the archive's order")
	}
}

func TestAboutReadsVersionAndLogsWhenFirstOpened(t *testing.T) {
	f := setup(t)
	if len(f.h.Called("updates.info")) != 0 {
		t.Fatal("updates read before the tab opened")
	}
	logs := filepath.Join(f.h.Env.Directory, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "host.log"), []byte(`{"event":"started"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var opened []string
	shellOpen = func(path string) error { opened = append(opened, path); return nil }
	t.Cleanup(func() { shellOpen = desktop.ShellOpen })
	f.tab(t, "关于与诊断")
	for _, text := range []string{"当前版本：1.2.3", "此版本暂未提供在线更新。", "已连接", "WinOne 日志"} {
		if !f.tt.HasText(text) {
			t.Fatalf("about misses %q: %q", text, f.tt.Texts())
		}
	}
	if *f.page.logs.texts["process"] != `{"event":"started"}`+"\n" {
		t.Fatalf("log %q", *f.page.logs.texts["process"])
	}
	f.click(t, "打开日志文件夹")
	if len(opened) != 1 || opened[0] != logs {
		t.Fatalf("opened %v", opened)
	}
	f.tab(t, "常规")
	f.tab(t, "关于与诊断")
	if len(f.h.Called("updates.info")) != 1 {
		t.Fatal("reopening the tab read again")
	}
}

func TestMissingLogsShowEmptyWithoutAFolder(t *testing.T) {
	f := setup(t)
	f.tab(t, "关于与诊断")
	if !f.tt.HasText("暂无日志") {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	if f.page.logs.data.FolderAvailable {
		t.Fatal("a missing folder was offered")
	}
}

func TestUpdateCheckFollowsTheHostAndOpensOnlyItsRelease(t *testing.T) {
	f := setup(t)
	states := []string{"checking", "available"}
	var id string
	f.fake.on("updates.info", func(json.RawMessage) (any, error) { return map[string]any{"version": "1.2.3", "configured": true}, nil })
	f.fake.on("updates.check", func(params json.RawMessage) (any, error) {
		var request struct{ ID string }
		_ = json.Unmarshal(params, &request)
		id = request.ID
		return map[string]any{"id": id, "state": states[0], "currentVersion": "1.2.3"}, nil
	})
	f.fake.on("updates.status", func(json.RawMessage) (any, error) {
		return map[string]any{"id": id, "state": states[1], "currentVersion": "1.2.3", "latestVersion": "1.3.0"}, nil
	})
	var released []string
	f.fake.on("updates.release", func(params json.RawMessage) (any, error) {
		var request struct{ ID string }
		_ = json.Unmarshal(params, &request)
		released = append(released, request.ID)
		return map[string]any{"url": "https://github.com/BIGFA/WinOne/releases/tag/v1.3.0"}, nil
	})
	var opened []string
	shellOpen = func(path string) error { opened = append(opened, path); return nil }
	t.Cleanup(func() { shellOpen = desktop.ShellOpen })
	f.tab(t, "关于与诊断")
	f.click(t, "检查更新")
	if !f.tt.HasText("发现新版本：1.3.0") {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	f.click(t, "查看发行说明")
	if len(released) != 1 || released[0] != id || len(opened) != 1 || opened[0] != "https://github.com/BIGFA/WinOne/releases/tag/v1.3.0" {
		t.Fatalf("released %v, opened %v", released, opened)
	}
	// A release page anywhere else is never opened.
	f.fake.on("updates.release", func(json.RawMessage) (any, error) {
		return map[string]any{"url": "https://example.com/v1.3.0"}, nil
	})
	f.click(t, "查看发行说明")
	if len(opened) != 1 || !f.tt.HasText("无法打开发行页面，请重试。") {
		t.Fatalf("opened %v, texts %q", opened, f.tt.Texts())
	}
}

func TestUpdateErrorsAndCancel(t *testing.T) {
	f := setup(t)
	f.fake.on("updates.info", func(json.RawMessage) (any, error) { return map[string]any{"version": "1.2.3", "configured": true}, nil })
	f.fake.on("updates.check", func(params json.RawMessage) (any, error) {
		return map[string]any{"state": "failed", "errorCode": "rate_limited"}, nil
	})
	f.tab(t, "关于与诊断")
	f.click(t, "检查更新")
	if !f.tt.HasText("检查过于频繁，请稍后重试。") {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	// Leaving the page cancels a check still running.
	f.page.updates.id = "pending-check"
	f.page.updates.checking = true
	cancelled := make(chan string, 1)
	f.fake.on("updates.cancel", func(params json.RawMessage) (any, error) {
		var request struct{ ID string }
		_ = json.Unmarshal(params, &request)
		cancelled <- request.ID
		return map[string]any{}, nil
	})
	f.page.Close()
	if id := <-cancelled; id != "pending-check" {
		t.Fatalf("cancelled %q", id)
	}
}

// descriptor announces a preview the page reads in chunks.
func descriptor(data []byte) previewtransfer.Descriptor {
	sum := sha256.Sum256(data)
	return previewtransfer.Descriptor{Token: strings.Repeat("ab", 16), Size: len(data), SHA256: hex.EncodeToString(sum[:])}
}
