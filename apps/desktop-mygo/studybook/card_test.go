package studybook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportActualCardDocumentsForOfflineRenderer(t *testing.T) {
	directory := os.Getenv("WINONE_ANKI_RENDER_FIXTURES")
	if directory == "" {
		t.Skip("explicit offline card rendering")
	}
	data, err := os.ReadFile(filepath.Join(directory, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Name, Body, CSS string }
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if err = os.WriteFile(filepath.Join(directory, fixture.Name+".html"), []byte(cardDocument(fixture.Body, fixture.CSS)), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, fixture.Name+"-dark.html"), []byte(cardDocument(fixture.Body, fixture.CSS, true)), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
