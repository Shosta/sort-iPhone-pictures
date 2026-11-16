package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	directory := t.TempDir()
	if err := generate(directory); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(directory, "photosort.png"))
	if err != nil {
		t.Fatal(err)
	}
	icon, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() != 512 || icon.Bounds().Dy() != 512 {
		t.Fatalf("unexpected PNG dimensions: %v", icon.Bounds())
	}
	if _, _, _, alpha := icon.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("PNG must preserve the transparent background")
	}
	if _, _, _, alpha := icon.At(256, 256).RGBA(); alpha == 0 {
		t.Fatal("PNG must contain the artwork")
	}
	icns, err := os.ReadFile(filepath.Join(directory, "photosort.icns"))
	if err != nil {
		t.Fatal(err)
	}
	if len(icns) < 8 || string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:8])) != len(icns) {
		t.Fatal("invalid ICNS header")
	}
	remaining := icns[8:]
	for _, expected := range []struct {
		kind string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}} {
		if len(remaining) < 8 || string(remaining[:4]) != expected.kind {
			t.Fatalf("missing ICNS representation %s", expected.kind)
		}
		length := int(binary.BigEndian.Uint32(remaining[4:8]))
		if length < 8 || length > len(remaining) {
			t.Fatal("invalid ICNS chunk length")
		}
		config, err := png.DecodeConfig(bytes.NewReader(remaining[8:length]))
		if err != nil || config.Width != expected.size || config.Height != expected.size {
			t.Fatalf("incorrect ICNS representation %s: %+v, %v", expected.kind, config, err)
		}
		remaining = remaining[length:]
	}
	if len(remaining) != 0 {
		t.Fatal("unexpected trailing ICNS data")
	}
}
