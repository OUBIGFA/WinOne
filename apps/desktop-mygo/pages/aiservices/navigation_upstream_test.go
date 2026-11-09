package aiservices

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

// A disabled built-in must leave navigation, while a plugin kept alive by an
// active consumer remains reachable, as in upstream's data-feature behavior.
func TestBuiltinNavigationFollowsSavedState(t *testing.T) {
	_, _, p, _ := setup(t)
	p.advanced = newAdvancedPanel(p)
	p.advanced.catalog = &magpieCatalog{BuiltinPlugins: []builtinPlugin{
		{ID: "sessions", On: false}, {ID: "library", On: false},
		{ID: "routing", On: false}, {ID: "redact", On: false},
		{ID: "usage", On: true, InUse: 1},
	}}
	p.tab = tabLibrary
	tt := ui.NewTester(p.upstreamNavigation, 560, 400)
	tt.Frame()
	for _, label := range []string{"Sessions", "Library", "Routing", "Routing groups", "Masking"} {
		if tt.HasText(upstreamT(label)) {
			t.Errorf("disabled feature still shown: %s", label)
		}
	}
	if !tt.HasText(upstreamT("Usage")) {
		t.Fatal("in-use feature disappeared")
	}
	if p.tab != tabHome {
		t.Fatal("disabled destination remained active")
	}
	p.advanced.catalog.BuiltinPlugins[1].On = true
	tt.Frame()
	if !tt.HasText(upstreamT("Library")) {
		t.Fatal("enabled destination did not return")
	}
}
