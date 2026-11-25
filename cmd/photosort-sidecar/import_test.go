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

func planAndCopy(t *testing.T, srv *server, out *bytes.Buffer, plan, copyReq request) (map[string]any, map[string]any) {
	t.Helper()
	ctx := context.Background()
	srv.handle(ctx, plan)
	srv.wg.Wait()
	p := lastEvent(t, out, plan.ID, "plan")
	copyReq.Op, copyReq.PlanID = "copy", plan.ID
	srv.handle(ctx, copyReq)
	srv.wg.Wait()
	return p, nil
}

func countEvents(t *testing.T, out *bytes.Buffer, id, kind string) int {
	t.Helper()
	n := 0
	for _, e := range events(t, out, id) {
		if e["type"] == kind {
			n++
		}
	}
	return n
}

func TestDevicePlanListsThenCopiesToLocal(t *testing.T) {
	base := t.TempDir()
	dcim := filepath.Join(base, "DCIM")
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0001.JPG"))
	writeJPEG(t, filepath.Join(dcim, "100APPLE", "IMG_0002.JPG"))
	if err := os.WriteFile(filepath.Join(dcim, "100APPLE", "IMG_0003.MOV"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv := newServer(&out)
	srv.journalDir = filepath.Join(base, "journal")
	dest := filepath.Join(base, "library")
	plan := request{ID: "p", Op: "plan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew, Destination: dest}
	p, _ := planAndCopy(t, srv, &out, plan, request{ID: "c"})
	if counts := p["counts"].(map[string]any); counts["entries"] != float64(2) {
		t.Fatalf("plan counts: %v", counts)
	}
	if got, _ := filepath.Glob(filepath.Join(dest, "2026", "2026-10", "*.jpg")); len(got) != 2 {
		t.Fatalf("sorted files: %v", got)
	}
	if n := countEvents(t, &out, "c", "progress"); n != 2 {
		t.Fatalf("progress events: %d", n)
	}
	for _, e := range events(t, &out, "c") {
		if e["type"] == "progress" {
			if src := e["entry"].(map[string]any)["source"].(string); filepath.Dir(filepath.Dir(src)) != mustCanon(t, dcim) {
				t.Fatalf("progress source %q must be the device path", src)
			}
		}
	}
	if done := lastEvent(t, &out, "c", "done"); done["copied"] != float64(2) {
		t.Fatalf("done: %v", done)
	}
	// Journaled after delivery: "new" now finds nothing, "all" finds everything.
	srv.handle(context.Background(), request{ID: "p2", Op: "plan", DevicePath: dcim, DeviceID: "dev", Mode: importModeNew, Destination: dest})
	srv.wg.Wait()
	if counts := lastEvent(t, &out, "p2", "plan")["counts"].(map[string]any); counts["entries"] != float64(0) {
		t.Fatalf("second plan must be empty: %v", counts)
	}
	srv.handle(context.Background(), request{ID: "p3", Op: "plan", DevicePath: dcim, DeviceID: "dev", Mode: importModeAll, Destination: dest})
	srv.wg.Wait()
	if counts := lastEvent(t, &out, "p3", "plan")["counts"].(map[string]any); counts["entries"] != float64(2) {
		t.Fatalf("all mode: %v", counts)
	}
}

func TestDeviceCopyToNASAndDryRun(t *testing.T) {
	transport := &flakyTransport{}
	srv, out, run, _ := importToNAS(t, transport)
	plan := request{ID: "p", Op: "plan", DevicePath: run.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run.Remote}
	// A dry run sends nothing and journals nothing.
	planAndCopy(t, srv, out, plan, request{ID: "dry", Remote: run.Remote, DryRun: true})
	if n := countEvents(t, out, "dry", "progress"); n != 0 {
		t.Fatalf("dry run reported %d photos", n)
	}
	if done := lastEvent(t, out, "dry", "done"); done["dryRun"] != true || done["remote"] != true {
		t.Fatalf("dry done: %v", done)
	}
	srv.handle(context.Background(), request{ID: "p1", Op: "plan", DevicePath: run.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run.Remote})
	srv.wg.Wait()
	if counts := lastEvent(t, out, "p1", "plan")["counts"].(map[string]any); counts["entries"] != float64(2) {
		t.Fatalf("dry run must not journal: %v", counts)
	}
	staged := len(transport.staged)
	planAndCopy(t, srv, out, plan, request{ID: "real", Remote: run.Remote})
	if n := countEvents(t, out, "real", "progress"); n != 2 || len(transport.staged) != staged+2 {
		t.Fatalf("progress %d staged %v", n, transport.staged)
	}
	if done := lastEvent(t, out, "real", "done"); done["copied"] != float64(2) || done["remote"] != true {
		t.Fatalf("done: %v", done)
	}
	srv.handle(context.Background(), request{ID: "p2", Op: "plan", DevicePath: run.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run.Remote})
	srv.wg.Wait()
	if counts := lastEvent(t, out, "p2", "plan")["counts"].(map[string]any); counts["entries"] != float64(0) {
		t.Fatalf("delivered photos must be journaled: %v", counts)
	}
}

func TestDeviceCopyToNASNetworkCutKeepsJournalClean(t *testing.T) {
	networkRetry = 10 * time.Millisecond
	defer func() { networkRetry = 5 * time.Second }()
	transport := &flakyTransport{failures: 1, failWith: &remote.Error{Kind: remote.KindUnreachable}}
	srv, out, run, _ := importToNAS(t, transport)
	plan := request{ID: "p", Op: "plan", DevicePath: run.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run.Remote}
	planAndCopy(t, srv, out, plan, request{ID: "c", Remote: run.Remote})
	paused := false
	for _, e := range events(t, out, "c") {
		paused = paused || (e["type"] == "importPaused" && e["reason"] == "network")
	}
	if !paused || countEvents(t, out, "c", "progress") != 2 {
		t.Fatalf("paused %v", paused)
	}

	failing := &flakyTransport{failures: 1, failWith: &remote.Error{Kind: remote.KindAuth, Detail: "denied"}}
	srv2, out2, run2, _ := importToNAS(t, failing)
	plan2 := request{ID: "p", Op: "plan", DevicePath: run2.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run2.Remote}
	planAndCopy(t, srv2, out2, plan2, request{ID: "c", Remote: run2.Remote})
	if e := lastEvent(t, out2, "c", "error"); e["code"] != "auth" {
		t.Fatalf("error %v", e)
	}
	if n := countEvents(t, out2, "c", "progress"); n != 0 {
		t.Fatalf("nothing is reported before delivery: %d", n)
	}
	srv2.handle(context.Background(), request{ID: "again", Op: "plan", DevicePath: run2.DevicePath, DeviceID: "dev", Mode: importModeNew, Remote: run2.Remote})
	srv2.wg.Wait()
	if counts := lastEvent(t, out2, "again", "plan")["counts"].(map[string]any); counts["entries"] != float64(2) {
		t.Fatalf("a failed send must not journal: %v", counts)
	}
}

func mustCanon(t *testing.T, path string) string {
	t.Helper()
	canon, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canon
}
