package aiservices

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

// Installs only this generated no-network plugin into the test runtime's own
// registry. It never uses the installed WinOne profile or a real account.
func TestNativePluginRealLocalInstallOptionsSwitchAndRemove(t *testing.T) {
	p, h, tt, _ := nativeAIWithRuntime(t)
	folder := t.TempDir()
	for file, body := range map[string]string{
		"package.json": `{"name":"winone-native-fixture","version":"1.0.0","type":"module","main":"index.mjs","magpie":{"name":"Native fixture"}}`,
		"index.mjs":    `export default async () => ({});`,
		"README.md":    "# Native plugin fixture\nNo network or account access.\n",
	} {
		if err := os.WriteFile(filepath.Join(folder, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p.switchTab(tabPlugins)
	settle(h, tt)
	x := p.plugins
	if x.state == nil || x.api.err != "" {
		t.Fatal("list:", x.api.err)
	}
	x.act("add", map[string]string{"spec": folder})
	settle(h, tt)
	if x.api.err != "" || len(x.state.Plugins) != 1 {
		t.Fatalf("install: %s %+v", x.api.err, x.state)
	}
	spec := x.state.Plugins[0].Spec
	x.act("options", map[string]any{"spec": spec, "options": map[string]any{"greeting": "kept"}})
	settle(h, tt)
	if x.api.err != "" || x.state.Plugins[0].Options["greeting"] != "kept" {
		t.Fatal("options:", x.api.err)
	}
	x.act("off", map[string]any{"spec": spec, "off": true})
	settle(h, tt)
	if x.api.err != "" || !x.state.Plugins[0].Off {
		t.Fatal("switch:", x.api.err)
	}
	x.load()
	settle(h, tt)
	if len(x.state.Plugins) != 1 || !x.state.Plugins[0].Off {
		t.Fatal("registry did not persist switch")
	}
	x.showReadme(spec)
	settle(h, tt)
	if x.readmeAPI.err != "" || x.readme == "" {
		t.Fatal("README:", x.readmeAPI.err)
	}
	x.readmeTitle = ""
	shot(t, tt, "native-plugin-real")
	x.act("remove", map[string]string{"spec": spec})
	settle(h, tt)
	if x.api.err != "" || len(x.state.Plugins) != 0 {
		t.Fatal("remove:", x.api.err)
	}
}

func TestNativeGroupDeletingMiddleRuleDoesNotLoseFollowingRules(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	doc := core.Preserved[routeGroup]{Value: routeGroup{
		ID: "rules", Members: []string{"p/m"},
		Rules: []core.Preserved[routeRule]{
			{Value: routeRule{Use: "p/m", Intent: "first"}},
			{Value: routeRule{Use: "p/m", Intent: "second"}},
			{Value: routeRule{Use: "p/m", Intent: "third"}},
		},
	}}
	r.begin(doc)
	// The editor opens on the identity section. Enter the rules section before
	// exercising the row action so the test follows the user-visible workflow.
	r.editorTab = 2
	tt := ui.NewTester(r.editor, 900, 4000)
	tt.Frame()
	// Click the first delete control while later rows are still rendered.
	if err := tt.Click(nativeT("deleteRule")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(r.edit.Value.Rules) != 2 || r.edit.Value.Rules[0].Value.Intent != "second" {
		t.Fatal("wrong rule removed")
	}
}
