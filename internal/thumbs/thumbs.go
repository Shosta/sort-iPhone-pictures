// Package thumbs decodes photos and renders small previews without any GUI
// dependency.
package thumbs

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sort-iphone-pictures/internal/jpeg8"

	"github.com/evanoberholster/imagemeta"
	_ "github.com/gen2brain/avif"
	_ "github.com/gen2brain/heic"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

const maxPixels = 100_000_000

// Decoded is a photo decoded at full size, as stored in the file.
type Decoded struct {
	Image image.Image
	// Format is the decoder used ("jpeg", "png"...); RAW files report the
	// format of their embedded preview.
	Format string
	// Orientation is the EXIF orientation (1 = as stored).
	Orientation int
}

// Preview returns the photo upright, scaled so its longest side is size.
func (d Decoded) Preview(size int) *image.NRGBA {
	return Orient(Fit(d.Image, size), d.Orientation)
}

// JPEG returns a JPEG thumbnail whose longest side is size pixels.
func JPEG(path string, size int) ([]byte, error) {
	img, err := Image(path, size)
	if err != nil {
		return nil, err
	}
	return Encode(img)
}

// Encode compresses a preview as JPEG.
func Encode(img image.Image) ([]byte, error) {
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Image returns the decoded, oriented image scaled so its longest side is size.
func Image(path string, size int) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoded, err := DecodeFor(file, path, size)
	if err != nil {
		return nil, err
	}
	return decoded.Preview(size), nil
}

// Analysis returns an upright preview for image analysis whose longest side is
// at most maxSide. Photos of at least 8*minSide pixels are decoded at 1/8
// scale (DC coefficients only, 2-3 times faster than a full decode) and keep
// that natural size when it already fits, so nothing is resampled twice.
func Analysis(path string, minSide, maxSide int) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoded, err := DecodeFor(file, path, minSide)
	if err != nil {
		return nil, err
	}
	bounds := decoded.Image.Bounds()
	if max(bounds.Dx(), bounds.Dy()) > maxSide {
		return decoded.Preview(maxSide), nil
	}
	plain := image.NewNRGBA(bounds.Sub(bounds.Min))
	draw.Draw(plain, plain.Bounds(), decoded.Image, bounds.Min, draw.Src)
	return Orient(plain, decoded.Orientation), nil
}

// Decode reads the full image from r; path is only used for its extension.
// RAW files fall back to their embedded JPEG preview.
func Decode(r io.ReadSeeker, path string) (Decoded, error) {
	return DecodeFor(r, path, 0)
}

// DecodeFor is Decode for a preview whose longest side is size pixels: JPEGs
// at least 8 times as large are decoded at 1/8 scale, several times faster.
func DecodeFor(r io.ReadSeeker, path string, size int) (Decoded, error) {
	reader := r
	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		var preview []byte
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return Decoded{}, err
		}
		if strings.EqualFold(filepath.Ext(path), ".cr3") {
			preview, err = imagemeta.PreviewCR3(r)
		} else {
			preview, err = imagemeta.PreviewTIFF(r, imagemeta.WithMaxPreviewLength(20<<20))
		}
		if err != nil {
			return Decoded{}, fmt.Errorf("thumbnail unavailable: %w", err)
		}
		reader = bytes.NewReader(preview)
		config, format, err = image.DecodeConfig(reader)
	}
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxPixels {
		return Decoded{}, fmt.Errorf("unsupported dimensions: %dx%d", config.Width, config.Height)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return Decoded{}, err
	}
	var img image.Image
	if format == "jpeg" && size > 0 && max(config.Width, config.Height)/8 >= size {
		img, err = jpeg8.Decode(reader)
		if err != nil {
			if _, err := reader.Seek(0, io.SeekStart); err != nil {
				return Decoded{}, err
			}
		}
	}
	if img == nil {
		if img, format, err = decodeFull(reader, format, config); err != nil {
			return Decoded{}, err
		}
	}
	decoded := Decoded{Image: img, Format: format, Orientation: 1}
	// HEIF decoders already apply the stored rotation.
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".heic" && ext != ".heif" {
		if _, err := r.Seek(0, io.SeekStart); err == nil {
			if metadata, err := imagemeta.Decode(r); err == nil {
				decoded.Orientation = int(metadata.IFD0.Orientation)
			}
		}
	}
	return decoded, nil
}

// Fit scales img so its longest side is size. Large images are first
// box-averaged by an integer factor, which is much faster than filtering the
// full-size image and avoids aliasing.
func Fit(img image.Image, size int) *image.NRGBA {
	bounds := img.Bounds()
	width, height := size, size
	if bounds.Dx() > bounds.Dy() {
		height = max(1, bounds.Dy()*size/bounds.Dx())
	} else {
		width = max(1, bounds.Dx()*size/bounds.Dy())
	}
	source := img
	if factor := min(bounds.Dx()/width, bounds.Dy()/height) / 2; factor >= 2 {
		source = shrink(img, factor)
	}
	small := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(small, small.Bounds(), source, source.Bounds(), draw.Src, nil)
	return small
}

// Orient applies an EXIF orientation to img.
func Orient(source *image.NRGBA, orientation int) *image.NRGBA {
	if orientation < 2 || orientation > 8 {
		return source
	}
	width, height := source.Rect.Dx(), source.Rect.Dy()
	targetWidth, targetHeight := width, height
	if orientation >= 5 {
		targetWidth, targetHeight = height, width
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for row := 0; row < height; row++ {
		from := source.Pix[row*source.Stride:]
		for column := 0; column < width; column++ {
			x, y := column, row
			switch orientation {
			case 2:
				x = width - 1 - column
			case 3:
				x, y = width-1-column, height-1-row
			case 4:
				y = height - 1 - row
			case 5:
				x, y = row, column
			case 6:
				x, y = height-1-row, column
			case 7:
				x, y = height-1-row, width-1-column
			case 8:
				x, y = row, width-1-column
			}
			copy(target.Pix[y*target.Stride+x*4:y*target.Stride+x*4+4], from[column*4:column*4+4])
		}
	}
	return target
}
