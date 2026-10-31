package convenience

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	"winone/internal/timedpower"
)

func modules(ids ...string) map[string]any {
	var entries []map[string]any
	for _, id := range ids {
		entries = append(entries, map[string]any{
			"manifest": map[string]any{"id": id, "name": map[string]string{"zh-CN": id, "en-US": id}},
			"enabled":  true, "runtime": map[string]any{"id": id, "state": "running"},
		})
	}
	return map[string]any{"revision": 3, "modules": entries}
}

func TestPinningSavesEditsAndKeepsUnknownFields(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	var saved json.RawMessage
	settings := map[string]any{"schemaVersion": 1, "revision": 4, "enabled": true, "frame": true, "frameThickness": 3,
		"frameColor": "#808080", "frameOpacity": 100, "useAccent": true, "roundCorners": true, "blockInGameMode": true,
		"sound": false, "excludedApps": []string{"game.exe"}}
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "modules.snapshot":
			return modules("always-on-top"), nil
		case "always-on-top.snapshot":
			return map[string]any{"settings": settings, "administrator": false}, nil
		case "always-on-top.settings.save":
			saved = params
			var next map[string]any
			_ = json.Unmarshal(params, &next)
			return next, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 900)
	h.Settle(tt)
	if !tt.HasText("显示置顶边框") || !tt.HasText("启用always-on-top") && !tt.HasText("开启 always-on-top") && len(h.Called("modules.snapshot")) == 0 {
		t.Fatalf("pinning tab %q", tt.Texts())
	}
	if h.Env.Rail.Save.Disabled == false {
		t.Fatal("Save is available before any edit")
	}
	if err := tt.Click("播放提示音"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if h.Env.Rail.Save.Disabled {
		t.Fatal("an edit does not enable Save")
	}
	if dirty, _ := h.Env.Activity.State(); !dirty {
		t.Fatal("an edit is not guarded")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	var sent map[string]any
	if err := json.Unmarshal(saved, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["sound"] != true || sent["enabled"] != true || sent["frameThickness"] != 3.0 {
		t.Fatalf("saved %s", saved)
	}
	if apps, _ := sent["excludedApps"].([]any); len(apps) != 1 || apps[0] != "game.exe" {
		t.Fatalf("exclusions %s", saved)
	}
	if dirty, _ := h.Env.Activity.State(); dirty {
		t.Fatal("still dirty after saving")
	}
}

func TestTimedPowerConfirmsBeforeStarting(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	preferences := timedpower.Defaults()
	preferences.Force = false
	var started []json.RawMessage
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "modules.snapshot":
			return modules("timed-power"), nil
		case "always-on-top.snapshot":
			return map[string]any{"settings": map[string]any{"schemaVersion": 1, "frameThickness": 3, "frameOpacity": 100, "frameColor": "#808080", "excludedApps": []string{}}}, nil
		case "timed-power.settings.get":
			return preferences, nil
		case "timed-power.snapshot":
			return timedpower.Snapshot{State: "idle"}, nil
		case "timed-power.start":
			started = append(started, params)
			task := preferences
			return timedpower.Snapshot{State: "scheduled", Active: true, Task: &task, Deadline: time.Now().Add(time.Hour)}, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 1400)
	h.Settle(tt)
	if err := tt.Click("定时关机"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !tt.HasText("倒计时时长") || !tt.HasText("暂无运行中的任务") {
		t.Fatalf("timed power %q", tt.Texts())
	}
	if err := tt.Click("开始计时"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if len(started) != 0 || !tt.HasText("开始定时电源操作？") {
		t.Fatalf("started without confirmation: %d %q", len(started), tt.Texts())
	}
	if err := tt.Click("确认并开始"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if len(started) != 1 {
		t.Fatalf("starts %d", len(started))
	}
	var request struct {
		Preferences timedpower.Preferences `json:"preferences"`
		Confirmed   bool                   `json:"confirmed"`
	}
	if err := json.Unmarshal(started[0], &request); err != nil || !request.Confirmed || request.Preferences.DelaySeconds != 3600 {
		t.Fatalf("start %s", started[0])
	}
	if !tt.HasText("取消任务") {
		t.Fatalf("running task %q", tt.Texts())
	}
}
