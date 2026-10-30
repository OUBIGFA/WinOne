// Package timedpower owns one explicitly armed, session-only power schedule.
// Saved preferences never arm a task on startup, import or module enable.
package timedpower

import (
	"errors"
	"fmt"
	"time"
)

type Preferences struct {
	SchemaVersion   int    `json:"schemaVersion"`
	Revision        uint64 `json:"revision"`
	Action          string `json:"action"`
	Mode            string `json:"mode"`
	DelaySeconds    int    `json:"delaySeconds"`
	TimeOfDay       string `json:"timeOfDay"`
	Weekdays        []int  `json:"weekdays"`
	ReminderSeconds int    `json:"reminderSeconds"`
	Force           bool   `json:"force"`
	DryRun          bool   `json:"dryRun"`
}

func Defaults() Preferences {
	return Preferences{SchemaVersion: 1, Action: "shutdown", Mode: "countdown", DelaySeconds: 3600, TimeOfDay: "23:00:00", Weekdays: []int{1, 2, 3, 4, 5}, ReminderSeconds: 60, Force: true}
}
func (p Preferences) Clone() Preferences { p.Weekdays = append([]int{}, p.Weekdays...); return p }
func (p Preferences) Validate() error {
	if p.SchemaVersion != 1 {
		return errors.New("不支持的定时关机配置版本")
	}
	switch p.Action {
	case "shutdown", "restart", "logoff", "sleep", "hibernate", "lock":
	default:
		return errors.New("请选择有效的电源操作")
	}
	switch p.Mode {
	case "countdown", "time", "weekly":
	default:
		return errors.New("请选择有效的定时方式")
	}
	if p.DelaySeconds < 1 || p.DelaySeconds > 604800 {
		return errors.New("倒计时须为 1 秒至 7 天")
	}
	if _, err := parseTimeOfDay(p.TimeOfDay); err != nil {
		return err
	}
	if p.ReminderSeconds < 0 || p.ReminderSeconds > 3600 {
		return errors.New("提前提醒须为 0–3600 秒")
	}
	seen := map[int]bool{}
	for _, day := range p.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return errors.New("星期选择无效或重复")
		}
		seen[day] = true
	}
	if p.Mode == "weekly" && len(seen) == 0 {
		return errors.New("请至少选择一天")
	}
	return nil
}

// Accept previous minute-only templates without losing their meaning. New UI
// writes seconds explicitly; fractional seconds and normalized overflow are not valid.
func parseTimeOfDay(value string) (time.Time, error) {
	layout := "15:04:05"
	if len(value) == 5 {
		layout = "15:04"
	}
	clock, err := time.Parse(layout, value)
	if err != nil || clock.Format(layout) != value {
		return time.Time{}, errors.New("执行时间须为有效的时、分、秒（24 小时制）")
	}
	return clock, nil
}

// Next uses local calendar days, not 24-hour increments: DST must not move the
// chosen time. Nonexistent spring-forward times are skipped, never normalized.
func (p Preferences) Next(now time.Time) (time.Time, error) {
	if err := p.Validate(); err != nil {
		return time.Time{}, err
	}
	if p.Mode == "countdown" {
		return now.Add(time.Duration(p.DelaySeconds) * time.Second), nil
	}
	clock, _ := parseTimeOfDay(p.TimeOfDay)
	// A weekly occurrence in the DST gap may be skipped, so search two weeks.
	for offset := 0; offset <= 14; offset++ {
		day := now.AddDate(0, 0, offset)
		candidate := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, now.Location())
		if candidate.Hour() != clock.Hour() || candidate.Minute() != clock.Minute() || candidate.Day() != day.Day() || !candidate.After(now) {
			continue
		}
		if p.Mode == "time" {
			return candidate, nil
		}
		for _, weekday := range p.Weekdays {
			if int(candidate.Weekday()) == weekday {
				return candidate, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("无法计算下一次执行时间")
}
