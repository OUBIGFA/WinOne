//go:build windows

package directorylinks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"winone/internal/recycle"
)

type migrationIntent struct {
	Plan     MigrationPlan
	Snapshot string
}
type migrationOps struct {
	checkpoint func(string) error
	recycle    func(context.Context, MigrationArtifact) error
}

func defaultMigrationOps() migrationOps {
	return migrationOps{recycle: func(ctx context.Context, a MigrationArtifact) error {
		check := func() error {
			f, err := openState(a.Path, false, false)
			if err != nil {
				return err
			}
			defer f.close()
			if !sameIdentity(f.identity, a.Identity) || f.directory != (a.Kind != "file") || f.reparse != (a.Kind == "junction") {
				return errors.New("备份身份或类型已变化，拒绝清理")
			}
			return nil
		}
		if a.Kind == "junction" {
			_, err := recycle.CheckedLink(ctx, a.Path, check)
			return err
		}
		_, err := recycle.CheckedDirectory(ctx, a.Path, check)
		return err
	}}
}

func (m *Manager) PreviewMigration(ctx context.Context, request MigrationRequest, progress migrationProgress) (MigrationPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var p MigrationPlan
	if m.migrating {
		return p, errors.New("目录搬迁正在进行，请等待或取消当前任务")
	}
	var source, destination string
	var err error
	p.Operation = "migrate"
	if request.RecordID != "" {
		index := m.index(request.RecordID)
		if index < 0 {
			return p, errors.New("搬迁记录不存在")
		}
		r := m.records[index]
		if r.Migration == nil || r.Migration.Phase != "complete" || r.Migration.Operation != "migrate" || r.CheckedAt != request.CheckedAt {
			return p, errors.New("请重新核查已经完成的搬迁记录后还原")
		}
		checked := checked(r)
		if checked.Status != "active" {
			return p, errors.New("原入口或搬迁目标已经变化，不能自动还原")
		}
		p.Operation = "restore"
		p.RecordID = r.ID
		p.LinkIdentity = r.EntryIdentity
		source, destination = r.TargetPath, r.LinkPath
	} else {
		source, err = migrationLocalPath(request.Source)
		if err != nil {
			return p, err
		}
		directory, e := migrationLocalPath(request.Directory)
		if e != nil {
			return p, e
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = filepath.Base(source)
		}
		if !validLinkName(name) {
			return p, errors.New("目标目录名称无效")
		}
		destination = filepath.Join(directory, name)
	}
	source, err = migrationLocalPath(source)
	if err != nil {
		return p, err
	}
	destination, err = migrationLocalPath(destination)
	if err != nil {
		return p, err
	}
	if migrationWithin(source, destination) || migrationWithin(destination, source) {
		return p, errors.New("源目录与目标目录不能相同或相互包含")
	}
	warnings, err := migrationDanger(source, m.directory)
	if err != nil {
		return p, err
	}
	if _, err = migrationDanger(destination, m.directory); err != nil {
		return p, err
	}
	pins, err := pinMigrationAncestors(filepath.Dir(source))
	if err != nil {
		return p, err
	}
	defer closeMigrationPins(pins)
	targetPins, err := pinMigrationAncestors(filepath.Dir(destination))
	if err != nil {
		return p, err
	}
	defer closeMigrationPins(targetPins)
	s, err := openState(source, false, false)
	if err != nil {
		return p, err
	}
	defer s.close()
	if !s.directory || s.reparse {
		return p, errors.New("源必须是本机普通目录，不能是现有链接")
	}
	for _, f := range []*fileState{s, targetPins[len(targetPins)-1]} {
		_, name, e := fileSystemInfo(f)
		if e != nil {
			return p, e
		}
		if !strings.EqualFold(name, "NTFS") {
			return p, errors.New("搬迁与还原只支持本机 NTFS 目录")
		}
	}
	if p.Operation == "migrate" {
		if _, e := os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
			return p, errors.New("目标位置已有同名项目或无法确认，不能覆盖")
		}
	}
	if err = checkMigrationProcesses(ctx, source); err != nil {
		return p, err
	}
	entries, bytes, err := scanMigrationTree(ctx, source, false, progress)
	if err != nil {
		return p, err
	}
	if err = migrationFreeSpace(filepath.Dir(destination), bytes); err != nil {
		return p, err
	}
	linkCounts := map[string]uint32{}
	for _, entry := range entries {
		if !entry.Directory {
			linkCounts[entry.Identity.File]++
		}
	}
	for _, entry := range entries {
		if !entry.Directory && entry.Links > linkCounts[entry.Identity.File] {
			warnings = append(warnings, "目录中存在指向外部位置的硬链接别名。搬迁后外部别名仍保留旧数据，不会随新位置一起修改。")
			break
		}
	}
	p.ID = rand.Text()
	p.Source = source
	p.Destination = destination
	p.SourceIdentity = s.identity
	p.SourceParentIdentity = pins[len(pins)-1].identity
	p.DestinationParentIdentity = targetPins[len(targetPins)-1].identity
	p.Bytes = bytes
	p.Files = len(entries)
	p.Warnings = warnings
	p.ExpiresAt = time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	if m.migrationPlans == nil {
		m.migrationPlans = map[string]migrationIntent{}
	}
	for id, old := range m.migrationPlans {
		expiry, e := time.Parse(time.RFC3339Nano, old.Plan.ExpiresAt)
		if e != nil || time.Now().After(expiry) {
			delete(m.migrationPlans, id)
		}
	}
	if len(m.migrationPlans) >= 64 {
		return MigrationPlan{}, errors.New("搬迁预览过多，请稍后重试")
	}
	stored := p
	stored.Warnings = append([]string{}, p.Warnings...)
	m.migrationPlans[p.ID] = migrationIntent{stored, migrationSnapshot(entries)}
	return p, nil
}

