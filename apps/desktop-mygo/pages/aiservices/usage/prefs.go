package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// viewPrefs are the charts' viewing choices: the metric and style of the
// trend charts and the heatmap's metric. They are not AI settings: the
// React window kept them in its WebView storage (winone.usage.metric,
// .chart and .heatmapMetric), so this window keeps them in the profile
// beside its other window state, apart from the Host's settings.
type viewPrefs struct {
	Version int    `json:"version"`
	Metric  string `json:"metric"`
	Chart   string `json:"chart"`
	Heatmap string `json:"heatmapMetric"`
	// directory is where they are kept; "" keeps them in memory only.
	directory string
}

const viewPrefsFile = "desktop-usage-view.json"

var (
	chartMetrics   = []string{"tokens", "cost", "calls", "speed"}
	chartStyles    = []string{"bars", "line"}
	heatmapMetrics = []string{"tokens", "calls", "cost"}
)

// loadViewPrefs reads the saved choices; a missing or unreadable file,
// or a choice the charts do not have, is the default, as React's
// readChartChoice fell back.
func loadViewPrefs(directory string) *viewPrefs {
	v := &viewPrefs{Version: 1, Metric: "tokens", Chart: "bars", Heatmap: "tokens", directory: directory}
	if directory == "" {
		return v
	}
	data, err := os.ReadFile(filepath.Join(directory, viewPrefsFile))
	if err != nil || len(data) > 4096 {
		return v
	}
	var saved viewPrefs
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		return v
	}
	if slices.Contains(chartMetrics, saved.Metric) {
		v.Metric = saved.Metric
	}
	if slices.Contains(chartStyles, saved.Chart) {
		v.Chart = saved.Chart
	}
	if slices.Contains(heatmapMetrics, saved.Heatmap) {
		v.Heatmap = saved.Heatmap
	}
	return v
}

// save writes the choices through a temporary file. A choice that cannot
// be written stays for this window, as React kept the in-memory choice
// when its storage was unavailable: the charts still show it.
func (v *viewPrefs) save() {
	if v.directory == "" {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	temporary := filepath.Join(v.directory, ".desktop-usage-view-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".tmp")
	if os.WriteFile(temporary, data, 0o600) != nil {
		return
	}
	if os.Rename(temporary, filepath.Join(v.directory, viewPrefsFile)) != nil {
		_ = os.Remove(temporary)
	}
}
