package wintray

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/desktop"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
)

func programDocument(id, name, path string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "exePath": path, "args": "--quiet", "runOnStartup": true, "collectTrayIcon": false,
		"launchViaLogonTask": false, "launchHiddenInBackground": false,
		"trayBehavior": map[string]any{"autoMinimizeAndHideOnLaunch": true, "closeDelaySeconds": 3},
		"schedule": map[string]any{"enabled": false, "startDelayMinutes": 0, "autoExitMinutes": 30, "frequencyEnabled": false,
			"frequencyDays": 1, "frequencyRuns": 1},
		"future": "kept",
	}
}

// fakeHost is the WinTray module behind the Host: its settings with a
// revision, the worker's errors, and the operations the page runs.
type fakeHost struct {
	t        *testing.T
	mu       sync.Mutex
	settings map[string]any
	revision int
	errors   map[string]string
	saved    []map[string]any
	failSave error
	running  bool
	jobs     map[string]func(params json.RawMessage) (any, error)
	pending  int
}

func newFakeHost(t *testing.T, apps ...map[string]any) *fakeHost {
	list := []any{}
	for _, app := range apps {
		list = append(list, app)
	}
	return &fakeHost{t: t, revision: 1, errors: map[string]string{}, jobs: map[string]func(json.RawMessage) (any, error){},
		settings: map[string]any{"schemaVersion": 3, "language": "zh-CN", "runAtLogon": false, "startMinimizedToTray": true,
			"exitAfterManagedAppsCompleted": false, "closeWindowRetrySeconds": 10, "startupIntervalSeconds": 3, "managedApps": list,
			"future": "kept"}}
}

func (f *fakeHost) snapshot() map[string]any {
	value := map[string]any{"revision": itoa(f.revision), "settings": f.settings}
	for key, message := range f.errors {
		value[key] = message
	}
	return value
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func (f *fakeHost) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	job := f.jobs[method]
	f.mu.Unlock()
	if job != nil {
		return job(params)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case "wintray.snapshot", "wintray.settings.get":
		return f.snapshot(), nil
	case "wintray.settings.save":
		if f.failSave != nil {
			return nil, f.failSave
		}
		var request struct {
			Revision string         `json:"revision"`
			Settings map[string]any `json:"settings"`
		}
		if err := json.Unmarshal(params, &request); err != nil || request.Revision != itoa(f.revision) {
			f.t.Errorf("save %s", params)
		}
		f.saved = append(f.saved, request.Settings)
		f.settings, f.revision = request.Settings, f.revision+1
		return map[string]any{"state": "completed", "result": f.snapshot()}, nil
	case "wintray.program.state":
		return map[string]any{"running": f.running, "hasWindow": f.running, "visible": false}, nil
	}
	f.t.Errorf("unexpected %s %s", method, params)
	return nil, errors.New("unexpected")
}

func (f *fakeHost) on(method string, job func(json.RawMessage) (any, error)) {
	f.mu.Lock()
	f.jobs[method] = job
	f.mu.Unlock()
}

type fixture struct {
	fake      *fakeHost
	h         *testhost.Harness
	page      *page
	tt        *ui.Tester
	refreshed int
	module    string
}

