package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func lastEvent(t *testing.T, out *bytes.Buffer, id, kind string) map[string]any {
	t.Helper()
	var found map[string]any
	for _, e := range events(t, out, id) {
		if e["type"] == kind {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("no %s event for %s: %v", kind, id, events(t, out, id))
	}
	return found
}

func TestImportRunJournalsAndSkipsNew(t *testing.T) {
	base := t.TempDir()
	dcim := filepath.Join(base, "iphone", "DCIM")
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0001.JPG"))
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0002.JPG"))
	if err := os.WriteFile(filepath.Join(dcim, "100APPLE", "IMG_0003.MOV"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv := newServer(&out)
	srv.devRoots = []string{filepath.Join(base, "iphone", "..")}
	srv.journalDir = filepath.Join(base, "journal")
	dest := filepath.Join(base, "library")
	ctx := context.Background()

	srv.handle(ctx, request{ID: "d", Op: "importDetect"})
	srv.wg.Wait()
	if devices := lastEvent(t, &out, "d", "importDevices")["devices"].([]any); len(devices) != 1 {
		t.Fatalf("devices: %v", devices)
	}

	req := request{ID: "s", Op: "importScan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew}
	srv.handle(ctx, req)
	srv.wg.Wait()
	if scan := lastEvent(t, &out, "s", "importScan"); scan["count"] != float64(2) || scan["videos"] != float64(1) {
		t.Fatalf("scan: %v", scan)
	}

	run := request{ID: "r", Op: "importRun", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew, Destination: dest}
	srv.handle(ctx, run)
	srv.wg.Wait()
	done := lastEvent(t, &out, "r", "importDone")
	if done["imported"] != float64(2) || done["videos"] != float64(1) {
		t.Fatalf("done: %v", done)
	}
	if got, _ := filepath.Glob(filepath.Join(dest, "2026", "2026-10", "*.jpg")); len(got) != 2 {
		t.Fatalf("sorted files: %v", got)
	}

	run.ID = "again"
	srv.handle(ctx, run)
	srv.wg.Wait()
	if again := lastEvent(t, &out, "again", "importDone"); again["total"] != float64(0) {
		t.Fatalf("second run must find nothing new: %v", again)
	}

	run.ID, run.Mode, run.From, run.To = "period", importModePeriod, "2020-01-01", "2020-12-31"
	srv.handle(ctx, run)
	srv.wg.Wait()
	if p := lastEvent(t, &out, "period", "importDone"); p["imported"] != float64(0) {
		t.Fatalf("period outside the photos: %v", p)
	}

	forget := request{ID: "f", Op: "importForget", DevicePath: dcim, DeviceID: "dev"}
	srv.handle(ctx, forget)
	srv.wg.Wait()
	srv.handle(ctx, request{ID: "s2", Op: "importScan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew})
	srv.wg.Wait()
	if scan := lastEvent(t, &out, "s2", "importScan"); scan["count"] != float64(2) {
		t.Fatalf("forgotten device must list everything: %v", scan)
	}
}
