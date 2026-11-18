package sorter

import (
	"bytes"
	"image"
	"os"

	"sort-iphone-pictures/internal/thumbs"
)

// BlurryFolder receives blurry photos when Engine.GroupBlurry is set.
const BlurryFolder = "Blurry"

// DefaultBlurThreshold is the sharpness (variance of the Laplacian of the
// grayscale image, longest side blurSize) under which a photo is blurry. It is
// deliberately low: only clearly failed shots are flagged.
const DefaultBlurThreshold = 15.0

const blurSize = 512

// sharpnessOf decodes the photo at reduced size and returns its sharpness.
func sharpnessOf(path string) (float64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	decoded, err := thumbs.DecodeFor(bytes.NewReader(data), path, blurSize)
	if err != nil {
		return 0, false
	}
	return sharpness(decoded.Preview(blurSize))
}

// sharpness is the variance of the 3x3 Laplacian of the grayscale image: high
// for crisp edges, low for blur or flat content.
func sharpness(img image.Image) (float64, bool) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 3 || h < 3 {
		return 0, false
	}
	gray := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			gray[y*w+x] = (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
		}
	}
	var sum, squares float64
	n := float64((w - 2) * (h - 2))
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			laplacian := gray[i-w] + gray[i+w] + gray[i-1] + gray[i+1] - 4*gray[i]
			sum += laplacian
			squares += laplacian * laplacian
		}
	}
	mean := sum / n
	return squares/n - mean*mean, true
}
