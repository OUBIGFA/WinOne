package selection

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
	"winone/internal/resultwindow"
)

func builtinActions(copy bool) []map[string]any {
	actions := []map[string]any{
		{"id": "translate", "name": "翻译", "englishName": "Translate", "icon": "translate", "kind": "translate", "prompt": "译文", "enabled": true},
		{"id": "explain", "name": "解释", "englishName": "Explain", "icon": "explain", "kind": "ai", "prompt": "解释内容", "enabled": true},
	}
	if copy {
		actions = append(actions, map[string]any{"id": "copy", "name": "复制", "englishName": "Copy", "icon": "copy", "kind": "copy", "enabled": true})
	}
	return actions
}

func selectionDocument() map[string]any {
	return map[string]any{"toolbarVersion": 1, "schemaVersion": 1, "revision": 3, "automatic": true, "keyboard": true,
		"excludedApps": []string{}, "actions": builtinActions(true)}
}

func quickDocument() map[string]any {
	return map[string]any{"schemaVersion": 1, "revision": 2, "alwaysOnTop": false, "actions": builtinActions(false)}
}

// fakeHost answers one assistant's requests and records what was saved.
type fakeHost struct {
	t        *testing.T
	module   string
	mu       sync.Mutex
	settings map[string]any
	saved    []map[string]any
	failGet  error
	shown    int
	target   resultwindow.WindowTarget
}

func (f *fakeHost) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch method {
	case f.module + ".settings.get":
		if f.failGet != nil {
			return nil, f.failGet
		}
		return f.settings, nil
	case f.module + ".settings.save":
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
	case "capture.ai.configuration.get":
		return map[string]any{"revision": 1, "tasks": map[string]any{}, "providers": []map[string]any{{
			"id": "work", "name": "工作服务", "authMode": "api-key", "hasKey": true, "models": []string{"fast", "precise"},
			"availableModels": []string{"disabled", "fast", "precise"},
		}}}, nil
	case "quick-assistant.show":
		f.shown++
		return map[string]any{"window": f.target.Window, "processId": f.target.ProcessID, "shown": true}, nil
	case "snapshot":
		return map[string]any{"settings": map[string]any{"schemaVersion": 1, "revision": 2, "theme": "system"}}, nil
	case "shortcuts.snapshot":
		return map[string]any{"bindings": []map[string]any{}, "lastError": ""}, nil
	}
	f.t.Errorf("unexpected %s", method)
	return nil, errors.New("unexpected")
}

func setup(t *testing.T, module, state string, settings map[string]any) (*fakeHost, *testhost.Harness, *page, *ui.Tester) {
	t.Helper()
	i18n.SetLocale(i18n.Chinese)
	fake := &fakeHost{t: t, module: module, settings: settings}
	h := testhost.New(t, fake.handle)
	h.Env.Snapshot = func() internalhost.Snapshot {
		return internalhost.Snapshot{Modules: []internalhost.Module{{ID: module, State: state}}}
	}
	p := newPage(h.Env, module)
	tt := ui.NewTester(p.View, 1000, 1400)
	h.Settle(tt)
	return fake, h, p, tt
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
}

// edit opens or closes an action's editor and lets it finish growing or
// shrinking: the tester's clock stands still until advanced.
func edit(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	click(t, tt, "编辑 "+name)
	tt.Advance(300 * time.Millisecond)
}

// shows reports whether a text of the last frame is exactly s; HasText
// also matches s inside longer texts, as the toolbar's hint.
func shows(tt *ui.Tester, s string) bool {
	for _, text := range tt.Texts() {
		if text == s {
			return true
		}
	}
	return false
}

func names(p *page) []string {
	var out []string
	for _, a := range p.draft.Value.Actions {
		out = append(out, a.Value.ID)
	}
	return out
}

func TestLoadFailureShowsRetry(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	fake := &fakeHost{t: t, module: "selection", settings: selectionDocument(), failGet: testhost.Error{Code: "configuration", Message: "settings unavailable"}}
	h := testhost.New(t, fake.handle)
	p := newPage(h.Env, "selection")
	tt := ui.NewTester(p.View, 1000, 900)
	h.Settle(tt)
	if !tt.HasText("settings unavailable") || tt.HasText("功能与顺序") {
		t.Fatalf("texts %q", tt.Texts())
	}
	fake.mu.Lock()
	fake.failGet = nil
	fake.mu.Unlock()
	click(t, tt, "重试")
	h.Settle(tt)
	if len(h.Called("selection.settings.get")) != 2 || !tt.HasText("功能与顺序") {
		t.Fatalf("after retry %q", tt.Texts())
	}
}

