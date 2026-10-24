package main

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSharePreparation(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "_temp", "interconnect-prepare-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "prepare-")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.txt")
	if err = os.WriteFile(source, []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	prepare := func(paths []string, name string) map[string]any {
		t.Helper()
		task := &task{spec: specification{Paths: paths, DataDir: filepath.Join(dir, name)}}
		result, err := task.prepareShare(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return result.(map[string]any)
	}
	result := prepare([]string{source}, "small")
	copied := result["paths"].([]string)[0]
	if copied == source || result["cached"] != true {
		t.Fatal("small selection was not copied")
	}
	if err = os.WriteFile(source, []byte("changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(copied); err != nil || string(data) != "original bytes" {
		t.Fatalf("copy changed: %q %v", data, err)
	}
	folder := filepath.Join(dir, "folder")
	if err = os.MkdirAll(filepath.Join(folder, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(folder, "nested.txt"), []byte("folder bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := prepare([]string{folder}, "directory")["paths"].([]string)[0]
	zipped, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zipped.Close()
	found := false
	for _, file := range zipped.File {
		if filepath.Base(file.Name) != "nested.txt" {
			continue
		}
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil || string(data) != "folder bytes" {
			t.Fatal("archive bytes changed")
		}
		found = true
	}
	if !found {
		t.Fatal("archive omitted nested file")
	}
	large := filepath.Join(dir, "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate(301 * 1024 * 1024)
	err = errors.Join(err, f.Close())
	if err != nil {
		t.Fatal(err)
	}
	result = prepare([]string{large}, "large")
	if result["paths"].([]string)[0] != large || result["cached"] != false {
		t.Fatal("large file was unexpectedly copied")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := &task{spec: specification{Paths: []string{source}, DataDir: filepath.Join(dir, "cancelled")}}
	if _, err = task.prepareShare(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
