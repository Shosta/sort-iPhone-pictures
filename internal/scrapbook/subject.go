package scrapbook

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

// saliencySize is the size of the thumbnail the subject is searched in.
const saliencySize = 96

// SubjectSquare crops img to the largest square that best contains the
// subject. People come first (skin tones, which is what faces and hands are),
// then whatever stands out from the picture by its detail and colour. The
// search needs no model and runs on a small thumbnail.
func SubjectSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	side := min(w, h)
	if w == h || side == 0 {
		return img
	}
	offset := subjectOffset(img, w, h)
	rect := image.Rect(0, 0, side, side)
	if w > h {
		rect = rect.Add(image.Pt(int(math.Round(offset*float64(w-side))), 0))
	} else {
		rect = rect.Add(image.Pt(0, int(math.Round(offset*float64(h-side)))))
	}
	rect = rect.Add(bounds.Min)
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	xdraw.Draw(out, out.Bounds(), img, rect.Min, xdraw.Src)
	return out
}

// subjectOffset returns where, from 0 to 1, the square sits along the long
// axis of the photo.
func subjectOffset(img image.Image, w, h int) float64 {
	scale := float64(saliencySize) / float64(max(w, h))
	tw, th := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	small := image.NewNRGBA(image.Rect(0, 0, tw, th))
	xdraw.BiLinear.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Src, nil)

	weights := saliency(small)
	horizontal := tw > th
	length, side := th, th
	if horizontal {
		length = tw
	} else {
		side = tw
	}
	// profile[i] is the weight of the column (or row) i.
	profile := make([]float64, length)
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			if horizontal {
				profile[x] += weights[y*tw+x]
			} else {
				profile[y] += weights[y*tw+x]
			}
		}
	}
	travel := length - side
	if travel <= 0 {
		return 0.5
	}
	best, bestScore := travel/2, math.Inf(-1)
	window := 0.0
	for i := 0; i < side; i++ {
		window += profile[i]
	}
	for start := 0; start <= travel; start++ {
		if start > 0 {
			window += profile[start+side-1] - profile[start-1]
		}
		// A gentle pull towards the middle breaks ties and avoids extremes.
		centre := 1 - 0.1*math.Abs(float64(start)-float64(travel)/2)/(float64(travel)/2)
		if score := window * centre; score > bestScore {
			best, bestScore = start, score
		}
	}
	return float64(best) / float64(travel)
}

// saliency weighs every pixel: skin tones count most, then local detail and
// strong colour.
func saliency(img *image.NRGBA) []float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]float64, w*h)
	luma := func(x, y int) float64 {
		c := img.NRGBAAt(min(max(x, 0), w-1), min(max(y, 0), h-1))
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			gx := luma(x+1, y) - luma(x-1, y)
			gy := luma(x, y+1) - luma(x, y-1)
			detail := math.Min(1, math.Hypot(gx, gy)/80)
			hi, lo := math.Max(float64(c.R), math.Max(float64(c.G), float64(c.B))), math.Min(float64(c.R), math.Min(float64(c.G), float64(c.B)))
			colourful := (hi - lo) / 255
			out[y*w+x] = 1 + 4*skin(c) + 2*detail + colourful
		}
	}
	return out
}

// skin reports 1 for a skin tone, from the usual chroma ranges, else 0.
func skin(c color.NRGBA) float64 {
	_, cb, cr := color.RGBToYCbCr(c.R, c.G, c.B)
	if cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173 {
		return 1
	}
	return 0
}
