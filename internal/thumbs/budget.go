package thumbs

import (
	"container/list"
	"image"
	"io"
	"runtime"
	"sync"
)

// HEIC and AVIF are decoded by WebAssembly modules. Measured on a 24 MP iPhone
// photo and on resampled copies of it, a decode needs about 36 bytes per pixel
// (430 MB for 12 MP, 850 MB for 24 MP, 1.3 to 2.2 GB for 48 MP), and the module
// keeps the memory of the largest photo it has seen. Left alone, one decode per
// sorter worker took 5 GB with eight 24 MP photos and 9 to 10 GB with eight
// 48 MP ones. Two limits bring this down to about 3 GB and 4 GB:
//
//   - the decodes run on wasmSlots long-lived goroutines. The decoder keeps its
//     modules in a sync.Pool, one per scheduler thread, so decodes spread over
//     all the workers also spread the modules, and each one holds on to its
//     memory until the garbage collector drops it;
//   - the estimated memory of the decodes in flight is capped to wasmBudget, so
//     large photos do not run side by side. A photo that costs more runs alone.
const (
	wasmBytesPerPixel = 36
	wasmBudget        = 2 << 30
	wasmSlots         = 4
)

var (
	wasmDecodes    = newBudget(wasmBudget)
	wasmJobs       = make(chan func())
	startWasmSlots sync.Once
)

// decodeFull decodes the whole image. HEIC and AVIF go through the bounded
// WebAssembly decoders.
func decodeFull(r io.Reader, format string, config image.Config) (img image.Image, name string, err error) {
	if format != "heic" && format != "avif" {
		return image.Decode(r)
	}
	runWasm(int64(config.Width)*int64(config.Height)*wasmBytesPerPixel, func() {
		img, name, err = image.Decode(r)
	})
	return img, name, err
}

// runWasm runs decode on one of the decoder goroutines once weight fits in the
// memory budget. A panic in decode is raised again in the caller.
func runWasm(weight int64, decode func()) {
	startWasmSlots.Do(func() {
		for range min(wasmSlots, runtime.GOMAXPROCS(0)) {
			go func() {
				for job := range wasmJobs {
					job()
				}
			}()
		}
	})
	done := make(chan any, 1)
	wasmJobs <- func() {
		defer func() { done <- recover() }()
		defer wasmDecodes.acquire(weight)()
		decode()
	}
	if p := <-done; p != nil {
		panic(p)
	}
}

// budget is a first-in first-out weighted semaphore: acquire waits until the
// requested weight fits next to what is already held.
type budget struct {
	mu       sync.Mutex
	capacity int64
	used     int64
	waiting  list.List
}

type waiter struct {
	weight int64
	ready  chan struct{}
}

func newBudget(capacity int64) *budget { return &budget{capacity: capacity} }

// acquire blocks until weight is available and returns the function that gives
// it back. A weight above the capacity is capped to it, so that it can run
// alone instead of waiting forever. Requests are served in arrival order, so a
// large one is not starved by a stream of small ones.
func (b *budget) acquire(weight int64) (release func()) {
	weight = min(max(weight, 1), b.capacity)
	b.mu.Lock()
	if b.waiting.Len() == 0 && b.used+weight <= b.capacity {
		b.used += weight
		b.mu.Unlock()
		return b.releaser(weight)
	}
	turn := &waiter{weight, make(chan struct{})}
	b.waiting.PushBack(turn)
	b.mu.Unlock()
	<-turn.ready
	return b.releaser(weight)
}

func (b *budget) releaser(weight int64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used -= weight
			for front := b.waiting.Front(); front != nil; front = b.waiting.Front() {
				turn := front.Value.(*waiter)
				if b.used+turn.weight > b.capacity {
					break
				}
				b.used += turn.weight
				b.waiting.Remove(front)
				close(turn.ready)
			}
			b.mu.Unlock()
		})
	}
}
