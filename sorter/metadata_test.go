package sorter

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writePhoto(t *testing.T, path string) {
	t.Helper()
	data := make([]byte, 96)
	copy(data, "II")
	binary.LittleEndian.PutUint16(data[2:], 42)
	binary.LittleEndian.PutUint32(data[4:], 8)
	binary.LittleEndian.PutUint16(data[8:], 2)
	putEntry := func(offset int, tag, kind uint16, count, value uint32) {
		binary.LittleEndian.PutUint16(data[offset:], tag)
		binary.LittleEndian.PutUint16(data[offset+2:], kind)
		binary.LittleEndian.PutUint32(data[offset+4:], count)
		binary.LittleEndian.PutUint32(data[offset+8:], value)
	}
	putEntry(10, 0x0110, 2, 7, 38)
	putEntry(22, 0x8769, 4, 1, 46)
	copy(data[38:], "iPhone\x00")
	binary.LittleEndian.PutUint16(data[46:], 1)
	putEntry(48, 0x9003, 2, 20, 66)
	copy(data[66:], "2026:10:20 12:34:56\x00")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.tiff")
	writePhoto(t, path)
	metadata, err := ReadMetadata(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Camera != "iPhone" || metadata.TakenAt.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}
