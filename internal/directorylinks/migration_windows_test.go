//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func migrationFixture(t *testing.T) (*Manager, MigrationRequest, migrationOps) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	for _, path := range []string{filepath.Join(source, "sub"), destination} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "sub", "keep.txt"), []byte("original data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "sub", "keep.txt:metadata"), []byte("named stream"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	trash := t.TempDir()
	n := 0
	ops := migrationOps{recycle: func(ctx context.Context, a MigrationArtifact) error {
		n++
		return os.Rename(a.Path, filepath.Join(trash, filepath.Base(a.Path)))
	}}
	return m, MigrationRequest{Source: source, Directory: destination, Name: "moved"}, ops
}
func TestMigrationAndRestoreUseVerifiedCopiesAndPreserveStreams(t *testing.T) {
	m, request, ops := migrationFixture(t)
	plan, err := m.PreviewMigration(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := m.executeMigration(context.Background(), plan.ID, true, nil, ops)
	if err != nil {
		t.Fatal(err, row)
	}
	if row.Status != "active" || row.Migration.Phase != "complete" || row.Migration.Digest == "" {
		t.Fatal(row)
	}
	for _, base := range []string{request.Source, plan.Destination} {
		for _, name := range []string{"keep.txt", "keep.txt:metadata"} {
			data, err := os.ReadFile(filepath.Join(base, "sub", name))
			if err != nil || len(data) == 0 {
				t.Fatal(base, name, err)
			}
		}
	}
	if _, err = m.executeMigration(context.Background(), plan.ID, true, nil, ops); err == nil {
		t.Fatal("migration token reused")
	}
	reopened, err := New(m.directory)
	if err != nil {
		t.Fatal(err)
	}
	row, err = reopened.Check(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := reopened.PreviewMigration(context.Background(), MigrationRequest{RecordID: row.ID, CheckedAt: row.CheckedAt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err = reopened.executeMigration(context.Background(), restore.ID, true, nil, ops)
	if err != nil {
		t.Fatal(err, row)
	}
	if row.Status != "restored" || row.Migration.Operation != "restore" {
		t.Fatal(row)
	}
	state, err := openState(request.Source, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	if state.reparse {
		t.Fatal("restore left a junction")
	}
	if data, err := os.ReadFile(filepath.Join(request.Source, "sub", "keep.txt:metadata")); err != nil || string(data) != "named stream" {
		t.Fatal("stream was not restored", err)
	}
	row, err = reopened.Check(context.Background(), row.ID)
	if err != nil || row.Status != "restored" {
		t.Fatal(row, err)
	}
}
func TestMigrationFailuresPreserveOriginalData(t *testing.T) {
	for _, phase := range []string{"copied", "verified", "backed-up"} {
		t.Run(phase, func(t *testing.T) {
			m, request, ops := migrationFixture(t)
			p, err := m.PreviewMigration(context.Background(), request, nil)
			if err != nil {
				t.Fatal(err)
			}
			ops.checkpoint = func(at string) error {
				if at == phase {
					return errors.New("injected failure")
				}
				return nil
			}
			row, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
			if err == nil {
				t.Fatal("failure hidden")
			}
			data, e := os.ReadFile(filepath.Join(request.Source, "sub", "keep.txt"))
			if e != nil || string(data) != "original data" {
				t.Fatal("original not restored", row, e)
			}
			state, e := openState(request.Source, false, false)
			if e != nil {
				t.Fatal(e)
			}
			state.close()
			if state.reparse {
				t.Fatal("failed migration left source as a link")
			}
		})
	}
}
func TestMigrationPreviewRejectsNestedAndReparseTrees(t *testing.T) {
	m, request, _ := migrationFixture(t)
	if _, err := m.PreviewMigration(context.Background(), MigrationRequest{Source: request.Source, Directory: request.Source}, nil); err == nil {
		t.Fatal("nested destination accepted")
	}
	if err := createJunction(filepath.Join(request.Source, "jump"), request.Directory); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PreviewMigration(context.Background(), request, nil); err == nil {
		t.Fatal("reparse tree accepted")
	}
}
func TestMigrationRejectsChangedPreviewAndCancellation(t *testing.T) {
	m, request, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(request.Source, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.executeMigration(context.Background(), p.ID, true, nil, ops); err == nil {
		t.Fatal("changed tree accepted")
	}
	p, err = m.PreviewMigration(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.executeMigration(ctx, p.ID, true, nil, ops); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(request.Source, "new.txt")); err != nil {
		t.Fatal("cancel changed source", err)
	}
}
