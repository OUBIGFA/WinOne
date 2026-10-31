package convenience

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
	"winone/modules/stroke"
)

// gestureHost serves preferences and records what is saved.
func gestureHost(t *testing.T, preferences stroke.Preferences, saved *stroke.Preferences) *testhost.Harness {
	return testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "modules.snapshot":
			return modules("stroke"), nil
		case "stroke.settings.get":
			return preferences, nil
		case "stroke.settings.save":
			if err := json.Unmarshal(params, saved); err != nil {
				t.Fatal(err)
			}
			next := *saved
			next.Revision++
			return next, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
}

func gestureView(tab *gesturesTab) func(*ui.Context) {
	return func(c *ui.Context) { kit.Page(c, func() { tab.View(c, false) }) }
}

func TestGesturesOpenExplorerFirstWithoutReorderingScopes(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	preferences := stroke.Defaults()
	preferences.Revision = 7
	slices.Reverse(preferences.Scopes)
	first := preferences.Scopes[0].ID
	var saved stroke.Preferences
	h := gestureHost(t, preferences, &saved)
	tab := newGesturesTab(h.Env)
	tt := ui.NewTester(gestureView(tab), 1100, 1600)
	h.Settle(tt)
	scope, _, ok := tab.scenes.scope()
	if !ok || scope.Kind != "explorer-folders" {
		t.Fatalf("first scene %+v", scope)
	}
	if !tt.HasText("新建文件夹") || !tt.HasText("复制") {
		t.Fatalf("explorer actions %q", tt.Texts())
	}
	if tab.draft.Scopes[0].ID != first {
		t.Fatal("showing the scenes reordered them")
	}
	if !h.Env.Rail.Save.Disabled {
		t.Fatal("Save is available before any edit")
	}
}

func TestAssigningAGestureCopiesOnlyThisScenesPackage(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	defaults := stroke.Defaults()
	preferences := stroke.Defaults()
	preferences.Revision = 7
	var saved stroke.Preferences
	h := gestureHost(t, preferences, &saved)
	tab := newGesturesTab(h.Env)
	tt := ui.NewTester(gestureView(tab), 1100, 2400)
	h.Settle(tt)
	if err := tt.Click("复制的手势"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("按住手势键 + 中键"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	var explorer, global stroke.Scope
	for _, scope := range saved.Scopes {
		switch scope.ID {
		case "explorer-folders":
			explorer = scope
		case "global":
			global = scope
		}
	}
	var assigned *stroke.Action
	for _, pack := range saved.ActionPackages {
		if !slices.Contains(explorer.Packages, pack.ID) {
			continue
		}
		for i, action := range pack.Actions {
			if action.ID == "copy" {
				assigned = &pack.Actions[i]
			}
		}
	}
	if assigned == nil || assigned.Gesture != "#0" {
		t.Fatalf("explorer's copy: %+v", assigned)
	}
	if !slices.Equal(global.Packages, []string{"global"}) {
		t.Fatalf("global packages %v", global.Packages)
	}
	for _, original := range defaults.ActionPackages {
		i := slices.IndexFunc(saved.ActionPackages, func(p stroke.ActionPackage) bool { return p.ID == original.ID })
		a, _ := json.Marshal(original)
		b, _ := json.Marshal(saved.ActionPackages[i])
		if i < 0 || string(a) != string(b) {
			t.Fatalf("shared package %s changed", original.ID)
		}
	}
	enabled := 0
	for _, scope := range saved.Scopes {
		if scope.Enabled {
			enabled++
		}
	}
	if enabled != 1 {
		t.Fatalf("%d scopes enabled", enabled)
	}
}

func TestScenesShowWhichActionWinsAGesture(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	preferences := stroke.Defaults()
	for i := range preferences.ActionPackages {
		pack := &preferences.ActionPackages[i]
		switch pack.ID {
		case "explorer":
			pack.Actions[0].Gesture, pack.Actions[0].Enabled = "up-then-left", true
		case "global":
			for j := range pack.Actions {
				if pack.Actions[j].ID == "copy" {
					pack.Actions[j].Enabled = true
				}
			}
		}
	}
	var saved stroke.Preferences
	h := gestureHost(t, preferences, &saved)
	tab := newGesturesTab(h.Env)
	tt := ui.NewTester(gestureView(tab), 1100, 2400)
	h.Settle(tt)
	scope, _, _ := tab.scenes.scope()
	overridden := map[string]string{}
	for _, entry := range sceneActions(*tab.draft, scope) {
		overridden[entry.action.Name] = entry.overriddenBy
	}
	if overridden["复制"] != "刷新" || overridden["刷新"] != "" {
		t.Fatalf("winners %v", overridden)
	}
	if !tt.HasText("此手势优先执行：刷新") {
		t.Fatalf("winner not shown %q", tt.Texts())
	}
}

func TestPenOffIsSavedWithoutChangingItsLook(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	preferences := stroke.Defaults()
	preferences.Revision = 4
	preferences.Pen.Enabled = nil
	var saved stroke.Preferences
	h := gestureHost(t, preferences, &saved)
	tab := newGesturesTab(h.Env)
	tt := ui.NewTester(gestureView(tab), 1100, 1600)
	h.Settle(tt)
	if err := tt.Click("常规"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if err := tt.Click("显示手势画笔"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if saved.Pen.Enabled == nil || *saved.Pen.Enabled || saved.Pen.Color != preferences.Pen.Color || saved.Pen.Thickness != preferences.Pen.Thickness {
		t.Fatalf("pen %+v", saved.Pen)
	}
}

func TestCompilerAndThreadSettingsStayEditable(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	preferences := stroke.Defaults()
	var saved stroke.Preferences
	h := gestureHost(t, preferences, &saved)
	tab := newGesturesTab(h.Env)
	tt := ui.NewTester(gestureView(tab), 1100, 1600)
	h.Settle(tt)
	tab.tab, tab.advanced = gesturesAdvanced, advancedCompile
	tt.Frame()
	if lines := strings.Split(tab.namespaces, "\n"); len(lines) != 37 || !strings.Contains(tab.assemblies, "System.Net.Http.dll") {
		t.Fatalf("namespaces %d, assemblies %q", len(lines), tab.assemblies)
	}
	if !tt.HasText("命名空间（每行一个）") {
		t.Fatalf("compile tab %q", tt.Texts())
	}
	tab.advanced = advancedThreads
	tt.Frame()
	if tab.draft.ScriptThreadCount != 8 || tab.draft.ScriptTimeoutSeconds != 0 || !tt.HasText("最大执行线程数") {
		t.Fatalf("threads %d timeout %d", tab.draft.ScriptThreadCount, tab.draft.ScriptTimeoutSeconds)
	}
}
