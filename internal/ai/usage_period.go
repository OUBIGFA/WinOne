package ai

import (
	"errors"
	"strings"
	"time"
)

// UsagePeriodRange validates the shared UI/Host period contract. Explicit
// ranges follow Magpie's Period: both local dates are included, with an
// exclusive midnight after the last day; an unset end means "through now".
func UsagePeriodRange(period string, now time.Time) (since, until time.Time, err error) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch period {
	case "today":
		return day, time.Time{}, nil
	case "7d":
		return day.AddDate(0, 0, -6), time.Time{}, nil
	case "30d":
		return day.AddDate(0, 0, -29), time.Time{}, nil
	case "all":
		return time.Time{}, time.Time{}, nil
	}
	from, to, ok := strings.Cut(period, "..")
	if ok && len(from) == 10 && len(to) == 10 {
		first, a := time.ParseInLocation(time.DateOnly, from, now.Location())
		last, b := time.ParseInLocation(time.DateOnly, to, now.Location())
		if a == nil && b == nil && !last.Before(first) {
			return first, last.AddDate(0, 0, 1), nil
		}
	}
	return time.Time{}, time.Time{}, errors.New("用量时间范围无效")
}
