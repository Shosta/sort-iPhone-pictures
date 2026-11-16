package thumbs

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/draw"
)

func gradient(rect image.Rectangle, ratio image.YCbCrSubsampleRatio) *image.YCbCr {
	img := image.NewYCbCr(rect, ratio)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.Y[img.YOffset(x, y)] = uint8((x*255/rect.Dx() + y*7) % 256)
			img.Cb[img.COffset(x, y)] = uint8(64 + y*128/rect.Dy())
			img.Cr[img.COffset(x, y)] = uint8(200 - x*100/rect.Dx())
		}
	}
	return img
}

func TestFitMatchesFullFilter(t *testing.T) {
	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422} {
		source := gradient(image.Rect(10, 6, 1610, 1206), ratio)
		fast := Fit(source, 100)
		if fast.Rect.Dx() != 100 || fast.Rect.Dy() != 75 {
			t.Fatalf("size %v", fast.Rect)
		}
		reference := image.NewNRGBA(fast.Rect)
		draw.CatmullRom.Scale(reference, reference.Rect, source, source.Rect, draw.Src, nil)
		var diff, n int
		for i := range fast.Pix {
			d := int(fast.Pix[i]) - int(reference.Pix[i])
			diff += max(d, -d)
			n++
		}
		if mean := float64(diff) / float64(n); mean > 4 {
			t.Errorf("%v: mean difference %.2f", ratio, mean)
		}
	}
}

func TestOrientRotates(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	rotated := Orient(img, 6)
	if rotated.Rect.Dx() != 2 || rotated.Rect.Dy() != 3 {
		t.Fatalf("size %v", rotated.Rect)
	}
	if got := rotated.NRGBAAt(1, 0); got.R != 255 {
		t.Errorf("top-left pixel should move to the top-right, got %v", got)
	}
}
