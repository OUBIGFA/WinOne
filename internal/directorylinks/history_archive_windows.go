//go:build windows

package directorylinks

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"winone/internal/configjson"
)

// HistoryArchive is a manual backup of relationships, not portable preferences
// or authority to mutate the filesystem. Machine-local identities are omitted.
type HistoryArchive struct {
	Version int            `json:"version"`
	Entries []HistoryEntry `json:"entries"`
}
type HistoryEntry struct {
	Name       string `json:"name"`
	LinkPath   string `json:"linkPath"`
	TargetPath string `json:"targetPath"`
	Kind       string `json:"kind"`
}
type HistoryImportPreview struct {
	ID        string      `json:"id"`
	ExpiresAt string      `json:"expiresAt"`
	Records   []Record    `json:"records"`
	Issues    []ScanIssue `json:"issues"`
}

// ExportHistory returns only the explicitly selected relationships. It neither
// refreshes metadata nor writes a file; the desktop's save dialog owns the file.
func (m *Manager) ExportHistory(ids []string) (HistoryArchive, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := HistoryArchive{Version: 1, Entries: []HistoryEntry{}}
	if len(ids) == 0 || len(ids) > maxRecords {
		return out, errors.New("请选择需要导出的链接记录")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		i := m.index(id)
		if i < 0 {
			return out, errors.New("链接记录不存在，请刷新后重试")
		}
		r := m.records[i]
		out.Entries = append(out.Entries, HistoryEntry{Name: r.Name, LinkPath: r.LinkPath, TargetPath: r.TargetPath, Kind: r.Kind})
	}
	return out, nil
}

// ExportHistoryFile creates a new backup in the chosen directory. Existing
// files are never overwritten; partial-write failures retain their path.
func (m *Manager) ExportHistoryFile(ids []string, directory string) (string, error) {
	archive, err := m.ExportHistory(ids)
	if err != nil {
		return "", err
	}
	path, err := localPath(directory)
	if err != nil {
		return "", err
	}
	parent, err := openState(path, true, false)
	if err != nil {
		return "", err
	}
	defer parent.close()
	if !parent.directory {
		return "", errors.New("请选择备份保存文件夹")
	}
	raw, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(parent.finalPath, "WinOne-links-*.json")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(raw)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return "", fmt.Errorf("备份写入未完成，已保留 %s：%w", file.Name(), err)
	}
	return file.Name(), nil
}

func (m *Manager) PreviewHistoryFile(ctx context.Context, path string) (HistoryImportPreview, error) {
	var out HistoryImportPreview
	path, err := localPath(path)
	if err != nil {
		return out, err
	}
	f, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return out, errors.New("请选择不超过 4 MiB 的链接历史 JSON 备份")
	}
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return out, err
	}
	return m.PreviewHistoryImport(ctx, data)
}