func (m *Manager) ExecuteMigration(ctx context.Context, token string, acknowledged bool, progress migrationProgress) (Record, error) {
	return m.executeMigration(ctx, token, acknowledged, progress, defaultMigrationOps())
}
func (m *Manager) executeMigration(ctx context.Context, token string, acknowledged bool, progress migrationProgress, ops migrationOps) (result Record, runErr error) {
	m.mu.Lock()
	intent, ok := m.migrationPlans[token]
	if !ok || m.migrating {
		m.mu.Unlock()
		return result, errors.New("请先预览，且等待当前目录任务结束")
	}
	delete(m.migrationPlans, token)
	p := intent.Plan
	expiry, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if err != nil || time.Now().After(expiry) {
		m.mu.Unlock()
		return result, errors.New("搬迁预览已过期，请重新预览")
	}
	if len(p.Warnings) > 0 && !acknowledged {
		m.mu.Unlock()
		return result, errors.New("请先确认应用目录的风险提示")
	}
	if p.Operation == "migrate" && len(m.records) >= maxRecords {
		m.mu.Unlock()
		return result, errors.New("链接历史记录已满")
	}
	m.migrating = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.migrating = false; m.mu.Unlock() }()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	sourcePins, err := pinMigrationAncestors(filepath.Dir(p.Source))
	if err != nil {
		return result, err
	}
	defer closeMigrationPins(sourcePins)
	destPins, err := pinMigrationAncestors(filepath.Dir(p.Destination))
	if err != nil {
		return result, err
	}
	defer closeMigrationPins(destPins)
	if !sameIdentity(sourcePins[len(sourcePins)-1].identity, p.SourceParentIdentity) || !sameIdentity(destPins[len(destPins)-1].identity, p.DestinationParentIdentity) {
		return result, errors.New("目录父级在预览后发生变化")
	}
	if err = checkMigrationProcesses(ctx, p.Source); err != nil {
		return result, err
	}
	current, _, err := scanMigrationTree(ctx, p.Source, false, progress)
	if err != nil {
		return result, err
	}
	if intent.Snapshot != migrationSnapshot(current) {
		return result, errors.New("源目录内容在预览后发生变化，请重新预览")
	}
	if err = migrationFreeSpace(filepath.Dir(p.Destination), p.Bytes); err != nil {
		return result, err
	}
	if p.Operation == "migrate" {
		if _, e := os.Lstat(p.Destination); !errors.Is(e, os.ErrNotExist) {
			return result, errors.New("目标目录在预览后被占用")
		}
	} else if err = verifyMigrationLink(p.Destination, p.Source, p.LinkIdentity); err != nil {
		return result, err
	}
	id := rand.Text()
	stage := filepath.Join(filepath.Dir(p.Destination), ".winone-migration-copy-"+id)
	backupBase := p.Source
	if p.Operation == "restore" {
		backupBase = p.Destination
	}
	backup := filepath.Join(filepath.Dir(backupBase), ".winone-migration-backup-"+id)
	j := &MigrationJournal{Operation: p.Operation, Phase: "copying", Source: p.Source, Destination: p.Destination, Staging: stage, Backup: backup, SourceIdentity: p.SourceIdentity, OriginalIdentity: p.SourceIdentity, Bytes: p.Bytes, Files: p.Files}
	result = Record{ID: id, Name: filepath.Base(p.Source), LinkPath: p.Source, TargetPath: p.Destination, TargetText: p.Destination, Kind: "junction", ObjectType: "directory", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "creating", Reason: "creating", Migration: j}
	if p.Operation == "restore" {
		m.mu.Lock()
		index := m.index(p.RecordID)
		if index < 0 {
			m.mu.Unlock()
			return result, errors.New("搬迁记录已移除")
		}
		result = m.records[index]
		m.mu.Unlock()
		j.OriginalIdentity = p.LinkIdentity
		if result.Migration != nil {
			previous := *result.Migration
			previous.Retained = append([]MigrationArtifact{}, previous.Retained...)
			j.Previous = &previous
			j.Retained = append(j.Retained, result.Migration.Retained...)
		}
		result.Migration = j
	}
	persist := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		before := append([]Record{}, m.records...)
		stored := result
		copyJournal := *j
		copyJournal.Retained = append([]MigrationArtifact{}, j.Retained...)
		stored.Migration = &copyJournal
		index := m.index(result.ID)
		if index < 0 {
			m.records = append(m.records, stored)
		} else {
			m.records[index] = stored
		}
		if err := m.save(); err != nil {
			m.records = before
			return err
		}
		return nil
	}
	// Persist the full recovery intent before creating or renaming anything.
	if err = persist(); err != nil {
		return result, err
	}
	committed, backedUp := false, false
	var stageIdentity, createdLink Identity
	checkpoint := func(phase string) error {
		if ops.checkpoint != nil {
			return ops.checkpoint(phase)
		}
		return nil
	}
	defer func() {
		if runErr == nil {
			return
		}
		// Roll back only known identities with no-overwrite native renames. A
		// colliding user entry is retained; its presence is never "cleaned up".
		if backedUp && !committed {
			if createdLink.File != "" {
				retained := p.Source + ".winone-rejected-" + id
				if e := renameMigrationEntry(p.Source, retained, createdLink); e != nil {
					runErr = errors.Join(runErr, e)
				} else {
					j.Retained = append(j.Retained, MigrationArtifact{retained, createdLink, "junction"})
				}
			}
			if e := renameMigrationEntry(backup, backupBase, j.OriginalIdentity); e != nil {
				j.Phase = "recovery-required"
				runErr = errors.Join(runErr, fmt.Errorf("自动恢复未完成，原数据保留在 %s：%w", backup, e))
			} else {
				j.Phase = "failed"
			}
		} else if committed {
			j.Phase = "recovery-required"
		} else {
			j.Phase = "failed"
		}
		if !committed && stageIdentity.File != "" && ops.recycle != nil {
			a := MigrationArtifact{stage, stageIdentity, "directory"}
			if e := ops.recycle(context.Background(), a); e != nil {
				j.Retained = append(j.Retained, a)
				runErr = errors.Join(runErr, fmt.Errorf("副本已保留在 %s：%w", stage, e))
			}
		}
		if p.Operation == "restore" && !committed && j.Phase != "recovery-required" && j.Previous != nil {
			previous := *j.Previous
			for _, a := range j.Retained {
				previous.Retained = addMigrationArtifact(previous.Retained, a)
			}
			previous.Message = "还原未完成，原联接和数据已保留，可重新检查后还原。" + runErr.Error()
			*j = previous
			result.Migration = j
			result = checked(result)
		} else {
			result.Status, result.Reason = "failed", "migration-failed"
			j.Message = runErr.Error()
		}
		runErr = errors.Join(runErr, persist())
	}()
	if err = os.Mkdir(stage, 0700); err != nil {
		return result, err
	}
	stageState, err := openState(stage, false, false)
	if err != nil {
		return result, err
	}
	stageIdentity = stageState.identity
	j.StagingIdentity = stageIdentity
	stageState.close()
	if err = persist(); err != nil {
		return result, err
	}
	if err = copyMigrationTree(ctx, p.Source, stage, stageIdentity, current, p.Bytes, progress); err != nil {
		return result, err
	}
	if err = checkpoint("copied"); err != nil {
		return result, err
	}
	if progress != nil {
		progress(MigrationProgress{Phase: "verifying", TotalFiles: p.Files, TotalBytes: p.Bytes})
	}
	j.Digest, err = verifyMigrationTrees(ctx, p.Source, stage, current, nil)
	if err != nil {
		return result, err
	}
	j.Phase = "verified"
	if err = persist(); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = checkpoint("verified"); err != nil {
		return result, err
	}
	if p.Operation == "restore" {
		if err = verifyMigrationLink(p.Destination, p.Source, p.LinkIdentity); err != nil {
			return result, err
		}
	}
	if err = renameMigrationEntry(backupBase, backup, j.OriginalIdentity); err != nil {
		return result, err
	}
	backedUp = true
	j.Phase = "backed-up"
	if err = persist(); err != nil {
		return result, err
	}
	if err = checkpoint("backed-up"); err != nil {
		return result, err
	}
	if p.Operation == "migrate" {
		// Recheck after the original path is moved aside. A late writer cannot
		// change what is certified merely by racing the preceding scan.
		if _, err = verifyMigrationTrees(ctx, backup, stage, current, nil); err != nil {
			return result, err
		}
	} else if _, err = verifyMigrationTrees(ctx, p.Source, stage, current, nil); err != nil {
		return result, err
	}
	if progress != nil {
		progress(MigrationProgress{Phase: "committing", Files: p.Files, TotalFiles: p.Files, Bytes: p.Bytes, TotalBytes: p.Bytes})
	}
	// Cancellation no longer interrupts the small namespace transaction. Finish
	// or roll back before honoring another user action.
	if p.Operation == "migrate" {
		if err = renameMigrationEntry(stage, p.Destination, stageIdentity); err != nil {
			return result, err
		}
		stage = p.Destination
		j.Staging = stage // Any later failure retains this full copy.
		createdLink, err = createMigrationJunction(p.Source, p.Destination)
		if err != nil {
			return result, err
		}
		result.EntryIdentity = createdLink
		if err = verifyMigrationLink(p.Source, p.Destination, result.EntryIdentity); err != nil {
			return result, err
		}
		result.TargetIdentity = stageIdentity
	} else {
		if err = renameMigrationEntry(stage, p.Destination, stageIdentity); err != nil {
			return result, err
		}
		result.EntryIdentity = stageIdentity
		result.TargetIdentity = stageIdentity
	}
	committed = true
	j.Phase = "committed"
	if err = persist(); err != nil {
		return result, err
	}
	if err = checkpoint("committed"); err != nil {
		return result, err
	}
	if progress != nil {
		progress(MigrationProgress{Phase: "cleanup", Files: p.Files, TotalFiles: p.Files, Bytes: p.Bytes, TotalBytes: p.Bytes})
	}
	clean := func(a MigrationArtifact) {
		if ops.recycle == nil {
			j.Retained = append(j.Retained, a)
			return
		}
		if e := ops.recycle(context.Background(), a); e != nil {
			j.Retained = append(j.Retained, a)
			j.Message = "搬迁已完成；部分备份未能进入回收站，请在历史详情中查看保留路径。"
		}
	}
	if p.Operation == "migrate" {
		clean(MigrationArtifact{backup, p.SourceIdentity, "directory"})
	} else {
		clean(MigrationArtifact{backup, p.LinkIdentity, "junction"})
		clean(MigrationArtifact{p.Source, p.SourceIdentity, "directory"})
	}
	j.Phase = "complete"
	j.Previous = nil
	result.Migration = j
	result = checked(result)
	if err = persist(); err != nil {
		return result, fmt.Errorf("文件操作已完成，但历史保存失败，请核对 %s：%w", p.Destination, err)
	}
	return result, nil
}