func setup(t *testing.T, state string, apps ...map[string]any) *fixture {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	programInfo = func(_, path string) (desktop.ProgramMetadata, error) {
		if strings.Contains(path, "broken") {
			return desktop.ProgramMetadata{}, errors.New("无法读取 " + path)
		}
		return desktop.ProgramMetadata{Name: "识别" + stem(path)}, nil
	}
	t.Cleanup(func() { programInfo = desktop.ProgramInfo })
	f := &fixture{fake: newFakeHost(t, apps...), module: state}
	f.h = testhost.New(t, f.fake.handle)
	f.h.Env.Snapshot = func() internalhost.Snapshot {
		return internalhost.Snapshot{Modules: []internalhost.Module{{ID: "wintray", State: f.module}}}
	}
	f.h.Env.Refresh = func() { f.refreshed++ }
	f.page = newPage(f.h.Env)
	f.tt = ui.NewTester(f.page.View, 1300, 900)
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

func (f *fixture) names() []string {
	var names []string
	for _, app := range f.page.apps() {
		names = append(names, app.Value.Name)
	}
	return names
}

func TestDisabledModuleReadsItsConfiguration(t *testing.T) {
	f := setup(t, "disabled")
	if len(f.h.Called("wintray.settings.get")) != 1 || len(f.h.Called("wintray.snapshot")) != 0 {
		t.Fatalf("calls %+v", f.h.Calls())
	}
	for _, text := range []string{"模块已关闭，设置将在开启后生效。", "还没有程序，添加后可设置启动方式", "选择一个程序编辑设置", "程序", "收纳", "更换", "顺序"} {
		if !f.tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, f.tt.Texts())
		}
	}
	// Turned on elsewhere: the worker's snapshot is read instead.
	f.module = "running"
	f.tt.Frame()
	f.h.Settle(f.tt)
	if len(f.h.Called("wintray.snapshot")) != 1 || f.tt.HasText("模块已关闭，设置将在开启后生效。") {
		t.Fatalf("after enabling %+v", f.h.Calls())
	}
}

func TestAddedProgramsTakeTheirDefaultsAndTheLastIsSelected(t *testing.T) {
	f := setup(t, "running")
	f.h.Picked["选择程序或脚本"] = []string{`C:\Apps\Player.exe`, `C:\Tools\backup.ps1`}
	f.click(t, "添加程序")
	apps := f.page.apps()
	if len(apps) != 2 || f.page.selected != apps[1].Value.ID {
		t.Fatalf("apps %+v selected %q", apps, f.page.selected)
	}
	exe, script := apps[0].Value, apps[1].Value
	if exe.Name != "识别Player" || mode(exe) != "tray" || !exe.RunOnStartup || exe.Schedule.Value.AutoExitMinutes != 30 || exe.Schedule.Value.FrequencyRuns != 1 {
		t.Fatalf("program %+v", exe)
	}
	if mode(script) != "hidden" || script.TrayBehavior.Value.AutoMinimizeAndHideOnLaunch {
		t.Fatalf("script %+v", script)
	}
	if !f.tt.HasText("有未保存的修改") {
		t.Fatalf("status %q", f.tt.Texts())
	}
	if dirty, _ := f.h.Env.Activity.State(); !dirty {
		t.Fatal("added programs are unsaved")
	}
	// One unreadable file adds none.
	f.h.Picked["选择程序或脚本"] = []string{`C:\Apps\ok.exe`, `C:\Apps\broken.exe`}
	f.click(t, "添加程序")
	if len(f.page.apps()) != 2 || !f.tt.HasText(`无法读取 C:\Apps\broken.exe`) {
		t.Fatalf("apps %d, texts %q", len(f.page.apps()), f.tt.Texts())
	}
}

func TestSaveSendsTheWholeDraftOnceAndReadsAgain(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.click(t, "启动与计划")
	f.click(t, "开机启动此程序")
	if f.h.Env.Rail.Save.Disabled {
		t.Fatal("an edit left Save unavailable")
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	saved := f.fake.saved
	if len(saved) != 1 || saved[0]["future"] != "kept" {
		t.Fatalf("saved %v", saved)
	}
	app := saved[0]["managedApps"].([]any)[0].(map[string]any)
	if app["runOnStartup"] != false || app["future"] != "kept" || app["args"] != "--quiet" {
		t.Fatalf("program %v", app)
	}
	if toasts := f.h.Toasts(); len(toasts) != 1 || toasts[0] != "已保存" {
		t.Fatalf("toasts %q", toasts)
	}
	if len(f.h.Called("wintray.snapshot")) != 2 || f.page.dirty() {
		t.Fatal("the save was not read back")
	}
}

func TestFailedSaveKeepsTheDraftAndOffersDiscard(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.click(t, "移除程序")
	f.fake.failSave = testhost.Error{Code: "conflict", Message: "配置已变化"}
	f.fake.errors["startupError"] = "登录任务失败"
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.page.apps()) != 0 || !f.tt.HasText("配置已变化 (conflict)") || !f.tt.HasText("登录任务失败") {
		t.Fatalf("after failure %q", f.tt.Texts())
	}
	f.click(t, "放弃修改并刷新")
	if len(f.page.apps()) != 1 || f.page.dirty() {
		t.Fatal("discarding kept the draft")
	}
}

