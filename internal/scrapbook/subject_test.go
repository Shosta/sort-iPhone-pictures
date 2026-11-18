package scrapbook

import (
	"image"
	"image/color"
	idraw "image/draw"
	"testing"
)

func scene(w, h int, subject image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	idraw.Draw(img, img.Bounds(), &image.Uniform{color.NRGBA{120, 150, 200, 255}}, image.Point{}, idraw.Src)
	idraw.Draw(img, subject, &image.Uniform{color.NRGBA{224, 172, 140, 255}}, image.Point{}, idraw.Src)
	return img
}

func TestSubjectSquareFollowsThePerson(t *testing.T) {
	cases := map[string]struct {
		img  *image.NRGBA
		want func(offset float64) bool
	}{
		"left":   {scene(400, 200, image.Rect(10, 60, 70, 140)), func(o float64) bool { return o < 0.25 }},
		"right":  {scene(400, 200, image.Rect(330, 60, 390, 140)), func(o float64) bool { return o > 0.75 }},
		"top":    {scene(200, 400, image.Rect(60, 10, 140, 70)), func(o float64) bool { return o < 0.25 }},
		"bottom": {scene(200, 400, image.Rect(60, 330, 140, 390)), func(o float64) bool { return o > 0.75 }},
	}
	for name, c := range cases {
		b := c.img.Bounds()
		if offset := subjectOffset(c.img, b.Dx(), b.Dy()); !c.want(offset) {
			t.Errorf("%s: offset %.2f", name, offset)
		}
		if got := SubjectSquare(c.img).Bounds(); got.Dx() != got.Dy() || got.Dx() != min(b.Dx(), b.Dy()) {
			t.Errorf("%s: crop %v", name, got)
		}
	}
}
