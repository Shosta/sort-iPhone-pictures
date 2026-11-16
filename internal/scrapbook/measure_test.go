package scrapbook

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// writeOriented writes a JPEG of width x height pixels carrying an Exif
// orientation tag.
func writeOriented(t *testing.T, path string, width, height int, orientation uint16) {
	t.Helper()
	tiff := make([]byte, 26)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:], 42)
	binary.LittleEndian.PutUint32(tiff[4:], 8)
	binary.LittleEndian.PutUint16(tiff[8:], 1)
	binary.LittleEndian.PutUint16(tiff[10:], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:], 3)
	binary.LittleEndian.PutUint32(tiff[14:], 1)
	binary.LittleEndian.PutUint16(tiff[18:], orientation)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	segment := append([]byte("Exif\x00\x00"), tiff...)
	var data bytes.Buffer
	data.Write([]byte{0xff, 0xd8, 0xff, 0xe1, byte((len(segment) + 2) >> 8), byte(len(segment) + 2)})
	data.Write(segment)
	data.Write(encoded.Bytes()[2:])
	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRatioFollowsExifOrientation(t *testing.T) {
	dir := t.TempDir()
	for orientation, want := range map[uint16]float64{1: 4.0 / 3.0, 6: 3.0 / 4.0, 8: 3.0 / 4.0, 3: 4.0 / 3.0} {
		path := filepath.Join(dir, "photo.jpg")
		writeOriented(t, path, 400, 300, orientation)
		got, err := Ratio(path)
		if err != nil {
			t.Fatal(err)
		}
		if got < want-0.01 || got > want+0.01 {
			t.Errorf("orientation %d: ratio %.3f, want %.3f", orientation, got, want)
		}
	}
}
