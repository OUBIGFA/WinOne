package config

import (
	"errors"
	"path/filepath"
	"strings"
	"winone/modules/wintray/internal/shellapp"
)

// Validate rejects invalid input instead of silently changing the user's requested behavior.
func Validate(s Settings) error {
	if s.SchemaVersion != 3 && s.SchemaVersion != 4 {
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
		packaged := shellapp.IsPath(a.ExePath)
		if strings.TrimSpace(a.Name) == "" || (!packaged && !filepath.IsAbs(a.ExePath)) {
			return errors.New("请填写程序名称和绝对路径")
		}
		ext := strings.ToLower(filepath.Ext(a.ExePath))
		if !packaged && ext != ".exe" && ext != ".bat" && ext != ".cmd" && ext != ".ps1" && ext != ".py" && ext != ".pyw" && ext != ".ahk" {
			return errors.New("不支持的程序类型")
		}
		if !packaged && ext != ".exe" && (a.LaunchViaLogonTask || a.TrayBehavior.AutoMinimizeAndHideOnLaunch || a.CollectTrayIcon) {
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

// PreparePreferences uses the upstream store's migration and mode projection
// for both old and current documents. Validate first: normal saves must never
// clamp invalid input or silently select a different mode.
func PreparePreferences(s Settings) (Settings, error) {
	if err := Validate(s); err != nil {
		return s, err
	}
	return migrate(s), nil
}
