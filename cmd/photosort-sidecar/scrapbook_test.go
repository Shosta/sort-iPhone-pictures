package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScrapbookWeekToPDF(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "Photos Classées")
	inWeek := []string{
		filepath.Join(library, "2026", "2026-05", "2026-05-12 - iPhone - 1.jpg"),
		filepath.Join(library, "2026", "2026-05", "2026-05-14 12.00.00 - Almost Duplicates", "2026-05-14 - iPhone - 2.jpg"),
	}
	for _, path := range append([]string{filepath.Join(library, "2026", "2026-05", "2026-05-20 - iPhone - 1.jpg")}, inWeek...) {
		writeJPEG(t, path)
	}
	broken := filepath.Join(library, "2026", "2026-05", "2026-05-13 - iPhone - 1.jpg")
	if err := os.WriteFile(broken, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	srv := newServer(&out)
	ctx := context.Background()
	srv.handle(ctx, request{ID: "days", Op: "weekDays", Root: library, From: "2026-05-04", To: "2026-06-14"})
	srv.handle(ctx, request{ID: "photos", Op: "weekPhotos", Root: library, From: "2026-05-11", To: "2026-05-17"})
	srv.handle(ctx, request{ID: "bad", Op: "weekPhotos", Root: library, From: "2026-05-17", To: "2026-05-11"})
	srv.wg.Wait()

	days := events(t, &out, "days")
	if len(days) != 1 || days[0]["type"] != "weekDays" || days[0]["rootOk"] != true {
		t.Fatalf("days = %v", days)
	}
	counts := days[0]["days"].(map[string]any)
	if counts["2026-05-12"] != 1.0 || counts["2026-05-14"] != 1.0 || counts["2026-05-20"] != 1.0 || counts["2026-05-13"] != 1.0 || len(counts) != 4 {
		t.Errorf("counts = %v", counts)
	}
	photos := events(t, &out, "photos")
	if len(photos) != 1 || photos[0]["type"] != "photos" || len(photos[0]["photos"].([]any)) != 3 {
		t.Fatalf("photos = %v", photos)
	}
	if bad := events(t, &out, "bad"); len(bad) != 1 || bad[0]["type"] != "error" {
		t.Errorf("a reversed range should be refused, got %v", bad)
	}

	files := append([]string{broken}, inWeek...)
	srv.handle(ctx, request{ID: "layout", Op: "scrapbookLayout", Files: files, Page: "a4-landscape", LongSide: 65, Gutter: 2})
	srv.wg.Wait()
	layout := events(t, &out, "layout")
	if len(layout) != 1 || layout[0]["type"] != "layout" || layout[0]["pageWidth"] != 297.0 || layout[0]["pageHeight"] != 210.0 {
		t.Fatalf("layout = %v", layout)
	}
	sheets := layout[0]["sheets"].([]any)
	placements := sheets[0].(map[string]any)["placements"].([]any)
	if len(sheets) != 1 || len(placements) != 2 {
		t.Fatalf("sheets = %v", sheets)
	}
	if failed := layout[0]["failed"].([]any); len(failed) != 1 || failed[0].(map[string]any)["path"] != broken {
		t.Errorf("failed = %v", failed)
	}
	first := placements[0].(map[string]any)
	if first["w"].(float64) != 65 || first["h"].(float64) != 48.75 {
		t.Errorf("a 4:3 photo should be 65 x 48.75 mm, got %v", first)
	}

	output := filepath.Join(root, "Téléchargements")
	srv.handle(ctx, request{ID: "pdf", Op: "scrapbook", Files: files, Page: "a4-landscape", LongSide: 65, Gutter: 5, OutDir: output, Name: "Semaine 2026-S20.pdf"})
	srv.handle(ctx, request{ID: "empty", Op: "scrapbook", OutDir: output})
	srv.wg.Wait()
	pdf := events(t, &out, "pdf")
	done := pdf[len(pdf)-1]
	if done["type"] != "done" || done["pages"] != 1.0 || done["photos"] != 2.0 || len(done["failed"].([]any)) != 1 || done["fallback"] != false {
		t.Fatalf("pdf events = %v", pdf)
	}
	path := done["path"].(string)
	if path != filepath.Join(output, "Semaine 2026-S20.pdf") {
		t.Errorf("path = %s", path)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("PDF not written: %v", err)
	}
	if empty := events(t, &out, "empty"); len(empty) != 1 || empty[0]["type"] != "error" {
		t.Errorf("an empty selection should be refused, got %v", empty)
	}
}
