package sorter

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
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

func TestSkipDuplicates(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	destination := filepath.Join(base, "destination")
	writePhoto(t, filepath.Join(source, "a.tiff"))
	writePhoto(t, filepath.Join(source, "copy of a.tiff"))
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for _, name := range []string{"p1.png", "p2.png"} {
		var buffer bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.NoCompression}
		if name == "p2.png" {
			encoder.CompressionLevel = png.BestCompression
		}
		if err := encoder.Encode(&buffer, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), buffer.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	engine := Engine{SkipDuplicates: true, ReadMetadata: func(context.Context, string) (Metadata, error) {
		return Metadata{TakenAt: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)}, nil
	}}
	plan, err := engine.Plan(context.Background(), []string{source}, destination)
	if err != nil || len(plan.Entries) != 2 || len(plan.Duplicates) != 2 {
		t.Fatalf("unexpected plan: %+v, %v", plan, err)
	}
	if _, err := Copy(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	next, err := engine.Plan(context.Background(), []string{source}, destination)
	if err != nil || len(next.Entries) != 1 || len(next.Duplicates) != 3 {
		t.Fatalf("destination duplicates not detected: %+v, %v", next, err)
	}
}

func TestGroupSimilar(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	for _, name := range []string{"a", "b", "c"} {
		writePhoto(t, filepath.Join(source, name+".tiff"))
		if err := os.WriteFile(filepath.Join(source, name+".tiff"), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	taken := map[string]time.Time{
		"a.tiff": time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		"b.tiff": time.Date(2026, 10, 20, 10, 0, 5, 0, time.UTC),
		"c.tiff": time.Date(2026, 10, 20, 10, 0, 9, 0, time.UTC),
	}
	hashes := map[string]uint64{"a.tiff": 0, "b.tiff": 0b11, "c.tiff": ^uint64(0)}
	engine := Engine{
		GroupSimilar: true,
		ReadMetadata: func(_ context.Context, path string) (Metadata, error) {
			return Metadata{TakenAt: taken[filepath.Base(path)]}, nil
		},
		Hash: func(path string) (uint64, bool) { return hashes[filepath.Base(path)], true },
	}
	plan, err := engine.Plan(context.Background(), []string{source}, filepath.Join(base, "dest"))
	if err != nil || len(plan.Entries) != 3 {
		t.Fatalf("unexpected plan: %+v, %v", plan, err)
	}
	group := "2026/2026-10/2026-10-20 10.00.00 - Almost Duplicates/"
	for _, entry := range plan.Entries {
		inGroup := strings.HasPrefix(filepath.ToSlash(entry.Target), group)
		if inGroup != (filepath.Base(entry.Source) != "c.tiff") {
			t.Fatalf("unexpected target for %s: %s", entry.Source, entry.Target)
		}
	}
}
