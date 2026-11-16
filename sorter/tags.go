package sorter

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"

	"sort-iphone-pictures/internal/tagger"
)

// TagRunner analyses copied photos on a pool of goroutines, so tagging runs
// alongside the copy instead of slowing it down. Tagging is best effort: a
// failure never affects the copy.
type TagRunner struct {
	jobs chan Entry
	wg   sync.WaitGroup
	done atomic.Int64
}

// NewTagRunner starts one worker per tagger lane. onProgress (optional) is
// called after each photo with the number of photos tagged so far; it must be
// safe for concurrent use.
func NewTagRunner(ctx context.Context, tg *tagger.Tagger, destination string, onProgress func(done int)) *TagRunner {
	runner := &TagRunner{jobs: make(chan Entry, 4096)}
	for i := 0; i < tg.Workers(); i++ {
		runner.wg.Add(1)
		go func() {
			defer runner.wg.Done()
			for entry := range runner.jobs {
				if ctx.Err() != nil {
					continue
				}
				m := entry.Metadata
				tags, _ := tg.Tags(tagger.Photo{Path: entry.Source, TakenAt: m.TakenAt, Lat: m.Lat, Lon: m.Lon,
					HasGPS: m.HasGPS, UTCOffset: m.UTCOffset, HasOffset: m.HasOffset})
				if len(tags) > 0 {
					_ = tagger.WriteXMP(filepath.Join(destination, entry.Target)+".xmp", tags)
				}
				n := int(runner.done.Add(1))
				if onProgress != nil {
					onProgress(n)
				}
			}
		}()
	}
	return runner
}

// Submit queues a copied photo for analysis. Entries beyond the queue
// capacity block the caller until a worker is free.
func (r *TagRunner) Submit(entry Entry) { r.jobs <- entry }

// Wait blocks until every submitted photo has been analysed.
func (r *TagRunner) Wait() {
	close(r.jobs)
	r.wg.Wait()
}
