//go:build windows

package directorylinks

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"winone/internal/configjson"
	"winone/internal/recycle"
)

const maxRecords = 1000

type Manager struct {
	mu        sync.Mutex
	directory string
	records   []Record
	plans     map[string]Plan
	batches   map[string][]Plan
}

// New reads only this tool's local history. Opening the settings page does not
// create directories, inspect unrelated disks, or activate a resident worker.
func New(directory string) (*Manager, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("链接历史目录必须是绝对路径")
	}
	m := &Manager{directory: filepath.Clean(directory), plans: map[string]Plan{}, records: []Record{}}
	data, err := os.ReadFile(filepath.Join(directory, "history.json"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("链接历史过大")
	}
	var saved struct {
		Version int      `json:"version"`
		Records []Record `json:"records"`
	}
	if err = configjson.DecodeObject(data, &saved); err != nil {
		return nil, err
	}
	if saved.Version != 1 || len(saved.Records) > maxRecords {
		return nil, errors.New("不支持的链接历史版本或数量")
	}
	seen := map[string]bool{}
	for _, r := range saved.Records {
		if r.ID == "" || seen[r.ID] || !filepath.IsAbs(r.LinkPath) || !filepath.IsAbs(r.TargetPath) || (r.Kind != "hardlink" && r.Kind != "junction" && r.Kind != "symlink" && r.Kind != "shortcut") {
			return nil, errors.New("链接历史记录无效")
		}
		seen[r.ID] = true
	}
	m.records = saved.Records
	return m, nil
}

func (m *Manager) List() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Record{}, m.records...)
}

func (m *Manager) Preview(r Request) (Plan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Kind != "auto" && r.Kind != "hardlink" && r.Kind != "junction" && r.Kind != "symlink" {
		return Plan{}, errors.New("请选择链接类型")
	}
	if !validLinkName(r.LinkName) {
		return Plan{}, errors.New("链接名称无效；请仅输入文件名或目录名")
	}
	source, err := localPath(r.Source)
	if err != nil {
		return Plan{}, err
	}
	directory, err := localPath(r.Directory)
	if err != nil {
		return Plan{}, err
	}
	entry, err := openState(source, false, false)
	if err != nil {
		return Plan{}, fmt.Errorf("无法读取源路径：%w", err)
	}
	if entry.reparse && r.Kind == "hardlink" {
		entry.close()
		return Plan{}, errors.New("硬链接源须为普通文件；可用符号链接指向现有入口")
	}
	if entry.reparse && r.Kind == "auto" {
		r.Kind = "symlink"
	}
	entry.close()
	s, err := openState(source, true, false)
	if err != nil {
		return Plan{}, err
	}
	defer s.close()
	d, err := openState(directory, true, false)
	if err != nil {
		return Plan{}, fmt.Errorf("无法读取链接位置：%w", err)
	}
	defer d.close()
	if !d.directory {
		return Plan{}, errors.New("链接位置必须是文件夹")
	}
	if r.Kind == "auto" {
		r.Kind = "hardlink"
		if s.directory {
			r.Kind = "junction"
		}
	}
	flags, _, err := fileSystemInfo(d)
	if err != nil {
		return Plan{}, err
	}
	if r.Kind == "hardlink" && flags&0x00400000 == 0 {
		return Plan{}, errors.New("链接位置的文件系统不支持硬链接")
	}
	if r.Kind != "hardlink" && flags&0x00000080 == 0 {
		return Plan{}, errors.New("链接位置的文件系统不支持重解析链接")
	}
	if r.Kind == "hardlink" && (s.directory || !sameVolume(s.identity, d.identity)) {
		return Plan{}, errors.New("硬链接仅支持同一卷中的文件")
	}
	if r.Kind == "junction" && !s.directory {
		return Plan{}, errors.New("目录联接仅支持文件夹")
	}
	if r.Kind == "junction" && (strings.HasPrefix(source, `\\`) || strings.HasPrefix(d.finalPath, `\\`)) {
		return Plan{}, errors.New("目录联接仅支持本机目录；网络目标请选择符号链接")
	}
	path := filepath.Join(d.finalPath, r.LinkName)
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return Plan{}, errors.New("链接位置已有同名项目，不能覆盖")
	}
	if s.directory {
		rel, e := filepath.Rel(s.finalPath, path)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return Plan{}, errors.New("不能在目标文件夹内创建指回目标的循环链接")
		}
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = r.LinkName
	}
	if len(name) > 200 {
		return Plan{}, errors.New("名称过长")
	}
	p := Plan{ID: rand.Text(), Name: name, Source: source, LinkPath: path, Kind: r.Kind, ObjectType: s.objectType(), TargetText: source, ExpiresAt: time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano), SourceIdentity: s.identity, ParentIdentity: d.identity}
	if r.Kind == "symlink" && r.Relative {
		p.TargetText, err = filepath.Rel(d.finalPath, source)
		if err != nil {
			return Plan{}, errors.New("跨卷符号链接请选择绝对目标路径")
		}
		p.Relative = true
	}
	for id, old := range m.plans {
		if expiry, e := time.Parse(time.RFC3339Nano, old.ExpiresAt); e != nil || time.Now().After(expiry) {
			delete(m.plans, id)
		}
	}
	if len(m.plans) > 64 {
		return Plan{}, errors.New("预览操作过多，请稍后重试")
	}
	m.plans[p.ID] = p
	return p, nil
}

