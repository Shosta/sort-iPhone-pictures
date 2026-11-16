package jpeg8

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// TestMatchesBlockAverage checks that each output pixel is the average of
// the corresponding 8×8 block of the fully decoded image, for baseline
// (4:2:0 and gray) and progressive (4:2:0, 4:4:4 with restarts) files.
func TestMatchesBlockAverage(t *testing.T) {
	files := map[string][]byte{"baseline420": encode(t, false), "gray": encode(t, true)}
	fixtures, _ := filepath.Glob("testdata/*.jpg")
	if len(fixtures) == 0 {
		t.Fatal("no fixtures")
	}
	for _, path := range fixtures {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(path)] = data
	}
	for name, data := range files {
		full, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(name, err)
		}
		small, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(name, err)
		}
		width, height := full.Bounds().Dx(), full.Bounds().Dy()
		if got, want := small.Bounds(), image.Rect(0, 0, (width+7)/8, (height+7)/8); got != want {
			t.Fatalf("%s: bounds %v, want %v", name, got, want)
		}
		var total, n float64
		for by := 0; by < height/8; by++ {
			for bx := 0; bx < width/8; bx++ {
				var sum [3]float64
				for y := by * 8; y < by*8+8; y++ {
					for x := bx * 8; x < bx*8+8; x++ {
						r, g, b, _ := full.At(x, y).RGBA()
						sum[0], sum[1], sum[2] = sum[0]+float64(r>>8), sum[1]+float64(g>>8), sum[2]+float64(b>>8)
					}
				}
				r, g, b, _ := small.At(bx, by).RGBA()
				for c, v := range []uint32{r >> 8, g >> 8, b >> 8} {
					diff := sum[c]/64 - float64(v)
					total += max(diff, -diff)
					n++
				}
			}
		}
		if mean := total / n; mean > 4 {
			t.Errorf("%s: mean difference %.2f with the block average", name, mean)
		}
	}
}

func encode(t *testing.T, gray bool) []byte {
	const width, height = 333, 251
	random := rand.New(rand.NewSource(1))
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rgba.Set(x, y, color.RGBA{uint8(x * 255 / width), uint8(y * 255 / height), uint8(random.Intn(64) + (x*y)%128), 255})
		}
	}
	var source image.Image = rgba
	if gray {
		g := image.NewGray(rgba.Bounds())
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				g.Set(x, y, rgba.At(x, y))
			}
		}
		source = g
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, source, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
