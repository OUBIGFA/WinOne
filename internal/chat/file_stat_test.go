package chat

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestFileStatsHandleFilesDirectoriesAndMissingHistoryWithoutReadingContents(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", workspace, "Stat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(workspace, "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(20 << 20); err != nil {
		t.Fatal(err)
	}
	file.Close()
	values, err := store.StatFiles(c.ID, []string{"large.bin", "docs", "deleted.txt", "large.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 || values[0].File == nil || values[0].File.Size != 20<<20 || values[0].File.Directory {
		t.Fatalf("wrong file metadata: %+v", values)
	}
	if values[1].File == nil || !values[1].File.Directory {
		t.Fatalf("directory was treated as an unreadable preview: %+v", values[1])
	}
	if !values[2].Missing || values[2].Error != "" {
		t.Fatalf("deleted historical output is not a normal missing result: %+v", values[2])
	}
	outside := filepath.Join(root, "outside.txt")
	os.WriteFile(outside, []byte("private"), 0600)
	denied, err := store.StatFiles(c.ID, []string{outside})
	if err != nil || len(denied) != 1 || denied[0].Error == "" || denied[0].File != nil {
		t.Fatal("ungranted external file was accepted", denied, err)
	}
	if _, err = store.StatFiles(c.ID, make([]string, 129)); err == nil {
		t.Fatal("unbounded metadata batch accepted")
	}
}
