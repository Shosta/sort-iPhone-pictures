package devimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path string, size int, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestScanFilterJournalBatches(t *testing.T) {
	dcim := t.TempDir()
	day := func(d int) time.Time { return time.Date(2026, 3, d, 12, 0, 0, 0, time.Local) }
	write(t, filepath.Join(dcim, "100APPLE", "IMG_2.JPG"), 100, day(10))
	write(t, filepath.Join(dcim, "100APPLE", "IMG_1.HEIC"), 100, day(2))
	write(t, filepath.Join(dcim, "100APPLE", "IMG_3.PNG"), 300, day(20))
	write(t, filepath.Join(dcim, "100APPLE", "IMG_3.AAE"), 5, day(20))
	write(t, filepath.Join(dcim, "100APPLE", "IMG_4.MOV"), 5, day(20))
	scan, err := ScanDCIM(context.Background(), dcim)
	if err != nil || len(scan.Files) != 3 || scan.Videos != 1 {
		t.Fatalf("scan: %+v, %v", scan, err)
	}
	if scan.Files[0].Rel != filepath.Join("100APPLE", "IMG_1.HEIC") {
		t.Fatalf("not oldest first: %+v", scan.Files)
	}
	if got := scan.Filter(nil, day(9), day(11)); len(got) != 1 || got[0].Size != 100 {
		t.Fatalf("period filter: %+v", got)
	}
	journal, err := OpenJournal(t.TempDir(), "afc-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Add(scan.Files[:1]); err != nil {
		t.Fatal(err)
	}
	if got := scan.Filter(journal, time.Time{}, time.Time{}); len(got) != 2 {
		t.Fatalf("journal filter: %+v", got)
	}
	reopened, err := OpenJournal(filepath.Dir(journal.path), "afc-1")
	if err != nil || !reopened.Has(scan.Files[0]) {
		t.Fatalf("journal not persisted: %v", err)
	}
	if err := reopened.Forget(); err != nil || reopened.Has(scan.Files[0]) {
		t.Fatalf("forget: %v", err)
	}
	batches := Batches(scan.Files, 250)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("batches: %+v", batches)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	mod := time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local)
	src := filepath.Join(dir, "src.jpg")
	write(t, src, 50, mod)
	dst := filepath.Join(dir, "out", "100APPLE", "src.jpg")
	if err := CopyFile(context.Background(), src, dst, File{Rel: "src.jpg", Size: 50, ModTime: mod}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dst); err != nil || !info.ModTime().Equal(mod) {
		t.Fatalf("copy: %v %v", info, err)
	}
	bad := filepath.Join(dir, "out", "bad.jpg")
	if err := CopyFile(context.Background(), src, bad, File{Rel: "x", Size: 51, ModTime: mod}); err == nil {
		t.Fatal("size mismatch must fail")
	}
	if _, err := os.Stat(bad + ".part"); err == nil {
		t.Fatal("partial file must be removed")
	}
}

func TestDetect(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "afc:host=ABC-1", "DCIM", "100APPLE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "SDCARD", "DCIM"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "USBSTICK", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	devices := Detect([]string{root, filepath.Join(root, "missing")})
	if len(devices) != 2 || devices[1].ID != "afc-ABC-1" || devices[1].Name != "iPhone" || devices[0].Name != "SDCARD" {
		t.Fatalf("devices: %+v", devices)
	}
}

func TestPrepareTransitRefusesSymlinkAndEmpties(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareTransit([]string{link}, 1); err == nil {
		t.Fatal("symlinked transit must be refused")
	}
	dir := filepath.Join(base, "transit")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "old", "x.jpg"), 1, time.Now())
	got, err := prepareTransit([]string{link, dir}, 1)
	if err != nil || got != dir {
		t.Fatalf("prepare: %q %v", got, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("transit must be emptied")
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", info.Mode())
	}
	if _, err := prepareTransit([]string{dir}, 1<<62); err == nil {
		t.Fatal("must report low disk")
	}
}
