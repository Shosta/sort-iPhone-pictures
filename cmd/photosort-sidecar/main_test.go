package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sort-iphone-pictures/sorter"
)

// writeJPEG writes a small JPEG whose Exif says it was taken on 2026-10-20.
func writeJPEG(t *testing.T, path string) {
	t.Helper()
	tiff := make([]byte, 96)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:], 42)
	binary.LittleEndian.PutUint32(tiff[4:], 8)
	binary.LittleEndian.PutUint16(tiff[8:], 1)
	putEntry := func(offset int, tag, kind uint16, count, value uint32) {
		binary.LittleEndian.PutUint16(tiff[offset:], tag)
		binary.LittleEndian.PutUint16(tiff[offset+2:], kind)
		binary.LittleEndian.PutUint32(tiff[offset+4:], count)
		binary.LittleEndian.PutUint32(tiff[offset+8:], value)
	}
	putEntry(10, 0x8769, 4, 1, 26)
	binary.LittleEndian.PutUint16(tiff[26:], 1)
	putEntry(28, 0x9003, 2, 20, 46)
	copy(tiff[46:], "2026:10:20 12:34:56\x00")
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 90, 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	segment := append([]byte("Exif\x00\x00"), tiff...)
	var data bytes.Buffer
	data.Write([]byte{0xff, 0xd8, 0xff, 0xe1, byte((len(segment) + 2) >> 8), byte(len(segment) + 2)})
	data.Write(segment)
	data.Write(encoded.Bytes()[2:])
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func events(t *testing.T, out *bytes.Buffer, id string) []map[string]any {
	t.Helper()
	var result []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	scanner.Buffer(nil, 16<<20)
	for scanner.Scan() {
		var e map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e["id"] == id {
			result = append(result, e)
		}
	}
	return result
}

func TestPlanStreamsDraftsAndCachesPreviews(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	writeJPEG(t, filepath.Join(source, "a.jpg"))
	writeJPEG(t, filepath.Join(source, "b.jpg"))
	var out bytes.Buffer
	srv := newServer(&out)
	ctx := context.Background()
	srv.handle(ctx, request{ID: "1", Op: "plan", Sources: []string{source}, Destination: filepath.Join(base, "dest"), Dedupe: true, Similar: true})
	srv.wg.Wait()

	planEvents := events(t, &out, "1")
	var drafted, duplicates int
	var stages []string
	var first string
	for _, e := range planEvents {
		switch e["type"] {
		case "draft":
			if entries, ok := e["entries"].([]any); ok {
				drafted += len(entries)
				if first == "" {
					first = entries[0].(map[string]any)["source"].(string)
				}
			}
			if dups, ok := e["duplicates"].([]any); ok {
				duplicates += len(dups)
			}
		case "status":
			stages = append(stages, e["stage"].(string))
		case "error":
			t.Fatal(e["message"])
		}
	}
	if last := planEvents[len(planEvents)-1]; last["type"] != "plan" {
		t.Fatalf("last event %v", last)
	}
	if drafted != 2 || duplicates != 1 {
		t.Fatalf("drafted %d entries and %d duplicates", drafted, duplicates)
	}
	if len(stages) == 0 || stages[0] != "exif" || !slices.Contains(stages, "hash") {
		t.Fatalf("stages %v", stages)
	}

	// The preview decoded for near-duplicates was cached while planning: it
	// is served even once the photo is gone.
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	srv.handle(ctx, request{ID: "2", Op: "thumbnail", Path: first, Size: 64})
	srv.wg.Wait()
	if got := events(t, &out, "2"); len(got) != 1 || got[0]["type"] != "thumbnail" {
		t.Fatalf("thumbnail events %v", got)
	}
}

// planPieces splits the events of a request into the lines of its plan
// chunks (as raw encoded sizes) and its closing plan event.
type planPieces struct {
	entries    []map[string]any
	skipped    []map[string]any
	duplicates []map[string]any
	chunks     int
	maxLine    int
	final      map[string]any
}

func readPlanPieces(t *testing.T, out *bytes.Buffer, id string) planPieces {
	t.Helper()
	var pieces planPieces
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	scanner.Buffer(nil, 64<<20)
	for scanner.Scan() {
		var e map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e["id"] != id || (e["type"] != "planChunk" && e["type"] != "plan") {
			continue
		}
		if pieces.final != nil {
			t.Fatalf("event %v after the plan", e["type"])
		}
		if e["type"] == "plan" {
			pieces.final = e
			continue
		}
		pieces.chunks++
		pieces.maxLine = max(pieces.maxLine, len(scanner.Bytes()))
		for key, into := range map[string]*[]map[string]any{"entries": &pieces.entries, "skipped": &pieces.skipped, "duplicates": &pieces.duplicates} {
			list, _ := e[key].([]any)
			for _, item := range list {
				*into = append(*into, item.(map[string]any))
			}
		}
	}
	if pieces.final == nil {
		t.Fatal("no plan event")
	}
	return pieces
}

