package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTravelOps(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "Photos Classées")
	photo := filepath.Join(library, "2026", "2026-07", "2026-07-02 - iPhone - 1.jpg")
	writeJPEG(t, photo)

	var out bytes.Buffer
	srv := newServer(&out)
	ctx := context.Background()
	steps := []travelStep{
		{Name: "Paris", Lat: 48.85, Lon: 2.35, Count: 3, First: "2026-07-01", Last: "2026-07-02", Photo: photo, Key: true},
		{Name: "Roma", Lat: 41.9, Lon: 12.5, Count: 1, First: "2026-07-05", Last: "2026-07-05", Photo: photo, Key: true},
	}
	srv.handle(ctx, request{ID: "trips", Op: "travelTrips", Root: library})
	srv.handle(ctx, request{ID: "scan", Op: "travelScan", Root: library, From: "2026-07-01", To: "2026-07-14"})
	srv.handle(ctx, request{ID: "badscan", Op: "travelScan", Root: library, From: "2026-07-14", To: "2026-07-01"})
	srv.handle(ctx, request{ID: "layout", Op: "travelLayout", Steps: steps, Page: "a4-portrait", Lang: "en"})
	srv.handle(ctx, request{ID: "none", Op: "travelLayout", Page: "a4-portrait"})
	srv.handle(ctx, request{ID: "baddate", Op: "travelLayout", Steps: []travelStep{{Name: "X", First: "x"}}, Page: "a4-portrait"})
	srv.handle(ctx, request{ID: "pdf", Op: "travelPdf", Steps: steps, Page: "a4-landscape", Title: "Summer", Subtitle: "2 steps", Name: "Voyage.pdf", OutDir: filepath.Join(root, "out")})
	srv.wg.Wait()

	trips := events(t, &out, "trips")
	if last := trips[len(trips)-1]; last["type"] != "trips" || last["total"] != 1.0 || len(last["trips"].([]any)) != 0 {
		t.Fatalf("a photo without GPS should give no trip, got %v", trips)
	}
	scan := events(t, &out, "scan")
	if last := scan[len(scan)-1]; last["type"] != "trip" || len(last["trip"].(map[string]any)["steps"].([]any)) != 0 {
		t.Fatalf("scan = %v", scan)
	}
	layout := events(t, &out, "layout")
	if len(layout) != 1 || layout[0]["pageWidth"] != 210.0 {
		t.Fatalf("layout = %v", layout)
	}
	stops := layout[0]["stops"].([]any)
	first := stops[0].(map[string]any)
	if len(stops) != 2 || first["n"] != 1.0 || first["dates"] != "Jul 1 – Jul 2" || first["card"] == nil || layout[0]["route"].(string)[0] != 'M' || layout[0]["distanceKm"].(float64) < 1000 {
		t.Errorf("stops = %v", layout[0])
	}
	for _, id := range []string{"badscan", "none", "baddate"} {
		if got := events(t, &out, id); len(got) != 1 || got[0]["type"] != "error" {
			t.Errorf("%s should be refused, got %v", id, got)
		}
	}
	pdf := events(t, &out, "pdf")
	done := pdf[len(pdf)-1]
	path, _ := done["path"].(string)
	if done["type"] != "done" || filepath.Base(path) != "Voyage.pdf" {
		t.Fatalf("pdf = %v", pdf)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("not a PDF: %v", err)
	}
}
