package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"os"
	"path/filepath"
"testing"

 "github.com/schollz/croc/v11/src/croc"
)

func TestUnifiedPreparationKeepsFiltersAndFolderMode(t *testing.T) {
 dir, source, _ := transferPaths(t)
 folder := filepath.Join(source, "bundle")
 if err := os.MkdirAll(folder, 0700); err != nil { t.Fatal(err) }
 for name, content := range map[string]string{"keep.txt":"kept","skip.txt":"excluded","private.log":"ignored",".gitignore":"*.log"} {
  if err := os.WriteFile(filepath.Join(folder,name), []byte(content),0600); err != nil { t.Fatal(err) }
 }
 for _, zipped := range []bool{false,true} {
  task := &task{spec:specification{Paths:[]string{folder},DataDir:filepath.Join(dir,fmt.Sprint(zipped)),Options:croc.Options{ZipFolder:zipped,GitIgnore:true,ExcludeFile:[]string{"skip.txt"}}}}
  result,err := task.prepareShare(context.Background()); if err != nil { t.Fatal(err) }
  paths := result.(map[string]any)["paths"].([]string)
  codePaths := result.(map[string]any)["codePaths"].([]string)
  if zipped {
   archive,err := zip.OpenReader(paths[0]); if err != nil { t.Fatal(err) }
   for _,file := range archive.File { if strings.HasSuffix(file.Name,"private.log") { t.Fatal("excluded content published: "+file.Name) } }; archive.Close()
  }
  if zipped { if codePaths[0] != paths[0] { t.Fatal("ZIP mode not shared") } } else {
   data,err := os.ReadFile(filepath.Join(codePaths[0],"keep.txt")); if err != nil || string(data)!="kept" { t.Fatalf("folder mode lost: %s %v",data,err) }
  }
 }
}

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
