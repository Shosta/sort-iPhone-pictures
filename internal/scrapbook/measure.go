package scrapbook

import (
	"context"
	"fmt"
	"image"
	"os"
	"runtime"
	"sync"

	"sort-iphone-pictures/internal/thumbs"

	"github.com/evanoberholster/imagemeta"
)

// previewSize is the size of the preview decoded to measure photos whose
// header does not tell their upright dimensions (HEIC, RAW, TIFF...).
const previewSize = 512

// Measurer computes the upright width/height ratio of photos, on a pool of
// goroutines, and remembers it as long as the file is unchanged.
type Measurer struct {
	cache sync.Map
}

type measureKey struct {
	path string
	size int64
	time int64
}

// Measure returns the ratio of each photo, and its error if it cannot be read
// (the ratio is then 0). Results come back in the order of paths.
func (m *Measurer) Measure(ctx context.Context, paths []string) ([]float64, []error) {
	ratios := make([]float64, len(paths))
	errs := make([]error, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), max(1, len(paths))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ratios[i], errs[i] = m.ratio(paths[i])
			}
		}()
	}
feed:
	for i := range paths {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		for i := range errs {
			if ratios[i] == 0 && errs[i] == nil {
				errs[i] = err
			}
		}
	}
	return ratios, errs
}

func (m *Measurer) ratio(path string) (float64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	key := measureKey{path, info.Size(), info.ModTime().UnixNano()}
	if cached, ok := m.cache.Load(key); ok {
		return cached.(float64), nil
	}
	ratio, err := Ratio(path)
	if err != nil {
		return 0, err
	}
	m.cache.Store(key, ratio)
	return ratio, nil
}

// Ratio is the width/height of the photo once upright. JPEG and PNG files are
// measured from their header, others from a small decoded preview.
func Ratio(path string) (float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if config, format, err := image.DecodeConfig(file); err == nil && (format == "jpeg" || format == "png") && config.Width > 0 && config.Height > 0 {
		width, height := config.Width, config.Height
		if _, err := file.Seek(0, 0); err == nil {
			if metadata, err := imagemeta.Decode(file); err == nil && metadata.IFD0.Orientation >= 5 {
				width, height = height, width
			}
		}
		return float64(width) / float64(height), nil
	}
	img, err := thumbs.Image(path, previewSize)
	if err != nil {
		return 0, err
	}
	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		return 0, fmt.Errorf("empty image")
	}
	return float64(bounds.Dx()) / float64(bounds.Dy()), nil
}
