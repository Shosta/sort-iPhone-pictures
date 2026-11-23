package thumbs

import (
	"bytes"
	"image"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBudgetCapsWeightInFlight(t *testing.T) {
	b := newBudget(10)
	var held, peak atomic.Int64
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := b.acquire(3)
			now := held.Add(3)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			held.Add(-3)
			release()
		}()
	}
	wg.Wait()
	if got := peak.Load(); got > 10 || got < 6 {
		t.Errorf("peak weight %d, want between 6 and 10", got)
	}
	if b.used != 0 {
		t.Errorf("%d still held", b.used)
	}
}

func TestBudgetRunsOversizedAlone(t *testing.T) {
	b := newBudget(10)
	small := b.acquire(1)
	entered := make(chan func())
	go func() { entered <- b.acquire(100) }()
	select {
	case <-entered:
		t.Fatal("an oversized request started next to another")
	case <-time.After(20 * time.Millisecond):
	}
	small()
	big := <-entered
	if b.used != 10 {
		t.Errorf("oversized request holds %d, want the whole capacity", b.used)
	}
	big()
	if b.used != 0 {
		t.Errorf("%d still held", b.used)
	}
}

func TestBudgetServesInArrivalOrder(t *testing.T) {
	b := newBudget(10)
	first := b.acquire(8)
	large := make(chan func())
	go func() { large <- b.acquire(5) }()
	for { // wait until the large request is queued
		b.mu.Lock()
		queued := b.waiting.Len()
		b.mu.Unlock()
		if queued == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	small := make(chan func())
	go func() { small <- b.acquire(1) }()
	select {
	case <-small:
		t.Fatal("a small request overtook the queued large one")
	case <-time.After(20 * time.Millisecond):
	}
	first()
	releaseLarge := <-large
	releaseSmall := <-small
	releaseLarge()
	releaseSmall()
	if b.used != 0 {
		t.Errorf("%d still held", b.used)
	}
}

func TestBudgetReleaseIsIdempotent(t *testing.T) {
	b := newBudget(10)
	release := b.acquire(4)
	other := b.acquire(4)
	release()
	release()
	if b.used != 4 {
		t.Errorf("held %d after releasing twice, want 4", b.used)
	}
	other()
}

func TestRunWasmRaisesPanicsAndReleases(t *testing.T) {
	defer func() {
		if recover() != "boom" {
			t.Error("the panic of the decoder did not reach the caller")
		}
		if wasmDecodes.used != 0 {
			t.Errorf("%d of the budget still held", wasmDecodes.used)
		}
	}()
	runWasm(1, func() { panic("boom") })
}

func decodeFixture(t *testing.T, data []byte) error {
	t.Helper()
	_, err := DecodeFor(bytes.NewReader(data), "tiny.heic", 64)
	return err
}

func TestDecodeHEICReleasesBudget(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.heic")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(bytes.NewReader(data), "tiny.heic")
	if err != nil {
		t.Fatal(err)
	}
	if b := decoded.Image.Bounds(); b.Dx() != 96 || b.Dy() != 64 || decoded.Format != "heic" {
		t.Fatalf("decoded a %v %s image", b, decoded.Format)
	}

	// The bytes after the header are the compressed picture: cut it short so
	// that the size is read but the decode fails.
	cut := data[:len(data)-100]
	if _, _, err := image.DecodeConfig(bytes.NewReader(cut)); err != nil {
		t.Fatalf("the header of the truncated file should still parse: %v", err)
	}
	if err := decodeFixture(t, cut); err == nil {
		t.Error("a truncated file decoded")
	}

	var wg sync.WaitGroup
	for range 3 * wasmSlots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := decodeFixture(t, data); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if wasmDecodes.used != 0 {
		t.Errorf("%d of the budget still held", wasmDecodes.used)
	}
}
