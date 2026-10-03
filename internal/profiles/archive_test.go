package profiles

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sampleArchive() Archive {
	return Archive{
		CreatedBy:   "0.1.0-alpha.1",
		Preferences: Preferences{Name: "公司与家里", Theme: "system", Language: "zh-CN", Enabled: map[string]bool{"capture": true, "example.optional": false}, Shortcuts: map[string]string{"capture.region": "Ctrl+Alt+S"}},
		Modules:     map[string]ModuleSettings{"capture": {SchemaVersion: 1, Settings: json.RawMessage(`{"outputDirectory":"${path:pictures}","copyToClipboard":true}`)}, "example.optional": {SchemaVersion: 7, Settings: json.RawMessage(`{"value":"保留未安装模块"}`)}},
		Paths:       map[string]PathReference{"pictures": {Kind: "known-folder", Label: "图片目录", Hint: "Pictures"}},
	}
}

func TestArchiveRoundTripPreservesUnknownModuleData(t *testing.T) {
	source := sampleArchive()
	data, err := Encode(source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source, decoded) {
		t.Fatalf("roundtrip changed settings: %+v", decoded)
	}
	repeated, err := Encode(source)
	if err != nil || !bytes.Equal(data, repeated) {
		t.Fatal("archive output is not deterministic")
	}
}

func rewriteArchive(t *testing.T, data []byte, modify func(*zip.Writer)) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, entry := range reader.File {
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		target, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = target.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	modify(writer)
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestArchiveRejectsUnsafeExtraAndDuplicateEntries(t *testing.T) {
	data, _ := Encode(sampleArchive())
	for _, name := range []string{"../settings.json", `..\settings.json`, "modules/../../run.json", "modules/capture.exe", "profile.json", "logs/log.json"} {
		t.Run(name, func(t *testing.T) {
			modified := rewriteArchive(t, data, func(writer *zip.Writer) {
				file, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				file.Write([]byte(`{}`))
			})
			if _, err := Decode(modified); err == nil {
				t.Fatal("unsafe or duplicate entry accepted")
			}
		})
	}
	modified := rewriteArchive(t, data, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: "modules/link.json"}
		header.SetMode(os.ModeSymlink | 0600)
		file, _ := writer.CreateHeader(header)
		file.Write([]byte("profile.json"))
	})
	if _, err := Decode(modified); err == nil {
		t.Fatal("symlink entry accepted")
	}
}

func TestArchiveRejectsChangedContentAndUnlistedFiles(t *testing.T) {
	data, _ := Encode(sampleArchive())
	// Stored ZIP entries expose the test string directly; checksum validation must reject tampering.
	changed := bytes.Replace(data, []byte("Ctrl+Alt+S"), []byte("Ctrl+Alt+T"), 1)
	if _, err := Decode(changed); err == nil {
		t.Fatal("corrupt content accepted")
	}
	reader, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	var validZIP bytes.Buffer
	writer := zip.NewWriter(&validZIP)
	for _, entry := range reader.File {
		stream, _ := entry.Open()
		content, _ := io.ReadAll(stream)
		stream.Close()
		if entry.Name == "profile.json" {
			content = bytes.Replace(content, []byte(`"theme":"system"`), []byte(`"theme":"dark"`), 1)
		}
		file, _ := writer.Create(entry.Name)
		file.Write(content)
	}
	writer.Close()
	if _, err := Decode(validZIP.Bytes()); err == nil || !strings.Contains(err.Error(), "校验") {
		t.Fatalf("manifest digest mismatch was not detected: %v", err)
	}
	extra := rewriteArchive(t, data, func(writer *zip.Writer) {
		file, _ := writer.Create("modules/extra.json")
		file.Write([]byte(`{"schemaVersion":1,"settings":{}}`))
	})
	if _, err := Decode(extra); err == nil {
		t.Fatal("unlisted module accepted")
	}
}

func TestConfigurationJSONRejectsAmbiguityAndSizeAbuse(t *testing.T) {
	for _, data := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`, `null`, strings.Repeat(`{"a":`, 34) + `{}` + strings.Repeat(`}`, 34)} {
		if err := objectJSON([]byte(data)); err == nil {
			t.Fatalf("invalid JSON accepted: %s", data)
		}
	}
	source := sampleArchive()
	source.Modules["capture"] = ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(`{"value":"` + strings.Repeat("x", MaxEntryBytes) + `"}`)}
	if _, err := Encode(source); err == nil {
		t.Fatal("oversized entry exported")
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	file, _ := writer.Create("profile.json")
	file.Write([]byte(strings.Repeat("x", MaxEntryBytes+1)))
	writer.Close()
	if _, err := Decode(output.Bytes()); err == nil {
		t.Fatal("compressed oversized entry imported")
	}
}
