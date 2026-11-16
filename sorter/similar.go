package sorter

import (
	"context"
	"image"
	"math/bits"
	"sort"
	"time"
)

const (
	similarDistance = 8
	similarWindow   = 10 * time.Minute
	similarSuffix   = " - Almost Duplicates"
)

type candidate struct {
	path     string
	metadata Metadata
	month    string
	hash     uint64
	hashOK   bool
}

// dHash is a 64-bit difference hash: robust to small changes of framing,
// exposure and compression, so successive shots of one scene land close.
func dHash(img image.Image) (uint64, bool) {
	const w, h = 9, 8
	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		return 0, false
	}
	var gray [h][w]float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			x0, x1 := bounds.Min.X+x*bounds.Dx()/w, bounds.Min.X+(x+1)*bounds.Dx()/w
			y0, y1 := bounds.Min.Y+y*bounds.Dy()/h, bounds.Min.Y+(y+1)*bounds.Dy()/h
			x1, y1 = max(x1, x0+1), max(y1, y0+1)
			var sum float64
			var n int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					r, g, b, _ := img.At(xx, yy).RGBA()
					sum += 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
					n++
				}
			}
			gray[y][x] = sum / float64(n)
		}
	}
	var hash uint64
	for y := 0; y < h; y++ {
		for x := 0; x < w-1; x++ {
			hash <<= 1
			if gray[y][x] > gray[y][x+1] {
				hash |= 1
			}
		}
	}
	return hash, true
}

// similarGroups returns, for each candidate, the name of its Almost
// Duplicates folder ("" when the photo has no close sibling). Photos are
// linked when taken within similarWindow of each other with similar hashes.
func similarGroups(ctx context.Context, items []candidate) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hashes := make([]uint64, len(items))
	ok := make([]bool, len(items))
	for i, item := range items {
		hashes[i], ok[i] = item.hash, item.hashOK
	}
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return items[order[a]].metadata.TakenAt.Before(items[order[b]].metadata.TakenAt) })
	parent := make([]int, len(items))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for a := 0; a < len(order); a++ {
		i := order[a]
		if !ok[i] {
			continue
		}
		for b := a + 1; b < len(order); b++ {
			j := order[b]
			if items[j].metadata.TakenAt.Sub(items[i].metadata.TakenAt) > similarWindow {
				break
			}
			if ok[j] && items[i].month == items[j].month && bits.OnesCount64(hashes[i]^hashes[j]) <= similarDistance {
				parent[find(j)] = find(i)
			}
		}
	}
	first := map[int]int{}
	size := map[int]int{}
	for _, i := range order {
		root := find(i)
		if _, found := first[root]; !found {
			first[root] = i
		}
		size[root]++
	}
	names := make([]string, len(items))
	for i := range items {
		root := find(i)
		if size[root] > 1 {
			names[i] = items[first[root]].metadata.TakenAt.Format("2006-01-02 15.04.05") + similarSuffix
		}
	}
	return names, nil
}