func verifyMigrationLink(path, target string, id Identity) error {
	f, err := openState(path, false, false)
	if err != nil {
		return err
	}
	defer f.close()
	tag, text, relative, err := readReparse(f)
	if err != nil {
		return err
	}
	if !sameIdentity(f.identity, id) || !f.directory || tag != tagJunction || relative || !strings.EqualFold(filepath.Clean(text), filepath.Clean(target)) {
		return errors.New("原位联接的身份或指向已变化，拒绝还原")
	}
	return nil
}
func renameMigrationEntry(path, destination string, id Identity) error {
	from, err := openState(filepath.Dir(path), true, false)
	if err != nil {
		return err
	}
	defer from.close()
	to, err := openState(filepath.Dir(destination), true, false)
	if err != nil {
		return err
	}
	defer to.close()
	f, err := openRenameEntry(from.handle, path)
	if err != nil {
		return err
	}
	defer f.close()
	if !sameIdentity(f.identity, id) {
		return errors.New("要移动的目录身份已经变化")
	}
	return renameEntry(f.handle, to.handle, filepath.Base(destination))
}

// Creation owns the new empty directory handle from birth. If setting the
// reparse data fails, rename that exact empty object out of the way so the
// original backup can return; never delete a path that a caller may replace.
func createMigrationJunction(path, target string) (Identity, error) {
	var zero Identity
	raw, err := junctionData(target)
	if err != nil {
		return zero, err
	}
	parent, err := openState(filepath.Dir(path), true, false)
	if err != nil {
		return zero, err
	}
	defer parent.close()
	name, err := windows.NewNTUnicodeString(filepath.Base(path))
	if err != nil {
		return zero, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: parent.handle, ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES|windows.DELETE|windows.SYNCHRONIZE, &attributes, &status, nil, windows.FILE_ATTRIBUTE_DIRECTORY, 0, windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return zero, err
	}
	f, err := readFileState(h, path, false)
	if err != nil {
		return zero, err
	}
	defer f.close()
	var count uint32
	if err = windows.DeviceIoControl(f.handle, windows.FSCTL_SET_REPARSE_POINT, &raw[0], uint32(len(raw)), nil, 0, &count, nil); err != nil {
		retained := filepath.Base(path) + ".winone-empty-" + rand.Text()
		return zero, errors.Join(err, renameEntry(f.handle, parent.handle, retained), fmt.Errorf("新建空入口保留位置：%s", filepath.Join(filepath.Dir(path), retained)))
	}
	return f.identity, nil
}

