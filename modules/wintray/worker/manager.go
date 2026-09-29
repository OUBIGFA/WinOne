// Package worker adapts WinTray's existing services to the WinOne module contract.
package worker

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"winone/modules/wintray/internal/config"
)

var ErrConflict = errors.New("程序设置已变更，请刷新后重试")

// Effects applies system changes using the upstream restore journals.
type Effects interface {
	Reconcile(config.Settings, bool) error
}

type Snapshot struct {
	Revision        string          `json:"revision"`
	Settings        config.Settings `json:"settings"`
	RecoveryError   string          `json:"recoveryError,omitempty"`
	BackgroundError string          `json:"backgroundError,omitempty"`
}

type Manager struct {
	mu            sync.Mutex
	store         *config.Store
	current       config.Settings
	effects       Effects
	recoveryError string
}

func NewManager(store *config.Store, initial config.Settings, effects Effects) *Manager {
	return &Manager{store: store, current: clone(initial), effects: effects}
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
	return Snapshot{Revision: revision(m.current), Settings: clone(m.current), RecoveryError: m.recoveryError}
}

// Save returns success only after disk and native settings both match the requested state.
// A failed compensation remains visible and blocks subsequent edits until recovery succeeds.
func (m *Manager) Save(expected string, next config.Settings) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryError != "" {
		return Snapshot{}, errors.New(m.recoveryError)
	}
	if expected != revision(m.current) {
		return Snapshot{}, ErrConflict
	}
	if err := Validate(next); err != nil {
		return Snapshot{}, err
	}
	next = clone(next)
	before := clone(m.current)
	if err := m.store.Save(next); err != nil {
		return Snapshot{}, err
	}
	if err := m.effects.Reconcile(next, true); err != nil {
		rollbackErr := errors.Join(m.effects.Reconcile(before, true), m.store.Save(before))
		if rollbackErr != nil {
			m.recoveryError = "恢复失败：" + rollbackErr.Error()
		}
		return Snapshot{}, errors.Join(err, rollbackErr)
	}
	m.current = next
	return Snapshot{Revision: revision(next), Settings: clone(next)}, nil
}

// Validate rejects invalid input instead of silently changing the user's requested behavior.
func Validate(s config.Settings) error {
	if s.SchemaVersion != 3 {
		return errors.New("不支持的 WinTray 配置版本")
	}
	if s.Language != "zh-CN" && s.Language != "en-US" {
		return errors.New("不支持的语言")
	}
	if s.StartupIntervalSeconds < 0 || s.StartupIntervalSeconds > 120 || s.CloseWindowRetrySeconds < 0 || s.CloseWindowRetrySeconds > 120 {
		return errors.New("启动间隔和窗口等待须为 0–120 秒")
	}
	ids := map[string]bool{}
	for _, a := range s.ManagedApps {
		if strings.TrimSpace(a.ID) == "" || ids[a.ID] {
			return errors.New("程序标识为空或重复")
		}
		ids[a.ID] = true
		if strings.TrimSpace(a.Name) == "" || !filepath.IsAbs(a.ExePath) {
			return errors.New("请填写程序名称和绝对路径")
		}
		ext := strings.ToLower(filepath.Ext(a.ExePath))
		if ext != ".exe" && ext != ".bat" && ext != ".cmd" && ext != ".ps1" && ext != ".py" && ext != ".pyw" {
			return errors.New("不支持的程序类型")
		}
		if ext != ".exe" && (a.LaunchViaLogonTask || a.TrayBehavior.AutoMinimizeAndHideOnLaunch || a.CollectTrayIcon) {
			return errors.New("脚本不支持开机即启动、收进托盘或托盘收纳")
		}
		modes := 0
		for _, v := range []bool{a.LaunchViaLogonTask, a.LaunchHiddenInBackground, a.TrayBehavior.AutoMinimizeAndHideOnLaunch} {
			if v {
				modes++
			}
		}
		if modes > 1 {
			return errors.New("启动方式互相冲突")
		}
		if a.TrayBehavior.CloseDelaySeconds < 0 || a.TrayBehavior.CloseDelaySeconds > 600 {
			return errors.New("收起前等待须为 0–600 秒")
		}
		if a.Schedule.StartDelayMinutes < 0 || a.Schedule.StartDelayMinutes > 1440 || a.Schedule.AutoExitMinutes < 0 || a.Schedule.AutoExitMinutes > 1440 {
			return errors.New("时间须为 0–1440 分钟")
		}
		if a.Schedule.FrequencyEnabled && (a.Schedule.FrequencyDays < 1 || a.Schedule.FrequencyDays > 365 || a.Schedule.FrequencyRuns < 1 || a.Schedule.FrequencyRuns > 1000) {
			return errors.New("运行次数限制超出范围")
		}
	}
	return nil
}
