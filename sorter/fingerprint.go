package sorter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"sort-iphone-pictures/internal/thumbs"
)

// PreviewSize bounds the longest side of the previews handed to
// Engine.Preview.
const PreviewSize = 256

// Files up to this size are read once into memory to be hashed and decoded.
const maxInMemory = 64 << 20

// fingerprint holds what duplicate and near-duplicate detection need.
type fingerprint struct {
	fileSum  hashKey
	fileOK   bool
	pixelSum hashKey
	pixelOK  bool
	imageSum uint64
	imageOK  bool
}

type fingerprintOptions struct {
	bytes, pixels, perceptual bool
	// hash overrides the perceptual hash (tests).
	hash    func(string) (uint64, bool)
	preview func(string, image.Image)
}

// fingerprintOf reads the photo once to compute the requested hashes:
//   - bytes: SHA-256 of the file;
//   - pixels: for JPEG, SHA-256 of the image data without metadata segments
//     (same photo with different EXIF/XMP), for PNG, of the decoded pixels;
//   - perceptual: dHash, from the Exif thumbnail when there is one.
//
// Only photos without usable Exif thumbnail (and PNGs) are fully decoded.
// Each decoded or embedded preview is handed to options.preview.
func fingerprintOf(path string, options fingerprintOptions) (result fingerprint) {
	ext := strings.ToLower(filepath.Ext(path))
	isJPEG, isPNG := ext == ".jpg" || ext == ".jpeg", ext == ".png"
	perceptual := options.perceptual && options.hash == nil
	if options.perceptual && options.hash != nil {
		result.imageSum, result.imageOK = options.hash(path)
	}
	pngPixels := options.pixels && isPNG
	if !options.bytes && !perceptual && !pngPixels && !(options.pixels && isJPEG) {
		return result
	}
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return result
	}
	var data []byte
	if info.Size() <= maxInMemory {
		if data, err = io.ReadAll(file); err != nil {
			return result
		}
		if options.bytes {
			result.fileSum, result.fileOK = sha256.Sum256(data), true
		}
	} else if options.bytes {
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return result
		}
		copy(result.fileSum[:], hash.Sum(nil))
		result.fileOK = true
	}
	if isJPEG && data != nil {
		var exif []byte
		content := sha256.New()
		content.Write([]byte("jpeg"))
		complete := thumbs.ScanJPEG(data, func(marker byte, segment []byte) {
			switch {
			case marker == 0xE1 && exif == nil && len(segment) > 10 && string(segment[4:10]) == "Exif\x00\x00":
				exif = segment[10:]
			case (marker >= 0xE0 && marker <= 0xEF && marker != 0xEE) || marker == 0xFE:
				// Metadata (Exif, XMP, ICC, JFIF...) and comments.
			case options.pixels:
				content.Write(segment)
			}
		})
		if complete && options.pixels {
			copy(result.pixelSum[:], content.Sum(nil))
			result.pixelOK = true
		}
		if perceptual {
			if thumbnail, ok := thumbs.FromExif(exif); ok {
				result.imageSum, result.imageOK = dHash(thumbnail)
				if options.preview != nil {
					options.preview(path, thumbnail)
				}
				perceptual = false
			}
		}
	}
	if !perceptual && !pngPixels {
		return result
	}
	var reader io.ReadSeeker = file
	if data != nil {
		reader = bytes.NewReader(data)
	} else if _, err := file.Seek(0, io.SeekStart); err != nil {
		return result
	}
	var decoded thumbs.Decoded
	if pngPixels {
		decoded, err = thumbs.Decode(reader, path)
	} else {
		decoded, err = thumbs.DecodeFor(reader, path, PreviewSize)
	}
	if err != nil {
		return result
	}
	if pngPixels && decoded.Format == "png" {
		result.pixelSum, result.pixelOK = pixelSum(decoded.Image), true
	}
	if perceptual || options.preview != nil {
		small := decoded.Preview(PreviewSize)
		if perceptual {
			result.imageSum, result.imageOK = dHash(small)
		}
		if options.preview != nil {
			options.preview(path, small)
		}
	}
	return result
}

// pixelSum hashes decoded pixels as RGBA, a strip at a time.
func pixelSum(img image.Image) hashKey {
	bounds := img.Bounds()
	hash := sha256.New()
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(bounds.Dx()))
	binary.BigEndian.PutUint32(header[4:], uint32(bounds.Dy()))
	hash.Write(header[:])
	const strip = 64
	buffer := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), strip))
	for y := 0; y < bounds.Dy(); y += strip {
		rows := min(strip, bounds.Dy()-y)
		draw.Draw(buffer, image.Rect(0, 0, bounds.Dx(), rows), img, image.Pt(bounds.Min.X, bounds.Min.Y+y), draw.Src)
		hash.Write(buffer.Pix[:rows*buffer.Stride])
	}
	var sum hashKey
	copy(sum[:], hash.Sum(nil))
	return sum
}

// workerCount bounds parallel decodes: each can hold a large bitmap in memory.
func workerCount() int { return max(1, min(runtime.NumCPU(), 8)) }

// parallel runs work(i) for every i in [0, n) on a worker pool and reports
// each finished index on the returned channel, which is closed once all
// started work is done. No new work starts after ctx is cancelled.
func parallel(ctx context.Context, n int, work func(int)) <-chan int {
	finished := make(chan int, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(workerCount(), max(n, 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				work(i)
				finished <- i
			}
		}()
	}
	go func() {
		defer func() {
			close(jobs)
			wg.Wait()
			close(finished)
		}()
		for i := 0; i < n; i++ {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	return finished
}
