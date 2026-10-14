package modulepackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewArchiveSelectsTrustedPublisherWithoutWritingFiles(t *testing.T) {
	raw, publisher := archiveFixture(t, nil, false)
	directory := t.TempDir()
	file := filepath.Join(directory, "module.winone-module")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	seen := ""
	preview, err := PreviewArchive(context.Background(), file, func(namespace string) (Publisher, error) { seen = namespace; return publisher, nil })
	if err != nil || seen != "example" || preview.Manifest().Metadata().Module.ID != "example.tool" {
		t.Fatal("wrong trusted publisher selection", err, seen)
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 1 {
		t.Fatal("preview extracted payload", err)
	}
	if _, err = PreviewArchive(context.Background(), file, func(string) (Publisher, error) { return Publisher{}, ErrUntrustedPublisher }); !errors.Is(err, ErrUntrustedPublisher) {
		t.Fatal("untrusted package accepted", err)
	}
	publisher.Namespace = "another"
	if _, err = PreviewArchive(context.Background(), file, func(string) (Publisher, error) { return publisher, nil }); err == nil {
		t.Fatal("publisher namespace mismatch accepted")
	}
}
