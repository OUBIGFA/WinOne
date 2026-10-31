package capture

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
)

// captureDocument is the Host's capture settings, with fields this page
// does not edit, which a save must send back unchanged.
func captureDocument() map[string]any {
	return map[string]any{
		"schemaVersion": 1, "revision": 4, "outputDirectory": `C:\Images`, "copyToClipboard": false,
		"askWhereToSave": true, "openEditor": true, "pinAfterCapture": false, "theme": "system",
		"scrollDelay": 300, "scrollFromTop": false, "toolbarTools": []string{"select", "crop"}, "toolbarVersion": 2,
		"fileNameTemplate": "",
	}
}

// fakeHost answers the capture page's requests and records the documents
// saved. Methods not listed fail the test.
type fakeHost struct {
	t        *testing.T
	mu       sync.Mutex
	settings map[string]any
	ai       map[string]any
	saved    []map[string]any
	aiSaved  []map[string]any
	previews []string
	failGet  error
	failSave error
	failName error
}

func newFakeHost(t *testing.T) *fakeHost {
	return &fakeHost{t: t, settings: captureDocument(), ai: aiDocumentFixture()}
}

func (f *fakeHost) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case "capture.settings.get":
		if f.failGet != nil {
			return nil, f.failGet
		}
		return f.settings, nil
	case "capture.snapshot":
		if f.failGet != nil {
			return nil, f.failGet
		}
		return map[string]any{"settings": f.settings, "busy": false, "pins": 0}, nil
	case "capture.settings.save":
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
	case "capture.filename.preview":
		var request struct{ Template string }
		_ = json.Unmarshal(params, &request)
		f.previews = append(f.previews, request.Template)
		if f.failName != nil {
			return nil, f.failName
		}
		return map[string]string{"name": strings.ReplaceAll(request.Template, "{date}", "20261031") + ".png"}, nil
	case "capture.ai.configuration.get":
		return f.ai, nil
	case "capture.ai.configuration.save":
		var request struct {
			Configuration map[string]any `json:"configuration"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			f.t.Errorf("ai save %s", params)
		}
		f.aiSaved = append(f.aiSaved, request.Configuration)
		f.ai = request.Configuration
		return request.Configuration, nil
	case "snapshot":
		return map[string]any{"settings": map[string]any{"schemaVersion": 1, "revision": 2, "theme": "system", "captureShortcut": "Shift+A"}}, nil
	case "shortcuts.snapshot":
		return map[string]any{"bindings": []map[string]any{{"id": "capture", "shortcut": "Shift+A", "registered": true}}, "lastError": ""}, nil
	}
	f.t.Errorf("unexpected %s", method)
	return nil, errors.New("unexpected")
}

func aiDocumentFixture() map[string]any {
	return map[string]any{
		"revision": 9,
		"providers": []map[string]any{{
			"id": "work", "name": "工作服务", "authMode": "api-key", "hasKey": true, "models": []string{"fast", "precise"},
			"availableModels": []string{"disabled", "fast", "precise"}, "keys": []map[string]any{{"id": "primary", "enabled": true, "hasKey": true}},
		}},
		"tasks": map[string]any{}, "engine": "google", "ocrEngine": "local", "target": "auto", "prompt": "",
		"magpieSettings": map[string]any{"proxy": "system"}, "showReasoning": true,
	}
}

func setup(t *testing.T, disabled bool) (*fakeHost, *testhost.Harness, *page, *ui.Tester) {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	fake := newFakeHost(t)
	h := testhost.New(t, fake.handle)
	state := "running"
	if disabled {
		state = "disabled"
	}
	h.Env.Snapshot = func() internalhost.Snapshot {
		return internalhost.Snapshot{Modules: []internalhost.Module{{ID: "capture", State: state}}}
	}
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 900)
	h.Settle(tt)
	return fake, h, p, tt
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
}

func TestLoadUsesSettingsWhenDisabledAndSnapshotWhenEnabled(t *testing.T) {
	_, h, _, tt := setup(t, true)
	if len(h.Called("capture.settings.get")) != 1 || len(h.Called("capture.snapshot")) != 0 {
		t.Fatalf("disabled module calls %+v", h.Calls())
	}
	if !tt.HasText("截图后编辑") || !tt.HasText(`C:\Images`) {
		t.Fatalf("texts %q", tt.Texts())
	}
	_, h, _, _ = setup(t, false)
	if len(h.Called("capture.snapshot")) != 1 || len(h.Called("capture.settings.get")) != 0 {
		t.Fatalf("enabled module calls %+v", h.Calls())
	}
}

func TestLoadingStateAndModuleSwitchReadAgain(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	fake := newFakeHost(t)
	h := testhost.New(t, fake.handle)
	state := "disabled"
	h.Env.Snapshot = func() internalhost.Snapshot {
		return internalhost.Snapshot{Modules: []internalhost.Module{{ID: "capture", State: state}}}
	}
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 800)
	if !tt.HasText("正在加载设置") || tt.HasText("处理方式") {
		t.Fatalf("before the answer %q", tt.Texts())
	}
	h.Settle(tt)
	if tt.HasText("正在加载设置") || !tt.HasText("处理方式") {
		t.Fatalf("after the answer %q", tt.Texts())
	}
	// The module was turned on elsewhere: its settings come from its
	// snapshot now, as React read them again on the disabled prop.
	state = "running"
	tt.Frame()
	h.Settle(tt)
	if len(h.Called("capture.settings.get")) != 1 || len(h.Called("capture.snapshot")) != 1 {
		t.Fatalf("calls %+v", h.Calls())
	}
}

func TestLoadFailureShowsRetry(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	fake := newFakeHost(t)
	fake.failGet = testhost.Error{Code: "configuration", Message: "capture unavailable"}
	h := testhost.New(t, fake.handle)
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 800)
	h.Settle(tt)
	if !tt.HasText("capture unavailable") || tt.HasText("处理方式") {
		t.Fatalf("texts %q", tt.Texts())
	}
	fake.mu.Lock()
	fake.failGet = nil
	fake.mu.Unlock()
	click(t, tt, "重试")
	h.Settle(tt)
	if len(h.Called("capture.snapshot")) != 2 || !tt.HasText("处理方式") {
		t.Fatalf("after retry %q", tt.Texts())
	}
}

func TestSaveSendsWholeDocumentOnce(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	if !h.Env.Rail.Save.Disabled {
		t.Fatal("Save must wait for a change")
	}
	click(t, tt, "复制到剪贴板")
	if dirty, _ := h.Env.Activity.State(); !dirty || h.Env.Rail.Save.Disabled {
		t.Fatal("a checked box is an unsaved change")
	}
	h.Env.Rail.Save.Run()
	tt.Frame()
	if !h.Env.Rail.Save.Busy {
		t.Fatal("Save shows its work")
	}
	if _, busy := h.Env.Activity.State(); !busy {
		t.Fatal("saving keeps the page busy")
	}
	p.save() // a second request while saving is ignored
	h.Settle(tt)
	if len(fake.saved) != 1 {
		t.Fatalf("saved %d times", len(fake.saved))
	}
	saved := fake.saved[0]
	if saved["copyToClipboard"] != true || saved["theme"] != "system" || saved["toolbarVersion"] != float64(2) ||
		saved["revision"] != float64(4) || saved["schemaVersion"] != float64(1) {
		t.Fatalf("saved %v", saved)
	}
	if tools, _ := saved["toolbarTools"].([]any); len(tools) != 2 {
		t.Fatalf("toolbar tools lost: %v", saved)
	}
	if dirty, busy := h.Env.Activity.State(); dirty || busy || !h.Env.Rail.Save.Disabled {
		t.Fatal("a saved page is clean")
	}
	if toasts := h.Toasts(); len(toasts) != 1 || toasts[0] != "已保存" {
		t.Fatalf("confirmation %q", toasts)
	}
}

func TestSaveFailureKeepsDraftAndOffersDiscard(t *testing.T) {
	fake, h, _, tt := setup(t, true)
	fake.failSave = testhost.Error{Code: "configuration", Message: "revision changed"}
	click(t, tt, "截图置顶")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if !tt.HasText("revision changed") || !tt.HasText("放弃修改并刷新") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("a failed save keeps the draft")
	}
	click(t, tt, "放弃修改并刷新")
	h.Settle(tt)
	if dirty, _ := h.Env.Activity.State(); dirty || tt.HasText("revision changed") {
		t.Fatal("discarding reads the settings again")
	}
}

func TestUndoRevertsLocally(t *testing.T) {
	_, h, p, tt := setup(t, true)
	click(t, tt, "截图置顶")
	if !p.draft.Value.PinAfterCapture {
		t.Fatal("check box did not change the draft")
	}
	calls := len(h.Calls())
	h.Env.Rail.Undo.Run()
	tt.Frame()
	if p.draft.Value.PinAfterCapture || len(h.Calls()) != calls {
		t.Fatal("Undo reverts the draft without the Host")
	}
	if dirty, _ := h.Env.Activity.State(); dirty {
		t.Fatal("reverted page is clean")
	}
}

func TestQuickSaveClearsAskWhereToSave(t *testing.T) {
	fake, h, _, tt := setup(t, true)
	if !tt.HasText("每次选择保存位置") {
		t.Fatal("editing first offers choosing a location")
	}
	click(t, tt, "截完保存")
	if tt.HasText("每次选择保存位置") {
		t.Fatal("saving at once has no location prompt")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.saved) != 1 || fake.saved[0]["openEditor"] != false || fake.saved[0]["askWhereToSave"] != false {
		t.Fatalf("saved %v", fake.saved)
	}
}

type pickerResult struct {
	path string
	err  error
}

type stubDialogs struct {
	core.Dialogs
	result pickerResult
	titles *[]string
}

func (d stubDialogs) PickFolder(title string) (string, error) {
	*d.titles = append(*d.titles, title)
	return d.result.path, d.result.err
}

func TestChooseDirectoryKeepsPageBusyUntilAnswered(t *testing.T) {
	_, h, p, tt := setup(t, true)
	var titles []string
	h.Env.Dialogs = stubDialogs{Dialogs: h.Env.Dialogs, result: pickerResult{path: `D:\Captures`}, titles: &titles}
	click(t, tt, "选择目录")
	if _, busy := h.Env.Activity.State(); !busy {
		t.Fatal("the page waits for the folder picker")
	}
	h.Settle(tt)
	if p.draft.Value.OutputDirectory != `D:\Captures` || !tt.HasText(`D:\Captures`) || len(titles) != 1 || titles[0] != "选择文件夹" {
		t.Fatalf("directory %q titles %q", p.draft.Value.OutputDirectory, titles)
	}
	if dirty, busy := h.Env.Activity.State(); !dirty || busy {
		t.Fatal("a chosen folder is an unsaved change")
	}
	h.Env.Rail.Undo.Run()
	h.Env.Dialogs = stubDialogs{Dialogs: h.Env.Dialogs, result: pickerResult{}, titles: &titles}
	tt.Frame()
	click(t, tt, "选择目录")
	h.Settle(tt)
	if p.draft.Value.OutputDirectory != `C:\Images` {
		t.Fatal("cancelling keeps the folder")
	}
	h.Env.Dialogs = stubDialogs{Dialogs: h.Env.Dialogs, result: pickerResult{err: errors.New("Folder picker failed")}, titles: &titles}
	click(t, tt, "选择目录")
	h.Settle(tt)
	if p.draft.Value.OutputDirectory != `C:\Images` || !tt.HasText("Folder picker failed") {
		t.Fatalf("failure %q", tt.Texts())
	}
	if dirty, busy := h.Env.Activity.State(); dirty || busy {
		t.Fatal("a failed pick changes nothing")
	}
}

func TestDropFolderOnSaveLocation(t *testing.T) {
	_, h, p, tt := setup(t, true)
	folder := t.TempDir()
	file := filepath.Join(folder, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The area moves down as an error shows above it: find it each time.
	drop := func(paths ...string) {
		t.Helper()
		at, ok := tt.Find("保存目录")
		if !ok {
			t.Fatal("no save location")
		}
		if !tt.DropFiles(at.X+at.W/2, at.Y+at.H/2, paths...) {
			t.Fatal("the save location takes dropped files")
		}
		h.Settle(tt)
	}
	drop(file)
	if !tt.HasText("此处只接收文件夹，原路径已保留。") || p.draft.Value.OutputDirectory != `C:\Images` {
		t.Fatalf("file drop %q", tt.Texts())
	}
	drop(folder, folder+"2")
	if !tt.HasText("每次请拖入一个项目。") {
		t.Fatalf("two folders %q", tt.Texts())
	}
	drop(folder)
	if p.draft.Value.OutputDirectory != folder || tt.HasText("每次请拖入一个项目。") {
		t.Fatalf("folder drop %q", p.draft.Value.OutputDirectory)
	}
}

func TestFileNamePreviewFollowsTheTemplate(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	click(t, tt, "文件命名")
	h.Settle(tt)
	if len(fake.previews) != 0 {
		t.Fatal("an empty template waits for the user")
	}
	p.name.input.Focus()
	tt.Frame()
	tt.Type("WinOne-{date}")
	h.Settle(tt)
	if !tt.HasText("示例: WinOne-20261031.png") {
		t.Fatalf("texts %q previews %q", tt.Texts(), fake.previews)
	}
	if last := fake.previews[len(fake.previews)-1]; last != "WinOne-{date}" {
		t.Fatalf("previews %q", fake.previews)
	}
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("a new template is an unsaved change")
	}
	fake.mu.Lock()
	fake.failName = testhost.Error{Code: "configuration", Message: "文件名模板无效"}
	fake.mu.Unlock()
	tt.Type("?")
	h.Settle(tt)
	if !tt.HasText("文件名模板无效") || tt.HasText("示例:") {
		t.Fatalf("invalid template %q", tt.Texts())
	}
	fake.mu.Lock()
	fake.failName = nil
	fake.mu.Unlock()
	before := len(fake.previews)
	click(t, tt, "重试")
	h.Settle(tt)
	if len(fake.previews) != before+1 || !tt.Focused("文件名模板") {
		t.Fatalf("retry previews %q", fake.previews)
	}
}

func TestInvalidScrollDelayBlocksSaveAndLeadsToField(t *testing.T) {
	for _, typed := range []string{"", "99", "2001", "300.5"} {
		fake, h, p, tt := setup(t, true)
		click(t, tt, "滚动截图")
		p.scrollField.Focus()
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		if typed != "" {
			tt.Type(typed)
		}
		if p.scrollText != typed || !tt.HasText("请输入 100–2000 的整数") || !h.Env.Rail.Save.Disabled {
			t.Fatalf("%q: text %q texts %q", typed, p.scrollText, tt.Texts())
		}
		click(t, tt, "截图后")
		if !tt.HasText("滚动间隔须为 100–2000 毫秒的整数。") || !h.Env.Rail.Save.Disabled {
			t.Fatalf("%q: notice %q", typed, tt.Texts())
		}
		p.save()
		click(t, tt, "检查滚动间隔")
		if p.tab != tabScroll || p.scrollText != typed || !tt.Focused("滚动间隔（毫秒）") {
			t.Fatalf("%q: tab %d text %q", typed, p.tab, p.scrollText)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("500")
		tt.Frame()
		h.Env.Rail.Save.Run()
		h.Settle(tt)
		if len(fake.saved) != 1 || fake.saved[0]["scrollDelay"] != float64(500) {
			t.Fatalf("%q: saved %v", typed, fake.saved)
		}
	}
}

func TestScrollDelayStepsWithKeysAndButtons(t *testing.T) {
	_, _, p, tt := setup(t, true)
	click(t, tt, "滚动截图")
	p.scrollField.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if p.scrollText != "301" || p.draft.Value.ScrollDelay != 301 {
		t.Fatalf("up %q %d", p.scrollText, p.draft.Value.ScrollDelay)
	}
	click(t, tt, "滚动间隔减 1 毫秒")
	tt.Frame()
	if p.scrollText != "300" || p.draft.Value.ScrollDelay != 300 {
		t.Fatalf("down %q", p.scrollText)
	}
	if stepScrollDelay("", true) != "100" || stepScrollDelay("2001", false) != "2000" || stepScrollDelay("300.5", true) != "301" {
		t.Fatal("steps go to the next whole number within the range")
	}
}

func TestTabSwitchGoesThroughGuard(t *testing.T) {
	_, h, p, tt := setup(t, true)
	var requested int
	h.Env.RequestChange = func(apply func()) { requested++ }
	click(t, tt, "文件命名")
	if requested != 1 || p.tab != tabGeneral {
		t.Fatal("switching tabs asks the guard first")
	}
}

func TestAISettingsSaveWholeConfigurationAndSurviveTabs(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	if len(h.Called("capture.ai.configuration.get")) != 1 || !tt.HasText("文字识别") || !tt.HasText("翻译时将识别文字发送到 Google。") {
		t.Fatalf("texts %q", tt.Texts())
	}
	click(t, tt, "AI 服务中的模型")
	if !tt.HasText("图片发送到所选供应商。请选择支持图像的模型。") {
		t.Fatalf("ai ocr %q", tt.Texts())
	}
	click(t, tt, "截图 OCR · 供应商")
	click(t, tt, "工作服务")
	if got := p.ai.draft.Value.Tasks["ocr"]; got.Provider != "work" || got.Model != "fast" {
		t.Fatalf("ocr task %+v", got)
	}
	click(t, tt, "截图 OCR · 模型名称")
	if tt.HasText("disabled") {
		t.Fatal("only enabled models are offered")
	}
	click(t, tt, "precise")
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("a chosen model is an unsaved change")
	}
	// The draft lives with the page, not the tab.
	click(t, tt, "截图后")
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	if len(h.Called("capture.ai.configuration.get")) != 1 || p.ai.draft.Value.Tasks["ocr"].Model != "precise" {
		t.Fatal("switching tabs keeps the AI draft")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.aiSaved) != 1 {
		t.Fatalf("saved %d", len(fake.aiSaved))
	}
	saved := fake.aiSaved[0]
	tasks, _ := saved["tasks"].(map[string]any)
	ocr, _ := tasks["ocr"].(map[string]any)
	if ocr["provider"] != "work" || ocr["model"] != "precise" || saved["ocrEngine"] != "ai" {
		t.Fatalf("saved %v", saved)
	}
	providers, _ := saved["providers"].([]any)
	first, _ := providers[0].(map[string]any)
	if len(providers) != 1 || first["keys"] == nil || first["authMode"] != "api-key" || saved["magpieSettings"] == nil || saved["revision"] != float64(9) {
		t.Fatalf("configuration lost fields: %v", saved)
	}
	if dirty, _ := h.Env.Activity.State(); dirty {
		t.Fatal("saved configuration is clean")
	}
}

func TestAIFollowDefaultRemovesTheTask(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	fake.ai["tasks"] = map[string]any{"chat": map[string]any{"provider": "work", "model": "gone"}}
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	if !tt.HasText("gone 未启用或已移除，请重新选择。") {
		t.Fatalf("a removed model is reported %q", tt.Texts())
	}
	click(t, tt, "结果对话 · 供应商")
	click(t, tt, "跟随 AI 服务默认设置")
	if _, ok := p.ai.draft.Value.Tasks["chat"]; ok {
		t.Fatal("following the default removes the task")
	}
	h.Env.Rail.Undo.Run()
	h.Settle(tt)
	if len(h.Called("capture.ai.configuration.get")) != 2 || p.ai.draft.Value.Tasks["chat"].Model != "gone" {
		t.Fatal("Undo reads the configuration again")
	}
}

func TestShortcutsTabUsesSharedEditor(t *testing.T) {
	_, h, _, tt := setup(t, true)
	if len(h.Called("shortcuts.snapshot")) != 0 {
		t.Fatal("shortcuts are read when their tab first shows")
	}
	click(t, tt, "快捷键")
	h.Settle(tt)
	if len(h.Called("shortcuts.snapshot")) != 1 || !tt.HasText("开始截图") || !tt.HasText("截图识别并复制") || !tt.HasText("截图翻译") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if h.Env.Rail.Save.Label != i18n.T("shortcuts.save") {
		t.Fatalf("rail %+v", h.Env.Rail.Save)
	}
}

func TestSavingDisablesTheSettings(t *testing.T) {
	_, h, p, tt := setup(t, true)
	click(t, tt, "复制到剪贴板")
	h.Env.Rail.Save.Run()
	tt.Frame()
	click(t, tt, "截图置顶")
	if p.draft.Value.PinAfterCapture {
		t.Fatal("settings cannot change while they are saved")
	}
	h.Settle(tt)
	click(t, tt, "截图置顶")
	if !p.draft.Value.PinAfterCapture {
		t.Fatal("settings change again once saved")
	}
	// Discarding from the unsaved-change guard reverts in place.
	h.Env.Activity.Discard()
	tt.Frame()
	if p.draft.Value.PinAfterCapture {
		t.Fatal("discarding reverts the draft")
	}
}

func TestScrollFromTopIsAChange(t *testing.T) {
	_, h, p, tt := setup(t, true)
	click(t, tt, "滚动截图")
	click(t, tt, "从窗口顶部开始")
	if !p.draft.Value.ScrollFromTop {
		t.Fatal("the switch sets scrolling from the top")
	}
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("an unsaved change")
	}
}

func TestAILoadFailureOffersRetry(t *testing.T) {
	fake, h, _, tt := setup(t, true)
	calls := 0
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		if method == "capture.ai.configuration.get" {
			calls++
			if calls == 1 {
				return nil, testhost.Error{Code: "ai", Message: "AI services unavailable"}
			}
		}
		return fake.handle(method, params)
	})
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	if !tt.HasText("AI services unavailable") || tt.HasText("文字识别") {
		t.Fatalf("texts %q", tt.Texts())
	}
	click(t, tt, "重试")
	h.Settle(tt)
	if !tt.HasText("文字识别") || tt.HasText("AI services unavailable") {
		t.Fatalf("after retry %q", tt.Texts())
	}
}

func TestAITranslationWithAModel(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	// The translation engine's second choice sits right of the first.
	builtin, ok := tt.Find("内置 Google 翻译")
	if !ok {
		t.Fatal("no translation engine")
	}
	tt.ClickAt(builtin.X+builtin.W+40, builtin.Y+builtin.H/2)
	if p.ai.draft.Value.Engine != "llm" || !tt.HasText("翻译提示词") || tt.HasText("翻译时将识别文字发送到 Google。") {
		t.Fatalf("engine %q texts %q", p.ai.draft.Value.Engine, tt.Texts())
	}
	click(t, tt, "截图翻译 · 供应商")
	click(t, tt, "工作服务")
	click(t, tt, "翻译提示词")
	tt.Type("译成 {target}")
	click(t, tt, "智能中英互译")
	click(t, tt, "日语")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	saved := fake.aiSaved[0]
	tasks, _ := saved["tasks"].(map[string]any)
	translate, _ := tasks["translate"].(map[string]any)
	if saved["engine"] != "llm" || saved["target"] != "ja" || saved["prompt"] != "译成 {target}" || translate["model"] != "fast" {
		t.Fatalf("saved %v", saved)
	}
	if toasts := h.Toasts(); len(toasts) != 1 || toasts[0] != "已保存" {
		t.Fatalf("toasts %q", toasts)
	}
}

func TestAIDiscardReadsAgain(t *testing.T) {
	_, h, p, tt := setup(t, true)
	click(t, tt, "识别与翻译")
	h.Settle(tt)
	click(t, tt, "AI 服务中的模型")
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("a new engine is an unsaved change")
	}
	h.Env.Activity.Discard()
	h.Settle(tt)
	if len(h.Called("capture.ai.configuration.get")) != 2 || p.ai.draft.Value.OCREngine != "local" {
		t.Fatal("discarding reads the configuration again")
	}
}
