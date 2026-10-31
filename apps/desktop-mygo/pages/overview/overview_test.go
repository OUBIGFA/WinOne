package overview

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/widgets"
)

func catalog(enabled bool) widgets.ModuleSnapshot {
	names := func(zh, en string) map[string]string { return map[string]string{"zh-CN": zh, "en-US": en} }
	return widgets.ModuleSnapshot{Revision: 7, Modules: []widgets.ModuleEntry{
		{Manifest: widgets.ModuleManifest{ID: "wintray", Name: names("程序管理", "Programs"), Distribution: "builtin"}, Enabled: enabled, Runtime: widgets.ModuleRuntime{ID: "wintray", State: "running"}},
		{Manifest: widgets.ModuleManifest{ID: "capture", Name: names("截图", "Capture"), Distribution: "builtin"}, Runtime: widgets.ModuleRuntime{ID: "capture", State: "disabled"}},
		{Manifest: widgets.ModuleManifest{ID: "stroke", Name: names("鼠标手势", "Gestures"), Distribution: "builtin"}},
	}}
}

func TestHomeListsToolsInOrderAndTogglesThroughTheHost(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	enabled := false
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "modules.snapshot":
			return catalog(enabled), nil
		case "modules.setEnabled":
			var request struct {
				ID       string `json:"id"`
				Enabled  bool   `json:"enabled"`
				Revision uint64 `json:"revision"`
			}
			if err := json.Unmarshal(params, &request); err != nil || request.ID != "wintray" || request.Revision != 7 {
				t.Errorf("setEnabled %s", params)
			}
			enabled = request.Enabled
			return map[string]string{"id": "job-1", "state": "completed"}, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
	page := newPage(h.Env)
	tt := ui.NewTester(page.View, 900, 700)
	h.Settle(tt)
	if !tt.HasText("截图") || !tt.HasText("程序管理") || tt.HasText("鼠标手势") {
		t.Fatalf("tools %q", tt.Texts())
	}
	capture, _ := tt.Find("截图")
	programs, _ := tt.Find("程序管理")
	if capture.Y > programs.Y {
		t.Fatal("Home must follow the sidebar's module order")
	}
	if err := tt.Click("启用程序管理"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if len(h.Called("modules.setEnabled")) != 1 || !enabled {
		t.Fatalf("calls %+v", h.Calls())
	}
	if !tt.HasText("已开启") {
		t.Fatalf("status after enabling %q", tt.Texts())
	}
}

// Home lists exactly the sidebar's features above its footer, in the same
// order. Adding a feature to the sidebar without telling Home how to show
// it (its module, or built in) fails here, as does a mapping left behind.
func TestHomeMatchesTheSidebar(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	features := shell.Features()
	names := func(zh, en string) map[string]string { return map[string]string{"zh-CN": zh, "en-US": en} }
	snapshot := widgets.ModuleSnapshot{Revision: 1}
	for _, feature := range features {
		id, ok := homeModules[feature.ID]
		if !ok && !builtIn[feature.ID] {
			t.Fatalf("Home does not know the sidebar feature %q", feature.ID)
		}
		if ok {
			snapshot.Modules = append(snapshot.Modules, widgets.ModuleEntry{Manifest: widgets.ModuleManifest{ID: id, Name: names(id, id),
				Distribution: "builtin"}, Runtime: widgets.ModuleRuntime{ID: id, State: "disabled"}})
		}
	}
	for id := range homeModules {
		if !slices.ContainsFunc(features, func(f shell.PageInfo) bool { return f.ID == id }) {
			t.Fatalf("Home maps %q, which the sidebar no longer has", id)
		}
	}
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "modules.snapshot" {
			return snapshot, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
	page := newPage(h.Env)
	tt := ui.NewTester(page.View, 900, 1400)
	h.Settle(tt)
	rows := page.rows()
	if len(rows) != len(features) {
		t.Fatalf("Home lists %d features, the sidebar %d", len(rows), len(features))
	}
	previous := float32(-1)
	for i, feature := range features {
		if rows[i].page.ID != feature.ID {
			t.Fatalf("row %d is %q, the sidebar's %q", i, rows[i].page.ID, feature.ID)
		}
		r, ok := tt.Find(i18n.T(feature.Title))
		if !ok || r.Y <= previous {
			t.Fatalf("%q is not shown in the sidebar's order: %q", i18n.T(feature.Title), tt.Texts())
		}
		previous = r.Y
	}
	if tt.HasText(i18n.T("onebot.title")) || tt.HasText(i18n.T("chat.title")) {
		t.Fatal("Home lists only the features above the sidebar's footer")
	}
}