func (m *Manager) Create(ctx context.Context, token string) (Record, error) {
	return m.CreateWith(ctx, token, Execute)
}

// CreateWith reserves the same single-use preview for a fixed trusted executor.
// The ordinary Host retains history ownership and verifies the created entry.
func (m *Manager) CreateWith(ctx context.Context, token string, execute func(Plan) error) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plans[token]
	if !ok {
		return Record{}, errors.New("请先预览本次链接")
	}
	delete(m.plans, token)
	expiry, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if err != nil || time.Now().After(expiry) {
		return Record{}, errors.New("预览已过期，请重新预览")
	}
	if err = ctx.Err(); err != nil {
		return Record{}, err
	}
	if err = verifyPlan(p); err != nil {
		return Record{}, err
	}
	if len(m.records) >= maxRecords {
		return Record{}, errors.New("历史记录已满，请先移除不需要的记录")
	}
	r := Record{ID: rand.Text(), Name: p.Name, LinkPath: p.LinkPath, TargetPath: p.Source, Kind: p.Kind, ObjectType: p.ObjectType, TargetText: p.TargetText, Relative: p.Relative, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "creating", Reason: "creating", TargetIdentity: p.SourceIdentity}
	// Persist intent before the filesystem mutation. A disk/reporting failure
	// must never strand an unreported entry or invite an automatic duplicate.
	m.records = append(m.records, r)
	if err = m.save(); err != nil {
		m.records = m.records[:len(m.records)-1]
		return Record{}, err
	}
	if err = execute(p); err != nil {
		r.Status, r.Reason, r.Detail = "failed", "create-failed", err.Error()
		r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		m.records[len(m.records)-1] = r
		return r, errors.Join(fmt.Errorf("未能创建链接：%w", err), m.save())
	}
	entry, err := openState(p.LinkPath, false, false)
	if err != nil {
		r.Status, r.Reason, r.Detail = "unknown", "created-unverified", err.Error()
	} else {
		r.EntryIdentity = entry.identity
		entry.close()
		r = checked(r)
	}
	m.records[len(m.records)-1] = r
	if err = m.save(); err != nil {
		return r, fmt.Errorf("链接已创建，但历史保存失败，请保留 %s：%w", r.LinkPath, err)
	}
	if r.Status != "active" {
		return r, errors.New("链接已创建，但当前核查未通过；请检查历史中的原因")
	}
	return r, nil
}

func (m *Manager) Check(ctx context.Context, id string) (Record, error) {
	rows, err := m.CheckMany(ctx, []string{id})
	if err != nil {
		return Record{}, err
	}
	return rows[0], nil
}

func (m *Manager) CheckMany(ctx context.Context, ids []string) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(ids) == 0 || len(ids) > maxRecords {
		return nil, errors.New("请选择需要检查的记录")
	}
	seen := map[string]bool{}
	indices := make([]int, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		i := m.index(id)
		if i < 0 {
			return nil, errors.New("链接记录不存在")
		}
		indices = append(indices, i)
	}
	rows := make([]Record, 0, len(indices))
	for _, i := range indices {
		if err := ctx.Err(); err != nil {
			return rows, errors.Join(err, m.save())
		}
		if m.records[i].Status == "removed" {
			rows = append(rows, m.records[i])
			continue
		}
		m.records[i] = checked(m.records[i])
		rows = append(rows, m.records[i])
	}
	return rows, m.save()
}