func TestSaveSendsParsedExclusions(t *testing.T) {
	fake, h, p, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "常规")
	p.excludedField.Focus()
	tt.Frame()
	tt.Type("notepad.exe\n Code.exe ")
	click(t, tt, "划词后显示工具栏")
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("edits are unsaved changes")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.saved) != 1 {
		t.Fatalf("saved %d", len(fake.saved))
	}
	saved := fake.saved[0]
	apps, _ := saved["excludedApps"].([]any)
	if saved["automatic"] != false || len(apps) != 2 || apps[0] != "notepad.exe" || apps[1] != "Code.exe" {
		t.Fatalf("saved %v", saved)
	}
	if saved["toolbarVersion"] != float64(1) || saved["schemaVersion"] != float64(1) || saved["revision"] != float64(3) || saved["keyboard"] != true {
		t.Fatalf("fields lost %v", saved)
	}
	if actions, _ := saved["actions"].([]any); len(actions) != 3 {
		t.Fatalf("actions %v", saved["actions"])
	}
	if _, quick := saved["alwaysOnTop"]; quick {
		t.Fatal("the Selection assistant has no window behavior")
	}
	if p.excluded != "notepad.exe\nCode.exe" {
		t.Fatalf("text after save %q", p.excluded)
	}
	if dirty, _ := h.Env.Activity.State(); dirty || len(h.Toasts()) != 1 || h.Toasts()[0] != "已保存" {
		t.Fatalf("after save toasts %q", h.Toasts())
	}
}

func TestUndoReadsTheSettingsAgain(t *testing.T) {
	_, h, p, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "显示 解释")
	if p.draft.Value.Actions[1].Value.Enabled {
		t.Fatal("switch did not hide the action")
	}
	h.Env.Rail.Undo.Run()
	h.Settle(tt)
	if len(h.Called("selection.settings.get")) != 2 || !p.draft.Value.Actions[1].Value.Enabled {
		t.Fatal("Undo reads the settings again")
	}
}

func TestKeyboardDependsOnAutomatic(t *testing.T) {
	document := selectionDocument()
	document["automatic"] = false
	_, _, p, tt := setup(t, "selection", "disabled", document)
	click(t, tt, "常规")
	if !tt.HasText("划词助手已关闭") || !tt.HasText("启用“划词后显示工具栏”后，可同时支持键盘选取。") {
		t.Fatalf("texts %q", tt.Texts())
	}
	click(t, tt, "键盘选中文字后也显示")
	if !*p.draft.Value.Keyboard {
		t.Fatal("a disabled switch does not change")
	}
	click(t, tt, "划词后显示工具栏")
	if tt.HasText("启用“划词后显示工具栏”后，可同时支持键盘选取。") {
		t.Fatal("the dependency note goes once the toolbar shows")
	}
	click(t, tt, "键盘选中文字后也显示")
	if *p.draft.Value.Keyboard {
		t.Fatal("the dependent switch works once enabled")
	}
}

func TestToolbarReadsAIConfigurationEachVisit(t *testing.T) {
	_, h, _, tt := setup(t, "selection", "running", selectionDocument())
	if len(h.Called("capture.ai.configuration.get")) != 1 {
		t.Fatal("the toolbar reads the AI services")
	}
	click(t, tt, "常规")
	click(t, tt, "工具栏")
	h.Settle(tt)
	if len(h.Called("capture.ai.configuration.get")) != 2 {
		t.Fatal("each visit reads them again")
	}
}

