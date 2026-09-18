package util

import (
	"fmt"
	"time"
)

// Shared formatters. Date/time, temperature, status text, plant type text and
// care topic text live together intentionally so a wording change ripples
// through every handler and service that renders text.

// FormatDate renders a time as YYYY-MM-DD.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// FormatDateTime renders a time as YYYY-MM-DD HH:mm.
func FormatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// FormatTemp renders a temperature range.
func FormatTemp(min, max float64) string {
	if min == 0 && max == 0 {
		return "不限"
	}
	return fmt.Sprintf("%.0f°C ~ %.0f°C", min, max)
}

// PlantTypeText maps a plant type code to Chinese text.
func PlantTypeText(t string) string {
	switch t {
	case "flower":
		return "观花"
	case "foliage":
		return "观叶"
	case "succulent":
		return "多肉"
	case "aquatic":
		return "水生"
	default:
		return "未知"
	}
}

// CareTopicText maps a care topic tag to Chinese text.
func CareTopicText(t string) string {
	switch t {
	case "fertilizing":
		return "施肥"
	case "pruning":
		return "修剪"
	case "repotting":
		return "换盆"
	case "pest_control":
		return "病虫害"
	case "propagation":
		return "繁殖"
	default:
		return "通用"
	}
}

// ReminderStatusText maps a reminder status to Chinese text.
func ReminderStatusText(s string) string {
	switch s {
	case "pending":
		return "待处理"
	case "done":
		return "已完成"
	case "overdue":
		return "已逾期"
	default:
		return "未知"
	}
}

// WateringRecurrenceText renders the watering plan code stored on an
// auto-created "首次浇水" reminder. An empty/unknown code renders as 待设置.
func WateringRecurrenceText(recurrence string, intervalDays int) string {
	switch recurrence {
	case "interval_days":
		if intervalDays <= 1 {
			return "每1天"
		}
		return fmt.Sprintf("每%d天", intervalDays)
	case "monthly":
		return "每月（按当日对齐）"
	default:
		return "待设置"
	}
}
