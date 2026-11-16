package scrapbook

import (
	"testing"
	"time"
)

func TestWeekOf(t *testing.T) {
	cases := []struct{ day, start, end string }{
		{"2026-05-13", "2026-05-11", "2026-05-17"}, // Wednesday
		{"2026-05-11", "2026-05-11", "2026-05-17"}, // Monday
		{"2026-05-17", "2026-05-11", "2026-05-17"}, // Sunday
		{"2026-01-01", "2025-12-29", "2026-01-04"}, // across the year
	}
	for _, c := range cases {
		day, err := ParseDay(c.day)
		if err != nil {
			t.Fatal(err)
		}
		week := WeekOf(day.Add(15 * time.Hour))
		if DayKey(week.Start) != c.start || DayKey(week.End) != c.end {
			t.Errorf("WeekOf(%s) = %s..%s, want %s..%s", c.day, DayKey(week.Start), DayKey(week.End), c.start, c.end)
		}
		if !week.Contains(day) || week.Contains(week.End.AddDate(0, 0, 1)) || week.Contains(week.Start.AddDate(0, 0, -1)) {
			t.Errorf("Contains is wrong for %s", c.day)
		}
	}
}

func TestParseDayRejectsGarbage(t *testing.T) {
	for _, text := range []string{"", "2026-13-01", "12/05/2026", "2026-02-30"} {
		if _, err := ParseDay(text); err == nil {
			t.Errorf("ParseDay(%q) should fail", text)
		}
	}
}
