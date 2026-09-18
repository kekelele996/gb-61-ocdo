package util

import (
	"testing"
	"time"
)

func TestParseWaterInterval(t *testing.T) {
	cases := []struct {
		freq         string
		days, months int
		ok           bool
	}{
		{"每天", 1, 0, true},
		{"每日", 1, 0, true},
		{"每天1次", 1, 0, true},
		{"每3天1次", 3, 0, true},
		{"每周1次", 7, 0, true},
		{"每周2次", 4, 0, true},
		{"每周3次", 2, 0, true},
		{"每星期2次", 4, 0, true},
		{"每两周1次", 14, 0, true},
		{"每2周1次", 14, 0, true},
		{"每月", 0, 1, true},
		{"每月1次", 0, 1, true},
		{"每月2次", 15, 0, true},
		{"每两月1次", 0, 2, true},
		{"每十天1次", 10, 0, true},
		{" 每周2次 ", 4, 0, true},
		{"保持水位", 0, 0, false},
		{"见干见湿", 0, 0, false},
		{"", 0, 0, false},
		{"   ", 0, 0, false},
	}
	for _, c := range cases {
		days, months, ok := ParseWaterInterval(c.freq)
		if days != c.days || months != c.months || ok != c.ok {
			t.Errorf("ParseWaterInterval(%q) = (%d, %d, %v), want (%d, %d, %v)",
				c.freq, days, months, ok, c.days, c.months, c.ok)
		}
	}
}

func TestFirstWaterDateDailyInterval(t *testing.T) {
	base := time.Date(2026, 9, 18, 15, 30, 0, 0, time.Local)
	got, ok := FirstWaterDate(base, "每周1次")
	if !ok {
		t.Fatal("expected ok for 每周1次")
	}
	want := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("FirstWaterDate = %v, want %v", got, want)
	}
}

func TestFirstWaterDateMonthlyAlignsToEntryDay(t *testing.T) {
	base := time.Date(2026, 9, 18, 15, 30, 0, 0, time.Local)
	got, ok := FirstWaterDate(base, "每月1次")
	if !ok {
		t.Fatal("expected ok for 每月1次")
	}
	want := time.Date(2026, 10, 18, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("FirstWaterDate = %v, want %v (月频按当日对齐)", got, want)
	}
}

func TestFirstWaterDateUnrecognized(t *testing.T) {
	if _, ok := FirstWaterDate(time.Now(), "保持水位"); ok {
		t.Error("expected ok=false for unrecognized frequency")
	}
}

func TestWaterFrequencyTag(t *testing.T) {
	cases := map[string]string{
		"每天1次":  "daily",
		"每周2次":  "weekly",
		"每月1次":  "monthly",
		"每3天1次": "",
		"保持水位":  "",
	}
	for freq, want := range cases {
		if got := WaterFrequencyTag(freq); got != want {
			t.Errorf("WaterFrequencyTag(%q) = %q, want %q", freq, got, want)
		}
	}
}
