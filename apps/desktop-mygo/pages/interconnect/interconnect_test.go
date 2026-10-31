package interconnect

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

type call struct {
	method string
	params map[string]any
}

// interconnectHost answers the interconnect service and records calls.
func interconnectHost(t *testing.T, generalSettings general, calls *[]call) *testhost.Harness {
	settings := preferences{Bind: "0.0.0.0", Port: 8080, DownloadDir: `C:\Downloads`}
	return testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		var args map[string]any
		_ = json.Unmarshal(params, &args)
		*calls = append(*calls, call{method, args})
		switch method {
		case "interconnect.get":
			return state{Settings: settings, Addresses: []address{{Name: "Wi-Fi", IP: "192.168.1.5"}}}, nil
		case "interconnect.general.get":
			return generalSettings, nil
		case "interconnect.transfer_list":
			return []taskRecord{}, nil
		case "interconnect.menu.get":
			return menuState{}, nil
		case "interconnect.entry.get":
			return entryState{}, nil
		case "interconnect.preferences.save":
			_ = json.Unmarshal(params, &settings)
			return settings, nil
		case "interconnect.transfer_start":
			return map[string]string{"id": "task-1"}, nil
		}
		t.Errorf("unexpected %s", method)
		return nil, nil
	})
}

func called(calls []call, method string) []call {
	var found []call
	for _, c := range calls {
		if c.method == method {
			found = append(found, c)
		}
	}
	return found
}

func TestSettingsSaveOnlyTheChangedPart(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	var calls []call
	h := interconnectHost(t, general{Notify: true, DownloadDir: `C:\Downloads`}, &calls)
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1100, 1400)
	h.Settle(tt)
	if err := tt.Click("常规"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !tt.HasText("共享设置") || !h.Env.Rail.Save.Disabled {
		t.Fatalf("general tab %q", tt.Texts())
	}
	p.draft.Port = 9090
	h.Settle(tt)
	if h.Env.Rail.Save.Disabled {
		t.Fatal("a changed port does not enable Save")
	}
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	saves := called(calls, "interconnect.preferences.save")
	if len(saves) != 1 || saves[0].params["port"] != 9090.0 || len(called(calls, "interconnect.general.save")) != 0 {
		t.Fatalf("saves %+v", calls)
	}
	if dirty, _ := h.Env.Activity.State(); dirty {
		t.Fatal("still dirty after saving")
	}
}

func TestSendingFilesStartsOneShare(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	var calls []call
	h := interconnectHost(t, general{DownloadDir: `C:\Downloads`}, &calls)
	p := newPage(h.Env)
	p.files.Apply(core.Intent{Requested: true, Paths: []string{`C:\a.txt`, `c:\A.TXT`, `C:\b.txt`}})
	tt := ui.NewTester(p.View, 1100, 1600)
	h.Settle(tt)
	if len(p.files.form.Paths) != 2 || !tt.HasText(`C:\b.txt`) {
		t.Fatalf("paths %v", p.files.form.Paths)
	}
	// "发送" also names the direction; start as the submit button does.
	p.files.start("send", nil)
	h.Settle(tt)
	starts := called(calls, "interconnect.transfer_start")
	if len(starts) != 1 || starts[0].params["mode"] != "share_server" {
		t.Fatalf("starts %+v", starts)
	}
	if transfer, _ := starts[0].params["transfer"].(map[string]any); transfer == nil {
		t.Fatalf("share without its code sender %+v", starts[0].params)
	}
	if len(p.files.form.Paths) != 0 || p.files.shareTarget != "task-1" {
		t.Fatalf("after sending: %v, target %q", p.files.form.Paths, p.files.shareTarget)
	}
}

func TestClipboardCodeIsOfferedNotUsed(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	var calls []call
	h := interconnectHost(t, general{Clipboard: true, DownloadDir: `C:\Downloads`}, &calls)
	focuses := 0
	h.Env.Focuses = func() int { return focuses }
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1100, 1600)
	h.Settle(tt)
	// The window regains the focus with a code on the clipboard.
	tt.SetClipboard("7392-alpha-bravo-charlie")
	focuses++
	tt.Frame()
	h.Settle(tt)
	if !tt.HasText("检测到传输口令") {
		t.Fatalf("no offer %q", tt.Texts())
	}
	if len(called(calls, "interconnect.transfer_start")) != 0 {
		t.Fatal("a clipboard code connected by itself")
	}
	if err := tt.Click("使用此口令"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if p.files.direction != "receive" || p.files.form.Code != "7392-alpha-bravo-charlie" || p.clipboard != nil {
		t.Fatalf("code not filled: %q %q", p.files.direction, p.files.form.Code)
	}
}

// Every tab switch asks the page's unsaved-change guard, which decides
// before the tab changes.
func TestTabsAskTheGuard(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	var calls []call
	h := interconnectHost(t, general{DownloadDir: `C:\Downloads`}, &calls)
	requested := 0
	h.Env.RequestChange = func(apply func()) { requested++ }
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1100, 1400)
	h.Settle(tt)
	if err := tt.Click("常规"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if requested != 1 || p.tab != tabFiles {
		t.Fatalf("requested %d, tab %d", requested, p.tab)
	}
}