func TestOnlyOneEditorOpensAndDraftsSurvive(t *testing.T) {
	_, _, p, tt := setup(t, "selection", "running", selectionDocument())
	if shows(tt, "功能名称") {
		t.Fatalf("editors start closed: %q", tt.Texts())
	}
	edit(t, tt, "解释")
	click(t, tt, "AI 指令")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("保留这条指令")
	edit(t, tt, "复制")
	if shows(tt, "AI 指令") || p.expanded != "copy" || !shows(tt, "功能名称") {
		t.Fatalf("one editor at a time: %q", tt.Texts())
	}
	edit(t, tt, "解释")
	if p.draft.Value.Actions[1].Value.Prompt != "保留这条指令" || p.expanded != "explain" {
		t.Fatalf("prompt %q", p.draft.Value.Actions[1].Value.Prompt)
	}
	click(t, tt, "常规")
	click(t, tt, "工具栏")
	if p.draft.Value.Actions[1].Value.Prompt != "保留这条指令" || !shows(tt, "AI 指令") {
		t.Fatal("switching tabs keeps the draft and the open editor")
	}
}

func TestPositionMovesAction(t *testing.T) {
	_, _, p, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "复制 · 位置")
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyEnter)
	if got := strings.Join(names(p), ","); got != "copy,translate,explain" {
		t.Fatalf("order %s", got)
	}
	copy, _ := tt.Find("编辑 复制")
	translate, _ := tt.Find("编辑 翻译")
	if copy.Y > translate.Y {
		t.Fatal("rows follow the new order")
	}
}

func TestDragReordersActions(t *testing.T) {
	_, _, p, tt := setup(t, "selection", "running", selectionDocument())
	from, _ := tt.Find("编辑 复制")
	to, _ := tt.Find("编辑 翻译")
	x := from.X + from.W/2
	tt.Press(x, from.Y+from.H/2)
	for y := from.Y + from.H/2; y > to.Y+to.H/2; y -= 8 {
		tt.Move(x, y)
	}
	tt.Move(x, to.Y+to.H/2)
	tt.Release(x, to.Y+to.H/2)
	tt.Frame()
	if got := strings.Join(names(p), ","); got != "copy,translate,explain" {
		t.Fatalf("order after drag %s", got)
	}
	if p.expanded != "" {
		t.Fatal("a drag is no click")
	}
}