// PreviewHistoryImport rebuilds every identity from the current local disk.
// Exported or hand-edited JSON cannot import an identity, overwrite an existing
// registration, or turn a normal directory into a removable link.
func (m *Manager) PreviewHistoryImport(ctx context.Context, data json.RawMessage) (HistoryImportPreview, error) {
	out := HistoryImportPreview{Records: []Record{}, Issues: []ScanIssue{}}
	if len(data) > 4<<20 {
		return out, errors.New("链接历史备份过大")
	}
	var archive HistoryArchive
	if err := configjson.DecodeObject(data, &archive); err != nil {
		return out, err
	}
	if archive.Version != 1 || len(archive.Entries) == 0 || len(archive.Entries) > maxRecords {
		return out, errors.New("链接历史备份版本或条目数量无效")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	known := map[string]bool{}
	for _, r := range m.records {
		known[strings.ToLower(filepath.Clean(r.LinkPath))] = true
	}
	for _, entry := range archive.Entries {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		path, err := localPath(entry.LinkPath)
		if err != nil {
			out.Issues = append(out.Issues, ScanIssue{entry.LinkPath, err.Error()})
			continue
		}
		key := strings.ToLower(path)
		if known[key] {
			out.Issues = append(out.Issues, ScanIssue{path, "该入口已登记或在备份中重复；不会覆盖现有记录"})
			continue
		}
		r, err := observeHistoryEntry(entry)
		if err != nil {
			out.Issues = append(out.Issues, ScanIssue{path, err.Error()})
			continue
		}
		known[key] = true
		out.Records = append(out.Records, r)
	}
	if len(out.Records) == 0 {
		return out, nil
	}
	if len(out.Records)+len(m.records) > maxRecords {
		return out, errors.New("导入后将超过链接历史数量上限")
	}
	if m.historyImports == nil {
		m.historyImports = map[string]HistoryImportPreview{}
	}
	now := time.Now()
	for id, p := range m.historyImports {
		expiry, e := time.Parse(time.RFC3339Nano, p.ExpiresAt)
		if e != nil || now.After(expiry) {
			delete(m.historyImports, id)
		}
	}
	if len(m.historyImports) >= 64 {
		return out, errors.New("导入预览过多，请稍后重试")
	}
	out.ID = rand.Text()
	out.ExpiresAt = now.Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	saved := out
	saved.Records = append([]Record{}, out.Records...)
	saved.Issues = append([]ScanIssue{}, out.Issues...)
	m.historyImports[out.ID] = saved
	return out, nil
}

func observeHistoryEntry(e HistoryEntry) (Record, error) {
	var r Record
	path, err := localPath(e.LinkPath)
	if err != nil {
		return r, err
	}
	target, err := localPath(e.TargetPath)
	if err != nil {
		return r, err
	}
	if len(e.Name) > 200 || strings.ContainsAny(e.Name, "\x00\r\n") {
		return r, errors.New("记录名称无效")
	}
	f, err := openState(path, false, false)
	if err != nil {
		return r, fmt.Errorf("无法核实本机入口：%w", err)
	}
	defer f.close()
	r = Record{ID: rand.Text(), Name: strings.TrimSpace(e.Name), LinkPath: path, TargetPath: target, Kind: e.Kind, ObjectType: f.objectType(), TargetText: target, EntryIdentity: f.identity, RegisteredAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if r.Name == "" {
		r.Name = filepath.Base(path)
	}
	switch e.Kind {
	case "junction", "symlink":
		if !f.reparse {
			return r, errors.New("本机入口不是重解析链接，拒绝登记为链接")
		}
		tag, text, relative, err := readReparse(f)
		if err != nil {
			return r, err
		}
		want := tagSymlink
		if e.Kind == "junction" {
			want = tagJunction
		}
		if tag != want {
			return r, errors.New("本机链接类型与备份不一致")
		}
		actual := text
		if relative {
			actual = filepath.Join(filepath.Dir(path), text)
		}
		if !strings.EqualFold(filepath.Clean(actual), target) {
			return r, errors.New("本机链接目标与备份不一致")
		}
		r.TargetText, r.Relative = text, relative
	case "shortcut":
		if f.directory || f.reparse || !strings.EqualFold(filepath.Ext(path), ".lnk") {
			return r, errors.New("本机入口不是快捷方式")
		}
		actual, err := shortcutTarget(path)
		if err != nil {
			return r, err
		}
		if !strings.EqualFold(filepath.Clean(actual), target) {
			return r, errors.New("本机快捷方式目标与备份不一致")
		}
	case "hardlink":
		if f.directory || f.reparse || f.links < 2 || strings.EqualFold(path, target) {
			return r, errors.New("本机入口不是可核实的文件硬链接")
		}
	default:
		return r, errors.New("备份中的链接类型无效")
	}
	t, err := openState(target, true, false)
	if err == nil {
		r.TargetIdentity = t.identity
		if e.Kind == "shortcut" {
			r.ObjectType = t.objectType()
		}
		t.close()
		if e.Kind == "hardlink" && !sameIdentity(r.EntryIdentity, r.TargetIdentity) {
			return r, errors.New("本机硬链接与目标不是同一文件")
		}
	} else if !missing(err) || e.Kind == "hardlink" {
		return r, fmt.Errorf("无法核实本机目标：%w", err)
	}
	r = checked(r)
	if r.Status != "active" && r.Status != "broken" {
		return r, fmt.Errorf("本机链接未通过核查：%s", r.Reason)
	}
	return r, nil
}

// ImportHistory consumes a preview once and commits all accepted rows together.
// A replacement since preview aborts the import, preserving both current history
// and all filesystem entries. Conflicts discovered at commit are not skipped.
func (m *Manager) ImportHistory(ctx context.Context, token string) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, ok := m.historyImports[token]
	if m.migrating {
		return nil, errMigrationBusy
	}
	if !ok {
		return nil, errors.New("请先预览本次历史导入")
	}
	delete(m.historyImports, token)
	expiry, err := time.Parse(time.RFC3339Nano, plan.ExpiresAt)
	if err != nil || time.Now().After(expiry) {
		return nil, errors.New("导入预览已过期，请重新预览")
	}
	if len(m.records)+len(plan.Records) > maxRecords {
		return nil, errors.New("链接历史记录已满")
	}
	added := make([]Record, 0, len(plan.Records))
	for _, old := range plan.Records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, known := range m.records {
			if strings.EqualFold(filepath.Clean(known.LinkPath), old.LinkPath) {
				return nil, errors.New("入口已被登记，请重新预览导入")
			}
		}
		now, err := observeHistoryEntry(HistoryEntry{Name: old.Name, LinkPath: old.LinkPath, TargetPath: old.TargetPath, Kind: old.Kind})
		if err != nil {
			return nil, err
		}
		if !sameIdentity(old.EntryIdentity, now.EntryIdentity) || old.TargetIdentity.File != now.TargetIdentity.File || old.TargetIdentity.File != "" && !sameIdentity(old.TargetIdentity, now.TargetIdentity) || old.Relative != now.Relative || old.TargetText != now.TargetText {
			return nil, errors.New("链接或目标在预览后发生变化，请重新预览导入")
		}
		now.ID = old.ID
		added = append(added, now)
	}
	before := m.records
	m.records = append(append([]Record{}, before...), added...)
	if err := m.save(); err != nil {
		m.records = before
		return nil, err
	}
	return added, nil
}
