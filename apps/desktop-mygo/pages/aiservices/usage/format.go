package usage

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	cldr "github.com/egoist/mygo/i18n"

	"winone/desktopmygo/i18n"
)

// Numbers and dates are written as the React window's Intl formatters
// wrote them, in the window's language (zh-CN or en-US): MyGo's i18n
// package follows Intl with the same CLDR data. Intl's compact notation
// and currency style, which it lacks, are written here for the two
// languages, as Chromium writes them.

func lang() string  { return i18n.Locale() }
func chinese() bool { return i18n.Locale() == i18n.Chinese }

// tr is i18n.T: WinOne's own words, by the React window's keys.
func tr(key string, args ...any) string { return i18n.T(key, args...) }
func zero(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return v
}

// num is Intl.NumberFormat(language).format(value || 0).
func num(v float64) string { return cldr.FormatNumber(lang(), zero(v), cldr.NumberOptions{}) }

// usd is Intl.NumberFormat(language, {style: "currency", currency: "USD",
// maximumFractionDigits}).format(value || 0): "$0.02" in English and
// "US$0.02" in Chinese, two fraction digits at least.
func usd(v float64, maxFraction int) string {
	v = zero(v)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	symbol := "$"
	if chinese() {
		symbol = "US$"
	}
	return sign + symbol + cldr.FormatNumber(lang(), v, cldr.NumberOptions{MinFractionDigits: 2, MaxFractionDigits: maxFraction})
}

// compactUnit is one step of Intl's short compact notation.
type compactUnit struct {
	scale  float64
	suffix string
}

var (
	compactEnglish = []compactUnit{{1e3, "K"}, {1e6, "M"}, {1e9, "B"}, {1e12, "T"}}
	compactChinese = []compactUnit{{1e4, "万"}, {1e8, "亿"}, {1e12, "万亿"}}
)

// fmtN (Magpie's name) is Intl.NumberFormat(language, {notation: "compact",
// maximumFractionDigits: 1}).format(value || 0): 1.2K, 12.3M in English,
// 1234, 1.2万, 1.2亿 in Chinese. A value that rounds up to the next unit
// takes it ("1M", not "1000K"), as Intl does.
func fmtN(v float64) string {
	v = zero(v)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	units := compactEnglish
	if chinese() {
		units = compactChinese
	}
	one := func(x float64) (float64, string) {
		text := cldr.FormatNumber(i18n.English, x, cldr.NumberOptions{MaxFractionDigits: 1, NoGrouping: true})
		rounded, _ := strconv.ParseFloat(text, 64)
		return rounded, text
	}
	unit := -1
	for i, u := range units {
		if v >= u.scale {
			unit = i
		}
	}
	for {
		scale, suffix := 1.0, ""
		if unit >= 0 {
			scale, suffix = units[unit].scale, units[unit].suffix
		}
		rounded, text := one(v / scale)
		if unit+1 < len(units) && rounded*scale >= units[unit+1].scale {
			unit++
			continue
		}
		return sign + text + suffix
	}
}

// toFixed is JavaScript's Number.prototype.toFixed: the exact binary value
// rounded to digits, a tie going to the larger, so 0.125 is "0.13".
func toFixed(v float64, digits int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	r := new(big.Rat).SetFloat64(v)
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)))
	r.Add(r, big.NewRat(1, 2))
	s := new(big.Int).Quo(r.Num(), r.Denom()).String()
	if digits > 0 {
		for len(s) <= digits {
			s = "0" + s
		}
		s = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	return sign + s
}

// round is JavaScript's Math.round: a half goes up.
func round(v float64) float64 {
	r := math.Floor(v)
	if v-r >= 0.5 {
		r++
	}
	return r
}

// parseTime reads a Host time (RFC 3339) in this computer's zone, where
// the browser showed it.
func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.Local(), true
}

// formatDate writes t in the window's language by a CLDR skeleton, as
// Intl's toLocaleString with the matching options did.
func formatDate(t time.Time, skeleton string) string {
	return cldr.FormatDate(lang(), t, cldr.DateOptions{Skeleton: skeleton})
}

// dateTime is new Date(s).toLocaleString(language): "2026/10/26 08:05:09",
// "10/26/2026, 8:05:09 AM". A time that cannot be read shows as it came.
func dateTime(s string) string {
	t, ok := parseTime(s)
	if !ok {
		return s
	}
	return formatDate(t, "yMdjms")
}

// localDay is a YYYY-MM-DD date at midnight in this computer's zone, as
// new Date(y, m-1, d) made it.
func localDay(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(time.DateOnly, s, time.Local)
	return t, err == nil
}

// isoDay writes a local day as YYYY-MM-DD.
func isoDay(t time.Time) string { return t.Format(time.DateOnly) }

// magpieText is a Magpie message in the window's language: its Chinese
// from table in Chinese, the English key itself otherwise, with each
// "{name}" replaced by its value (pairs of name and value), as React's
// usageMessage did.
func magpieText(table map[string]string, key string, values ...any) string {
	text := key
	if chinese() {
		if zh, ok := table[key]; ok {
			text = zh
		}
	}
	for i := 0; i+1 < len(values); i += 2 {
		text = strings.ReplaceAll(text, "{"+toString(values[i])+"}", toString(values[i+1]))
	}
	return text
}

func toString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
