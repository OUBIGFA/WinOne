package worker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/modules/wintray/internal/config"
)

// transaction is persisted before changing either the configuration or Windows startup state.
type transaction struct {
	Version       int             `json:"version"`
	Before        config.Settings `json:"before"`
	After         config.Settings `json:"after"`
	Committed     bool            `json:"committed"`
	BeforeExisted bool            `json:"beforeExisted"`
}

func (m *Manager) journalPath() string { return m.store.Path() + ".transaction.json" }

func (m *Manager) loadTransaction(initial config.Settings) {
	data, err := os.ReadFile(m.journalPath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		m.recoveryError = "无法读取恢复记录：" + err.Error()
		return
	}
	var entry transaction
	if json.Unmarshal(data, &entry) != nil || entry.Version != 1 || Validate(entry.Before) != nil || Validate(entry.After) != nil {
		m.recoveryError = "恢复记录损坏，请保留文件并检查备份"
		return
	}
	m.pending = &entry
	if revision(initial) != revision(entry.Before) && revision(initial) != revision(entry.After) {
		m.recoveryError = "配置与恢复记录不一致，请先检查文件"
		return
	}
	if entry.Committed {
		if revision(initial) != revision(entry.After) {
			m.recoveryError = "已提交配置与恢复记录不一致"
			return
		}
		if err := m.clearTransaction(); err != nil {
			m.recoveryError = "恢复记录清理失败：" + err.Error()
		}
		return
	}
	m.current = clone(entry.Before)
	m.recoveryError = "上次保存未完成，请先恢复设置"
}

func (m *Manager) writeTransaction(entry transaction) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	path := m.journalPath()
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".transaction-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return replaceJournal(temporary, path)
}

func (m *Manager) clearTransaction() error {
	if err := os.Remove(m.journalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		m.recoveryError = "恢复记录清理失败：" + err.Error()
		return err
	}
	m.pending = nil
	m.recoveryError = ""
	return nil
}
