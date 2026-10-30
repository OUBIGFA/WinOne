package settings

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicStreamRetainsDestinationOnProducerFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.json")
	if err := WriteAtomic(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	aborted := errors.New("producer stopped")
	err := WriteAtomicStream(path, func(w io.Writer) error { io.WriteString(w, "incomplete"); return aborted })
	if !errors.Is(err, aborted) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("failed export replaced destination", err)
	}
	if err := WriteAtomicStream(path, func(w io.Writer) error { _, err := io.WriteString(w, "complete"); return err }); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "complete" {
		t.Fatal("successful export missing")
	}
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Fatal("temporary output left behind")
	}
}
