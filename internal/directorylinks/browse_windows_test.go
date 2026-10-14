//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseShowsActualLinkKindsWithoutRegisteringChildren(t *testing.T) {
	for _, tc := range []struct{ kind, object string }{{"hardlink", "file"}, {"junction", "directory"}, {"symlink", "file"}, {"symlink", "directory"}} {
		t.Run(tc.kind+"-"+tc.object, func(t *testing.T) {
			manager, link, source := linkFixture(t, tc.kind, tc.object)
			before, err := os.ReadFile(filepath.Join(manager.directory, "history.json"))
			if err != nil {
				t.Fatal(err)
			}
			listing, err := manager.Browse(context.Background(), filepath.Dir(source), 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range listing.Entries {
				if entry.Path == link.LinkPath {
					found = true
					if entry.Kind != tc.kind || entry.ObjectType != tc.object || entry.Error != "" {
						t.Fatalf("wrong real entry: %+v", entry)
					}
				}
				if entry.Name == "keep.txt" {
					t.Fatal("browsing recursed into the target")
				}
			}
			if !found {
				t.Fatal("created link is absent from the destination view")
			}
			after, err := os.ReadFile(filepath.Join(manager.directory, "history.json"))
			if err != nil || string(before) != string(after) {
				t.Fatal("browsing changed link history", err)
			}
			if tc.object == "directory" {
				inside, err := manager.Browse(context.Background(), link.LinkPath, 0)
				if err != nil || inside.Directory != link.LinkPath || len(inside.Entries) != 1 || inside.Entries[0].Path != filepath.Join(link.LinkPath, "keep.txt") {
					t.Fatalf("explicit navigation lost its selected address: %+v %v", inside, err)
				}
			}
		})
	}
}

func TestBrowsePagesWithoutLosingEntriesAndRejectsInvalidTargets(t *testing.T) {
	root := t.TempDir()
	manager, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range 503 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("entry-%03d.txt", index)), []byte("kept"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := manager.Browse(context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	next, err := manager.Browse(context.Background(), root, len(first.Entries))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 500 || !first.HasMore || first.Total != 503 || len(next.Entries) != 3 || next.HasMore || next.Entries[0].Name != "entry-500.txt" {
		t.Fatalf("incomplete pagination: first=%d next=%+v", len(first.Entries), next)
	}
	if _, err := manager.Browse(context.Background(), first.Entries[0].Path, 0); err == nil {
		t.Fatal("a file was accepted as a folder")
	}
	if _, err := manager.Browse(context.Background(), root, -1); err == nil {
		t.Fatal("a negative list offset was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Browse(ctx, root, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("plain files were registered as links")
	}
}
