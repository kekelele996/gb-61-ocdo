package service

import (
	"testing"
	"time"
)

func TestParseWaterSchedule(t *testing.T) {
	base := time.Date(2026, 9, 18, 10, 30, 0, 0, time.Local) // 入圃日（含时分）
	cases := []struct {
		name       string
		input      string
		recognized bool
		recurrence string
		interval   int
		firstDate  string // YYYY-MM-DD, "" => 待设置（零值）
		encoded    string
	}{
		{
			name:       "每周1次",
			input:      "每周1次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   7,
			firstDate:  "2026-09-25",
			encoded:    "water:interval_days:7",
		},
		{
			name:       "每周2次整除向下取整",
			input:      "每周2次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   3, // 7/2 = 3
			firstDate:  "2026-09-21",
			encoded:    "water:interval_days:3",
		},
		{
			name:       "每周3次",
			input:      "每周3次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   2,
			firstDate:  "2026-09-20",
			encoded:    "water:interval_days:2",
		},
		{
			name:       "每月1次月频按当日对齐",
			input:      "每月1次",
			recognized: true,
			recurrence: WateringMonthlyByDay,
			interval:   0,
			firstDate:  "2026-10-18",
			encoded:    "water:monthly:0",
		},
		{
			name:       "每月2次月内均摊",
			input:      "每月2次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   15,
			firstDate:  "2026-10-03",
			encoded:    "water:interval_days:15",
		},
		{
			name:       "每两周1次",
			input:      "每两周1次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   14,
			firstDate:  "2026-10-02",
			encoded:    "water:interval_days:14",
		},
		{
			name:       "每天1次",
			input:      "每天1次",
			recognized: true,
			recurrence: WateringIntervalDays,
			interval:   1,
			firstDate:  "2026-09-19",
			encoded:    "water:interval_days:1",
		},
		{
			name:       "无法识别-水生",
			input:      "保持水位",
			recognized: false,
			recurrence: WateringUnrecognized,
			interval:   0,
			firstDate:  "",
			encoded:    "",
		},
		{
			name:       "无法识别-空串",
			input:      "",
			recognized: false,
			recurrence: WateringUnrecognized,
			firstDate:  "",
			encoded:    "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := ParseWaterSchedule(c.input)
			if plan.Recognized() != c.recognized {
				t.Fatalf("recognized = %v, want %v", plan.Recognized(), c.recognized)
			}
			if plan.Recurrence != c.recurrence {
				t.Fatalf("recurrence = %q, want %q", plan.Recurrence, c.recurrence)
			}
			if plan.Recognized() && plan.IntervalDays != c.interval {
				t.Fatalf("intervalDays = %d, want %d", plan.IntervalDays, c.interval)
			}
			got := plan.FirstWateringDate(base)
			if c.firstDate == "" {
				if !got.IsZero() {
					t.Fatalf("firstDate = %s, want zero (待设置)", got.Format("2006-01-02"))
				}
			} else if got.Format("2006-01-02") != c.firstDate {
				t.Fatalf("firstDate = %s, want %s", got.Format("2006-01-02"), c.firstDate)
			}
			if enc := EncodeWateringFrequency(plan); enc != c.encoded {
				t.Fatalf("encoded = %q, want %q", enc, c.encoded)
			}
		})
	}
}

// 月频按当日对齐：入圃日的“日”在次月保留；月末由 AddDate 归一化，不报错。
func TestMonthlyAlignmentMonthEnd(t *testing.T) {
	plan := ParseWaterSchedule("每月1次")
	jan31 := time.Date(2026, 1, 31, 9, 0, 0, 0, time.Local)
	got := plan.FirstWateringDate(jan31)
	if got.Format("2006-01-02") != "2026-03-03" {
		t.Fatalf("month-end alignment = %s, want 2026-03-03 (Jan 31 + 1 month normalized)", got.Format("2006-01-02"))
	}
}

// 首次浇水日期按入圃日零点对齐，时分秒归零。
func TestFirstWateringDateTruncatesToDay(t *testing.T) {
	plan := ParseWaterSchedule("每周1次")
	base := time.Date(2026, 9, 18, 23, 59, 59, 0, time.Local)
	got := plan.FirstWateringDate(base)
	if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
		t.Fatalf("first watering date not truncated to day: %v", got)
	}
}

func TestDecodeWateringFrequency(t *testing.T) {
	rec, days, ok := DecodeWateringFrequency("water:interval_days:4")
	if !ok || rec != WateringIntervalDays || days != 4 {
		t.Fatalf("decode = (%q,%d,%v)", rec, days, ok)
	}
	if _, _, ok := DecodeWateringFrequency("weekly"); ok {
		t.Fatal("manual frequency must not decode as watering plan")
	}
	if _, _, ok := DecodeWateringFrequency(""); ok {
		t.Fatal("empty frequency must not decode as watering plan")
	}
	if _, _, ok := DecodeWateringFrequency("water:bogus"); ok {
		t.Fatal("malformed watering frequency must not decode")
	}
}
