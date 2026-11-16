package scrapbook

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"sort-iphone-pictures/sorter"
)

// Photo is a photo of the library with the day it was taken.
type Photo struct {
	Path string
	Day  time.Time
}

var (
	datePrefix = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})`)
	yearFolder = regexp.MustCompile(`^\d{4}$`)
	// monthFolder also matches month folders with a suffix.
	monthFolder = regexp.MustCompile(`^\d{4}-\d{2}`)
)

// LooksLikeRoot reports whether dir contains year (2026) or month (2026-05)
// folders, which is how a sorted library is organised.
func LooksLikeRoot(dir string) bool {
	items, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, item := range items {
		if item.IsDir() && (yearFolder.MatchString(item.Name()) || monthFolder.MatchString(item.Name())) {
			return true
		}
	}
	return false
}

// Scan lists the photos of the library under root taken between from and to
// (inclusive days), in chronological order. A sorted library is organised as
// root/YYYY/YYYY-MM[/anything]/YYYY-MM-DD - … .jpg; month folders directly under
// root (when root is a year folder) are accepted too, and every sub-folder of a
// month is searched, near-duplicate groups included.
func Scan(ctx context.Context, root string, from, to time.Time) ([]Photo, error) {
	from, to = midnight(from), midnight(to)
	var photos []Photo
	for month := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !month.After(to); month = month.AddDate(0, 1, 0) {
		name := month.Format("2006-01")
		for _, dir := range []string{filepath.Join(root, month.Format("2006"), name), filepath.Join(root, name)} {
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				continue
			}
			found, err := scanMonth(ctx, dir, from, to)
			if err != nil {
				return nil, err
			}
			photos = append(photos, found...)
		}
	}
	sort.Slice(photos, func(i, j int) bool {
		a, b := photos[i], photos[j]
		if !a.Day.Equal(b.Day) {
			return a.Day.Before(b.Day)
		}
		if an, bn := filepath.Base(a.Path), filepath.Base(b.Path); an != bn {
			return an < bn
		}
		return a.Path < b.Path
	})
	return photos, nil
}

// CountByDay returns the number of photos of each day, keyed by DayKey.
func CountByDay(photos []Photo) map[string]int {
	counts := map[string]int{}
	for _, photo := range photos {
		counts[DayKey(photo.Day)]++
	}
	return counts
}

func scanMonth(ctx context.Context, dir string, from, to time.Time) ([]Photo, error) {
	var photos []Photo
	err := filepath.WalkDir(dir, func(path string, item fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == dir {
				return walkErr
			}
			return nil
		}
		if item.IsDir() {
			if path != dir && len(item.Name()) > 1 && item.Name()[0] == '.' {
				return filepath.SkipDir
			}
			return nil
		}
		if !item.Type().IsRegular() || !sorter.IsPhoto(path) {
			return nil
		}
		if day := dayOf(ctx, path, item); !day.Before(from) && !day.After(to) {
			photos = append(photos, Photo{Path: path, Day: day})
		}
		return nil
	})
	return photos, err
}

// dayOf is the day written at the start of the file name by the sorter, else
// the capture day from the Exif data, else the modification day.
func dayOf(ctx context.Context, path string, item fs.DirEntry) time.Time {
	if match := datePrefix.FindStringSubmatch(filepath.Base(path)); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		if date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC); date.Year() == year && int(date.Month()) == month && date.Day() == day {
			return date
		}
	}
	if metadata, err := sorter.ReadMetadata(ctx, path); err == nil && !metadata.TakenAt.IsZero() {
		return midnight(metadata.TakenAt)
	}
	if info, err := item.Info(); err == nil {
		return midnight(info.ModTime())
	}
	return time.Time{}
}
