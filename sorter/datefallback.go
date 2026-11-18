package sorter

import (
	"os"
	"regexp"
	"strconv"
	"time"
)

// Date sources reported in Metadata.DateSource when the date is not from EXIF.
const (
	DateFromFilename = "filename"
	DateFromModTime  = "modtime"
)

// earliestFallbackYear rejects implausible dates (zero or epoch-like
// modification times) so such photos stay undated.
const earliestFallbackYear = 1990

var filenameDate = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d{2})[-_.]?(\d{2})[-_.]?(\d{2})(?:[^0-9]|$)`)

// dateFromFilename extracts a YYYYMMDD, YYYY-MM-DD, YYYY_MM_DD or YYYY.MM.DD
// date from a file name such as IMG_20230714_153022.jpg.
func dateFromFilename(name string, now time.Time) (time.Time, bool) {
	for _, match := range filenameDate.FindAllStringSubmatch(name, -1) {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		date := time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.Local)
		if date.Year() == year && int(date.Month()) == month && date.Day() == day && !date.After(now) {
			return date, true
		}
	}
	return time.Time{}, false
}

// fallbackDate returns the best date of a photo without EXIF capture date:
// the date in its file name, then its modification time.
func fallbackDate(path, name string, now time.Time) (time.Time, string, bool) {
	if date, ok := dateFromFilename(name, now); ok {
		return date, DateFromFilename, true
	}
	if info, err := os.Stat(path); err == nil {
		if modified := info.ModTime(); modified.Year() >= earliestFallbackYear && !modified.After(now) {
			return modified, DateFromModTime, true
		}
	}
	return time.Time{}, "", false
}