func copyMigrationTree(ctx context.Context, source, target string, targetIdentity Identity, entries []migrationEntry, total int64, progress migrationProgress) error {
	var copied int64
	links := map[string]string{}
	for index, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Relative == "." {
			continue
		}
		from, to := filepath.Join(source, e.Relative), filepath.Join(target, e.Relative)
		pins, err := pinMigrationAncestors(filepath.Dir(from))
		if err != nil {
			return err
		}
		err = func() error {
			targetPins, pinErr := pinMigrationAncestors(filepath.Dir(to))
			if pinErr != nil {
				return pinErr
			}
			defer closeMigrationPins(targetPins)
			root, rootErr := openState(target, false, false)
			if rootErr != nil {
				return rootErr
			}
			sameRoot := sameIdentity(root.identity, targetIdentity) && !root.reparse
			root.close()
			if !sameRoot {
				return errors.New("复制副本目录已被替换，拒绝写入")
			}
			state, err := openState(from, false, false)
			if err != nil {
				return err
			}
			defer state.close()
			if !sameIdentity(state.identity, e.Identity) || state.reparse || state.directory != e.Directory {
				return errors.New("复制期间源对象发生变化")
			}
			if e.Directory {
				return os.Mkdir(to, 0700)
			}
			file, err := openMigrationFile(from)
			if err != nil {
				return err
			}
			defer file.Close()
			key := fmt.Sprintf("%s:%d:%s", e.Identity.VolumeID, e.Identity.Volume, e.Identity.File)
			if previous := links[key]; previous != "" {
				if err = os.Link(previous, to); err != nil {
					return err
				}
			} else {
				if err = copyMigrationFile(ctx, from, to, func(n int64) {
					if progress != nil {
						progress(MigrationProgress{Phase: "copying", Files: index, TotalFiles: len(entries), Bytes: copied + n, TotalBytes: total})
					}
				}); err != nil {
					return err
				}
				links[key] = to
			}
			copied += e.Bytes
			return nil
		}()
		closeMigrationPins(pins)
		if err != nil {
			return err
		}
	}
	// Directory timestamps are restored after populating their children.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.Directory {
			continue
		}
		path := filepath.Join(target, e.Relative)
		if err := setMigrationDirectoryTimes(path, e, target, targetIdentity); err != nil {
			return err
		}
	}
	return nil
}
