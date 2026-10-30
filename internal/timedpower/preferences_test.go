package timedpower

import (
	"testing"
	"time"
)

func TestTimeOfDaySecondsAndLegacyMinutes(t *testing.T) {
	now := time.Date(2026, 10, 26, 21, 15, 20, 0, time.UTC)
	for _, test := range []struct {
		clock string
		mode  string
		want  time.Time
	}{
		{"21:15:37", "time", time.Date(2026, 10, 26, 21, 15, 37, 0, time.UTC)},
		{"21:15:37", "weekly", time.Date(2026, 10, 26, 21, 15, 37, 0, time.UTC)},
		{"21:15:20", "time", time.Date(2026, 10, 27, 21, 15, 20, 0, time.UTC)},
		{"21:15", "time", time.Date(2026, 10, 27, 21, 15, 0, 0, time.UTC)},
		{"00:00:01", "time", time.Date(2026, 10, 27, 0, 0, 1, 0, time.UTC)},
	} {
		p := Defaults()
		p.Mode = test.mode
		p.TimeOfDay = test.clock
		got, err := p.Next(now)
		if err != nil || !got.Equal(test.want) {
			t.Fatal(test, got, err)
		}
	}
	for _, clock := range []string{"24:00:00", "23:60:00", "23:59:60", "1:15:30", "12:30:00.5", "12:30:00,5", "12:30:", "12:30:000", "-1:00:00"} {
		p := Defaults()
		p.TimeOfDay = clock
		if p.Validate() == nil {
			t.Fatal("invalid clock accepted", clock)
		}
	}
}
