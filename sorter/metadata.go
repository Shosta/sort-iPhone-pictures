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
	return Metadata{TakenAt: data.OriginalDate(), Camera: data.IFD0.Model}, nil
}
