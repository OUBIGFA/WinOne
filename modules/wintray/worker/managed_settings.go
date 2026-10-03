package worker

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"winone/internal/configjson"
	"winone/modules/wintray/internal/config"
)

type managedProjection struct {
	Revision uint64          `json:"revision"`
	Settings config.Settings `json:"settings"`
}

func readManagedProjection(path string) (managedProjection, error) {
	file, err := os.Open(path)
	if err != nil {
		return managedProjection{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return managedProjection{}, err
	}
	if len(data) > 2<<20 {
		return managedProjection{}, errors.New("程序配置过大")
	}
	var wire struct {
		Revision *uint64         `json:"revision"`
		Settings config.Settings `json:"settings"`
	}
	if err = configjson.DecodeObject(data, &wire); err != nil {
		return managedProjection{}, err
	}
	if wire.Revision == nil {
		return managedProjection{}, errors.New("配置修订号缺失")
	}
	value := managedProjection{Revision: *wire.Revision, Settings: wire.Settings}
	if err = Validate(value.Settings); err != nil {
		return value, err
	}
	return value, nil
}

// The Host must finish legacy preference transactions before switching ownership.
// Native task/tray recovery journals stay with their existing services.
func newManagedManager(store *config.Store, path string, effects Effects) (*Manager, error) {
	if _, err := os.Stat(store.Path() + ".transaction.json"); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("请先恢复旧版程序设置事务，再迁入统一配置")
	}
	value, err := readManagedProjection(path)
	if err != nil {
		return nil, err
	}
	return &Manager{store: store, current: clone(value.Settings), effects: effects, managedPath: path, managedRevision: value.Revision}, nil
}

func (m *Manager) snapshotLocked() Snapshot {
	currentRevision := revision(m.current)
	if m.managedPath != "" {
		currentRevision = strconv.FormatUint(m.managedRevision, 10)
	}
	return Snapshot{Revision: currentRevision, Settings: clone(m.current), RecoveryError: m.recoveryError, ConfigurationManaged: m.managedPath != ""}
}

// ReloadManaged changes system settings without saving a second preference copy
// or launching programs. The Host journals the enclosing configuration transaction.
func (m *Manager) ReloadManaged(allowElevated bool) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.managedPath == "" {
		return m.snapshotLocked(), errors.New("未启用统一配置")
	}
	value, err := readManagedProjection(m.managedPath)
	if err != nil {
		return m.snapshotLocked(), err
	}
	if m.recoveryError == "" && revision(value.Settings) == revision(m.current) {
		m.managedRevision = value.Revision
		return m.snapshotLocked(), nil
	}
	before := clone(m.current)
	if err = m.effects.Reconcile(value.Settings, allowElevated); err != nil {
		rollbackErr := m.effects.Reconcile(before, allowElevated)
		if rollbackErr != nil {
			m.recoveryError = "恢复失败：" + rollbackErr.Error()
		} else {
			m.recoveryError = ""
		}
		return m.snapshotLocked(), errors.Join(err, rollbackErr)
	}
	m.current = clone(value.Settings)
	m.managedRevision = value.Revision
	m.recoveryError = ""
	return m.snapshotLocked(), nil
}

// MergeHistory stays in the worker so it shares the launch reservation lock.
// Launch facts are additive and are retained even if a later preference save fails.
func (m *Manager) MergeHistory(raw json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryError != "" {
		return errors.New(m.recoveryError)
	}
	return config.NewStartupHistory(filepath.Join(filepath.Dir(m.store.Path()), "startup-history.json")).MergeImport(raw)
}
