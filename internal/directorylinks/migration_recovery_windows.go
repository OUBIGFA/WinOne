//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func addMigrationArtifact(items []MigrationArtifact, entry MigrationArtifact) []MigrationArtifact {
	for _, old := range items {
		if strings.EqualFold(old.Path, entry.Path) && sameIdentity(old.Identity, entry.Identity) {
			return items
		}
	}
	return append(items, entry)
}

func checkedRestoredMigration(r Record) Record {
	r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Accessible = false
	r.Status = "unknown"
	r.Reason = "cannot-check"
	r.Detail = ""
	r.ActualType = ""
	r.ActualTarget = ""
	r.ResolvedTarget = ""
	f, err := openState(r.LinkPath, false, true)
	if err != nil {
		return failedCheck(r, err, "entry")
	}
	defer f.close()
	r.ObservedEntry = f.identity
	if !sameIdentity(f.identity, r.EntryIdentity) || !f.directory || f.reparse {
		r.Status = "replaced"
		r.Reason = "entry-replaced"
		return r
	}
	r.Status = "restored"
	r.Reason = "migration-restored"
	r.Accessible = true
	r.ActualType = "directory"
	r.ResolvedTarget = r.LinkPath
	return r
}

// RecoverMigration is explicit and non-destructive. If the original was
// already moved aside, put that exact identity back only when its name is free.
// All copies are retained; recovery never infers deletion authority from a name.
func (m *Manager) RecoverMigration(ctx context.Context, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	index := m.index(id)
	if index < 0 {
		return Record{}, errors.New("搬迁记录不存在")
	}
	r := m.records[index]
	if m.migrating || r.Migration == nil || r.Migration.Phase == "complete" {
		return r, errors.New("此记录没有可恢复的中断任务")
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	j := *r.Migration
	if j.Operation != "migrate" && j.Operation != "restore" {
		return r, errors.New("搬迁恢复记录无效")
	}
	j.Retained = append([]MigrationArtifact{}, j.Retained...)
	r.Migration = &j
	original := j.Source
	if j.Operation == "restore" {
		original = j.Destination
	}
	for _, path := range []string{original, j.Backup, j.Staging, j.Destination} {
		if _, err := migrationLocalPath(path); err != nil {
			return r, err
		}
	}
	pins, err := pinMigrationAncestors(filepath.Dir(original))
	if err != nil {
		return r, err
	}
	defer closeMigrationPins(pins)
	current, err := openState(original, false, false)
	if err == nil {
		// A crash may happen after the switch but before the last journal save.
		// Recognize an already completed switch by the verified copy identity;
		// finalization writes history only and retains all backup data.
		complete := false
		if j.StagingIdentity.File != "" {
			if j.Operation == "restore" {
				complete = current.directory && !current.reparse && sameIdentity(current.identity, j.StagingIdentity)
			} else {
				if target, e := openState(j.Destination, false, false); e == nil {
					complete = target.directory && !target.reparse && sameIdentity(target.identity, j.StagingIdentity) && verifyMigrationLink(original, j.Destination, current.identity) == nil
					target.close()
				}
			}
		}
		if complete {
			r.EntryIdentity = current.identity
			r.TargetIdentity = j.StagingIdentity
			current.close()
			artifacts := []MigrationArtifact{{j.Backup, j.OriginalIdentity, "directory"}}
			if j.Operation == "restore" {
				artifacts[0].Kind = "junction"
				artifacts = append(artifacts, MigrationArtifact{j.Source, j.SourceIdentity, "directory"})
			}
			for _, a := range artifacts {
				if f, e := openState(a.Path, false, false); e == nil {
					if sameIdentity(f.identity, a.Identity) {
						j.Retained = addMigrationArtifact(j.Retained, a)
					}
					f.close()
				}
			}
			j.Phase = "complete"
			j.Previous = nil
			j.Message = "已核实文件操作完成；保留的备份尚未自动清理。"
			r = checked(r)
			before := m.records[index]
			m.records[index] = r
			if err = m.save(); err != nil {
				m.records[index] = before
				return r, err
			}
			return r, nil
		}
		same := sameIdentity(current.identity, j.OriginalIdentity)
		current.close()
		if !same {
			return r, errors.New("原位置已被使用。所有副本均保留，请先人工核对；不会覆盖当前入口")
		}
	} else {
		if !missing(err) {
			return r, err
		}
		if err = renameMigrationEntry(j.Backup, original, j.OriginalIdentity); err != nil {
			return r, fmt.Errorf("无法恢复原入口，数据与副本仍保留：%w", err)
		}
	}
	for _, path := range []string{j.Staging, j.Destination} {
		if path == original {
			continue
		}
		if f, e := openState(path, false, false); e == nil {
			if !f.reparse && sameIdentity(f.identity, j.StagingIdentity) {
				j.Retained = addMigrationArtifact(j.Retained, MigrationArtifact{path, f.identity, "directory"})
			}
			f.close()
		}
	}
	if j.Operation == "restore" && j.Previous != nil {
		previous := *j.Previous
		for _, a := range j.Retained {
			previous.Retained = addMigrationArtifact(previous.Retained, a)
		}
		r.EntryIdentity = j.OriginalIdentity
		r.TargetIdentity = j.SourceIdentity
		previous.Message = "中断的还原已撤回，原联接和数据保留，可重新检查后还原。"
		j = previous
		r.Migration = &j
		r = checked(r)
	} else {
		j.Phase = "failed"
		j.Message = "原入口已恢复；未自动删除任何副本，请核对历史中保留的位置。"
		r.Status = "failed"
		r.Reason = "migration-failed"
		r.Detail = j.Message
	}
	before := m.records[index]
	m.records[index] = r
	if err = m.save(); err != nil {
		m.records[index] = before
		return r, err
	}
	return r, nil
}

// CleanupMigrationArtifact is a separate explicit action. The recycle layer
// keeps a final identity/type check and never falls back to permanent deletion.
func (m *Manager) CleanupMigrationArtifact(ctx context.Context, id string, index int, expectedPath string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at := m.index(id)
	if at < 0 {
		return Record{}, errors.New("搬迁记录不存在")
	}
	r := m.records[at]
	if m.migrating || r.Migration == nil || index < 0 || index >= len(r.Migration.Retained) {
		return r, errors.New("请刷新搬迁备份列表后重试")
	}
	a := r.Migration.Retained[index]
	if !strings.EqualFold(filepath.Clean(expectedPath), a.Path) {
		return r, errors.New("备份列表已变化，请刷新并重新确认所选路径")
	}
	// Never offer the live data directory as an ordinary cleanup candidate.
	if a.Path == r.LinkPath || a.Path == r.TargetPath && r.Migration.Operation == "migrate" && r.Migration.Phase == "complete" {
		return r, errors.New("该位置仍是当前数据入口，不能作为备份清理")
	}
	if _, err := os.Lstat(a.Path); err != nil {
		return r, err
	}
	if err := defaultMigrationOps().recycle(ctx, a); err != nil {
		return r, err
	}
	j := *r.Migration
	j.Retained = append([]MigrationArtifact{}, j.Retained...)
	j.Retained = append(j.Retained[:index], j.Retained[index+1:]...)
	r.Migration = &j
	m.records[at] = r
	return r, m.save()
}
