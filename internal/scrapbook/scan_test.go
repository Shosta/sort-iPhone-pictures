package scrapbook

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePhoto(t *testing.T, path string, width, height int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / width), uint8(y * 255 / height), 120, 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatal(err)
	}
}

func day(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := ParseDay(text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestScanPicksTheWeekAcrossMonths(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"2026/2026-04/2026-04-29 - iPhone - 1.jpg",
		"2026/2026-04/2026-04-30 - iPhone - 1.jpg",
		"2026/2026-04/Paris/2026-04-30 - iPhone - 2.jpg",
		"2026/2026-05/2026-05-03 - iPhone - 1.jpg",
		"2026/2026-05/2026-05-03 12.00.00 - Almost Duplicates/2026-05-03 - iPhone - 2.jpg",
		"2026/2026-05/2026-05-04 - iPhone - 1.jpg", // after the week
		"2026/2026-05/.hidden/2026-05-02 - iPhone - 9.jpg",
	}
	for _, file := range files {
		writePhoto(t, filepath.Join(root, file), 8, 6)
	}
	// Not photos, or not dated inside the week.
	for _, file := range []string{"2026/2026-05/2026-05-02 - iPhone - 1.xmp", "2026/2026-05/notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	photos, err := Scan(context.Background(), root, day(t, "2026-04-30"), day(t, "2026-05-03"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, photo := range photos {
		rel, _ := filepath.Rel(root, photo.Path)
		got = append(got, rel)
	}
	want := []string{
		"2026/2026-04/2026-04-30 - iPhone - 1.jpg",
		"2026/2026-04/Paris/2026-04-30 - iPhone - 2.jpg",
		"2026/2026-05/2026-05-03 - iPhone - 1.jpg",
		"2026/2026-05/2026-05-03 12.00.00 - Almost Duplicates/2026-05-03 - iPhone - 2.jpg",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("photo %d = %s, want %s", i, got[i], want[i])
		}
	}
	counts := CountByDay(photos)
	if counts["2026-04-30"] != 2 || counts["2026-05-03"] != 2 || len(counts) != 2 {
		t.Errorf("counts = %v", counts)
	}
}

func TestScanAcceptsAYearFolderAsRoot(t *testing.T) {
	root := t.TempDir()
	writePhoto(t, filepath.Join(root, "2026-05", "2026-05-12 - iPhone - 1.jpg"), 8, 6)
	photos, err := Scan(context.Background(), root, day(t, "2026-05-11"), day(t, "2026-05-17"))
	if err != nil || len(photos) != 1 {
		t.Fatalf("photos = %v, err = %v", photos, err)
	}
	if !LooksLikeRoot(root) || LooksLikeRoot(t.TempDir()) {
		t.Error("LooksLikeRoot is wrong")
	}
}

func TestScanDatesFilesWithoutSorterName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "2026", "2026-05", "IMG_0001.jpg")
	writePhoto(t, path, 8, 6)
	modified := time.Date(2026, 5, 12, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	photos, err := Scan(context.Background(), root, day(t, "2026-05-11"), day(t, "2026-05-17"))
	if err != nil || len(photos) != 1 || DayKey(photos[0].Day) != "2026-05-12" {
		t.Fatalf("photos = %v, err = %v", photos, err)
	}
}

func TestScanStopsWhenCancelled(t *testing.T) {
	root := t.TempDir()
	writePhoto(t, filepath.Join(root, "2026", "2026-05", "2026-05-12 - iPhone - 1.jpg"), 8, 6)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, root, day(t, "2026-05-11"), day(t, "2026-05-17")); err == nil {
		t.Error("expected a cancellation error")
	}
}
