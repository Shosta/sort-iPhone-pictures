package scrapbook

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"Semaine 2026-S20.pdf": "Semaine 2026-S20.pdf",
		"a/b:c*d":              "a-b-c-d.pdf",
		"":                     "Photosort.pdf",
		"  ..  ":               "Photosort.pdf",
		"week.PDF":             "week.pdf",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SafeName(strings.Repeat("é", 500)); len([]rune(got)) != 124 {
		t.Errorf("long names should be cut, got %d characters", len([]rune(got)))
	}
}

func renderJob(t *testing.T, dir string, files []string, ratios []float64) Job {
	t.Helper()
	paper := a4Landscape(t)
	return Job{
		Paper:  paper,
		Photos: files,
		Sheets: Layout(paper, DefaultLongSide, MinGutter, ratios),
		Dir:    dir,
		Name:   "Semaine.pdf",
	}
}

func TestRenderWritesAPDFWithOnePagePerSheet(t *testing.T) {
	dir := t.TempDir()
	var files []string
	var ratios []float64
	for i := 0; i < 11; i++ {
		path := filepath.Join(dir, "in", string(rune('a'+i))+".jpg")
		writePhoto(t, path, 300, 400)
		files = append(files, path)
		ratios = append(ratios, 0.75)
	}
	out := filepath.Join(dir, "out")
	job := renderJob(t, out, files, ratios)
	var calls, last int
	job.Progress = func(done, total int) {
		calls++
		last = done
		if total != 11 {
			t.Errorf("total = %d", total)
		}
	}
	result, err := Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages != 2 || result.Fallback || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if calls != 11 || last != 11 {
		t.Errorf("progress calls = %d, last = %d", calls, last)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) || len(regexp.MustCompile(`/Type /Page[^s]`).FindAll(data, -1)) != 2 {
		t.Errorf("not a two-page PDF (%d bytes)", len(data))
	}
	if box := regexp.MustCompile(`/MediaBox \[[^\]]*\]`).Find(data); !strings.Contains(string(box), "841.89 595.28") {
		t.Errorf("pages are not A4 landscape: %s", box)
	}
	if filepath.Dir(result.Path) != out || filepath.Base(result.Path) != "Semaine.pdf" {
		t.Errorf("path = %s", result.Path)
	}
	// No temporary file is left behind.
	entries, _ := os.ReadDir(out)
	if len(entries) != 1 {
		t.Errorf("output folder holds %d files, want 1", len(entries))
	}
}

func TestRenderNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "a.jpg")
	writePhoto(t, photo, 40, 30)
	job := renderJob(t, filepath.Join(dir, "out"), []string{photo}, []float64{4.0 / 3.0})
	var names []string
	for range 3 {
		result, err := Render(context.Background(), job)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.Base(result.Path))
	}
	want := []string{"Semaine.pdf", "Semaine (2).pdf", "Semaine (3).pdf"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names = %v, want %v", names, want)
			break
		}
	}
}

func TestRenderLeavesAGapForAnUnreadablePhoto(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.jpg"), filepath.Join(dir, "bad.jpg")
	writePhoto(t, good, 40, 30)
	if err := os.WriteFile(bad, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := renderJob(t, filepath.Join(dir, "out"), []string{good, bad}, []float64{4.0 / 3.0, 4.0 / 3.0})
	result, err := Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failed) != 1 || result.Failed[0].Path != bad {
		t.Errorf("failed = %+v", result.Failed)
	}
}

func TestRenderFallsBackWhenTheFolderIsUnusable(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "a.jpg")
	writePhoto(t, photo, 40, 30)
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := renderJob(t, filepath.Join(blocker, "sub"), []string{photo}, []float64{4.0 / 3.0})
	if _, err := Render(context.Background(), job); err == nil {
		t.Error("expected an error without fallback")
	}
	job.FallbackDir = filepath.Join(dir, "fallback")
	result, err := Render(context.Background(), job)
	if err != nil || !result.Fallback || filepath.Dir(result.Path) != job.FallbackDir {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestRenderCancelledLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	photo := filepath.Join(dir, "a.jpg")
	writePhoto(t, photo, 400, 300)
	out := filepath.Join(dir, "out")
	job := renderJob(t, out, []string{photo}, []float64{4.0 / 3.0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, job); err == nil {
		t.Fatal("expected a cancellation error")
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Errorf("%d files left behind", len(entries))
	}
}

func TestMeasurerReadsUprightRatios(t *testing.T) {
	dir := t.TempDir()
	landscape, portrait, broken := filepath.Join(dir, "l.jpg"), filepath.Join(dir, "p.jpg"), filepath.Join(dir, "x.jpg")
	writePhoto(t, landscape, 400, 300)
	writePhoto(t, portrait, 300, 400)
	if err := os.WriteFile(broken, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	var measurer Measurer
	for range 2 { // the second pass is served from the cache
		ratios, errs := measurer.Measure(context.Background(), []string{landscape, portrait, broken})
		if ratios[0] < 1.33 || ratios[0] > 1.34 || ratios[1] < 0.74 || ratios[1] > 0.76 {
			t.Errorf("ratios = %v", ratios)
		}
		if errs[0] != nil || errs[1] != nil || errs[2] == nil {
			t.Errorf("errors = %v", errs)
		}
	}
}

func TestRenderFramedPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in", "a.jpg")
	writePhoto(t, path, 300, 400)
	for _, frame := range []Frame{FramePolaroid, FrameStamp, FrameTape} {
		job := renderJob(t, filepath.Join(dir, string(frame)), []string{path}, []float64{0.75})
		job.Frame = frame
		job.Sheets = LayoutFramed(job.Paper, frame, DefaultLongSide, MinGutter, []float64{0.75})
		result, err := Render(context.Background(), job)
		if err != nil || result.Pages != 1 || len(result.Failed) != 0 {
			t.Errorf("%s: %+v, %v", frame, result, err)
		}
	}
}

func TestRenderCollageOnColouredPage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in", "a.jpg")
	writePhoto(t, path, 300, 400)
	back, ok := ParseBackground("Cream")
	if !ok || back == "" {
		t.Fatal("cream not accepted")
	}
	if _, ok := ParseBackground("plaid"); ok {
		t.Error("unknown background accepted")
	}
	for _, frame := range []Frame{FrameNone, FramePolaroid, FrameStamp, FrameTape} {
		job := renderJob(t, filepath.Join(dir, string(frame)), []string{path, path}, []float64{0.75, 0.75})
		job.Frame, job.Background = frame, back
		job.Sheets = LayoutStyled(job.Paper, StyleCollage, frame, DefaultLongSide, MinGutter, []float64{0.75, 0.75})
		result, err := Render(context.Background(), job)
		if err != nil || result.Pages != 1 || len(result.Failed) != 0 {
			t.Errorf("%s: %+v, %v", frame, result, err)
		}
	}
}
