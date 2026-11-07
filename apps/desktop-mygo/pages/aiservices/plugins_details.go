package aiservices

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

type pluginNPM struct {
	Version     string `json:"version"`
	Description string `json:"description"`
	Publisher   string `json:"publisher"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Repository  string `json:"repository"`
	Weekly      *int   `json:"weekly"`
}

// Discover first shows the upstream listings, then fills npm metadata. It does
// not replace an unknown package with a fabricated version or a missing result.
func (x *pluginsPanel) loadNPMInfo() {
	var names []string
	for _, entry := range x.market {
		if entry.Package != "" && len(names) < 100 {
			names = append(names, entry.Package)
		}
	}
	if len(names) == 0 {
		return
	}
	x.npmAPI.request(x.p, "GET", "/api/plugins/npm?names="+url.QueryEscape(strings.Join(names, ",")), nil, func(raw json.RawMessage) {
		var value struct {
			NPM map[string]pluginNPM `json:"npm"`
		}
		if !nativeDecode(&x.npmAPI, raw, &value) {
			return
		}
		for i := range x.market {
			if info, ok := value.NPM[x.market[i].Package]; ok {
				copy := info
				x.market[i].NPM = &copy
			}
		}
	})
}
func (x *pluginsPanel) marketInfoView(c *ui.Context, entry marketPlugin) {
	var info pluginNPM
	if entry.NPM != nil {
		info = *entry.NPM
	} else {
		_ = json.Unmarshal(entry.Raw, &info)
	}
	var facts []string
	if info.Version != "" {
		facts = append(facts, nativeT("latestVersion")+": "+info.Version)
	}
	if info.Publisher != "" {
		facts = append(facts, nativeT("publisher")+": "+info.Publisher)
	}
	if info.License != "" {
		facts = append(facts, info.License)
	}
	if info.Weekly != nil {
		facts = append(facts, nativeT("weeklyDownloads")+": "+kit.Itoa(*info.Weekly))
	}
	if len(facts) > 0 {
		kit.Help(c, strings.Join(facts, " · "))
	}
	for _, link := range []struct{ label, value string }{{"pluginHomepage", info.Homepage}, {"pluginRepository", info.Repository}, {"pluginRepository", entry.URL}} {
		if parsed, err := url.Parse(link.value); err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			ui.Link(c, nativeT(link.label), link.value)
		}
	}
}
func (x *pluginsPanel) checkUpdates() {
	// Refresh Git heads as well as npm versions; identical package versions
	// at different commits still represent a real Git plugin update.
	x.api.request(x.p, "GET", "/api/plugins/git", nil, func(json.RawMessage) {
		x.api.request(x.p, "POST", "/api/plugins/check", nil, func(raw json.RawMessage) {
			var value struct {
				State managedPlugins `json:"state"`
			}
			if nativeDecode(&x.api, raw, &value) {
				if value.State.Plugins == nil {
					x.api.err = nativeT("invalidResponse")
					return
				}
				x.state = &value.State
			}
		})
	})
}

func (x *pluginsPanel) pluginDetailsView(c *ui.Context, plugin managedPlugin) {
	var details struct {
		Page *struct {
			Title    string   `json:"title"`
			Reads    []string `json:"reads"`
			Replaces string   `json:"replaces"`
		} `json:"page"`
		Panels []struct {
			Title string `json:"title"`
			Slot  string `json:"slot"`
		} `json:"panels"`
		PanelReads []string `json:"panelReads"`
		Actions    []struct {
			Title string `json:"title"`
			Menu  string `json:"menu"`
		} `json:"actions"`
		Gateway    json.RawMessage `json:"gateway"`
		Middleware json.RawMessage `json:"middleware"`
		Agent      *struct {
			Name    string `json:"name"`
			Missing bool   `json:"missing"`
			Looks   string `json:"looks"`
			Notice  string `json:"notice"`
			Error   string `json:"error"`
		} `json:"agent"`
		AutoUpdated json.RawMessage `json:"autoUpdated"`
	}
	if json.Unmarshal(plugin.Raw, &details) != nil {
		return
	}
	if details.Page != nil {
		kit.Strong(c, details.Page.Title)
		kit.Help(c, nativeT("declaredReads")+": "+strings.Join(details.Page.Reads, " · "))
		if details.Page.Replaces != "" {
			kit.Help(c, nativeT("replacesPage")+": "+details.Page.Replaces)
		}
	}
	for _, panel := range details.Panels {
		kit.Strong(c, panel.Title)
		kit.Help(c, nativeT("panelSlot")+": "+panel.Slot)
	}
	if len(details.PanelReads) > 0 {
		kit.Help(c, nativeT("declaredReads")+": "+strings.Join(details.PanelReads, " · "))
	}
	for _, action := range details.Actions {
		kit.Help(c, nativeT("menuAction")+": "+action.Title+" · "+action.Menu)
	}
	if details.Agent != nil {
		kit.Strong(c, details.Agent.Name)
		if details.Agent.Missing {
			kit.Help(c, nativeT("agentMissing")+": "+details.Agent.Looks)
			kit.Help(c, details.Agent.Notice)
		}
		if details.Agent.Error != "" {
			kit.InlineError(c, details.Agent.Error)
		}
	}
	for _, part := range []struct {
		label string
		data  json.RawMessage
	}{{"gatewayRuntime", details.Gateway}, {"middlewareRuntime", details.Middleware}, {"autoUpdateReceipt", details.AutoUpdated}} {
		if len(part.data) > 0 && string(part.data) != "null" {
			kit.Strong(c, nativeT(part.label))
			nativeJSON(c, part.data)
		}
	}
}
