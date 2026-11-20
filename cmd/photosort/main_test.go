package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sort-iphone-pictures/internal/remote"
)

func TestRunPreviewJSON(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	destination := filepath.Join(base, "dest")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "invalid.jpg"), []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if err := run(context.Background(), []string{"--source", source, "--destination", destination, "--json"}, &output, &diagnostics); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Applied bool
		Copied  int
		Plan    struct{ Entries, Skipped []json.RawMessage }
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Copied != 0 || len(result.Plan.Entries) != 1 || len(result.Plan.Skipped) != 0 {
		t.Fatalf("unexpected JSON: %s", output.String())
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("preview created destination: %v", err)
	}
}

func TestRunFlags(t *testing.T) {
	for _, args := range [][]string{nil, {"--unknown"}, {"unexpected"}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output, &output); err == nil {
			t.Errorf("expected failure for %v", args)
		}
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &output, &output); err != nil {
		t.Fatal(err)
	}
}

type fakeTransport struct {
	cfg    remote.Config
	dryRun bool
	calls  int
}

func (f *fakeTransport) Name() string { return "fake" }

func (f *fakeTransport) Test(_ context.Context, cfg remote.Config) (remote.TestResult, error) {
	f.cfg = cfg
	return remote.TestResult{Entries: 3}, nil
}

func (f *fakeTransport) Upload(_ context.Context, cfg remote.Config, job remote.Job) (remote.Result, error) {
	f.cfg, f.dryRun = cfg, job.DryRun
	f.calls++
	sent := []string{}
	for _, item := range job.Items {
		sent = append(sent, item.Target)
	}
	return remote.Result{DryRun: job.DryRun, Sent: sent}, nil
}

func TestRunRemote(t *testing.T) {
	fake := &fakeTransport{}
	previous := transport
	transport = fake
	defer func() { transport = previous }()
	source := filepath.Join(t.TempDir(), "source")
	os.Mkdir(source, 0755)
	os.WriteFile(filepath.Join(source, "a.jpg"), []byte("not an image"), 0600)
	passwordFile := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(passwordFile, []byte("secret\n"), 0600)
	url := "rsync://me@nas/photos/Library"

	var output, diagnostics bytes.Buffer
	if err := run(context.Background(), []string{"--source", source, "--remote", url}, &output, &diagnostics); err != nil || fake.calls != 0 {
		t.Fatalf("preview must not connect: %v", err)
	}
	output.Reset()
	if err := run(context.Background(), []string{"--source", source, "--remote", url, "--remote-dry-run", "--remote-password-file", passwordFile, "--remote-verify"}, &output, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if !fake.dryRun || fake.cfg.Password != "secret" || !fake.cfg.Checksum || fake.cfg.Module != "photos" || !strings.Contains(output.String(), "Would send: 1") {
		t.Fatalf("dry run: %+v\n%s", fake.cfg, output.String())
	}
	output.Reset()
	if err := run(context.Background(), []string{"--source", source, "--remote", url, "--apply", "--json"}, &output, &diagnostics); err != nil || fake.dryRun || fake.calls != 2 || strings.Contains(output.String(), "secret") {
		t.Fatalf("apply: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := run(context.Background(), []string{"--remote", url, "--remote-test"}, &output, &diagnostics); err != nil || !strings.Contains(output.String(), "3 entries") {
		t.Fatalf("test: %v %s", err, output.String())
	}
	for _, args := range [][]string{
		{"--source", source, "--remote", url, "--destination", "/x"},
		{"--source", source, "--remote", url, "--tag-meta"},
		{"--source", source, "--destination", "/x", "--remote-verify"},
		{"--source", source, "--remote", "rsync://u:pw@nas/m"},
	} {
		if err := run(context.Background(), args, &output, &diagnostics); err == nil {
			t.Errorf("expected failure for %v", args)
		}
	}
}
