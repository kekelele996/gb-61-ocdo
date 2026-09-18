package service

import (
	"regexp"
	"time"
)

// Watering recurrence codes derived from a plant's free-form water_frequency.
// They are stored on care reminders' frequency column and mapped to Chinese
// text by util.WateringRecurrenceText.
const (
	WateringIntervalDays = "interval_days" // next = base + N days
	WateringMonthlyByDay = "monthly"       // next keeps the same day-of-month as base
	WateringUnrecognized = ""              // frequency text cannot be converted to a plan
)

var (
	// 每日/每天浇水：提醒按天粒度，统一为次日首次浇水。
	waterDailyRe = regexp.MustCompile(`每(?:日|天)\s*[0-9]+\s*次`)
	// 每两周N次 / 每两周浇N次：14 天内浇水 N 次。
	waterBiweeklyRe = regexp.MustCompile(`每两周\s*([0-9]+)\s*次`)
	// 每周N次 / 每星期N次：7 天内浇水 N 次。
	waterWeeklyRe = regexp.MustCompile(`每(?:周|星期)\s*([0-9]+)\s*次`)
	// 每月N次 / 每个月N次：N>1 时按月内均摊折算间隔天数。
	waterMonthlyRe = regexp.MustCompile(`每(?:个)?月\s*([0-9]+)\s*次`)
)

// WaterSchedule is the parsed watering plan for a plant variety.
type WaterSchedule struct {
	// Recurrence is one of WateringIntervalDays / WateringMonthlyByDay /
	// WateringUnrecognized.
	Recurrence string
	// IntervalDays is set for WateringIntervalDays plans.
	IntervalDays int
	// FrequencyText is the original water_frequency copied from the plant.
	FrequencyText string
}

// ParseWaterSchedule converts a plant's free-form water_frequency text into a
// watering plan. Text like "保持水位" or an empty string yields a schedule with
// Recurrence == WateringUnrecognized; callers must treat that as "待设置" and
// must not block the plant from entering the garden.
func ParseWaterSchedule(frequencyText string) WaterSchedule {
	plan := WaterSchedule{FrequencyText: frequencyText}
	if m := waterBiweeklyRe.FindStringSubmatch(frequencyText); m != nil {
		plan.Recurrence = WateringIntervalDays
		plan.IntervalDays = intervalDays(14, atoiMinOne(m[1]))
		return plan
	}
	if m := waterWeeklyRe.FindStringSubmatch(frequencyText); m != nil {
		plan.Recurrence = WateringIntervalDays
		plan.IntervalDays = intervalDays(7, atoiMinOne(m[1]))
		return plan
	}
	if m := waterMonthlyRe.FindStringSubmatch(frequencyText); m != nil {
		times := atoiMinOne(m[1])
		if times == 1 {
			// 月频按当日对齐：次月同日，AddDate 自动处理月末溢出。
			plan.Recurrence = WateringMonthlyByDay
			return plan
		}
		plan.Recurrence = WateringIntervalDays
		plan.IntervalDays = intervalDays(30, times)
		return plan
	}
	if waterDailyRe.MatchString(frequencyText) {
		// 提醒按天粒度，一日多次也无法在同一天之外再细分，统一为次日首次浇水。
		plan.Recurrence = WateringIntervalDays
		plan.IntervalDays = 1
		return plan
	}
	return plan
}

// FirstWateringDate computes the first watering date for a plant entering the
// garden at base, following the parsed schedule. The zero time is returned for
// unrecognized frequencies (待设置).
func (p WaterSchedule) FirstWateringDate(base time.Time) time.Time {
	base = truncateDay(base)
	switch p.Recurrence {
	case WateringIntervalDays:
		return base.AddDate(0, 0, p.IntervalDays)
	case WateringMonthlyByDay:
		// 月频按当日对齐：保留入圃日的“日”，向后推一个月。
		return base.AddDate(0, 1, 0)
	default:
		return time.Time{}
	}
}

// Recognized reports whether the frequency text could be converted to a plan.
func (p WaterSchedule) Recognized() bool {
	return p.Recurrence != WateringUnrecognized
}

func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// intervalDays distributes period days across that many waterings, floored at
// 1 day so an implausibly high count (e.g. 每周10次) still lands at tomorrow
// rather than the entry day itself.
func intervalDays(period, times int) int {
	days := period / times
	if days < 1 {
		return 1
	}
	return days
}

// atoiMinOne parses a small positive decimal string, falling back to 1.
func atoiMinOne(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			continue
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 {
		return 1
	}
	return n
}
