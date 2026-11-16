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
	"testing"
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
