package preferences

import (
	"encoding/json"
	"testing"
)

// Unified Host preferences must receive the same migration as the standalone
// store, without silently accepting invalid values during a normal save.
func TestDecodeMigratesPausedModesAndAcceptsPackagedApps(t *testing.T) {
	for _, version := range []int{3, 4} {
		value := Defaults()
		value.SchemaVersion = version
		raw, _ := json.Marshal(value)
		var fields map[string]any
		json.Unmarshal(raw, &fields)
		fields["managedApps"] = []any{map[string]any{"id": "reader", "name": "Reader", "exePath": `shell:AppsFolder\Example.Reader_123456789abcd!App`, "launchViaLogonTask": true}}
		raw, _ = json.Marshal(fields)
		got, err := Decode(raw)
		if err != nil || got.SchemaVersion != 4 || got.ManagedApps[0].LaunchViaLogonTask != (version == 4) {
			t.Fatalf("schema %d: %+v, %v", version, got, err)
		}
	}
	value := Defaults()
	value.SchemaVersion = 3
	value.StartupIntervalSeconds = 121
	raw, _ := json.Marshal(value)
	if _, err := Decode(raw); err == nil {
		t.Fatal("migration hid an invalid user value")
	}
}

// RunOnStartup is a compatibility projection of the selected mode in v4;
// loading through Host preferences must derive it just as the upstream store.
func TestDecodeKeepsCurrentStartupProjectionConsistentWithMode(t *testing.T) {
	for _, test := range []struct {
		name            string
		mode            string
		saved, expected bool
	}{
		{"normal", "normal", true, false},
		{"tray", "tray", false, true},
		{"background", "hidden", false, true},
		{"task", "task", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := Defaults()
			raw, _ := json.Marshal(value)
			var fields map[string]any
			json.Unmarshal(raw, &fields)
			fields["managedApps"] = []any{map[string]any{
				"id": "reader", "name": "Reader", "exePath": `C:\Apps\reader.exe`,
				"runOnStartup":             test.saved,
				"launchViaLogonTask":       test.mode == "task",
				"launchHiddenInBackground": test.mode == "hidden",
				"trayBehavior":             map[string]any{"autoMinimizeAndHideOnLaunch": test.mode == "tray"},
			}}
			raw, _ = json.Marshal(fields)
			got, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.ManagedApps[0].RunOnStartup != test.expected {
				t.Fatalf("mode %s retained a stale startup projection: %+v", test.mode, got.ManagedApps[0])
			}
		})
	}
}
