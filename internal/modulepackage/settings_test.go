package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"winone/internal/distribution"
)

const settingsSchemaFixture = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","$defs":{"size":{"type":"integer","minimum":1,"maximum":20}},"properties":{"size":{"$ref":"#/$defs/size"},"name":{"type":"string","maxLength":80}},"required":["size","name"],"additionalProperties":false}`

type settingsMigrationFile struct {
	step SettingsMigration
	data string
}

func settingsStage(t testing.TB, schema, defaults string, migrations ...settingsMigrationFile) *StagedPackage {
	t.Helper()
	manifest, publisher, key := signedTestManifest(t)
	manifest.Metadata.SchemaVersion = 2
	manifest.Metadata.Settings = &SettingsFiles{Schema: "settings/schema.json", Defaults: "settings/defaults.json"}
	files := map[string][]byte{"bin/worker.exe": []byte("inert worker"), "LICENSE.txt": []byte("test license"), "settings/schema.json": []byte(schema), "settings/defaults.json": []byte(defaults)}
	for _, migration := range migrations {
		manifest.Metadata.SchemaVersion = 3
		manifest.Metadata.Settings.Migrations = append(manifest.Metadata.Settings.Migrations, migration.step)
		if migration.step.To > manifest.Metadata.Module.SettingsSchemaVersion {
			manifest.Metadata.Module.SettingsSchemaVersion = migration.step.To
		}
		files[migration.step.Patch] = []byte(migration.data)
	}
	manifest.Files = nil
	for name, body := range files {
		sum := sha256.Sum256(body)
		manifest.Files = append(manifest.Files, distribution.File{Path: name, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["module.json"] = raw
	files["module.json.minisig"] = minisign.Sign(key, raw)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "settings.winone-module")
	if err = os.WriteFile(file, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyArchive(context.Background(), file, publisher)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := StageArchive(context.Background(), file, StageOptions{Directory: t.TempDir(), Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()})
	if err != nil {
		t.Fatal(err)
	}
	return stage
}

func TestSignedSettingsValidateLocallyAndKeepPrivateValuesOutOfErrors(t *testing.T) {
	stage := settingsStage(t, settingsSchemaFixture, `{"size":3,"name":"default"}`)
	settings, err := stage.LoadSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.Validate(json.RawMessage(`{"size":5,"name":"ok"}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"size":0,"name":"private-value"}`, `{"size":3,"name":"ok","unexpected":"private-value"}`, `{"size":3,"size":4,"name":"ok"}`, `[]`} {
		if err = settings.Validate(json.RawMessage(raw)); err == nil || strings.Contains(err.Error(), "private-value") {
			t.Fatal("invalid settings accepted or value disclosed", err)
		}
	}
	first := settings.Defaults()
	first[0] = '!'
	if !json.Valid(settings.Defaults()) {
		t.Fatal("caller mutated cached defaults")
	}
	metadata := stage.Manifest().Metadata()
	metadata.Settings.Schema = "changed"
	if stage.Manifest().Metadata().Settings.Schema != "settings/schema.json" {
		t.Fatal("caller mutated signed settings identity")
	}
}

func TestSignedSettingsNeverLoadReferencesFromNetworkOrDisk(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"type":"object"}`))
	}))
	defer server.Close()
	for _, ref := range []string{server.URL + "/schema", "file:///C:/private/settings.json"} {
		raw, _ := json.Marshal(map[string]string{"$schema": "https://json-schema.org/draft/2020-12/schema", "$ref": ref})
		stage := settingsStage(t, string(raw), `{}`)
		if _, err := stage.LoadSettings(context.Background()); err == nil {
			t.Fatal("external schema accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("schema validation contacted the network")
	}
}

func TestSignedSettingsRejectInvalidDefaultsAndChangedSignedFiles(t *testing.T) {
	stage := settingsStage(t, settingsSchemaFixture, `{"size":99,"name":"invalid"}`)
	if _, err := stage.LoadSettings(context.Background()); err == nil {
		t.Fatal("invalid defaults accepted")
	}
	stage = settingsStage(t, settingsSchemaFixture, `{"size":3,"name":"default"}`)
	if err := os.WriteFile(filepath.Join(stage.Directory(), "settings", "schema.json"), []byte(`{"type":"object"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.LoadSettings(context.Background()); err == nil {
		t.Fatal("modified settings schema trusted")
	}
}

func TestSignedMigrationFilesAreRevalidatedAndMetadataCopiesCannotRedirectThem(t *testing.T) {
	stage := settingsStage(t, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`, `{"name":"default"}`, settingsMigrationFile{SettingsMigration{From: 1, To: 2, Patch: "migration.json"}, `[{"op":"move","from":"/oldName","path":"/name"}]`})
	metadata := stage.Manifest().Metadata()
	metadata.Settings.Migrations[0].Patch = "outside.json"
	definition, err := stage.LoadSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := definition.Migrate(1, json.RawMessage(`{"oldName":"kept"}`))
	if err != nil || string(migrated) != `{"name":"kept"}` {
		t.Fatal("signed migration did not preserve its value", err)
	}
	if err = os.WriteFile(filepath.Join(stage.Directory(), "migration.json"), []byte(`[{"op":"remove","path":"/oldName"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = stage.LoadSettings(context.Background()); err == nil {
		t.Fatal("modified migration bytes were trusted")
	}
}
