package thumbs

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
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

func TestAnalysisKeepsOneEighthSize(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, rect image.Rectangle) string {
		path := filepath.Join(dir, name)
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := jpeg.Encode(file, gradient(rect, image.YCbCrSubsampleRatio420), nil); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// 1/8 scale is large enough: the natural 1/8 size is kept, not resampled.
	img, err := Analysis(write("large.jpg", image.Rect(0, 0, 3200, 2400)), 300, 640)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 300 {
		t.Errorf("large photo: got %v, want 400x300", b)
	}
	// Too small for 1/8 scale: decoded fully and fitted to maxSide.
	img, err = Analysis(write("small.jpg", image.Rect(0, 0, 1000, 500)), 300, 640)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 320 {
		t.Errorf("small photo: got %v, want 640x320", b)
	}
	// Above maxSide even at 1/8 scale: fitted down.
	img, err = Analysis(write("huge.jpg", image.Rect(0, 0, 8000, 4000)), 300, 640)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 320 {
		t.Errorf("huge photo: got %v, want 640x320", b)
	}
}
