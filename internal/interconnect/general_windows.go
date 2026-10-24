package interconnect

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/configjson"
	"winone/internal/settings"
)

type General struct {
	Notify      bool   `json:"notify"`
	Clipboard   bool   `json:"clipboard"`
	DownloadDir string `json:"downloadDir"`
}

func (m *Manager) General() General { m.mu.Lock(); defer m.mu.Unlock(); return m.general }
func (m *Manager) SaveGeneral(value General) error {
	if !filepath.IsAbs(value.DownloadDir) {
		return errors.New("请选择有效的接收目录")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err = settings.WriteAtomic(filepath.Join(filepath.Dir(m.data), "general.json"), data); err != nil {
		return err
	}
	m.general = value
	return nil
}
func (m *Manager) readGeneral() error {
	m.general = General{Notify: true, DownloadDir: filepath.Join(filepath.Dir(m.data), "received")}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(m.data), "general.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return configjson.DecodeObject(data, &m.general)
}

// Notification delivery belongs to Host's existing tray service. File names,
// private paths, codes and receipt tokens are never placed in desktop notices.
func (m *Manager) SetNotifier(notify func(string, string) error) {
	m.transfers.notify = func(mode, status string) error {
		if !m.General().Notify {
			return nil
		}
		return notify(mode, status)
	}
}
