// Package thumbs renders JPEG thumbnails without any GUI dependency.
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

	"github.com/evanoberholster/imagemeta"
	_ "github.com/gen2brain/avif"
	_ "github.com/gen2brain/heic"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
)

const maxPixels = 100_000_000

// JPEG returns a JPEG thumbnail whose longest side is size pixels.
func JPEG(path string, size int) ([]byte, error) {
	img, err := render(path, size)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func render(path string, size int) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.ReadSeeker = file
	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		var preview []byte
		if _, err := file.Seek(0, 0); err != nil {
			return nil, err
		}
		if strings.EqualFold(filepath.Ext(path), ".cr3") {
			preview, err = imagemeta.PreviewCR3(file)
		} else {
			preview, err = imagemeta.PreviewTIFF(file, imagemeta.WithMaxPreviewLength(20<<20))
		}
		if err != nil {
			return nil, fmt.Errorf("thumbnail unavailable: %w", err)
		}
		reader = bytes.NewReader(preview)
		config, _, err = image.DecodeConfig(reader)
	}
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxPixels {
		return nil, fmt.Errorf("unsupported dimensions: %dx%d", config.Width, config.Height)
	}
	if _, err := reader.Seek(0, 0); err != nil {
		return nil, err
	}
	original, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	width, height := size, size
	if config.Width > config.Height {
		height = max(1, config.Height*size/config.Width)
	} else {
		width = max(1, config.Width*size/config.Height)
	}
	small := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(small, small.Bounds(), original, original.Bounds(), draw.Src, nil)
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".heic" && ext != ".heif" {
		if _, err := file.Seek(0, 0); err == nil {
			if metadata, err := imagemeta.Decode(file); err == nil {
				return orient(small, int(metadata.IFD0.Orientation)), nil
			}
		}
	}
	return small, nil
}

func orient(source image.Image, orientation int) image.Image {
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	targetWidth, targetHeight := width, height
	if orientation >= 5 && orientation <= 8 {
		targetWidth, targetHeight = height, width
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for row := 0; row < height; row++ {
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
			target.Set(x, y, source.At(column+source.Bounds().Min.X, row+source.Bounds().Min.Y))
		}
	}
	return target
}
