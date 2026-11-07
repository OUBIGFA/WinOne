package dictionary

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMissingDictionaryIsAnError(t *testing.T) {
	if _, err := Open(t.TempDir() + "/missing.db"); err == nil {
		t.Fatal("missing database was created or hidden")
	}
}

func TestBundledDictionaryAndInflections(t *testing.T) {
	path := os.Getenv("WINONE_TEST_LEARNING_DICTIONARY")
	if path == "" {
		t.Skip("explicit dictionary fixture not provided")
	}
	start := time.Now()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, word := range []string{"hello", "went", "better", "mice", "running"} {
		entries, err := db.Lookup(context.Background(), word)
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s: %v %+v", word, err, entries)
		}
		if word == "went" {
			found := false
			for _, e := range entries {
				for _, lemma := range e.Lemmas {
					if lemma == "go" {
						found = true
					}
				}
				if e.Word == "go" {
					found = true
				}
			}
			if !found {
				t.Fatal("went has no go lemma")
			}
		}
	}
	elapsed := time.Since(start)
	entries, err := db.Extract(context.Background(), "The mice went running. Sustainable development requires sophisticated coordination and substantial investment.", "intermediate")
	if err != nil || len(entries) == 0 {
		t.Fatalf("passage: %v %+v", err, entries)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = db.Lookup(ctx, "hello"); err == nil {
		t.Fatal("cancel ignored")
	}
	t.Logf("cold open + 5 queries: %s; passage keywords: %d", elapsed, len(entries))
}
