//go:build windows

package chat

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestBundledSearchDoesNotNeedPATHAndKeepsIgnoreAndUnicodeSemantics(t *testing.T) {
	executable := os.Getenv("WINONE_CHAT_RG_EXE")
	if executable == "" {
		t.Skip("explicit bundled search fixture")
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0700)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0600)
	os.WriteFile(filepath.Join(root, "中文 notes.txt"), []byte("中 😀 needle\nsecond NEEDLE\n"), 0600)
	os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("needle\n"), 0600)
	t.Setenv("PATH", "")
	t.Setenv("RIPGREP_CONFIG_PATH", filepath.Join(root, "missing-config"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	index, err := ReadFileIndex(ctx, executable, root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(index.Files, "中文 notes.txt") || slices.Contains(index.Files, "ignored.txt") {
		t.Fatal("index lost ignore or Unicode behavior", index)
	}
	found, err := SearchWorkspaceFiles(ctx, executable, root, "needle", false, 500)
	if err != nil {
		t.Fatal(err)
	}
	if found.TotalMatches != 2 || found.TotalFiles != 1 || len(found.Results[0].Matches) != 2 {
		t.Fatal("search results changed", found)
	}
	span := found.Results[0].Matches[0].Submatches[0]
	if span.Start != 5 || span.End != 11 {
		t.Fatalf("renderer offsets must be UTF-16, got %+v", span)
	}
}
