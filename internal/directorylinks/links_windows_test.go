//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func linkFixture(t *testing.T, kind, object string) (*Manager, Record, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "源文件 %!&.txt")
	if object == "directory" {
		source = filepath.Join(root, "目标目录 %!&")
		if err := os.Mkdir(source, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("target data"), 0600); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(source, []byte("target data"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := m.Preview(Request{Name: "my link", Source: source, Directory: root, LinkName: "入口 %!&", Kind: kind, Relative: kind == "symlink"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ObjectType != object {
		t.Fatalf("object=%s", plan.ObjectType)
	}
	record, err := m.Create(context.Background(), plan.ID)
	if err != nil {
		if kind == "symlink" && errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			if _, e := os.Lstat(plan.LinkPath); !os.IsNotExist(e) {
				t.Fatal("failed privileged creation left an entry")
			}
			if os.Getenv("WINONE_LINKS_REQUIRE_SYMLINK_TEST") == "1" {
				t.Fatal("symbolic-link verification requires the approved elevated token; Windows returned 1314")
			}
			t.Skip("Windows 1314 verified; rerun this native case with approved elevation or existing Developer Mode")
		}
		t.Fatal(err)
	}
	return m, record, source
}

func TestCreatesAndChecksAllLinkKinds(t *testing.T) {
	for _, tc := range []struct{ kind, object string }{{"hardlink", "file"}, {"junction", "directory"}, {"symlink", "file"}, {"symlink", "directory"}} {
		t.Run(tc.kind+"-"+tc.object, func(t *testing.T) {
			m, r, source := linkFixture(t, tc.kind, tc.object)
			got, err := m.Check(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "active" || got.CheckedAt == "" {
				t.Fatalf("not checked active: %+v", got)
			}
			path := r.LinkPath
			if tc.object == "directory" {
				path = filepath.Join(path, "keep.txt")
			}
			if raw, err := os.ReadFile(path); err != nil || string(raw) != "target data" {
				t.Fatalf("not a working link: %s %v", raw, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatal("source removed", err)
			}
		})
	}
}

func TestHardLinkDistinguishesDataAndAssociation(t *testing.T) {
	m, r, source := linkFixture(t, "hardlink", "file")
	third := source + ".other"
	if err := os.Link(source, third); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	} // Simulate an external deletion of this owned fixture name.
	got, err := m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "other-links" || got.LinkCount != 2 {
		t.Fatalf("other alias lost: %+v", got)
	}
	if err := os.Remove(third); err != nil {
		t.Fatal(err)
	}
	got, err = m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unlinked" || !got.Accessible || got.LinkCount != 1 {
		t.Fatalf("readable lone file called active: %+v", got)
	}
	if err := os.WriteFile(source, []byte("target data"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "source-changed" {
		t.Fatalf("replaced source called same: %+v", got)
	}
}

func TestReparseLinksDetectMissingChangedAndReplacedObjects(t *testing.T) {
	for _, tc := range []struct{ kind, object string }{{"junction", "directory"}, {"symlink", "directory"}, {"symlink", "file"}} {
		t.Run(tc.kind+tc.object, func(t *testing.T) {
			m, r, source := linkFixture(t, tc.kind, tc.object)
			if err := os.Rename(source, source+".saved"); err != nil {
				t.Fatal(err)
			}
			got, err := m.Check(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "broken" {
				t.Fatalf("missing target: %+v", got)
			}
			if tc.object == "directory" {
				err = os.Mkdir(source, 0700)
			} else {
				err = os.WriteFile(source, []byte("new target"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err = m.Check(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "target-changed" {
				t.Fatalf("changed target hidden: %+v", got)
			}
			if err := os.Rename(r.LinkPath, r.LinkPath+".saved"); err != nil {
				t.Fatal(err)
			}
			if tc.object == "directory" {
				err = os.Mkdir(r.LinkPath, 0700)
			} else {
				err = os.WriteFile(r.LinkPath, []byte("ordinary file"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err = m.Check(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "replaced" {
				t.Fatalf("ordinary entry called a link: %+v", got)
			}
		})
	}
}

func TestPlansRefuseConflictsAndRecheckSourceIdentity(t *testing.T) {
	m, r, source := linkFixture(t, "hardlink", "file")
	if _, err := m.Preview(Request{Source: source, Directory: filepath.Dir(source), LinkName: filepath.Base(r.LinkPath), Kind: "hardlink"}); err == nil {
		t.Fatal("existing path accepted")
	}
	if _, err := m.Preview(Request{Source: filepath.Dir(source), Directory: filepath.Dir(source), LinkName: "bad", Kind: "hardlink"}); err == nil {
		t.Fatal("directory hardlink accepted")
	}
	p, err := m.Preview(Request{Source: source, Directory: filepath.Dir(source), LinkName: "pending", Kind: "hardlink"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, source+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), p.ID); err == nil {
		t.Fatal("changed source accepted after preview")
	}
	if _, err := os.Lstat(p.LinkPath); !os.IsNotExist(err) {
		t.Fatal("stale preview wrote an entry")
	}
}
