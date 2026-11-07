package aiservices

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Presentation choices belong to this profile's window state, separate from
// AI configuration. They must never turn a view change into an unsaved policy.
type routingViewPrefs struct {
	GroupBy string          `json:"groupBy"`
	Metrics map[string]bool `json:"metrics"`
}

func (t *nativeTrace) loadViewPrefs(directory string) {
	if directory == "" {
		return
	}
	t.prefsPath = filepath.Join(directory, "desktop-routing-view.json")
	data, err := os.ReadFile(t.prefsPath)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.prefsError = err.Error()
		return
	}
	var value routingViewPrefs
	if err = json.Unmarshal(data, &value); err != nil {
		t.prefsError = err.Error()
		return
	}
	if value.GroupBy == "" || value.GroupBy == "session" || value.GroupBy == "key" {
		t.groupBy = value.GroupBy
	}
	t.metrics = value.Metrics
}
func (t *nativeTrace) saveViewPrefs() {
	if t.prefsPath == "" {
		return
	}
	data, err := json.Marshal(routingViewPrefs{t.groupBy, t.metrics})
	if err == nil {
		err = os.WriteFile(t.prefsPath, data, 0600)
	}
	t.prefsError = ""
	if err != nil {
		t.prefsError = err.Error()
	}
}
