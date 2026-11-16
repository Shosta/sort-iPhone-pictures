package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"

	"sort-iphone-pictures/internal/appicon"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./packaging/icon OUTPUT_DIRECTORY")
		os.Exit(2)
	}
	if err := generate(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	source, err := png.Decode(bytes.NewReader(appicon.PNG))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	icns := make([]byte, 8)
	copy(icns, "icns")
	for _, representation := range []struct {
		size int
		kind string
	}{
		{16, "icp4"},
		{32, "icp5"},
		{64, "icp6"},
		{128, "ic07"},
		{256, "ic08"},
		{512, "ic09"},
		{1024, "ic10"},
	} {
		icon := image.NewNRGBA(image.Rect(0, 0, representation.size, representation.size))
		draw.CatmullRom.Scale(icon, icon.Bounds(), source, source.Bounds(), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, icon); err != nil {
			return err
		}
		if representation.size == 512 {
			if err := os.WriteFile(filepath.Join(directory, "photosort.png"), encoded.Bytes(), 0644); err != nil {
				return err
			}
		}
		chunk := make([]byte, 8)
		copy(chunk, representation.kind)
		binary.BigEndian.PutUint32(chunk[4:8], uint32(8+encoded.Len()))
		icns = append(icns, chunk...)
		icns = append(icns, encoded.Bytes()...)
	}
	binary.BigEndian.PutUint32(icns[4:8], uint32(len(icns)))
	return os.WriteFile(filepath.Join(directory, "photosort.icns"), icns, 0644)
}
