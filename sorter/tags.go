package sorter

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"sort-iphone-pictures/internal/tagger"
)

// TagResult reports one analysed photo.
type TagResult struct {
	Path  string
	Tags  []string
	Err   error
	Done  int
	Total int
}

// PhotosToTag lists, in path order, the supported photos under root (hidden
// folders and symbolic links excluded) that have no <photo>.xmp sidecar yet.
func PhotosToTag(ctx context.Context, root string) ([]string, error) {
	var photos []string
	err := filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if item.IsDir() {
			if path != root && len(item.Name()) > 1 && item.Name()[0] == '.' {
				return filepath.SkipDir
			}
			return nil
		}
		if !item.Type().IsRegular() || !supported(path) {
			return nil
		}
		if _, err := os.Lstat(path + ".xmp"); err == nil {
			return nil
		}
		photos = append(photos, path)
		return nil
	})
	sort.Strings(photos)
	return photos, err
}

// TagPhotos analyses photos on a pool of goroutines and writes the
// tags of each to <photo>.xmp, never overwriting an existing file. Photos are
// never modified. onTagged (optional) is called after each photo, one call at
// a time. It returns ctx.Err() if cancelled.
func TagPhotos(ctx context.Context, tg *tagger.Tagger, paths []string, onTagged func(TagResult)) error {
	jobs := make(chan string)
	var (
		wg   sync.WaitGroup
		lock sync.Mutex
		done int
	)
	// Twice as many workers as model lanes, so decoding overlaps inference.
	for range min(2*tg.Workers(), max(1, len(paths))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				m, _ := ReadMetadata(ctx, path)
				tags, err := tg.Tags(tagger.Photo{Path: path, TakenAt: m.TakenAt, Lat: m.Lat, Lon: m.Lon,
					HasGPS: m.HasGPS, UTCOffset: m.UTCOffset, HasOffset: m.HasOffset, Shot: m.Shot})
				if err == nil && len(tags) > 0 {
					err = tagger.WriteXMP(path+".xmp", tags)
				}
				lock.Lock()
				done++
				if onTagged != nil {
					onTagged(TagResult{Path: path, Tags: tags, Err: err, Done: done, Total: len(paths)})
				}
				lock.Unlock()
			}
		}()
	}
feed:
	for _, path := range paths {
		select {
		case jobs <- path:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}
