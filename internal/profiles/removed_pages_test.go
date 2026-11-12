package profiles

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"testing"
)

// Build the checksummed wire format written by the full edition. The current
// encoder must still reject selecting removed pages in newly saved profiles.
func TestFullEditionArchiveRestoresRemovedDefaultPageAndKeepsRetainedData(t *testing.T) {
	for _, page := range []string{"chat", "onebot", "flash-chat"} {
		t.Run(page, func(t *testing.T) {
			original := sampleArchive()
			original.Preferences.Enabled["onebot"] = true
			original.Modules["onebot"] = ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(`{"kept":"historical data"}`)}
			data, err := Encode(original)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{}
			for _, entry := range reader.File {
				stream, err := entry.Open()
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name], err = io.ReadAll(stream)
				closeErr := stream.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("read archive: %v / %v", err, closeErr)
				}
			}
			original.Preferences.DefaultPage = &page
			files["profile.json"], err = json.Marshal(original.Preferences)
			if err != nil {
				t.Fatal(err)
			}
			var meta manifest
			if err = json.Unmarshal(files["manifest.json"], &meta); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(files["profile.json"])
			meta.Files["profile.json"] = hex.EncodeToString(digest[:])
			files["manifest.json"], err = json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			writer := zip.NewWriter(&wire)
			for _, entry := range reader.File {
				out, err := writer.Create(entry.Name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = out.Write(files[entry.Name]); err != nil {
					t.Fatal(err)
				}
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(wire.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Preferences.DefaultPage == nil || *decoded.Preferences.DefaultPage != "overview" || decoded.Preferences.Language != original.Preferences.Language || !bytes.Equal(decoded.Modules["capture"].Settings, original.Modules["capture"].Settings) {
				t.Fatal("restoring a removed page changed retained configuration")
			}
			plan, err := PrepareArchive(decoded, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, module := range plan.Modules {
				if module.ID == "onebot" && module.State != "missing_module" {
					t.Fatal("retired module became installable")
				}
			}
		})
	}
}
