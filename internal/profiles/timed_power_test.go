package profiles

import (
	"bytes"
	"encoding/json"
	"testing"
	"winone/internal/timedpower"
)

func TestTimedPowerTemplateRoundTrip(t *testing.T) {
	p := timedpower.Defaults()
	p.Revision = 25
	p.Mode = "weekly"
	raw, _ := json.Marshal(p)
	exported, err := ExportBuiltinSettings("timed-power", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PrepareBuiltinImport("timed-power", exported.Module, exported.Paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got timedpower.Preferences
	if err = json.Unmarshal(preview.Patch, &got); err != nil || got.Revision != 0 || got.Mode != "weekly" || !got.Force {
		t.Fatal(got, err)
	}
	exported.Module.Settings = bytes.Replace(exported.Module.Settings, []byte(`"revision":0`), []byte(`"revision":0,"active":true`), 1)
	if _, err = PrepareBuiltinImport("timed-power", exported.Module, exported.Paths, nil); err == nil {
		t.Fatal("import accepted active runtime state")
	}
}
