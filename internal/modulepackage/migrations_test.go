package modulepackage

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"winone/internal/configjson"
)

func TestSettingsMigrationUsesTheVersionChainAndPreservesTheInput(t *testing.T) {
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["width","large","enabled"],"properties":{"width":{"type":"integer"},"large":{"type":"integer"},"enabled":{"type":"boolean"}}}`)
	defaults := []byte(`{"width":1,"large":1,"enabled":true}`)
	definition, err := compileSettings(schema, defaults)
	if err != nil {
		t.Fatal(err)
	}
	patches := map[string][]byte{
		"one.json": []byte(`[{"op":"test","path":"/size","value":7},{"op":"move","from":"/size","path":"/width"},{"op":"copy","from":"/secret","path":"/copy"},{"op":"remove","path":"/obsolete"},{"op":"add","path":"/list/-","value":3},{"op":"replace","path":"/secret","value":"changed"}]`),
		"two.json": []byte(`[{"op":"add","path":"/enabled","value":true}]`),
	}
	if err = definition.configureMigrations(3, []SettingsMigration{{From: 1, To: 2, Patch: "one.json"}, {From: 2, To: 3, Patch: "two.json"}}, func(path string) ([]byte, error) { return patches[path], nil }); err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"size":7,"secret":"original","large":9223372036854775807,"list":[1,2],"obsolete":true}`)
	original := string(input)
	migrated, err := definition.Migrate(1, input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := configjson.ParseObject(migrated)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := configjson.ParseObject([]byte(`{"width":7,"secret":"changed","copy":"original","large":9223372036854775807,"list":[1,2,3],"enabled":true}`))
	if !reflect.DeepEqual(got, want) || string(input) != original {
		t.Fatalf("migration changed values or input: %s", migrated)
	}
	again, err := definition.Migrate(1, input)
	if err != nil || string(again) != string(migrated) {
		t.Fatal("migration reused mutable state", err)
	}
	if _, err = definition.Migrate(4, input); err == nil {
		t.Fatal("reverse migration accepted")
	}
}

func TestMigrationRejectsAmbiguousOperationsAndInvalidResults(t *testing.T) {
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	for _, patch := range []string{
		" null ",
		`[{"op":"add","op":"remove","path":"/x","value":1}]`,
		`[{"op":"add","path":"/bad~2path","value":1}]`,
		`[{"op":"move","path":"/x"}]`,
		`[{"op":"add","path":"/x"}]`,
		`[{"op":"remove","path":"/missing"}]`,
		`[{"op":"replace","path":"/list/-1","value":1}]`,
		`[{"op":"replace","path":"","value":[]}]`,
		`[{"op":"test","path":"/secret","value":"wrong"}]`,
	} {
		definition, err := compileSettings(schema, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		err = definition.configureMigrations(2, []SettingsMigration{{From: 1, To: 2, Patch: "patch.json"}}, func(string) ([]byte, error) { return []byte(patch), nil })
		if err == nil {
			_, err = definition.Migrate(1, json.RawMessage(`{"list":[1],"secret":"private-fixture-value"}`))
		}
		if err == nil || strings.Contains(err.Error(), "private-fixture-value") {
			t.Fatalf("invalid patch accepted or input leaked: %s: %v", patch, err)
		}
	}
	definition, _ := compileSettings(schema, []byte("{}"))
	if err := definition.configureMigrations(2, []SettingsMigration{{From: 1, To: 2, Patch: "patch.json"}}, func(string) ([]byte, error) { return nil, errors.New("signed read failed") }); err == nil {
		t.Fatal("missing signed patch ignored")
	}
}

func TestMigrationOutputCannotExceedSettingsLimit(t *testing.T) {
	definition, _ := compileSettings([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), []byte("{}"))
	if err := definition.configureMigrations(2, []SettingsMigration{{From: 1, To: 2, Patch: "copy.json"}}, func(string) ([]byte, error) { return []byte(`[{"op":"copy","from":"/large","path":"/another"}]`), nil }); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"large": strings.Repeat("x", MaxSettingsBytes/2+1)})
	if _, err := definition.Migrate(1, raw); err == nil {
		t.Fatal("oversized migrated settings accepted")
	}
}

func TestMigrationMetadataRequiresExplicitUnambiguousSignedPaths(t *testing.T) {
	valid := testMetadata()
	valid.SchemaVersion = 3
	valid.Module.SettingsSchemaVersion = 3
	valid.Settings = &SettingsFiles{Schema: "schema.json", Defaults: "defaults.json", Migrations: []SettingsMigration{{From: 1, To: 2, Patch: "one.json"}, {From: 2, To: 3, Patch: "two.json"}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*Metadata){
		func(m *Metadata) { m.SchemaVersion = 2 },
		func(m *Metadata) { m.Settings.Migrations[1].From = 1 },
		func(m *Metadata) { m.Settings.Migrations = m.Settings.Migrations[:1] },
		func(m *Metadata) { m.Settings.Migrations[0].To = 1 },
		func(m *Metadata) { m.Settings.Migrations[0].Patch = "../patch.json" },
		func(m *Metadata) { m.Settings.Migrations[0].Patch = "schema.json" },
	} {
		copy := valid
		settings := *valid.Settings
		settings.Migrations = append([]SettingsMigration(nil), settings.Migrations...)
		copy.Settings = &settings
		alter(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatal("invalid migration metadata accepted")
		}
	}
}