var customID = regexp.MustCompile(`^custom-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestCustomActionFlow(t *testing.T) {
	fake, h, p, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "添加自定义功能")
	if !tt.Focused("功能名称") {
		t.Fatal("the new action's name takes the focus")
	}
	tt.Type("邮件回复")
	click(t, tt, "AI 指令")
	tt.Type("根据选中文字写一封简短的邮件回复。")
	// Another tab and back keeps the new action as typed.
	click(t, tt, "常规")
	click(t, tt, "工具栏")
	h.Settle(tt)
	click(t, tt, "邮件回复 · 供应商")
	click(t, tt, "工作服务")
	click(t, tt, "邮件回复 · 模型名称")
	if tt.HasText("disabled") {
		t.Fatal("only enabled models are offered")
	}
	click(t, tt, "precise")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.saved) != 1 {
		t.Fatalf("saved %d", len(fake.saved))
	}
	actions, _ := fake.saved[0]["actions"].([]any)
	added, _ := actions[len(actions)-1].(map[string]any)
	selection, _ := added["selection"].(map[string]any)
	if added["name"] != "邮件回复" || added["kind"] != "ai" || added["icon"] != "polish" || added["enabled"] != true ||
		added["prompt"] != "根据选中文字写一封简短的邮件回复。" || selection["provider"] != "work" || selection["model"] != "precise" {
		t.Fatalf("added %v", added)
	}
	if id, _ := added["id"].(string); !customID.MatchString(id) {
		t.Fatalf("id %q", added["id"])
	}
	_ = p
}

func TestFollowDefaultRemovesTheActionsModel(t *testing.T) {
	document := selectionDocument()
	document["actions"].([]map[string]any)[1]["selection"] = map[string]any{"provider": "work", "model": "fast"}
	fake, h, _, tt := setup(t, "selection", "running", document)
	edit(t, tt, "解释")
	click(t, tt, "解释 · 供应商")
	click(t, tt, "跟随 AI 服务默认设置")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	actions, _ := fake.saved[0]["actions"].([]any)
	explain, _ := actions[1].(map[string]any)
	if _, ok := explain["selection"]; ok {
		t.Fatalf("following the default removes the model: %v", explain)
	}
}

func TestRemoveCustomAction(t *testing.T) {
	document := selectionDocument()
	document["actions"] = append(document["actions"].([]map[string]any),
		map[string]any{"id": "custom-1", "name": "邮件", "icon": "mail", "kind": "ai", "prompt": "写邮件", "enabled": true})
	_, _, p, tt := setup(t, "selection", "running", document)
	edit(t, tt, "解释")
	if shows(tt, "删除") {
		t.Fatal("built-in actions cannot be deleted")
	}
	edit(t, tt, "邮件")
	click(t, tt, "删除")
	if got := strings.Join(names(p), ","); got != "translate,explain,copy" || p.expanded != "" {
		t.Fatalf("order %s expanded %q", got, p.expanded)
	}
}

func TestAddDisabledAtLimit(t *testing.T) {
	document := selectionDocument()
	actions := builtinActions(true)
	for len(actions) < maxActions {
		id := "custom-" + string(rune('a'+len(actions)))
		actions = append(actions, map[string]any{"id": id, "name": id, "icon": "mail", "kind": "ai", "prompt": "x", "enabled": true})
	}
	document["actions"] = actions
	_, _, p, tt := setup(t, "selection", "running", document)
	click(t, tt, "添加自定义功能")
	if len(p.draft.Value.Actions) != maxActions {
		t.Fatalf("%d actions", len(p.draft.Value.Actions))
	}
}

func TestDropFilesAppendsNames(t *testing.T) {
	_, h, p, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "常规")
	folder := t.TempDir()
	notepad, code := filepath.Join(folder, "notepad.exe"), filepath.Join(folder, "Code.exe")
	for _, path := range []string{notepad, code} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	drop := func(paths ...string) {
		t.Helper()
		at, ok := tt.Find("每行一个程序文件名，例如 notepad.exe。")
		if !ok || !tt.DropFiles(at.X+at.W/2, at.Y, paths...) {
			t.Fatal("the exclusion list takes dropped files")
		}
		h.Settle(tt)
	}
	drop(notepad, code)
	drop(notepad)
	if p.excluded != "notepad.exe\nCode.exe" {
		t.Fatalf("excluded %q", p.excluded)
	}
	drop(folder)
	if !tt.HasText("此处只接收文件，原内容已保留。") || p.excluded != "notepad.exe\nCode.exe" {
		t.Fatalf("folder drop %q", tt.Texts())
	}
}

func TestQuickLoadsItsOwnSettingsAndSavesItsFields(t *testing.T) {
	fake, h, _, tt := setup(t, "quick-assistant", "running", quickDocument())
	if len(h.Called("quick-assistant.settings.get")) != 1 || len(h.Called("selection.settings.get")) != 0 {
		t.Fatalf("calls %+v", h.Calls())
	}
	if !tt.HasText("使用这些功能") || tt.HasText("编辑 复制") {
		t.Fatalf("texts %q", tt.Texts())
	}
	edit(t, tt, "解释")
	// The name focuses the picker; Down opens it, as on the desktop.
	click(t, tt, "图标")
	tt.Key(0, ui.KeyDown)
	if !shows(tt, "列表") || shows(tt, "复制") {
		t.Fatal("the Quick assistant has no copy action icon")
	}
	click(t, tt, "列表")
	click(t, tt, "常规")
	if tt.HasText("显示方式") || tt.HasText("排除应用") {
		t.Fatal("the Quick assistant has no triggers or exclusions")
	}
	click(t, tt, "默认置顶快捷助手")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	saved := fake.saved[0]
	for _, field := range []string{"automatic", "keyboard", "excludedApps", "showReasoning", "toolbarVersion"} {
		if _, ok := saved[field]; ok {
			t.Fatalf("%s was added: %v", field, saved)
		}
	}
	actions, _ := saved["actions"].([]any)
	explain, _ := actions[1].(map[string]any)
	if saved["alwaysOnTop"] != true || explain["icon"] != "summarize" {
		t.Fatalf("saved %v", saved)
	}
	click(t, tt, "显示快捷助手的思考过程")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if fake.saved[1]["showReasoning"] != false {
		t.Fatalf("saved %v", fake.saved[1])
	}
}

func TestOpenQuickAssistantShowsAndReportsActivationFailure(t *testing.T) {
	fake, h, _, tt := setup(t, "quick-assistant", "running", quickDocument())
	fake.target = resultwindow.WindowTarget{Window: "0", ProcessID: 0}
	click(t, tt, "打开快捷助手")
	h.Settle(tt)
	if fake.shown != 1 || !tt.HasText("快捷助手窗口已关闭或无法激活，请再次点击打开") {
		t.Fatalf("shown %d texts %q", fake.shown, tt.Texts())
	}
	var activated resultwindow.WindowTarget
	activate = func(target resultwindow.WindowTarget) error { activated = target; return nil }
	t.Cleanup(func() { activate = func(target resultwindow.WindowTarget) error { return target.Activate() } })
	fake.target = resultwindow.WindowTarget{Window: "4242", ProcessID: 7}
	click(t, tt, "打开快捷助手")
	h.Settle(tt)
	if fake.shown != 2 || activated != fake.target || tt.HasText("快捷助手窗口已关闭或无法激活，请再次点击打开") {
		t.Fatalf("activated %+v texts %q", activated, tt.Texts())
	}
	_, h, _, tt = setup(t, "quick-assistant", "disabled", quickDocument())
	click(t, tt, "打开快捷助手")
	h.Settle(tt)
	if len(h.Called("quick-assistant.show")) != 0 {
		t.Fatal("a disabled assistant cannot be opened")
	}
}

func TestShortcutsTabUsesSharedEditor(t *testing.T) {
	_, h, _, tt := setup(t, "selection", "running", selectionDocument())
	click(t, tt, "快捷键")
	h.Settle(tt)
	if len(h.Called("shortcuts.snapshot")) != 1 || !tt.HasText("显示划词工具栏") || h.Env.Rail.Save.Label != i18n.T("shortcuts.save") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestHeadingsAndBusyDisabling(t *testing.T) {
	_, h, p, tt := setup(t, "selection", "running", selectionDocument())
	for _, text := range []string{"功能与顺序", "点击功能名称编辑详情，调整位置改变顺序。", "功能", "位置", "显示"} {
		if !shows(tt, text) {
			t.Fatalf("%q missing: %q", text, tt.Texts())
		}
	}
	click(t, tt, "显示 解释")
	h.Env.Rail.Save.Run()
	tt.Frame()
	click(t, tt, "显示 翻译")
	if !p.draft.Value.Actions[0].Value.Enabled {
		t.Fatal("actions cannot change while they are saved")
	}
	h.Settle(tt)
	click(t, tt, "显示 翻译")
	if p.draft.Value.Actions[0].Value.Enabled {
		t.Fatal("actions change again once saved")
	}
}

func TestEnglishWindowEditsTheEnglishName(t *testing.T) {
	_, _, p, tt := setup(t, "selection", "running", selectionDocument())
	i18n.SetLocale(i18n.English)
	t.Cleanup(func() { i18n.SetLocale(i18n.Chinese) })
	tt.Frame()
	click(t, tt, "Edit Explain")
	tt.Advance(300 * time.Millisecond)
	click(t, tt, "Action name")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Clarify")
	explain := p.draft.Value.Actions[1].Value
	if explain.EnglishName != "Clarify" || explain.Name != "解释" || !shows(tt, "Clarify") {
		t.Fatalf("names %q / %q", explain.EnglishName, explain.Name)
	}
}

func TestQuickDisabledNoteAndShortcuts(t *testing.T) {
	_, h, _, tt := setup(t, "quick-assistant", "disabled", quickDocument())
	click(t, tt, "常规")
	if !shows(tt, "快捷助手已关闭") {
		t.Fatalf("texts %q", tt.Texts())
	}
	click(t, tt, "快捷键")
	h.Settle(tt)
	if len(h.Called("shortcuts.snapshot")) != 1 || h.Env.Rail.Save.Label != i18n.T("shortcuts.save") {
		t.Fatalf("calls %+v", h.Calls())
	}
}

func TestErrorsShowAboveEveryTab(t *testing.T) {
	fake, h, _, tt := setup(t, "quick-assistant", "running", quickDocument())
	fake.target = resultwindow.WindowTarget{Window: "0"}
	click(t, tt, "快捷键")
	h.Settle(tt)
	click(t, tt, "打开快捷助手")
	h.Settle(tt)
	if !shows(tt, "快捷助手窗口已关闭或无法激活，请再次点击打开") {
		t.Fatalf("an error of the page shows on the shortcuts tab too: %q", tt.Texts())
	}
}
