package convenience

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
	"winone/internal/timedpower"
)

// Temporary: renders timed power idle and running for a visual check.
func TestZZShots(t *testing.T) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		t.Skip()
	}
	i18n.SetLocale(i18n.Chinese)
	for _, running := range []bool{false, true} {
		preferences := timedpower.Defaults()
		h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
			switch method {
			case "modules.snapshot":
				return modules("timed-power"), nil
			case "always-on-top.snapshot":
				return map[string]any{"settings": map[string]any{"schemaVersion": 1, "frameThickness": 3, "frameOpacity": 100, "frameColor": "#808080", "excludedApps": []string{}}}, nil
			case "timed-power.settings.get":
				return preferences, nil
			case "timed-power.snapshot":
				if running {
					task := preferences
					return timedpower.Snapshot{State: "scheduled", Active: true, Task: &task, Deadline: time.Now().Add(42*time.Minute + 17*time.Second)}, nil
				}
				return timedpower.Snapshot{State: "idle"}, nil
			}
			return nil, nil
		})
		p := newPage(h.Env)
		tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 1000, 900)
		h.Settle(tt)
		tt.Click("定时关机")
		h.Settle(tt)
		name := "power-idle.png"
		if running {
			name = "power-running.png"
		}
		f, _ := os.Create(filepath.Join(dir, name))
		png.Encode(f, tt.Image())
		f.Close()
	}
}
