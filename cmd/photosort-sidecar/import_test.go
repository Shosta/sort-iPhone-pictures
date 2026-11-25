package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sort-iphone-pictures/internal/remote"
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

// flakyTransport is a fake rsync: the first Upload fails like a network cut
// after sending nothing, the next ones deliver.
type flakyTransport struct {
	recordingTransport
	failures int
	failWith error
	calls    int
}

func (f *flakyTransport) Upload(ctx context.Context, cfg remote.Config, job remote.Job) (remote.Result, error) {
	f.calls++
	if f.failures > 0 {
		f.failures--
		return remote.Result{}, f.failWith
	}
	return f.recordingTransport.Upload(ctx, cfg, job)
}

func importToNAS(t *testing.T, transport remote.Transport) (*server, *bytes.Buffer, request, string) {
	t.Helper()
	base := t.TempDir()
	dcim := filepath.Join(base, "DCIM")
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0001.JPG"))
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0002.JPG"))
	var out bytes.Buffer
	srv := newServer(&out)
	srv.journalDir = filepath.Join(base, "journal")
	srv.transport = transport
	srv.remoteCache = t.TempDir()
	cfg := &remote.Config{Protocol: remote.Daemon, Host: "nas", Module: "photos", Password: "pw"}
	return srv, &out, request{ID: "r", Op: "importRun", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew, Remote: cfg}, dcim
}

func TestImportToNASJournalsAfterDelivery(t *testing.T) {
	transport := &flakyTransport{}
	srv, out, run, dcim := importToNAS(t, transport)
	ctx := context.Background()
	srv.handle(ctx, run)
	srv.wg.Wait()
	done := lastEvent(t, out, "r", "importDone")
	if done["imported"] != float64(2) || len(transport.staged) != 2 {
		t.Fatalf("done %v staged %v", done, transport.staged)
	}
	for _, e := range events(t, out, "r") {
		if e["type"] == "done" || e["type"] == "progress" {
			t.Fatalf("a batch must not emit %v", e["type"])
		}
	}
	srv.handle(ctx, request{ID: "s", Op: "importScan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew})
	srv.wg.Wait()
	if scan := lastEvent(t, out, "s", "importScan"); scan["count"] != float64(0) {
		t.Fatalf("delivered photos must be journaled: %v", scan)
	}
}

func TestImportToNASNetworkCutPausesAndResumes(t *testing.T) {
	networkRetry = 10 * time.Millisecond
	defer func() { networkRetry = 5 * time.Second }()
	transport := &flakyTransport{failures: 1, failWith: &remote.Error{Kind: remote.KindUnreachable}}
	srv, out, run, _ := importToNAS(t, transport)
	srv.handle(context.Background(), run)
	srv.wg.Wait()
	var paused, resumed bool
	for _, e := range events(t, out, "r") {
		paused = paused || (e["type"] == "importPaused" && e["reason"] == "network")
		resumed = resumed || e["type"] == "importResumed"
	}
	if !paused || !resumed || transport.calls != 2 {
		t.Fatalf("paused %v resumed %v calls %d", paused, resumed, transport.calls)
	}
	if done := lastEvent(t, out, "r", "importDone"); done["imported"] != float64(2) {
		t.Fatalf("done %v", done)
	}
}

func TestImportToNASFailureLeavesJournalEmpty(t *testing.T) {
	transport := &flakyTransport{failures: 1, failWith: &remote.Error{Kind: remote.KindAuth, Detail: "denied"}}
	srv, out, run, dcim := importToNAS(t, transport)
	ctx := context.Background()
	srv.handle(ctx, run)
	srv.wg.Wait()
	if e := lastEvent(t, out, "r", "error"); e["code"] != "auth" {
		t.Fatalf("error %v", e)
	}
	srv.handle(ctx, request{ID: "s", Op: "importScan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew})
	srv.wg.Wait()
	if scan := lastEvent(t, out, "s", "importScan"); scan["count"] != float64(2) {
		t.Fatalf("nothing may be journaled before delivery: %v", scan)
	}
}