func TestInvalidNumbersStayAsTypedAndBlockSaving(t *testing.T) {
	f := setup(t, "running")
	f.click(t, "启动设置")
	if err := f.tt.Click("程序之间的启动间隔（秒）"); err != nil {
		t.Fatal(err)
	}
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("121")
	f.h.Settle(f.tt)
	if !f.tt.HasText("请输入 0–120 的整数") || !f.tt.HasText("请检查数值：启动设置") || !f.h.Env.Rail.Save.Disabled {
		t.Fatalf("texts %q, save %+v", f.tt.Texts(), f.h.Env.Rail.Save)
	}
	if f.page.draft.Value.StartupIntervalSeconds != 3 {
		t.Fatal("an invalid number reached the draft")
	}
	// The summary stays on the other tab, and Save stays unavailable.
	f.click(t, "程序列表")
	if !f.tt.HasText("请检查数值：启动设置") || !f.h.Env.Rail.Save.Disabled {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	f.click(t, "启动设置")
	if err := f.tt.Click("程序之间的启动间隔（秒）"); err != nil {
		t.Fatal(err)
	}
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("8")
	f.h.Settle(f.tt)
	if f.tt.HasText("请检查数值：启动设置") || f.h.Env.Rail.Save.Disabled || f.page.draft.Value.StartupIntervalSeconds != 8 {
		t.Fatalf("corrected %q", f.tt.Texts())
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 || f.fake.saved[0]["startupIntervalSeconds"] != 8.0 {
		t.Fatalf("saved %v", f.fake.saved)
	}
}

func TestEmptyNameBlocksSavingAcrossTabs(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.click(t, "程序信息")
	if err := f.tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Key(0, ui.KeyBackspace)
	f.h.Settle(f.tt)
	if f.page.active().Name != "" || !f.tt.HasText("请在程序信息中填写名称：识别player") || !f.h.Env.Rail.Save.Disabled {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	f.page.save()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 0 {
		t.Fatal("a program without a name was saved")
	}
}

func TestProgramsReorderByDraggingAndByPosition(t *testing.T) {
	f := setup(t, "running", programDocument("1", "甲", `C:\Apps\1.exe`), programDocument("2", "乙", `C:\Apps\2.exe`), programDocument("3", "丙", `C:\Apps\3.exe`))
	from, _ := f.tt.Find("丙")
	to, _ := f.tt.Find("甲")
	f.tt.Press(from.X+from.W/2, from.Y+from.H/2)
	for step := float32(1); step <= 6; step++ {
		f.tt.Move(from.X+from.W/2, from.Y+from.H/2+(to.Y-from.Y)*step/6)
	}
	f.tt.Release(to.X+to.W/2, to.Y+to.H/2)
	f.h.Settle(f.tt)
	if got := strings.Join(f.names(), ","); got != "丙,甲,乙" {
		t.Fatalf("after dragging %s", got)
	}
	// The position list moves a row from the keyboard: 丙 back to 3.
	f.click(t, "1")
	f.tt.Key(0, ui.KeyEnd)
	f.tt.Key(0, ui.KeyEnter)
	f.h.Settle(f.tt)
	if got := strings.Join(f.names(), ","); got != "甲,乙,丙" {
		t.Fatalf("after choosing a position %s", got)
	}
}

func TestReplacingWithAScriptKeepsIdentityAndSchedule(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.page.draft.Value.ManagedApps[0].Value.CollectTrayIcon = true
	f.h.Picked["选择程序或脚本"] = `C:\Tools\job.cmd`
	f.click(t, "更换 播放器 的程序")
	app := f.page.apps()[0].Value
	if app.ID != "a" || app.Args != "--quiet" || app.ExePath != `C:\Tools\job.cmd` || app.Name != "识别job" || app.CollectTrayIcon ||
		mode(app) != "hidden" || app.Schedule.Value.AutoExitMinutes != 30 {
		t.Fatalf("replaced %+v", app)
	}
	// The same file, whatever its case and slashes, changes nothing.
	before := app
	f.h.Picked["选择程序或脚本"] = `c:/tools/JOB.cmd`
	f.click(t, "更换 识别job 的程序")
	if f.page.apps()[0].Value.ExePath != before.ExePath {
		t.Fatal("the same file replaced the program")
	}
}

func TestRemovingSelectsTheFirstRemainingProgram(t *testing.T) {
	f := setup(t, "running", programDocument("1", "甲", `C:\Apps\1.exe`), programDocument("2", "乙", `C:\Apps\2.exe`))
	// A row is chosen by a click on its name; its switch and buttons
	// keep their own clicks.
	row, _ := f.tt.Find("乙")
	f.tt.ClickAt(row.X+48, row.Y+row.H/2)
	f.h.Settle(f.tt)
	if f.page.selected != "2" {
		t.Fatalf("selected %q", f.page.selected)
	}
	f.click(t, "移除程序")
	if strings.Join(f.names(), ",") != "甲" || f.page.selected != "1" {
		t.Fatalf("after removing %v %q", f.names(), f.page.selected)
	}
}

func TestRunningProgramIsClosedOnlyAfterConfirming(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.fake.mu.Lock()
	f.fake.running = true
	f.fake.mu.Unlock()
	f.page.state.refresh()
	f.tt.Frame()
	f.h.Settle(f.tt)
	if !f.tt.HasText("关闭程序") || !f.tt.HasText("显示窗口") {
		t.Fatalf("actions %q", f.tt.Texts())
	}
	var stopped json.RawMessage
	f.fake.on("wintray.program.stop", func(params json.RawMessage) (any, error) {
		stopped = params
		return map[string]any{"id": "job-1", "state": "running"}, nil
	})
	f.fake.on("wintray.jobs.get", func(json.RawMessage) (any, error) {
		return map[string]any{"id": "job-1", "state": "completed"}, nil
	})
	f.click(t, "关闭程序")
	if stopped != nil || !f.tt.HasText("关闭 播放器？") {
		t.Fatalf("stopped without asking: %s", stopped)
	}
	f.click(t, "确认关闭")
	if string(stopped) != `{"confirmed":true,"id":"a"}` {
		t.Fatalf("stop %s", stopped)
	}
	if toasts := f.h.Toasts(); len(toasts) != 1 || toasts[0] != "程序已关闭" {
		t.Fatalf("toasts %q", toasts)
	}
}

func TestLaunchFollowsItsJobAndReportsFailures(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.fake.on("wintray.launch", func(json.RawMessage) (any, error) { return map[string]any{"state": "running"}, nil })
	f.click(t, "立即启动")
	if !f.tt.HasText("无法确认操作结果") {
		t.Fatalf("a running job without an ID %q", f.tt.Texts())
	}
	f.fake.on("wintray.launch", func(json.RawMessage) (any, error) { return map[string]any{"id": "j", "state": "running"}, nil })
	f.fake.on("wintray.jobs.get", func(json.RawMessage) (any, error) {
		return map[string]any{"id": "j", "state": "failed", "error": "找不到程序文件"}, nil
	})
	f.click(t, "立即启动")
	if !f.tt.HasText("找不到程序文件") {
		t.Fatalf("job failure %q", f.tt.Texts())
	}
	f.fake.on("wintray.jobs.get", func(json.RawMessage) (any, error) { return map[string]any{"id": "j", "state": "completed"}, nil })
	f.click(t, "立即启动")
	if toasts := f.h.Toasts(); len(toasts) == 0 || toasts[len(toasts)-1] != "程序已启动" {
		t.Fatalf("toasts %q", toasts)
	}
}

func TestRecoveryBlocksEditingUntilRecovered(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.fake.errors["recoveryError"] = "上次保存未完成"
	f.page.reload(false, nil)
	f.h.Settle(f.tt)
	if !f.tt.HasText("上次保存未完成") || !f.tt.HasText("恢复上次设置") || !f.page.blocked() {
		t.Fatalf("texts %q", f.tt.Texts())
	}
	f.page.add([]string{`C:\Apps\more.exe`})
	if len(f.page.apps()) != 1 {
		t.Fatal("a program was added while the settings must be recovered")
	}
	recovered := false
	f.fake.on("wintray.recover", func(json.RawMessage) (any, error) {
		f.fake.mu.Lock()
		delete(f.fake.errors, "recoveryError")
		f.fake.mu.Unlock()
		recovered = true
		return map[string]any{"state": "completed"}, nil
	})
	f.click(t, "恢复上次设置")
	if !recovered || f.page.blocked() || f.h.Toasts()[0] != "设置已恢复" {
		t.Fatalf("recovered %v, texts %q", recovered, f.tt.Texts())
	}
}

func TestStartupErrorRetriesTheSignInTask(t *testing.T) {
	f := setup(t, "running")
	f.fake.errors["startupError"] = "登录任务失败"
	f.page.reload(false, nil)
	f.h.Settle(f.tt)
	retried := 0
	f.fake.on("wintray.startup.retry", func(json.RawMessage) (any, error) { retried++; return map[string]any{"state": "completed"}, nil })
	f.click(t, "重试登录任务")
	if retried != 1 {
		t.Fatalf("retried %d", retried)
	}
}

func TestResetAsksWithTheCountAndTurnsTheModuleOff(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.fake.on("modules.resetWinTray.preview", func(json.RawMessage) (any, error) {
		return map[string]any{"configurationRevision": 4, "moduleRevision": 2, "programCount": 1}, nil
	})
	var reset map[string]any
	f.fake.on("modules.resetWinTray", func(params json.RawMessage) (any, error) {
		_ = json.Unmarshal(params, &reset)
		return map[string]any{"state": "completed"}, nil
	})
	f.click(t, "启动设置")
	f.click(t, "清空托管程序并还原")
	if reset != nil || !f.tt.HasText("将还原托管窗口和启动项，清空 1 个程序并关闭此模块。") || !f.tt.HasText("保留 WinOne 登录设置、其他模块、运行历史和恢复资料。") {
		t.Fatalf("dialog %q", f.tt.Texts())
	}
	f.click(t, "确认清空")
	if reset["configurationRevision"] != 4.0 || reset["moduleRevision"] != 2.0 || reset["confirmed"] != true {
		t.Fatalf("reset %v", reset)
	}
	if !f.tt.HasText("托管程序已清空，模块已关闭") || f.refreshed == 0 || len(f.h.Called("wintray.settings.get")) != 1 {
		t.Fatalf("after reset %q, calls %+v", f.tt.Texts(), f.h.Calls())
	}
}

func TestDroppedFilesAreAddedAndFoldersRefused(t *testing.T) {
	f := setup(t, "running")
	directory := t.TempDir()
	program := filepath.Join(directory, "tool.exe")
	if err := os.WriteFile(program, []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	at, _ := f.tt.Find("还没有程序，添加后可设置启动方式")
	f.tt.DropFiles(at.X+at.W/2, at.Y+at.H/2, directory)
	f.h.Settle(f.tt)
	if len(f.page.apps()) != 0 || !f.tt.HasText("此处只接收文件，原内容已保留。") {
		t.Fatalf("folder %q", f.tt.Texts())
	}
	f.tt.DropFiles(at.X+at.W/2, at.Y+at.H/2, program)
	f.h.Settle(f.tt)
	if len(f.page.apps()) != 1 || f.page.apps()[0].Value.ExePath != program {
		t.Fatalf("dropped %+v", f.page.apps())
	}
}

func TestNarrowWindowStacksTheListAboveTheDetails(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	if _, ok := f.tt.Find("调整列表与设置宽度"); !ok {
		t.Fatal("a wide window has no divider")
	}
	f.tt.SetSize(760, 800)
	f.tt.Frame()
	f.tt.Frame()
	if _, ok := f.tt.Find("调整列表与设置宽度"); ok {
		t.Fatal("a narrow window kept the divider")
	}
	list, _ := f.tt.Find("播放器")
	details, _ := f.tt.Find("启动与计划")
	if details.Y <= list.Y {
		t.Fatal("the details are not below the list")
	}
}

func TestStartupChoicesWrapInsideANarrowPanel(t *testing.T) {
	f := setup(t, "running", programDocument("a", "Player", `C:\Apps\player.exe`))
	i18n.SetLocale(i18n.English)
	t.Cleanup(func() { i18n.SetLocale(i18n.Chinese) })
	for _, width := range []int{1300, 600} {
		f.tt.SetSize(width, 900)
		f.tt.Frame()
		f.tt.Frame()
		row, ok := f.tt.Find("Start this program at sign-in")
		last, found := f.tt.Find("Start normally")
		first, _ := f.tt.Find("Minimize to tray")
		if !ok || !found {
			t.Fatalf("width %v: texts %q", width, f.tt.Texts())
		}
		// The choices stay inside the panel's rows, wrapping when needed.
		if last.X+last.W > row.X+row.W || first.X < row.X {
			t.Fatalf("width %v: choices %v..%v outside the row %v", width, first, last, row)
		}
		// With room, they keep their own width: the track ends well
		// before the row does.
		if width == 1300 && last.X+last.W > row.X+row.W-40 {
			t.Fatalf("the choices fill the row %v: %v", row, last)
		}
	}
}

func TestTabsAndProgramPagesGoThroughTheUnsavedChangeGuard(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	requested := 0
	f.h.Env.RequestChange = func(func()) { requested++ }
	f.click(t, "启动设置")
	f.click(t, "程序信息")
	if requested != 2 || f.page.tab != tabPrograms || f.page.detailTab != 0 {
		t.Fatalf("switched without the guard: %d, tab %d, sub-tab %d", requested, f.page.tab, f.page.detailTab)
	}
}

func TestReadFailureOffersRetry(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	f.fake.on("wintray.snapshot", func(json.RawMessage) (any, error) {
		return nil, testhost.Error{Code: "unavailable", Message: "模块未响应"}
	})
	f.page = newPage(f.h.Env)
	f.tt = ui.NewTester(f.page.View, 1300, 900)
	f.h.Settle(f.tt)
	if !f.tt.HasText("模块未响应 (unavailable)") || f.tt.HasText("播放器") {
		t.Fatalf("failure %q", f.tt.Texts())
	}
	f.fake.on("wintray.snapshot", nil)
	f.click(t, "重试")
	if !f.tt.HasText("播放器") || f.tt.HasText("模块未响应 (unavailable)") {
		t.Fatalf("after retrying %q", f.tt.Texts())
	}
}

func TestUndoReadsTheSavedSettingsAgain(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	if !f.h.Env.Rail.Undo.Disabled {
		t.Fatal("Undo is available without a change")
	}
	f.click(t, "移除程序")
	if !f.page.dirty() || f.h.Env.Rail.Undo.Disabled {
		t.Fatal("the removal is not undoable")
	}
	f.h.Env.Rail.Undo.Run()
	f.h.Settle(f.tt)
	if len(f.page.apps()) != 1 || f.page.dirty() || len(f.fake.saved) != 0 || len(f.h.Called("wintray.snapshot")) != 2 {
		t.Fatalf("after undoing %d programs, calls %+v", len(f.page.apps()), f.h.Calls())
	}
}

func TestUnreadableStateOffersRetryAndTheWindowFollowsIt(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	var mu sync.Mutex
	failing := true
	f.fake.on("wintray.program.state", func(json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return nil, errors.New("pipe closed")
		}
		return map[string]any{"running": true, "hasWindow": true, "visible": true}, nil
	})
	f.page.state.refresh()
	f.tt.Frame()
	f.h.Settle(f.tt)
	if !f.tt.HasText("无法读取程序状态") {
		t.Fatalf("state failure %q", f.tt.Texts())
	}
	mu.Lock()
	failing = false
	mu.Unlock()
	f.click(t, "重试")
	if f.tt.HasText("无法读取程序状态") || !f.tt.HasText("隐藏窗口") || !f.tt.HasText("关闭程序") {
		t.Fatalf("after retrying %q", f.tt.Texts())
	}
	var window json.RawMessage
	f.fake.on("wintray.program.window", func(params json.RawMessage) (any, error) {
		window = params
		return map[string]any{"state": "completed"}, nil
	})
	f.click(t, "隐藏窗口")
	if string(window) != `{"id":"a","visible":false}` {
		t.Fatalf("window %s", window)
	}
	if toasts := f.h.Toasts(); len(toasts) != 1 || toasts[0] != "窗口状态已更新" {
		t.Fatalf("toasts %q", toasts)
	}
}

func TestAProgramWithoutANameOpensOnItsInformation(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`), programDocument("b", "", `C:\Apps\tool.exe`))
	if f.page.detailTab != 0 || !f.tt.HasText("开机启动此程序") {
		t.Fatalf("named program %q", f.tt.Texts())
	}
	row, ok := f.tt.Find("识别tool")
	if !ok {
		t.Fatalf("rows %q", f.tt.Texts())
	}
	f.tt.ClickAt(row.X+48, row.Y+row.H/2)
	f.h.Settle(f.tt)
	if f.page.selected != "b" || f.page.detailTab != 1 || !f.tt.HasText("启动参数（可选）") {
		t.Fatalf("unnamed program %q", f.tt.Texts())
	}
}

func TestScheduleFieldsFollowTheirSwitches(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	if f.tt.HasText("延后启动（分钟）") || !f.tt.HasText("开启“自定义时间”后，可限制自动运行次数。") {
		t.Fatalf("schedule off %q", f.tt.Texts())
	}
	// Run limits wait for the schedule.
	_ = f.tt.Click("限制自动运行次数")
	f.h.Settle(f.tt)
	if f.page.active().Schedule.Value.FrequencyEnabled {
		t.Fatal("run limits turned on without a schedule")
	}
	f.click(t, "自定义时间")
	if !f.tt.HasText("延后启动（分钟）") || !f.tt.HasText("自动退出（分钟，0 为不退出）") {
		t.Fatalf("schedule on %q", f.tt.Texts())
	}
	f.click(t, "限制自动运行次数")
	if !f.tt.HasText("最近天数") || !f.tt.HasText("最多运行次数") {
		t.Fatalf("run limits %q", f.tt.Texts())
	}
	f.click(t, "启动设置")
	f.click(t, "登录任务完成后，无常驻功能时退出 WinOne")
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 || f.fake.saved[0]["exitAfterManagedAppsCompleted"] != true {
		t.Fatalf("saved %v", f.fake.saved)
	}
	plan := f.fake.saved[0]["managedApps"].([]any)[0].(map[string]any)["schedule"].(map[string]any)
	if plan["enabled"] != true || plan["frequencyEnabled"] != true || plan["autoExitMinutes"] != 30.0 || plan["frequencyRuns"] != 1.0 {
		t.Fatalf("schedule %v", plan)
	}
}

func TestLoadingSaysTheModuleIsBeingReached(t *testing.T) {
	f := setup(t, "running", programDocument("a", "播放器", `C:\Apps\player.exe`))
	release := make(chan struct{})
	f.fake.on("wintray.snapshot", func(json.RawMessage) (any, error) {
		<-release
		f.fake.mu.Lock()
		defer f.fake.mu.Unlock()
		return f.fake.snapshot(), nil
	})
	f.page = newPage(f.h.Env)
	f.tt = ui.NewTester(f.page.View, 1300, 900)
	f.tt.Frame()
	if !f.tt.HasText("正在连接程序管理模块") || f.tt.HasText("播放器") {
		t.Fatalf("loading %q", f.tt.Texts())
	}
	close(release)
	f.h.Settle(f.tt)
	if f.tt.HasText("正在连接程序管理模块") || !f.tt.HasText("播放器") {
		t.Fatalf("loaded %q", f.tt.Texts())
	}
}
