package sorter

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sort-iphone-pictures/internal/tagger"
)

func TestPhotosToTagAndTagPhotos(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg", "b.jpg.xmp", "notes.txt", ".hidden/c.jpg", "2026/d.HEIC"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	photos, err := PhotosToTag(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "2026/d.HEIC"), filepath.Join(root, "a.jpg")}
	if !reflect.DeepEqual(photos, want) {
		t.Fatalf("photos %v, want %v", photos, want)
	}

	tg, err := tagger.New("", tagger.Options{Moment: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	var done []int
	if err := TagPhotos(context.Background(), tg, photos, func(result TagResult) {
		if result.Total != 2 {
			t.Errorf("total %d", result.Total)
		}
		done = append(done, result.Done)
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(done, []int{1, 2}) {
		t.Fatalf("done %v", done)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := TagPhotos(ctx, tg, photos, nil); err != context.Canceled {
		t.Fatalf("cancelled run returned %v", err)
	}
}
