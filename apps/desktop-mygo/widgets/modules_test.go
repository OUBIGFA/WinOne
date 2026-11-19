package widgets_test

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/widgets"
)

func TestModuleStartupRefreshesUntilItSettlesWithoutFocusChange(t *testing.T) {
	var settled atomic.Bool
	h := testhost.New(t, func(string, json.RawMessage) (any, error) {
		state := "starting"
		if settled.Load() {
			state = "faulted"
		}
		return widgets.ModuleSnapshot{Modules: []widgets.ModuleEntry{{
			Manifest: widgets.ModuleManifest{ID: "wintray"}, Enabled: true,
			Runtime: widgets.ModuleRuntime{ID: "wintray", State: state, Detail: "startup failed"},
		}}}, nil
	})
	m := widgets.NewModules(h.Env)
	tt := ui.NewTester(m.Notices, 560, 400)
	h.Settle(tt)
	settled.Store(true)
	// A real elapsed refresh interval exercises the production view's timer;
	// no focus event or explicit refresh supplies the new module state.
	time.Sleep(1100 * time.Millisecond)
	tt.Frame()
	h.Settle(tt)
	if m.Snapshot.Modules[0].Runtime.State != "faulted" {
		t.Fatal("startup status stayed stale without a focus change")
	}
	reads := len(h.Called("modules.snapshot"))
	time.Sleep(1100 * time.Millisecond)
	tt.Frame()
	h.Settle(tt)
	if len(h.Called("modules.snapshot")) != reads {
		t.Fatal("settled modules kept polling while idle")
	}
}