func TestPlanChunksReassembleToThePlan(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	writeJPEG(t, filepath.Join(source, "a.jpg"))
	writeJPEG(t, filepath.Join(source, "b.jpg"))
	if err := os.WriteFile(filepath.Join(source, "broken.jpg"), []byte("not a photo"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv := newServer(&out)
	srv.handle(context.Background(), request{ID: "1", Op: "plan", Sources: []string{source}, Destination: filepath.Join(base, "dest"), Dedupe: true})
	srv.wg.Wait()

	plan := srv.plans["1"]
	if len(plan.Entries) == 0 || len(plan.Duplicates) == 0 {
		t.Fatalf("plan has %d entries and %d duplicates", len(plan.Entries), len(plan.Duplicates))
	}
	pieces := readPlanPieces(t, &out, "1")
	if len(pieces.entries) != len(plan.Entries) || len(pieces.duplicates) != len(plan.Duplicates) || len(pieces.skipped) != len(plan.Skipped) {
		t.Fatalf("chunks hold %d/%d/%d items, plan has %d/%d/%d", len(pieces.entries), len(pieces.skipped), len(pieces.duplicates), len(plan.Entries), len(plan.Skipped), len(plan.Duplicates))
	}
	for i, entry := range plan.Entries {
		got := pieces.entries[i]
		if got["source"] != entry.Source || got["target"] != entry.Target {
			t.Fatalf("entry %d: %v, want %s -> %s", i, got, entry.Source, entry.Target)
		}
		if _, ok := got["metadata"]; ok {
			t.Fatalf("entry %d carries metadata", i)
		}
	}
	for i, duplicate := range plan.Duplicates {
		if got := pieces.duplicates[i]; got["path"] != duplicate.Path || got["of"] != duplicate.Of {
			t.Fatalf("duplicate %d: %v, want %v", i, got, duplicate)
		}
	}
	for i, issue := range plan.Skipped {
		if got := pieces.skipped[i]; got["path"] != issue.Path || got["reason"] != issue.Reason {
			t.Fatalf("skipped %d: %v, want %v", i, got, issue)
		}
	}
	final := pieces.final["plan"].(map[string]any)
	if final["destination"] != plan.Destination {
		t.Fatalf("destination %v, want %s", final["destination"], plan.Destination)
	}
	if _, ok := final["entries"]; ok {
		t.Fatal("the plan event still carries entries")
	}
	counts := pieces.final["counts"].(map[string]any)
	if counts["entries"] != float64(len(plan.Entries)) || counts["skipped"] != float64(len(plan.Skipped)) || counts["duplicates"] != float64(len(plan.Duplicates)) {
		t.Fatalf("counts %v", counts)
	}

	// The copy still works from the full plan kept in the sidecar.
	out.Reset()
	srv.handle(context.Background(), request{ID: "2", Op: "copy", PlanID: "1"})
	srv.wg.Wait()
	got := events(t, &out, "2")
	if last := got[len(got)-1]; last["type"] != "done" {
		t.Fatalf("copy events %v", got)
	}
}

func TestLargePlanIsSentInBoundedChunks(t *testing.T) {
	plan := sorter.Plan{Destination: "/dest"}
	for i := range 5000 {
		name := strings.Repeat("x", i%40)
		plan.Entries = append(plan.Entries, sorter.Entry{Source: "/src/" + name + ".jpg", Target: "/dest/2026/05/" + name + ".jpg"})
	}
	for i := range 30 {
		plan.Skipped = append(plan.Skipped, sorter.Issue{Path: strings.Repeat("s", 100), Reason: "unreadable " + string(rune('a'+i%26))})
		plan.Duplicates = append(plan.Duplicates, sorter.Duplicate{Path: strings.Repeat("d", 100), Of: "/src/a.jpg"})
	}
	var out bytes.Buffer
	srv := newServer(&out)
	srv.sendPlan("1", plan)
	pieces := readPlanPieces(t, &out, "1")
	if pieces.chunks < 4 || len(pieces.entries) != 5000 || len(pieces.skipped) != 30 || len(pieces.duplicates) != 30 {
		t.Fatalf("%d chunks, %d entries, %d skipped, %d duplicates", pieces.chunks, len(pieces.entries), len(pieces.skipped), len(pieces.duplicates))
	}
	if pieces.maxLine > planChunkBytes {
		t.Fatalf("a chunk takes %d bytes", pieces.maxLine)
	}
	for i, entry := range plan.Entries {
		if pieces.entries[i]["source"] != entry.Source {
			t.Fatalf("entry %d out of order", i)
		}
	}

	// Long paths shrink the chunks instead of growing the lines.
	out.Reset()
	long := sorter.Plan{Destination: "/dest"}
	for i := range 200 {
		path := strings.Repeat("p", 8000)
		long.Entries = append(long.Entries, sorter.Entry{Source: path + string(rune('a'+i%26)), Target: path})
	}
	srv.sendPlan("2", long)
	pieces = readPlanPieces(t, &out, "2")
	if pieces.chunks < 10 || len(pieces.entries) != 200 {
		t.Fatalf("%d chunks, %d entries", pieces.chunks, len(pieces.entries))
	}
	if pieces.maxLine > planChunkBytes+20000 {
		t.Fatalf("a chunk takes %d bytes", pieces.maxLine)
	}

	// An empty plan is a plan event and nothing else.
	out.Reset()
	srv.sendPlan("3", sorter.Plan{Destination: "/dest"})
	pieces = readPlanPieces(t, &out, "3")
	if pieces.chunks != 0 || pieces.final["counts"].(map[string]any)["entries"] != float64(0) {
		t.Fatalf("empty plan: %v", pieces)
	}
}

func TestTagSkipsTaggedPhotos(t *testing.T) {
	folder := t.TempDir()
	writeJPEG(t, filepath.Join(folder, "a.jpg"))
	writeJPEG(t, filepath.Join(folder, "b.jpg"))
	if err := os.WriteFile(filepath.Join(folder, "b.jpg.xmp"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv := newServer(&out)
	srv.handle(context.Background(), request{ID: "1", Op: "tag", Folder: folder, TagMoment: true})
	srv.wg.Wait()
	got := events(t, &out, "1")
	if len(got) != 3 || got[0]["type"] != "status" || got[1]["type"] != "tagged" || got[2]["type"] != "done" {
		t.Fatalf("events %v", got)
	}
	if got[1]["path"] != filepath.Join(folder, "a.jpg") || got[2]["total"] != float64(1) {
		t.Fatalf("events %v", got)
	}
}
