package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"sort-iphone-pictures/internal/remote"
)

type recordingTransport struct {
	cfg      remote.Config
	staged   []string
	dryRun   bool
	existing string
	err      error
}

func (x *recordingTransport) List(context.Context, remote.Config, string) (remote.Listing, error) {
	return remote.Listing{}, nil
}

func (x *recordingTransport) ListTree(context.Context, remote.Config) ([]remote.RemoteFile, error) {
	return nil, nil
}

func (x *recordingTransport) FetchIndex(context.Context, remote.Config) ([]byte, error) {
	return nil, nil
}

func (x *recordingTransport) StoreIndex(context.Context, remote.Config, []byte) error { return nil }

func (r *recordingTransport) Name() string { return "recording" }

func (r *recordingTransport) Test(context.Context, remote.Config) (remote.TestResult, error) {
	return remote.TestResult{Entries: 1, Sample: []string{"2026"}}, r.err
}

func (r *recordingTransport) Upload(_ context.Context, cfg remote.Config, job remote.Job) (remote.Result, error) {
	r.cfg, r.dryRun = cfg, job.DryRun
	result := remote.Result{DryRun: job.DryRun, Sent: []string{}}
	for _, item := range job.Items {
		// The staged tree must hold readable photos at their final paths.
		if _, err := os.Stat(filepath.Join(job.Dir, item.Target)); err != nil {
			return result, err
		}
		r.staged = append(r.staged, item.Target)
		if item.Target != r.existing && !job.DryRun {
			result.Sent = append(result.Sent, item.Target)
			job.OnSent(item)
		}
	}
	return result, r.err
}

func TestRemoteSortStagesAndSends(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	writeJPEG(t, filepath.Join(source, "a.jpg"))
	var out bytes.Buffer
	srv := newServer(&out)
	transport := &recordingTransport{}
	srv.transport = transport
	cfg := &remote.Config{Protocol: remote.Daemon, Host: "nas", Module: "photos", Password: "pw"}
	ctx := context.Background()

	srv.handle(ctx, request{ID: "1", Op: "plan", Sources: []string{source}, Remote: cfg})
	srv.wg.Wait()
	var plan map[string]any
	for _, e := range events(t, &out, "1") {
		if e["type"] == "error" {
			t.Fatal(e["message"])
		}
		if e["type"] == "plan" {
			plan = e["plan"].(map[string]any)
		}
	}
	stage := plan["destination"].(string)
	if _, err := os.Stat(stage); err == nil {
		t.Fatal("scratch folder should not linger between plan and copy")
	}

	out.Reset()
	srv.handle(ctx, request{ID: "2", Op: "copy", PlanID: "1", Remote: cfg})
	srv.wg.Wait()
	got := events(t, &out, "2")
	last := got[len(got)-1]
	if last["type"] != "done" || last["remote"] != true || last["sent"] != float64(1) || len(transport.staged) != 1 {
		t.Fatalf("events %v staged %v", got, transport.staged)
	}
	if got[0]["type"] != "progress" || transport.cfg.Password != "pw" {
		t.Fatalf("events %v", got)
	}
	if _, err := os.Stat(stage); err == nil {
		t.Fatal("staged tree not cleaned up")
	}
	if _, err := os.Stat(filepath.Join(source, "a.jpg")); err != nil {
		t.Fatal("photo removed")
	}

	out.Reset()
	srv.handle(ctx, request{ID: "3", Op: "copy", PlanID: "1", Remote: cfg, DryRun: true})
	srv.wg.Wait()
	got = events(t, &out, "3")
	if !transport.dryRun || len(got) != 1 || got[0]["type"] != "done" || got[0]["dryRun"] != true {
		t.Fatalf("dry run events %v", got)
	}
}

func TestRemoteErrorsCarryACode(t *testing.T) {
	var out bytes.Buffer
	srv := newServer(&out)
	srv.transport = &recordingTransport{err: &remote.Error{Kind: remote.KindAuth, Detail: "auth failed"}}
	srv.handle(context.Background(), request{ID: "1", Op: "remoteTest", Remote: &remote.Config{Host: "nas", Module: "m"}})
	srv.wg.Wait()
	got := events(t, &out, "1")
	if len(got) != 1 || got[0]["type"] != "error" || got[0]["code"] != "auth" || got[0]["resumable"] != false {
		t.Fatalf("events %v", got)
	}
}
