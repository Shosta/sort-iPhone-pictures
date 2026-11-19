package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sort-iphone-pictures/internal/mapbase"
)

func TestYearMapOps(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "Photos Classées")
	photo := filepath.Join(library, "2026", "2026-05", "2026-05-12 - iPhone - 1.jpg")
	writeJPEG(t, photo)
	if err := os.MkdirAll(filepath.Join(library, "2024"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	srv := newServer(&out)
	ctx := context.Background()
	places := []yearPlace{
		{Name: "Paris", Lat: 48.85, Lon: 2.35, Count: 3, Shown: []string{photo}},
		{Name: "Roskilde", Lat: 55.64, Lon: 12.08, Count: 1, Shown: []string{photo}},
	}
	srv.handle(ctx, request{ID: "years", Op: "yearMapYears", Root: library})
	srv.handle(ctx, request{ID: "scan", Op: "yearMapScan", Root: library, Year: 2026})
	srv.handle(ctx, request{ID: "layout", Op: "yearMapLayout", Places: places, Page: "a4-portrait"})
	srv.handle(ctx, request{ID: "none", Op: "yearMapLayout", Page: "a4-portrait"})
	srv.handle(ctx, request{ID: "page", Op: "yearMapLayout", Places: places, Page: "nope"})
	srv.handle(ctx, request{ID: "pdf", Op: "yearMapPdf", Places: places, Page: "a4-landscape", Title: "2026", Subtitle: "4 photos", Name: "Carte 2026.pdf", OutDir: filepath.Join(root, "out")})
	srv.wg.Wait()

	years := events(t, &out, "years")
	if len(years) != 1 || years[0]["rootOk"] != true || len(years[0]["years"].([]any)) != 2 || years[0]["years"].([]any)[0] != 2026.0 {
		t.Fatalf("years = %v", years)
	}
	scan := events(t, &out, "scan")
	last := scan[len(scan)-1]
	if last["type"] != "places" || last["total"] != 1.0 || last["located"] != 0.0 {
		t.Fatalf("a photo without GPS should be counted but not placed, got %v", scan)
	}
	layout := events(t, &out, "layout")
	if len(layout) != 1 || layout[0]["pageWidth"] != 210.0 || layout[0]["pageHeight"] != 297.0 {
		t.Fatalf("layout = %v", layout)
	}
	pins := layout[0]["pins"].([]any)
	if len(pins) != 2 || !strings.HasPrefix(layout[0]["land"].(string), "M") || pins[0].(map[string]any)["card"] == nil {
		t.Errorf("pins = %v", pins)
	}
	for _, id := range []string{"none", "page"} {
		if got := events(t, &out, id); len(got) != 1 || got[0]["type"] != "error" {
			t.Errorf("%s should be refused, got %v", id, got)
		}
	}
	pdf := events(t, &out, "pdf")
	done := pdf[len(pdf)-1]
	path, _ := done["path"].(string)
	if done["type"] != "done" || filepath.Base(path) != "Carte 2026.pdf" {
		t.Fatalf("pdf = %v", pdf)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("not a PDF: %v", err)
	}
}

func TestYearMapOnlineBackdropAndFallback(t *testing.T) {
	saved := tileProviders
	defer func() { tileProviders = saved }()
	var out bytes.Buffer
	srv := newServer(&out)
	ctx := context.Background()
	places := []yearPlace{{Name: "Paris", Lat: 48.85, Lon: 2.35, Count: 1, Shown: []string{"x.jpg"}}}

	// An unreachable tile server falls back to the offline map.
	tileProviders = []mapbase.Provider{&mapbase.TileServer{Key: "osm", Credit: "© test", URL: "http://127.0.0.1:1/{z}/{x}/{y}.png", Zoom: 19}}
	srv.handle(ctx, request{ID: "down", Op: "yearMapLayout", Places: places, Page: "a4-landscape", Provider: "osm"})
	srv.wg.Wait()
	down := events(t, &out, "down")
	if len(down) != 1 || down[0]["backdropError"] == nil || down[0]["backdrop"] != nil || down[0]["land"] == "" {
		t.Fatalf("down = %v", down)
	}

	tiles := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 256, 256)))
	}))
	defer tiles.Close()
	tileProviders = []mapbase.Provider{&mapbase.TileServer{Key: "osm", Credit: "© test", URL: tiles.URL + "/{z}/{x}/{y}.png", Zoom: 19}}
	srv.handle(ctx, request{ID: "up", Op: "yearMapLayout", Places: places, Page: "a4-landscape", Provider: "osm"})
	srv.wg.Wait()
	up := events(t, &out, "up")
	if len(up) != 1 || !strings.HasPrefix(up[0]["backdrop"].(string), "data:image/jpeg") || up[0]["credit"] != "© test" || up[0]["land"] != nil {
		t.Fatalf("up = %v", up)
	}
}

func TestYearMapBlockedProviderFallsBack(t *testing.T) {
	saved := tileProviders
	defer func() { tileProviders = saved }()
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusForbidden)
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 256, 256)))
	}))
	defer blocked.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 256, 256)))
	}))
	defer good.Close()
	dir := t.TempDir()
	tileProviders = []mapbase.Provider{
		&mapbase.TileServer{Key: "esritopo", Label: "Esri Topo", Credit: "© good", URL: good.URL + "/{z}/{x}/{y}.png", Zoom: 19, CacheDir: dir},
		&mapbase.TileServer{Key: "osm", Label: "OSM", Credit: "© osm", URL: blocked.URL + "/{z}/{x}/{y}.png", Zoom: 19, CacheDir: dir},
	}
	var out bytes.Buffer
	srv := newServer(&out)
	places := []yearPlace{{Name: "Paris", Lat: 48.85, Lon: 2.35, Count: 1, Shown: []string{"x.jpg"}}}
	srv.handle(context.Background(), request{ID: "b", Op: "yearMapLayout", Places: places, Page: "a4-landscape", Provider: "osm"})
	srv.wg.Wait()
	ev := events(t, &out, "b")
	if len(ev) != 1 || ev[0]["credit"] != "© good" || ev[0]["backdropError"] == nil || ev[0]["backdrop"] == nil {
		t.Fatalf("ev = %v", ev)
	}
	if _, err := os.Stat(filepath.Join(dir, "v2", "osm")); err == nil {
		t.Fatal("error tiles were cached")
	}
}
