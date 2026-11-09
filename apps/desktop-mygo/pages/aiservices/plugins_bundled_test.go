package aiservices

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestBundledPluginRemoveAndReinstallUseSavedChoice(t *testing.T) {
	_, h, p, _ := setup(t)
	p.advanced = newAdvancedPanel(p)
	p.advanced.catalog = &magpieCatalog{BundledPlugins: []bundledPlugin{{Directory: "fixture", Package: "@fixture/plugin", Name: "Fixture", Version: "1.0.0", Providers: []string{"fixture"}}}}
	x := p.plugins
	x.state = &managedPlugins{}
	tt := ui.NewTester(func(c *ui.Context) { ui.Text(c, "fixture") }, 400, 100)
	x.act("remove", map[string]string{"spec": "@fixture/plugin"})
	settle(h, tt)
	if x.installed("@fixture/plugin") {
		t.Fatal("removed bundled plugin still appears installed")
	}
	x.act("add", map[string]string{"spec": "@fixture/plugin"})
	settle(h, tt)
	if !x.installed("@fixture/plugin") {
		t.Fatal("reinstall did not restore the saved bundle choice")
	}
	if !p.draft.Value.settings().BundledPlugins["fixture"].Value.Enabled {
		t.Fatal("reinstall left the plugin disabled")
	}
}
