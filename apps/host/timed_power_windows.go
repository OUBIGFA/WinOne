//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"fyne.io/systray"
	"path/filepath"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/timedpower"
)

func timedPowerConfiguration() configuration.ModuleAdapter {
	return configuration.ModuleAdapter{
		ID:   "timed-power",
		Load: func() (json.RawMessage, error) { return json.Marshal(timedpower.Defaults()) },
		Validate: func(raw json.RawMessage) error {
			var p timedpower.Preferences
			if err := configjson.DecodeObject(raw, &p); err != nil {
				return err
			}
			if p.Revision != 0 {
				return errors.New("定时关机修订号须由 Host 管理")
			}
			return p.Validate()
		},
		// Preferences are templates only. Applying/importing must never change a
		// currently armed task, create a timer, or request a Windows power action.
		Apply: func(json.RawMessage, uint64) error { return nil },
	}
}
func timedPowerConfigurationHandler(store *configuration.HostStore, next func(host.Request) host.Response) func(host.Request) host.Response {
	return func(r host.Request) host.Response {
		if r.Method != "settings.get" && r.Method != "settings.save" {
			return next(r)
		}
		fail := func(err error) host.Response {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
		}
		raw, revision, err := store.Module("timed-power")
		if err != nil {
			return fail(err)
		}
		if r.Method == "settings.save" {
			var p timedpower.Preferences
			if err = configjson.DecodeObject(r.Params, &p); err != nil {
				return fail(err)
			}
			expected := p.Revision
			p.Revision = 0
			raw, err = json.Marshal(p)
			if err != nil {
				return fail(err)
			}
			raw, revision, err = store.SaveModule("timed-power", expected, raw)
			if err != nil {
				return fail(err)
			}
		}
		var p timedpower.Preferences
		if err = json.Unmarshal(raw, &p); err != nil {
			return fail(err)
		}
		p.Revision = revision
		raw, err = json.Marshal(p)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	}
}

// Verify the real desktop peer before admitting arming/cancellation. A random
// same-user process must not use an elevated Host as a power-control proxy.
func timedPowerHandler(manager *timedpower.Manager, desktop string) func(host.Request) host.Response {
	return func(r host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "timed_power", Message: err.Error()}}
		}
		if r.Method != "snapshot" {
			peer, err := processidentity.Open(r.PeerPID, desktop)
			if err != nil {
				return fail(err)
			}
			peer.Close()
		}
		var snapshot timedpower.Snapshot
		var err error
		switch r.Method {
		case "snapshot":
			snapshot = manager.Snapshot()
		case "start":
			var request struct {
				Preferences timedpower.Preferences `json:"preferences"`
				Confirmed   bool                   `json:"confirmed"`
			}
			if err = configjson.DecodeObject(r.Params, &request); err != nil {
				return fail(err)
			}
			snapshot, err = manager.Start(request.Preferences, request.Confirmed)
		case "cancel":
			snapshot, err = manager.Cancel()
		default:
			return fail(errors.New("不支持的定时关机操作"))
		}
		if err != nil {
			return fail(err)
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	}
}
func timedPowerNotifier(data string, language func() string) func(string, timedpower.Snapshot) error {
	return func(event string, s timedpower.Snapshot) error {
		var failure error
		if s.Error != "" {
			failure = errors.New(s.Error)
		}
		logErr := recordHostEvent(data, "timed-power."+event, failure)
		title, text := "WinOne · 定时关机", "电源操作已提交给 Windows。"
		action := ""
		if s.Task != nil {
			action = map[string]string{"shutdown": "关机", "restart": "重启", "logoff": "注销", "sleep": "睡眠", "hibernate": "休眠", "lock": "锁定"}[s.Task.Action]
		}
		switch event {
		case "reminder":
			text = fmt.Sprintf("将在 %s 执行%s。可从 WinOne 托盘菜单取消。", s.Deadline.Local().Format("15:04:05"), action)
		case "reminded":
			text = "定时时间已到。仅提醒，未执行电源操作。"
		case "failed":
			text = "电源操作未完成，请打开定时关机查看原因。"
		case "missed":
			text = "已跳过错过的定时任务，不会补执行。"
		}
		if language() == "en-US" {
			title = "WinOne · Shutdown timer"
			text = "Power request submitted to Windows."
			switch event {
			case "reminder":
				text = fmt.Sprintf("%s at %s. Cancel from the WinOne tray menu.", s.Task.Action, s.Deadline.Local().Format("15:04:05"))
			case "reminded":
				text = "Timer finished. Reminder only; no power action was performed."
			case "failed":
				text = "Power action failed. Open Shutdown timer for details."
			case "missed":
				text = "Missed timer skipped. No delayed power action was performed."
			}
		}
		if event == "reminder" && s.Task != nil && s.Task.DryRun {
			if language() == "en-US" {
				text += " (Reminder only)"
			} else {
				text += "（仅提醒）"
			}
		}
		return errors.Join(logErr, systray.ShowMessage(title, text))
	}
}
func timedPowerStatus(manager *timedpower.Manager) host.Module {
	state := "ready"
	if manager.Snapshot().Active {
		state = "running"
	}
	return host.Module{ID: "timed-power", Name: "定时关机", State: state}
}

// Existing tray refresh owns the cadence; the module adds no idle poller.
func updateTimedPowerMenu(item *systray.MenuItem, manager *timedpower.Manager, language string) {
	s := manager.Snapshot()
	title := "取消定时关机"
	if language == "en-US" {
		title = "Cancel shutdown timer"
	}
	if s.Active {
		title += " · " + s.Deadline.Local().Format(time.TimeOnly)
		item.SetTitle(title)
		item.Show()
		if s.State == "executing" {
			item.Disable()
		} else {
			item.Enable()
		}
	} else {
		item.Hide()
	}
}

func timedPowerDesktop(executable string) string {
	return filepath.Join(filepath.Dir(executable), "WinOne.exe")
}
