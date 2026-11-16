package sorter

import (
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Duplicate is a source photo that is identical to another one and is
// therefore not planned for copy.
type Duplicate struct {
	Path string `json:"path"`
	Of   string `json:"of"`
	// InDestination is true when Of is already in the destination folder.
	InDestination bool `json:"inDestination"`
}

type hashKey [sha256.Size]byte

// deduper detects true duplicates: byte-identical files, and JPEG/PNG files
// whose decoded pixels are identical even when metadata differs. Byte-identical
// files already in the destination are considered too.
type deduper struct {
	destination string
	destLoaded  bool
	bySize      map[int64][]string // known files (destination) not yet hashed
	bytes       map[hashKey]string
	pixels      map[hashKey]string
	hashed      map[string]bool
}

func newDeduper(destination string) *deduper {
	return &deduper{
		destination: destination,
		bySize:      map[int64][]string{},
		bytes:       map[hashKey]string{},
		pixels:      map[hashKey]string{},
		hashed:      map[string]bool{},
	}
}

func (d *deduper) loadDestination() {
	if d.destLoaded {
		return
	}
	d.destLoaded = true
	filepath.WalkDir(d.destination, func(path string, item fs.DirEntry, err error) error {
		if err != nil || !item.Type().IsRegular() || !supported(path) {
			return nil
		}
		if info, err := item.Info(); err == nil {
			d.bySize[info.Size()] = append(d.bySize[info.Size()], path)
		}
		return nil
	})
}

// check returns the path of an identical earlier file, if any, and otherwise
// registers path so later files can be compared to it.
func (d *deduper) check(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	d.loadDestination()
	if known := d.bySize[info.Size()]; len(known) > 0 {
		for _, other := range known {
			d.register(other)
		}
		delete(d.bySize, info.Size())
	}
	if sum, ok := fileHash(path); ok {
		if other, found := d.bytes[sum]; found {
			return other, true
		}
		d.bytes[sum] = path
	}
	if sum, ok := pixelHash(path); ok {
		if other, found := d.pixels[sum]; found {
			return other, true
		}
		d.pixels[sum] = path
	}
	return "", false
}

func (d *deduper) register(path string) {
	if d.hashed[path] {
		return
	}
	d.hashed[path] = true
	if sum, ok := fileHash(path); ok {
		if _, found := d.bytes[sum]; !found {
			d.bytes[sum] = path
		}
	}
}

func fileHash(path string) (hashKey, bool) {
	var sum hashKey
	file, err := os.Open(path)
	if err != nil {
		return sum, false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return sum, false
	}
	copy(sum[:], hash.Sum(nil))
	return sum, true
}

func pixelHash(path string) (hashKey, bool) {
	var sum hashKey
	var decode func(io.Reader) (image.Image, error)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		decode = jpeg.Decode
	case ".png":
		decode = png.Decode
	default:
		return sum, false
	}
	file, err := os.Open(path)
	if err != nil {
		return sum, false
	}
	defer file.Close()
	img, err := decode(file)
	if err != nil {
		return sum, false
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	hash := sha256.New()
	var size [8]byte
	binary.BigEndian.PutUint32(size[:4], uint32(bounds.Dx()))
	binary.BigEndian.PutUint32(size[4:], uint32(bounds.Dy()))
	hash.Write(size[:])
	hash.Write(rgba.Pix)
	copy(sum[:], hash.Sum(nil))
	return sum, true
}