func (m *Manager) Remove(ctx context.Context, id string, checkedAt ...string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.index(id)
	if i < 0 {
		return Record{}, errors.New("链接记录不存在")
	}
	r := m.records[i]
	if len(checkedAt) > 0 && r.CheckedAt != checkedAt[0] {
		return r, errors.New("核查结果已变化，请重新检查并确认移除")
	}
	// Deletion is bound to the latest displayed inspection, while the original
	// target remains intact for change detection. Uninspected retargeting fails.
	expectedTarget, expectedRelative := r.TargetText, r.Relative
	if r.ActualTarget != "" {
		expectedTarget = r.ActualTarget
		if r.Kind != "shortcut" {
			expectedRelative = r.ActualRelative
		}
	}
	check := func() error {
		f, err := openState(r.LinkPath, false, false)
		if err != nil {
			return err
		}
		defer f.close()
		if !sameObserved(f.identity, r.EntryIdentity) {
			return errors.New("入口已被替换，拒绝移除")
		}
		if r.Kind == "hardlink" || r.Kind == "shortcut" {
			if f.directory || f.reparse {
				return errors.New("入口类型已变化，拒绝移除")
			}
			if r.Kind == "shortcut" {
				target, err := shortcutTarget(r.LinkPath)
				if err != nil {
					return err
				}
				if !strings.EqualFold(filepath.Clean(target), filepath.Clean(expectedTarget)) {
					return errors.New("快捷方式目标已变化，拒绝移除")
				}
			}
			return nil
		}
		if !f.reparse {
			return errors.New("入口已变成普通项目，拒绝移除")
		}
		tag, target, relative, err := readReparse(f)
		if err != nil {
			return err
		}
		want := tagSymlink
		if r.Kind == "junction" {
			want = tagJunction
		}
		if tag != want || relative != expectedRelative || !strings.EqualFold(filepath.Clean(target), filepath.Clean(expectedTarget)) {
			return errors.New("链接指向已变化，拒绝移除")
		}
		return nil
	}
	if err := check(); err != nil {
		return r, err
	}
	var err error
	if r.Kind == "hardlink" || r.Kind == "shortcut" {
		_, err = recycle.CheckedFile(ctx, r.LinkPath, check)
	} else {
		_, err = recycle.CheckedLink(ctx, r.LinkPath, check)
	}
	if err != nil {
		return r, err
	}
	r.Status, r.Reason, r.Accessible = "removed", "recycled", false
	r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Detail = ""
	m.records[i] = r
	return r, m.save()
}

// ForgetInvalid rechecks history before one atomic history-only write. Offline
// and unverified objects remain recorded; no filesystem entry is removed.
func (m *Manager) ForgetInvalid(ctx context.Context) ([]Record, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.records
	kept := make([]Record, 0, len(before))
	removed := 0
	for _, record := range before {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if record.Status != "removed" {
			record = checked(record)
		}
		switch record.Status {
		case "broken", "replaced", "target-changed", "source-changed", "unlinked", "removed":
			removed++
		default:
			kept = append(kept, record)
		}
	}
	m.records = kept
	if err := m.save(); err != nil {
		m.records = before
		return nil, 0, err
	}
	return append([]Record{}, kept...), removed, nil
}

func (m *Manager) Forget(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.index(id)
	if i < 0 {
		return errors.New("链接记录不存在")
	}
	before := append([]Record{}, m.records...)
	m.records = append(m.records[:i], m.records[i+1:]...)
	if err := m.save(); err != nil {
		m.records = before
		return err
	}
	return nil
}
func (m *Manager) index(id string) int {
	for i, r := range m.records {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func (m *Manager) save() error {
	data, err := json.MarshalIndent(struct {
		Version int      `json:"version"`
		Records []Record `json:"records"`
	}{1, m.records}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(m.directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(m.directory, "history.json")
	if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
		return errors.New("历史文件不是普通文件")
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	f, err := os.CreateTemp(m.directory, ".history-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
