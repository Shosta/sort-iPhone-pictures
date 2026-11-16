// Package scrapbook prepares printable sheets of the photos of a week: it
// finds them in a sorted library, places them on pages at a fixed size with a
// cutting space between them, and writes the PDF.
package scrapbook

import (
	"fmt"
	"time"
)

const dayLayout = "2006-01-02"

// Week is a range of calendar days from a Monday to the following Sunday.
// Days are dates at midnight UTC, so date arithmetic ignores time zones.
type Week struct {
	Start, End time.Time
}

// WeekOf returns the Monday-to-Sunday week containing day.
func WeekOf(day time.Time) Week {
	date := midnight(day)
	start := date.AddDate(0, 0, -((int(date.Weekday()) + 6) % 7))
	return Week{Start: start, End: start.AddDate(0, 0, 6)}
}

// Contains reports whether day falls inside the week.
func (w Week) Contains(day time.Time) bool {
	date := midnight(day)
	return !date.Before(w.Start) && !date.After(w.End)
}

// ParseDay reads a YYYY-MM-DD date.
func ParseDay(text string) (time.Time, error) {
	day, err := time.Parse(dayLayout, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q, want YYYY-MM-DD", text)
	}
	return day, nil
}

// DayKey formats a day as YYYY-MM-DD.
func DayKey(day time.Time) string {
	return day.Format(dayLayout)
}

func midnight(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
}
