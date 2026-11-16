package sorter

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evanoberholster/imagemeta"
)

type Metadata struct {
	TakenAt time.Time
	Camera  string
	Lat     float64
	Lon     float64
	HasGPS  bool
	// UTCOffset is the capture time zone offset in seconds, valid if HasOffset.
	UTCOffset int
	HasOffset bool
}

type MetadataReader func(context.Context, string) (Metadata, error)

func ReadMetadata(ctx context.Context, path string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer file.Close()
	data, err := imagemeta.Decode(file)
	if err != nil {
		return Metadata{}, fmt.Errorf("read EXIF: %w", err)
	}
	lat, lon := data.GPS.Latitude(), data.GPS.Longitude()
	hasGPS := (lat != 0 || lon != 0) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
	metadata := Metadata{TakenAt: data.OriginalDate(), Camera: data.IFD0.Model, Lat: lat, Lon: lon, HasGPS: hasGPS}
	if loc := data.ExifIFD.OffsetTimeOriginal; loc != nil && !metadata.TakenAt.IsZero() {
		wall := metadata.TakenAt
		_, metadata.UTCOffset = time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, loc).Zone()
		metadata.HasOffset = true
	}
	return metadata, nil
}
