package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
		Plan    struct{ Skipped []json.RawMessage }
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Copied != 0 || len(result.Plan.Skipped) != 1 {
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
