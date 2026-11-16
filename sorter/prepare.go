package sorter

import (
	"context"
	"runtime"
	"sync"
)

type preparedPhoto struct {
	metadata Metadata
	err      error
	fileSum  hashKey
	fileOK   bool
	pixelSum hashKey
	pixelOK  bool
	imageSum uint64
	imageOK  bool
}

func validDate(m Metadata) bool {
	return !m.TakenAt.IsZero() && m.TakenAt.Year() >= 1 && m.TakenAt.Year() <= 9999
}

// workerCount bounds parallel decodes: each can hold a large bitmap in memory.
func workerCount() int { return max(1, min(runtime.NumCPU(), 8)) }

// prepare runs work for every path on a worker pool and returns the results
// in the order of paths. Remaining paths are skipped once ctx is cancelled.
func prepare(ctx context.Context, paths []string, work func(string) preparedPhoto) []preparedPhoto {
	results := make([]preparedPhoto, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workerCount(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = work(paths[i])
			}
		}()
	}
	for i := range paths {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}
