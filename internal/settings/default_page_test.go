package settings

import (
	"path/filepath"
	"testing"
)

func TestDefaultPagePersistsAndRejectsUnknownNavigationTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	value := store.Snapshot()
	value.DefaultPage = "chat"
	if _, err = store.Save(value); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().DefaultPage != "chat" {
		t.Fatal("default page was not persisted")
	}
	value = reopened.Snapshot()
	value.DefaultPage = "missing-page"
	if _, err = reopened.Save(value); err == nil {
		t.Fatal("unknown page accepted")
	}
}
