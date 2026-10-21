package desktop

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
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

func thumbnail(path string) (image.Image, error) {
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
			return nil, fmt.Errorf("miniature indisponible : %w", err)
		}
		reader = bytes.NewReader(preview)
		config, _, err = image.DecodeConfig(reader)
	}
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 100_000_000 {
		return nil, fmt.Errorf("dimensions non prises en charge : %dx%d", config.Width, config.Height)
	}
	if _, err := reader.Seek(0, 0); err != nil {
		return nil, err
	}
	original, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	width, height := 96, 96
	if config.Width > config.Height {
		height = max(1, config.Height*96/config.Width)
	} else {
		width = max(1, config.Width*96/config.Height)
	}
	small := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(small, small.Bounds(), original, original.Bounds(), draw.Src, nil)
	if _, err := file.Seek(0, 0); err == nil {
		metadata, err := imagemeta.Decode(file)
		if err == nil && !strings.EqualFold(filepath.Ext(path), ".heic") && !strings.EqualFold(filepath.Ext(path), ".heif") {
			return orient(small, int(metadata.IFD0.Orientation)), nil
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
			xPosition, yPosition := column, row
			switch orientation {
			case 2:
				xPosition = width - 1 - column
			case 3:
				xPosition, yPosition = width-1-column, height-1-row
			case 4:
				yPosition = height - 1 - row
			case 5:
				xPosition, yPosition = row, column
			case 6:
				xPosition, yPosition = height-1-row, column
			case 7:
				xPosition, yPosition = height-1-row, width-1-column
			case 8:
				xPosition, yPosition = row, width-1-column
			}
			target.Set(xPosition, yPosition, source.At(column+source.Bounds().Min.X, row+source.Bounds().Min.Y))
		}
	}
	return target
}
