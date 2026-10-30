//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMigrationDetectsCorruptedAlternateStream(t *testing.T) {
	m, req, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Bytes != int64(len("original data")+len("named stream")) {
		t.Fatalf("alternate streams were not counted: %d", p.Bytes)
	}
	ops.checkpoint = func(phase string) error {
		if phase == "copied" {
			return os.WriteFile(filepath.Join(m.List()[0].Migration.Staging, "sub", "keep.txt:metadata"), []byte("wrong stream"), 0600)
		}
		return nil
	}
	r, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
	if err == nil || r.Migration.Phase != "failed" {
		t.Fatal("corrupt named stream certified", r, err)
	}
	if data, err := os.ReadFile(filepath.Join(req.Source, "sub", "keep.txt:metadata")); err != nil || string(data) != "named stream" {
		t.Fatal("source changed", err)
	}
}
func TestMigrationTargetCollisionNeverOverwritesAndRollsBack(t *testing.T) {
	m, req, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops.checkpoint = func(phase string) error {
		if phase == "verified" {
			if err := os.Mkdir(p.Destination, 0700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(p.Destination, "user.txt"), []byte("user data"), 0600)
		}
		return nil
	}
	if _, err = m.executeMigration(context.Background(), p.ID, true, nil, ops); err == nil {
		t.Fatal("collision overwritten")
	}
	if data, err := os.ReadFile(filepath.Join(p.Destination, "user.txt")); err != nil || string(data) != "user data" {
		t.Fatal("user collision changed", err)
	}
	if data, err := os.ReadFile(filepath.Join(req.Source, "sub", "keep.txt")); err != nil || string(data) != "original data" {
		t.Fatal("original not recovered", err)
	}
}
func TestMigrationBlockedRollbackRetainsOriginalAndExplicitRecoveryRefusesCollision(t *testing.T) {
	m, req, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops.checkpoint = func(phase string) error {
		if phase == "backed-up" {
			if err := os.Mkdir(req.Source, 0700); err != nil {
				return err
			}
			return errors.New("replacement appeared")
		}
		return nil
	}
	r, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
	if err == nil || r.Migration.Phase != "recovery-required" {
		t.Fatal(r, err)
	}
	if data, err := os.ReadFile(filepath.Join(r.Migration.Backup, "sub", "keep.txt")); err != nil || string(data) != "original data" {
		t.Fatal("backup lost", err)
	}
	if _, err = m.RecoverMigration(context.Background(), r.ID); err == nil {
		t.Fatal("recovery overwrote collision")
	}
	if err = os.Rename(req.Source, req.Source+".user-kept"); err != nil {
		t.Fatal(err)
	}
	r, err = m.RecoverMigration(context.Background(), r.ID)
	if err != nil || r.Migration.Phase != "failed" {
		t.Fatal(r, err)
	}
	if data, err := os.ReadFile(filepath.Join(req.Source, "sub", "keep.txt")); err != nil || string(data) != "original data" {
		t.Fatal("recovery did not restore original", err)
	}
}
func TestMigrationCommittedJournalCanBeFinalizedWithoutDeletingBackup(t *testing.T) {
	m, req, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops.checkpoint = func(phase string) error {
		if phase == "committed" {
			return errors.New("simulated finalization failure")
		}
		return nil
	}
	r, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
	if err == nil || r.Migration.Phase != "recovery-required" {
		t.Fatal(r, err)
	}
	reopened, err := New(m.directory)
	if err != nil {
		t.Fatal(err)
	}
	r, err = reopened.RecoverMigration(context.Background(), r.ID)
	if err != nil || r.Status != "active" || r.Migration.Phase != "complete" || len(r.Migration.Retained) != 1 {
		t.Fatal(r, err)
	}
	if _, err = os.Stat(r.Migration.Retained[0].Path); err != nil {
		t.Fatal("finalization deleted backup", err)
	}
	rows, forgotten, err := reopened.ForgetInvalid(context.Background())
	if err != nil || forgotten != 0 || len(rows) != 1 || len(rows[0].Migration.Retained) != 1 {
		t.Fatal("history cleanup lost recovery data", rows, forgotten, err)
	}
}
func TestRestoreRollbackKeepsOriginalMigrationRetryable(t *testing.T) {
	for _, phase := range []string{"copied", "backed-up"} {
		t.Run(phase, func(t *testing.T) {
			m, req, ops := migrationFixture(t)
			p, err := m.PreviewMigration(context.Background(), req, nil)
			if err != nil {
				t.Fatal(err)
			}
			r, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
			if err != nil {
				t.Fatal(err)
			}
			p, err = m.PreviewMigration(context.Background(), MigrationRequest{RecordID: r.ID, CheckedAt: r.CheckedAt}, nil)
			if err != nil {
				t.Fatal(err)
			}
			ops.checkpoint = func(at string) error {
				if at == phase {
					return errors.New("restore failure")
				}
				return nil
			}
			r, err = m.executeMigration(context.Background(), p.ID, true, nil, ops)
			if err == nil || r.Status != "active" || r.Migration.Operation != "migrate" || r.Migration.Phase != "complete" {
				t.Fatal("restore failure lost retry path", r, err)
			}
			if _, err = m.PreviewMigration(context.Background(), MigrationRequest{RecordID: r.ID, CheckedAt: r.CheckedAt}, nil); err != nil {
				t.Fatal("restore cannot be retried", err)
			}
		})
	}
}
func TestMigrationOccupancyAndMetadataFailuresLeaveSourceAlone(t *testing.T) {
	m, req, ops := migrationFixture(t)
	name, _ := windows.UTF16PtrFromString(filepath.Join(req.Source, "sub", "keep.txt"))
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.PreviewMigration(context.Background(), req, nil)
	windows.CloseHandle(h)
	if err == nil {
		t.Fatal("writable file handle not detected")
	}
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(m.directory, "history.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.executeMigration(context.Background(), p.ID, true, nil, ops); err == nil || len(m.List()) != 0 {
		t.Fatal("failed intent save mutated history", err)
	}
	if _, err = os.Stat(req.Source); err != nil {
		t.Fatal("failed intent save moved source", err)
	}
}
func TestMigrationExternalHardlinksWarnAndInternalHardlinksRemainLinked(t *testing.T) {
	m, req, ops := migrationFixture(t)
	sourceFile := filepath.Join(req.Source, "sub", "keep.txt")
	if err := os.Link(sourceFile, filepath.Join(req.Source, "sub", "alias.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sourceFile, filepath.Join(filepath.Dir(req.Source), "external.txt")); err != nil {
		t.Fatal(err)
	}
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil || len(p.Warnings) == 0 {
		t.Fatal(p, err)
	}
	if _, err = m.executeMigration(context.Background(), p.ID, false, nil, ops); err == nil {
		t.Fatal("risk acknowledgment bypassed")
	}
	p, err = m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.executeMigration(context.Background(), p.ID, true, nil, ops); err != nil {
		t.Fatal(err)
	}
	a, err := openState(filepath.Join(p.Destination, "sub", "keep.txt"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	b, err := openState(filepath.Join(p.Destination, "sub", "alias.txt"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	if !sameIdentity(a.identity, b.identity) {
		t.Fatal("internal hardlink relationship lost")
	}
}
func TestMigrationCopyCancellationAndBackupCleanupFailure(t *testing.T) {
	m, req, ops := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = m.executeMigration(ctx, p.ID, true, func(progress MigrationProgress) {
		if progress.Phase == "copying" {
			cancel()
		}
	}, ops)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("copy did not cancel", err)
	}
	p, err = m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops.recycle = func(context.Context, MigrationArtifact) error { return errors.New("recycle unavailable") }
	r, err := m.executeMigration(context.Background(), p.ID, true, nil, ops)
	if err != nil || r.Status != "active" || len(r.Migration.Retained) != 1 {
		t.Fatal("retained backup was hidden or reported as failed migration", r, err)
	}
	if _, err = os.Stat(r.Migration.Retained[0].Path); err != nil {
		t.Fatal(err)
	}
}
