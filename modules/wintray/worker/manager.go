// Package worker adapts WinTray's existing services to the WinOne module contract.
package worker

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"winone/modules/wintray/internal/config"
)

var ErrConflict = errors.New("程序设置已变更，请刷新后重试")

// Effects applies system changes using the upstream restore journals.
type Effects interface {
	Reconcile(config.Settings, bool) error
}

type Snapshot struct {
	Revision             string          `json:"revision"`
	Settings             config.Settings `json:"settings"`
	RecoveryError        string          `json:"recoveryError"`
	BackgroundError      string          `json:"backgroundError"`
	StartupError         string          `json:"startupError"`
	ProcessID            uint32          `json:"processId"`
	ConfigurationManaged bool            `json:"configurationManaged"`
	InitializationState  string          `json:"initializationState"`
	InitializationError  string          `json:"initializationError"`
}

type Manager struct {
	mu              sync.Mutex
	store           *config.Store
	current         config.Settings
	effects         Effects
	recoveryError   string
	pending         *transaction
	managedPath     string
	managedRevision uint64
}

func NewManager(store *config.Store, initial config.Settings, effects Effects) *Manager {
	m := &Manager{store: store, current: clone(initial), effects: effects}
	m.loadTransaction(initial)
	return m
}

func clone(s config.Settings) config.Settings {
	s.ManagedApps = append([]config.ManagedAppEntry{}, s.ManagedApps...)
	return s
}
func revision(s config.Settings) string {
	b, _ := json.Marshal(s)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

// Activate serializes startup reconciliation with subsequent preference reloads,
// so a delayed startup job cannot reapply an older settings snapshot.
func (m *Manager) Activate(allowElevated bool) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryError != "" {
		return m.snapshotLocked(), errors.New(m.recoveryError)
	}
	if err := m.effects.Reconcile(clone(m.current), allowElevated); err != nil {
		return m.snapshotLocked(), err
	}
	return m.snapshotLocked(), nil
}

// Save returns success only after disk and native settings both match the requested state.
// A failed compensation remains visible and blocks subsequent edits until recovery succeeds.
func (m *Manager) Save(expected string, next config.Settings) (Snapshot, error) {
	return m.save(expected, next, nil)
}

// SaveImported merges launch facts before applying settings; retries are idempotent.
func (m *Manager) SaveImported(expected string, next config.Settings, history json.RawMessage) (Snapshot, error) {
	return m.save(expected, next, history)
}

func (m *Manager) save(expected string, next config.Settings, history json.RawMessage) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.managedPath != "" {
		return m.snapshotLocked(), errors.New("请通过 WinOne 保存程序设置")
	}
	if m.recoveryError != "" {
		return Snapshot{}, errors.New(m.recoveryError)
	}
	if expected != revision(m.current) {
		return Snapshot{}, ErrConflict
	}
	if err := Validate(next); err != nil {
		return Snapshot{}, err
	}
	if len(history) > 0 {
		if err := config.NewStartupHistory(filepath.Join(filepath.Dir(m.store.Path()), "startup-history.json")).MergeImport(history); err != nil {
			return Snapshot{}, err
		}
	}
	next = clone(next)
	before := clone(m.current)
	entry := transaction{Version: 1, Before: before, After: next}
	if _, err := os.Stat(m.store.Path()); err == nil {
		entry.BeforeExisted = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, err
	}
	if err := m.writeTransaction(entry); err != nil {
		return Snapshot{}, err
	}
	m.pending = &entry
	if err := m.store.Save(next); err != nil {
		return Snapshot{}, errors.Join(err, m.clearTransaction())
	}
	if err := m.effects.Reconcile(next, true); err != nil {
		rollbackErr := m.rollback(true)
		return Snapshot{}, errors.Join(err, rollbackErr)
	}
	entry.Committed = true
	if err := m.writeTransaction(entry); err != nil {
		return Snapshot{}, errors.Join(err, m.rollback(true))
	}
	m.pending = &entry
	m.current = next
	if err := m.clearTransaction(); err != nil {
		m.recoveryError = "设置已保存，恢复记录清理失败：" + err.Error()
	}
	return Snapshot{Revision: revision(next), Settings: clone(next), RecoveryError: m.recoveryError}, nil
}

// Recover restores the last committed settings and their native effects after an interrupted save.
func (m *Manager) Recover(allowElevated bool) (Snapshot, error) {
	if m.managedPath != "" {
		return m.ReloadManaged(allowElevated)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		if m.recoveryError != "" {
			return Snapshot{}, errors.New(m.recoveryError)
		}
		return Snapshot{Revision: revision(m.current), Settings: clone(m.current)}, nil
	}
	disk, err := m.store.LoadWithError()
	if err != nil {
		return Snapshot{}, err
	}
	if _, statErr := os.Stat(m.store.Path()); errors.Is(statErr, os.ErrNotExist) && !m.pending.BeforeExisted {
		disk = clone(m.pending.Before)
	}
	if revision(disk) != revision(m.pending.Before) && revision(disk) != revision(m.pending.After) {
		return Snapshot{}, errors.New("配置已被外部修改，请先检查恢复记录")
	}
	if m.pending.Committed {
		if revision(disk) != revision(m.pending.After) {
			return Snapshot{}, errors.New("已提交配置与恢复记录不一致")
		}
		m.current = clone(m.pending.After)
		if err = m.effects.Reconcile(m.current, allowElevated); err != nil {
			return Snapshot{}, err
		}
		err = m.clearTransaction()
	} else {
		err = m.rollback(allowElevated)
	}
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Revision: revision(m.current), Settings: clone(m.current)}, nil
}

func (m *Manager) rollback(allowElevated bool) error {
	before := clone(m.pending.Before)
	err := errors.Join(m.effects.Reconcile(before, allowElevated), m.store.Save(before))
	if err == nil {
		m.current = before
		err = m.clearTransaction()
	}
	if err != nil {
		m.recoveryError = "恢复失败：" + err.Error()
	}
	return err
}

// Validate shares the configuration contract with Host migration.
func Validate(s config.Settings) error { return config.Validate(s) }
