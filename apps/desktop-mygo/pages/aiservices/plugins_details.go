package aiservices

import (
	"encoding/json"
	"fmt"
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
type pluginVersionCheck struct {
	Spec, Package, Status, Latest, Head, Why, Error string
	Limited                                         *struct {
		Until string
		Token bool
	}
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
		if x.state != nil {
			for i := range x.state.Plugins {
				p := &x.state.Plugins[i]
				name := p.Package
				if name == "" {
					name = p.Spec
				}
				if info, ok := value.NPM[name]; ok {
					p.Latest = info.Version
				}
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
	// The upstream check returns fresh npm/Git results and a state snapshot in
	// one operation. Preserve each unknown result rather than implying success.
	x.api.request(x.p, "POST", "/api/plugins/check", nil, func(raw json.RawMessage) {
		var value struct {
			State   managedPlugins `json:"state"`
			Plugins []pluginVersionCheck
		}
		if nativeDecode(&x.api, raw, &value) {
			if value.State.Plugins == nil {
				x.api.err = nativeT("invalidResponse")
				return
			}
			x.state = &value.State
			x.checks = map[string]pluginVersionCheck{}
			for _, check := range value.Plugins {
				x.checks[check.Spec] = check
			}
		}
	})
}

func pluginCheckReason(check pluginVersionCheck) string {
	if check.Limited != nil {
		return upstreamT("GitHub's rate limit for your GitHub token is used up for now") + " · " + check.Limited.Until
	}
	if pluginRepository(check.Spec) != "" {
		return upstreamT("Couldn't ask {repo} for its newest commit: {error}", "repo", check.Spec, "error", check.Error)
	}
	switch check.Why {
	case "offline":
		return upstreamT("npm couldn't be reached — check your connection or proxy")
	case "limited":
		return upstreamT("npm is turning requests away for now (too many); try again in a few minutes")
	case "missing":
		return upstreamT("npm doesn't have it")
	default:
		return upstreamT("npm answered with an error: {error}", "error", check.Error)
	}
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
		Gateway *struct {
			LastOutcome                string `json:"lastOutcome"`
			Calls, LastBytes, Failures int
			AvgMillis                  float64 `json:"avgMillis"`
		} `json:"gateway"`
		Middleware *struct {
			Hooks, Strategies, Events              []string
			Calls, Orders, Rows, Failures, Dropped int
			Usage                                  bool
			AvgMicros                              float64 `json:"avgMicros"`
			Error, LastError                       string
		} `json:"middleware"`
		Agent *struct {
			Name    string `json:"name"`
			Missing bool   `json:"missing"`
			Looks   string `json:"looks"`
			Notice  string `json:"notice"`
			Error   string `json:"error"`
		} `json:"agent"`
		AutoUpdated *struct {
			From, To string
			At       int64
		} `json:"autoUpdated"`
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
	if m := details.Middleware; m != nil {
		if m.Error != "" {
			kit.InlineError(c, upstreamT("Middleware didn't load: {error}", "error", m.Error))
		}
		if len(m.Hooks) > 0 {
			kit.Help(c, upstreamT("Gateway middleware: {hooks}", "hooks", strings.Join(m.Hooks, ", "))+" · "+upstreamT("{n} calls, {us} µs each", "n", m.Calls, "us", fmt.Sprintf("%.1f", m.AvgMicros)))
		}
		if len(m.Strategies) > 0 {
			kit.Help(c, upstreamT("Routings: {routings}", "routings", strings.Join(m.Strategies, ", "))+" · "+upstreamT("asked {n} times", "n", m.Orders))
		}
		if m.Usage {
			kit.Help(c, upstreamT("Usage observer: {n} rows", "n", m.Rows))
		}
		if len(m.Events) > 0 {
			kit.Help(c, upstreamT("onEvent sees only {events} events", "events", strings.Join(m.Events, ", ")))
		}
		if m.Failures > 0 {
			kit.InlineError(c, upstreamT("{n} failed", "n", m.Failures)+" · "+m.LastError)
		}
		if m.Dropped > 0 {
			kit.Help(c, upstreamT("{n} rows dropped", "n", m.Dropped))
		}
	}
	if g := details.Gateway; g != nil {
		labels := map[string]string{"idle": "Waiting for a request", "ready": "Context injected", "empty": "No matching context", "unbound": "Scope not bound", "unauthorized": "Authorization failed", "unavailable": "Context service unavailable", "invalid": "Invalid context response", "skipped": "Request skipped", "cancelled": "Request cancelled", "timeout": "Recall timed out", "error": "Request hook failed"}
		label := labels[g.LastOutcome]
		if label == "" {
			label = labels["idle"]
		}
		kit.Help(c, upstreamT("Gateway request hook")+": "+upstreamT(label)+" · "+upstreamT("{n} calls, {ms} ms each", "n", g.Calls, "ms", fmt.Sprintf("%.1f", g.AvgMillis)))
		if g.LastBytes > 0 {
			kit.Help(c, upstreamT("{n} bytes injected", "n", g.LastBytes))
		}
		if g.Failures > 0 {
			kit.InlineError(c, upstreamT("{n} failed", "n", g.Failures))
		}
	}
	if u := details.AutoUpdated; u != nil && u.To == plugin.Version {
		kit.Help(c, upstreamT("Auto-updated")+" · "+u.From+" → "+u.To)
	}
	for _, tool := range plugin.Tools {
		kit.Help(c, tool.Name+" · "+tool.Description).Tooltip(tool.MCP)
	}
}
