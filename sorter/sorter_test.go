package sorter

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanAndCopy(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	destination := filepath.Join(base, "destination")
	first := filepath.Join(source, "a.tiff")
	second := filepath.Join(source, "nested", "b.TIFF")
	writePhoto(t, first)
	writePhoto(t, second)
	plan, err := (Engine{}).Plan(context.Background(), []string{source, filepath.Dir(second)}, destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 2 || len(plan.Skipped) != 0 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	for index, want := range []string{"2026/2026-10/2026-10-20 - iPhone - 1.tiff", "2026/2026-10/2026-10-20 - iPhone - 2.tiff"} {
		if filepath.ToSlash(plan.Entries[index].Target) != want {
			t.Fatalf("unexpected target: %s", plan.Entries[index].Target)
		}
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planning must not create destination: %v", err)
	}
	report, err := Copy(context.Background(), plan)
	if err != nil || report.Copied != 2 {
		t.Fatalf("copy: %+v, %v", report, err)
	}
	for _, entry := range plan.Entries {
		original, err := os.ReadFile(entry.Source)
		if err != nil {
			t.Fatal(err)
		}
		copied, err := os.ReadFile(filepath.Join(plan.Destination, entry.Target))
		if err != nil || !bytes.Equal(original, copied) {
			t.Fatalf("copy differs: %v", err)
		}
	}
	if _, err := Copy(context.Background(), plan); !errors.Is(err, os.ErrExist) {
		t.Fatalf("must refuse overwrite: %v", err)
	}
	next, err := (Engine{}).Plan(context.Background(), []string{source}, destination)
	if err != nil || !strings.HasSuffix(next.Entries[0].Target, " - 3.tiff") {
		t.Fatalf("existing filenames not reserved: %+v, %v", next, err)
	}
}

func TestPlanMissingMetadata(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	writePhoto(t, filepath.Join(source, "a.tiff"))
	writePhoto(t, filepath.Join(source, "b.tiff"))
	engine := Engine{ReadMetadata: func(_ context.Context, path string) (Metadata, error) {
		if filepath.Base(path) == "a.tiff" {
			return Metadata{}, errors.New("invalid EXIF")
		}
		return Metadata{}, nil
	}}
	plan, err := engine.Plan(context.Background(), []string{source}, filepath.Join(base, "dest"))
	if err != nil || len(plan.Entries) != 0 || len(plan.Skipped) != 2 {
		t.Fatalf("unexpected result: %+v, %v", plan, err)
	}
}

func TestPlanRejectsOverlap(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	writePhoto(t, filepath.Join(source, "photo.tiff"))
	for _, destination := range []string{source, filepath.Join(source, "output"), base} {
		if _, err := (Engine{}).Plan(context.Background(), []string{source}, destination); err == nil {
			t.Fatalf("accepted overlapping destination %s", destination)
		}
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := (Engine{}).Plan(context.Background(), []string{source}, filepath.Join(alias, "output")); err == nil {
		t.Fatal("accepted overlap through symlink")
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base := t.TempDir()
	_, err := (Engine{}).Plan(ctx, []string{base}, filepath.Join(filepath.Dir(base), "unused"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("plan cancellation: %v", err)
	}
	if _, err := Copy(ctx, Plan{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy cancellation: %v", err)
	}
}

func TestCameraName(t *testing.T) {
	for input, want := range map[string]string{
		"":                  "Unknown camera",
		" ../iPhone:Pro\\ ": "iPhone-Pro",
		"iPhone  15 Pro":    "iPhone 15 Pro",
	} {
		if got := cameraName(input); got != want {
			t.Errorf("cameraName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCopyRejectsEscapingTarget(t *testing.T) {
	plan := Plan{Destination: t.TempDir(), Entries: []Entry{{Target: "../outside.tiff"}}}
	if _, err := Copy(context.Background(), plan); err == nil {
		t.Fatal("accepted unsafe target")
	}
}

func TestCopyPartialFailure(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "photo.tiff")
	writePhoto(t, source)
	plan := Plan{
		Destination: filepath.Join(base, "dest"),
		Entries: []Entry{
			{Source: source, Target: "2026/first.tiff"},
			{Source: filepath.Join(base, "missing.tiff"), Target: "2026/second.tiff"},
		},
	}
	var completed []Entry
	report, err := CopyWithProgress(context.Background(), plan, func(entry Entry) {
		completed = append(completed, entry)
	})
	if err == nil || report.Copied != 1 {
		t.Fatalf("unexpected partial result: %+v, %v", report, err)
	}
	if len(completed) != 1 || completed[0].Target != "2026/first.tiff" {
		t.Fatalf("unexpected progress: %+v", completed)
	}
	if _, err := os.Stat(filepath.Join(plan.Destination, "2026/first.tiff")); err != nil {
		t.Fatal("completed copy must remain:", err)
	}
}

func TestCopyRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "photo.tiff")
	writePhoto(t, source)
	destination := filepath.Join(base, "dest")
	outside := filepath.Join(base, "outside")
	for _, path := range []string{destination, outside} {
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(destination, "2026")); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Destination: destination, Entries: []Entry{{Source: source, Target: "2026/2026-10/photo.tiff"}}}
	if _, err := Copy(context.Background(), plan); err == nil {
		t.Fatal("accepted symlink escape")
	}
	items, err := os.ReadDir(outside)
	if err != nil || len(items) != 0 {
		t.Fatalf("wrote outside destination: %v, %v", items, err)
	}
}

func TestPlanUnknownCameraAndSymlink(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	photo := filepath.Join(source, "photo.tiff")
	writePhoto(t, photo)
	if err := os.Symlink(photo, filepath.Join(source, "link.tiff")); err != nil {
		t.Fatal(err)
	}
	engine := Engine{ReadMetadata: func(context.Context, string) (Metadata, error) {
		return Metadata{TakenAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}, nil
	}}
	plan, err := engine.Plan(context.Background(), []string{source}, filepath.Join(base, "dest"))
	if err != nil || len(plan.Entries) != 1 || len(plan.Skipped) != 1 {
		t.Fatalf("unexpected result: %+v, %v", plan, err)
	}
	if !strings.Contains(plan.Entries[0].Target, "Unknown camera") {
		t.Fatal("missing camera fallback")
	}
}

func TestPlanByLocation(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	for _, name := range []string{"a.tiff", "b.tiff", "c.tiff"} {
		writePhoto(t, filepath.Join(source, name))
	}
	reader := func(_ context.Context, path string) (Metadata, error) {
		metadata := Metadata{TakenAt: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), Camera: "iPhone"}
		if !strings.HasSuffix(path, "c.tiff") {
			metadata.Lat, metadata.Lon, metadata.HasGPS = 49.44, 1.09, true
		}
		return metadata, nil
	}
	locate := func(float64, float64) (string, bool) { return "Rouen/..", true }
	plan, err := (Engine{ReadMetadata: reader, ByLocation: true, Locate: locate}).Plan(context.Background(), []string{source}, filepath.Join(base, "dest"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"2026/2026-06/Rouen/2026-06-12 - iPhone - 1.tiff",
		"2026/2026-06/Rouen/2026-06-12 - iPhone - 2.tiff",
		"2026/2026-06/2026-06-12 - iPhone - 1.tiff",
	}
	for index, entry := range plan.Entries {
		if filepath.ToSlash(entry.Target) != want[index] {
			t.Fatalf("entry %d: got %s, want %s", index, entry.Target, want[index])
		}
	}
	plan, err = (Engine{ReadMetadata: reader, Locate: locate}).Plan(context.Background(), []string{source}, filepath.Join(base, "dest"))
	if err != nil || strings.Contains(plan.Entries[0].Target, "Rouen") {
		t.Fatalf("location must be opt-in: %+v, %v", plan, err)
	}
}
