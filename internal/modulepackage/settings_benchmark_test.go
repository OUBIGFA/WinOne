package modulepackage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func settingsBenchmarkFixture(b *testing.B) (*StagedPackage, json.RawMessage) {
	b.Helper()
	properties := map[string]any{}
	values := map[string]any{}
	required := []string{}
	for index := range 100 {
		key := fmt.Sprintf("field%d", index)
		properties[key] = map[string]any{"type": "string", "maxLength": 512}
		values[key] = "sample setting"
		required = append(required, key)
	}
	schema, _ := json.Marshal(map[string]any{"$schema": settingsDialect, "type": "object", "properties": properties, "required": required, "additionalProperties": false})
	raw, _ := json.Marshal(values)
	return settingsStage(b, string(schema), string(raw)), raw
}
func BenchmarkSignedSettingsLoad100Fields(b *testing.B) {
	stage, _ := settingsBenchmarkFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := stage.LoadSettings(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkSignedSettingsValidate100Fields(b *testing.B) {
	stage, raw := settingsBenchmarkFixture(b)
	definition, err := stage.LoadSettings(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := definition.Validate(raw); err != nil {
			b.Fatal(err)
		}
	}
}
