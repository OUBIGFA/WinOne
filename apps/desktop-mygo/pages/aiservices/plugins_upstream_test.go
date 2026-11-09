package aiservices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

func TestPluginManagerStartsAtUpstreamDiscover(t *testing.T) {
	_, _, p, _ := setup(t)
	if p.plugins.tab != 1 {
		t.Fatalf("plugins must open Discover (index 1), got %d", p.plugins.tab)
	}
}

// A market card's Install is the install action, not navigation to another
// source form where the user must find and press Install a second time.
func TestPluginMarketInstallsFromItsCard(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	var installed string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/key/api/plugins/add" {
			var body struct{ Spec string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			installed = body.Spec
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"plugins":[]}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	x := p.plugins
	x.state = &managedPlugins{}
	x.marketLoaded = true
	x.market = []marketPlugin{{Package: "@fixture/plugin", Name: "Fixture plugin"}}
	tt := ui.NewTester(func(c *ui.Context) { x.marketView(c, false) }, 900, 2400)
	tt.Frame()
	if err := tt.Click(nativeT("install")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if installed != "@fixture/plugin" {
		t.Fatalf("card did not install its package: %q", installed)
	}
}

func TestInstalledPluginDoesNotOfferDowngrade(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	p.plugins.state = &managedPlugins{Plugins: []managedPlugin{{Spec: "@fixture/plugin", OwnName: "Fixture", Version: "2.0.0", Latest: "1.9.9"}}}
	tt := ui.NewTester(func(c *ui.Context) { p.plugins.installedView(c, false) }, 900, 1600)
	settle(h, tt)
	if tt.HasText(nativeT("update")) {
		t.Fatal("older npm version must not be offered as an update")
	}
}

// Discovery must act on the selected source: empty package metadata cannot
// accidentally match the first installed Git plugin, and pinned npm versions
// still name the same package. These cases exercise the visible next action.
func TestPluginMarketUsesSourceIdentity(t *testing.T) {
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	for _, tc := range []struct {
		name                string
		installed           managedPlugin
		entry               marketPlugin
		label, action, spec string
	}{
		{"unrelated git", managedPlugin{Spec: "github:first/plugin"}, marketPlugin{Spec: "github:second/plugin"}, nativeT("install"), "add", "github:second/plugin"},
		{"pinned npm", managedPlugin{Spec: "@fixture/plugin@1.0.0", Version: "1.0.0", Latest: "1.1.0"}, marketPlugin{Package: "@fixture/plugin"}, nativeT("update"), "upgrade", "@fixture/plugin@1.0.0"},
		{"disabled npm", managedPlugin{Spec: "@fixture/plugin", Off: true}, marketPlugin{Package: "@fixture/plugin"}, upstreamT("Switch on"), "off", "@fixture/plugin"},
		{"github url", managedPlugin{Spec: "git+https://github.com/owner/plugin.git", Commit: "abc", Head: "def"}, marketPlugin{Spec: "github:owner/plugin", Repo: "owner/plugin"}, nativeT("update"), "upgrade", "git+https://github.com/owner/plugin.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, h, p, _ := setup(t)
			i18n.SetLocale(i18n.English)
			var action, spec string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				action = r.URL.Path
				var body struct{ Spec string }
				_ = json.NewDecoder(r.Body).Decode(&body)
				spec = body.Spec
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"plugins":[]}`))
			}))
			defer server.Close()
			p.extensions.state.URL = server.URL + "/session/key/plugins.html"
			p.plugins.state = &managedPlugins{Plugins: []managedPlugin{tc.installed}}
			tt := ui.NewTester(func(c *ui.Context) { p.plugins.marketAction(c, tc.entry, false) }, 600, 180)
			tt.Frame()
			if err := tt.Click(tc.label); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if action != "/session/key/api/plugins/"+tc.action || spec != tc.spec {
				t.Fatalf("wrong selected plugin action: %s %s", action, spec)
			}
		})
	}
}

func TestPluginUpdateCheckShowsUnresolvedResults(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/session/key/api/plugins/check" {
			http.Error(w, "wrong check operation", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"plugins":[{"spec":"@fixture/plugin","status":"unknown","why":"offline","error":"fixture network unavailable"}],"state":{"plugins":[{"spec":"@fixture/plugin","version":"1.0.0"}]}}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	x := p.plugins
	x.state = &managedPlugins{Plugins: []managedPlugin{{Spec: "@fixture/plugin", Version: "1.0.0"}}}
	tt := ui.NewTester(func(c *ui.Context) { x.installedView(c, false) }, 1000, 1600)
	x.checkUpdates()
	settle(h, tt)
	if calls != 1 || !tt.HasText(upstreamT("Couldn't check")) {
		t.Fatalf("update check lost failure: calls=%d texts=%q error=%s", calls, tt.Texts(), x.api.err)
	}
}

// Tool sharing is a Library operation: show failures here and read the actual
// saved plugin state instead of optimistically flipping the button.
func TestPluginToolsSharingReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, receipt, visible string
		status, reads          int
	}{
		{"failed", `{"error":"fixture client write failed"}`, "fixture client write failed", 500, 0},
		{"invalid", `{}`, nativeT("invalidResponse"), 200, 0},
		{"problems", `{"agents":[],"servers":[],"skills":[],"problems":["fixture client unavailable"]}`, "fixture client unavailable", 200, 1},
		{"saved", `{"agents":[],"servers":[],"skills":[]}`, "", 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, h, p, _ := setup(t)
			var posts, reads int
			const state = `{"plugins":[{"spec":"fixture-tools","ownName":"Fixture tools","toolsServer":{"plugin":"fixture-tools","on":false}}]}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/session/key/api/library/servers/plugin-on":
					var body struct {
						Name string
						On   bool
					}
					if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Name != "fixture-tools" || !body.On {
						http.Error(w, "wrong tool-sharing action", 400)
						return
					}
					posts++
					w.WriteHeader(tc.status)
					w.Write([]byte(tc.receipt))
				case "/session/key/api/plugins":
					reads++
					w.Write([]byte(state))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			p.extensions.state.URL = server.URL + "/session/key/plugins.html"
			x := p.plugins
			x.source = p.extensions.state.URL
			x.tab = 2
			if err := json.Unmarshal([]byte(state), &x.state); err != nil {
				t.Fatal(err)
			}
			p.external = newExternalPanel(p)
			p.external.url = server.URL + "/session/key/library.html"
			tt := ui.NewTester(x.View, 1000, 1800)
			settle(h, tt)
			if err := tt.Click(upstreamT("Give to agents")); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if posts != 1 || reads != tc.reads || x.state.Plugins[0].ToolsServer.On {
				t.Fatalf("sharing did not use saved state: posts=%d reads=%d state=%+v", posts, reads, x.state.Plugins[0].ToolsServer)
			}
			if tc.visible != "" && !tt.HasText(tc.visible) {
				t.Fatalf("sharing result not visible: want %q texts=%q", tc.visible, tt.Texts())
			}
		})
	}
}
