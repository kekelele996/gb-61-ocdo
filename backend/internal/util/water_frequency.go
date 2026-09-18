package util

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Watering frequency parsing. Species store water_frequency as free text such
// as "每周2次" or "每月1次"; the garden entry plan converts it into the first
// watering date. Unrecognized text yields ok=false so callers can show
// "待设置" without blocking the garden entry.

// waterPattern matches 每<num><unit><times>次 with optional parts, e.g.
// "每天", "每周2次", "每两周1次", "每3天1次", "每月2次".
var waterPattern = regexp.MustCompile(`^每([0-9]+|[一二两三四五六七八九十]+)?(天|日|星期|周|月)([0-9]+|[一二两三四五六七八九十]+)?次?$`)

// ParseWaterInterval converts a water frequency description into a watering
// interval. Whole-month intervals are returned in months so the caller can
// align the first watering to the entry day-of-month; anything else is days.
func ParseWaterInterval(freq string) (days, months int, ok bool) {
	s := strings.NewReplacer(" ", "", "　", "", "，", "", ",", "").Replace(strings.TrimSpace(freq))
	if s == "" {
		return 0, 0, false
	}
	m := waterPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	period := parseCount(m[1])
	times := parseCount(m[3])
	if period <= 0 || times <= 0 {
		return 0, 0, false
	}
	switch m[2] {
	case "天", "日":
		days = period
	case "周", "星期":
		days = 7 * period
	case "月":
		if times == 1 {
			return 0, period, true
		}
		days = 30 * period
	default:
		return 0, 0, false
	}
	d := roundDiv(days, times)
	if d < 1 {
		d = 1
	}
	return d, 0, true
}

// FirstWaterDate computes the first watering date after base for the given
// frequency. Monthly frequencies align to the same day-of-month as base
// (月频按当日对齐); other frequencies add whole days.
func FirstWaterDate(base time.Time, freq string) (time.Time, bool) {
	days, months, ok := ParseWaterInterval(freq)
	if !ok {
		return time.Time{}, false
	}
	day := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location())
	if months > 0 {
		return day.AddDate(0, months, 0), true
	}
	return day.AddDate(0, 0, days), true
}

// WaterFrequencyTag maps a frequency description to the reminder frequency
// tag used by reminder displays (daily/weekly/monthly), or "" when unknown.
func WaterFrequencyTag(freq string) string {
	s := strings.NewReplacer(" ", "", "　", "", "，", "", ",", "").Replace(strings.TrimSpace(freq))
	m := waterPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	switch m[2] {
	case "天", "日":
		if parseCount(m[1]) == 1 {
			return "daily"
		}
	case "周", "星期":
		return "weekly"
	case "月":
		return "monthly"
	}
	return ""
}

// parseCount reads an Arabic or simple Chinese numeral; empty means 1.
func parseCount(s string) int {
	if s == "" {
		return 1
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	digits := map[rune]int{'一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	runes := []rune(s)
	switch {
	case len(runes) == 1 && runes[0] == '十':
		return 10
	case len(runes) == 1:
		return digits[runes[0]]
	case len(runes) == 2 && runes[0] == '十':
		return 10 + digits[runes[1]]
	case len(runes) == 2 && runes[1] == '十':
		return digits[runes[0]] * 10
	case len(runes) == 3 && runes[1] == '十':
		return digits[runes[0]]*10 + digits[runes[2]]
	}
	return 0
}

// roundDiv divides total by n rounding half up.
func roundDiv(total, n int) int {
	return (total + n/2) / n
}
