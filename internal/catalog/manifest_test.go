package catalog

import "testing"

func TestBuiltinCatalogKeepsWinTrayWithoutShortcuts(t *testing.T) {
	modules := Builtins()
	if len(modules) != 3 {
		t.Fatal("default package must describe three base modules")
	}
	for _, m := range modules {
		if m.ID == "wintray" && len(m.ShortcutActions) != 0 {
			t.Fatal("WinTray must not invent shortcut actions")
		}
	}
	modules[0].ShortcutActions[0] = "changed"
	if Builtins()[0].ShortcutActions[0] == "changed" {
		t.Fatal("catalog mutation escaped snapshot")
	}
}

func TestManifestRejectsPathsOutsidePackage(t *testing.T) {
	m := Builtins()[0]
	for _, value := range []string{"..", "../worker.exe", "sub/../worker.exe", `C:\worker.exe`, `\\server\worker.exe`, "worker.exe:stream", "./worker.exe", "/worker.exe"} {
		m.Worker = value
		if err := m.Validate(); err == nil {
			t.Fatalf("accepted unsafe worker path %q", value)
		}
	}
}

func TestOptionalModulesCannotShadowProductNamespaces(t *testing.T) {
	m := Builtins()[0]
	m.Distribution = "optional"
	for _, id := range []string{"tool", "capture.capture", "settings.save", "modules.catalog", "shortcuts.snapshot", "residency.snapshot", "profiles.import", "application.restart", "updates.check", "packages.install"} {
		m.ID = id
		if err := m.Validate(); err == nil {
			t.Fatalf("accepted reserved or unqualified ID %q", id)
		}
	}
	m.ID = "example.tool"
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}
