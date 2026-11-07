package automations

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

// cronForm ports Cindy cronCodexPreset's reversible fixed-slot presets.
// Unrecognised expressions stay custom, byte-for-byte until explicitly edited;
// relative intervals remain Definition.IntervalMs, never guessed from cron.
type cronForm struct {
	Mode                                            string
	Hour, Minute, Weekday, MonthDay, Hours, Minutes float64
}

func readCron(expr string) *cronForm {
	f := &cronForm{Mode: "custom", Hour: 9, MonthDay: 1, Weekday: 1, Hours: 2, Minutes: 5}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return f
	}
	if expr == "* * * * *" {
		f.Mode = "minute"
		return f
	}
	if expr == "0 * * * *" {
		f.Mode = "hourly"
		return f
	}
	if parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" && strings.HasPrefix(parts[0], "*/") {
		if n, err := strconv.Atoi(strings.TrimPrefix(parts[0], "*/")); err == nil && n >= 1 && n <= 59 {
			f.Mode = "minuteStep"
			f.Minutes = float64(n)
			return f
		}
	}
	if parts[0] == "0" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" && strings.HasPrefix(parts[1], "*/") {
		if n, err := strconv.Atoi(strings.TrimPrefix(parts[1], "*/")); err == nil && n >= 1 && n <= 23 {
			f.Mode = "hourStep"
			f.Hours = float64(n)
			return f
		}
	}
	minute, e1 := strconv.Atoi(parts[0])
	hour, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || minute < 0 || minute > 59 || hour < 0 || hour > 23 || parts[3] != "*" {
		return f
	}
	f.Minute = float64(minute)
	f.Hour = float64(hour)
	if parts[2] == "*" {
		switch parts[4] {
		case "*":
			f.Mode = "daily"
		case "1-5":
			f.Mode = "weekdays"
		case "0,6", "6,0":
			f.Mode = "weekends"
		default:
			if day, err := strconv.Atoi(parts[4]); err == nil && day >= 0 && day <= 6 {
				f.Mode = "weekly"
				f.Weekday = float64(day)
			}
		}
	} else if parts[4] == "*" {
		if day, err := strconv.Atoi(parts[2]); err == nil && day >= 1 && day <= 31 {
			f.Mode = "monthly"
			f.MonthDay = float64(day)
		}
	}
	return f
}
func (f *cronForm) expression() string {
	clock := fmt.Sprintf("%d %d", int(f.Minute), int(f.Hour))
	switch f.Mode {
	case "minute":
		return "* * * * *"
	case "hourly":
		return "0 * * * *"
	case "daily":
		return clock + " * * *"
	case "weekdays":
		return clock + " * * 1-5"
	case "weekends":
		return clock + " * * 0,6"
	case "weekly":
		return fmt.Sprintf("%s * * %d", clock, int(f.Weekday))
	case "monthly":
		return fmt.Sprintf("%s %d * *", clock, int(f.MonthDay))
	case "hourStep":
		return fmt.Sprintf("0 */%d * * *", int(f.Hours))
	case "minuteStep":
		if f.Minutes <= 1 {
			return "* * * * *"
		}
		return fmt.Sprintf("*/%d * * * *", int(f.Minutes))
	}
	return ""
}

// CronEditor is the same reversible preset editor used by both ordinary
// automations and teammate routines. Unknown expressions stay verbatim.
type CronEditor struct{ state *cronForm }

func NewCronEditor(expression string) *CronEditor { return &CronEditor{state: readCron(expression)} }
func (e *CronEditor) View(c *ui.Context, expression *string) bool {
	return renderCron(c, e.state, expression)
}

func (p *Page) cronFields(c *ui.Context, d *Draft) {
	if d.cron == nil {
		d.cron = readCron(d.Value.CronExpr)
	}
	renderCron(c, d.cron, &d.Value.CronExpr)
}
func renderCron(c *ui.Context, f *cronForm, expression *string) bool {
	before := *expression
	changed := choice(c, tr("frequency"), &f.Mode, option("minute", "everyMinute"), option("hourly", "hourly"), option("daily", "daily"), option("weekly", "weekly"), option("weekdays", "weekdays"), option("weekends", "weekends"), option("monthly", "monthly"), option("minuteStep", "minuteStep"), option("hourStep", "hourStep"), option("custom", "customCron"))
	switch f.Mode {
	case "daily", "weekly", "weekdays", "weekends", "monthly":
		tm := time.Date(2000, 1, 1, int(f.Hour), int(f.Minute), 0, 0, time.UTC)
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if ui.TimeInput(c, &tm).Label(tr("time")).Changed() {
				f.Hour = float64(tm.Hour())
				f.Minute = float64(tm.Minute())
				changed = true
			}
			if f.Mode == "weekly" {
				day := strconv.Itoa(int(f.Weekday))
				options := []kit.SelectOption{}
				for i := 0; i < 7; i++ {
					options = append(options, option(strconv.Itoa(i), fmt.Sprintf("weekday%d", i)))
				}
				if choice(c, tr("weekday"), &day, options...) {
					n, _ := strconv.Atoi(day)
					f.Weekday = float64(n)
					changed = true
				}
			}
			if f.Mode == "monthly" {
				kit.Label(c, tr("monthDay"))
				if ui.NumberInput(c, &f.MonthDay, 1, 31, 1).Label(tr("monthDay")).Changed() {
					changed = true
				}
			}
		})
	case "minuteStep":
		kit.Label(c, tr("minuteStep"))
		if ui.NumberInput(c, &f.Minutes, 1, 59, 1).Label(tr("minuteStep")).Changed() {
			changed = true
		}
	case "hourStep":
		kit.Label(c, tr("hourStep"))
		if ui.NumberInput(c, &f.Hours, 1, 23, 1).Label(tr("hourStep")).Changed() {
			changed = true
		}
	case "custom":
		field(c, tr("cron"), expression, false)
	}
	if changed && f.Mode != "custom" {
		*expression = f.expression()
	}
	if f.Mode != "custom" {
		kit.Help(c, *expression)
	}
	return before != *expression
}
