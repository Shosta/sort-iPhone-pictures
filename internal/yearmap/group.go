// Package yearmap builds the "photos of the year" map: the geotagged photos of
// one year of a sorted library are grouped by place, each place becomes a pin
// on an offline base map (package mapbase) with one or two photos and its name,
// and the page is written as a PDF.
package yearmap

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"sort-iphone-pictures/internal/geo"
	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/scrapbook"
	"sort-iphone-pictures/sorter"
)

const (
	// sameNameKm: photos with the same city name farther apart than this are
	// different places (two towns called alike).
	sameNameKm = 100.0
	// anonymousCell is the size in degrees of the grid grouping photos with no
	// known city nearby.
	anonymousCell = 0.2
)

// Photo is a geotagged photo.
type Photo struct {
	Path string
	Day  time.Time
	// At is the time the photo was taken; the zero time when unknown.
	At  time.Time
	Pos mapbase.LatLon
}

// Place is a group of photos taken at the same place.
type Place struct {
	Name string
	Pos  mapbase.LatLon
	// Count is the number of photos taken there.
	Count int
	// Shown are the one or two photos pictured on the pin, oldest first.
	Shown []string
}

// Resolver names a position.
type Resolver interface {
	Nearest(lat, lon float64) (string, bool)
}

var yearFolder = regexp.MustCompile(`^\d{4}$`)

// Years lists the year folders (2026, 2025…) of a sorted library, newest first.
func Years(root string) []int {
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var years []int
	for _, item := range items {
		if !item.IsDir() || !yearFolder.MatchString(item.Name()) {
			continue
		}
		if year, err := strconv.Atoi(item.Name()); err == nil {
			years = append(years, year)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))
	return years
}

// Summary tells what was found for a year.
type Summary struct {
	Places []Place
	// Total is the number of photos of the year, Located those with a GPS
	// position.
	Total, Located int
}

// Collect reads the photos of year under root and groups the geotagged ones by
// place. progress (optional) is called after each photo read, from one
// goroutine at a time.
func Collect(ctx context.Context, root string, year int, progress func(done, total int)) (Summary, error) {
	photos, total, err := Locate(ctx, root, time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC), progress)
	if err != nil {
		return Summary{}, err
	}
	return Summary{Places: Group(photos, geo.Default()), Total: total, Located: len(photos)}, nil
}

// Locate reads the photos taken from one day to another (inclusive) under root
// and returns those with a GPS position, in chronological order, and the number
// of photos looked at. progress (optional) is called after each photo read,
// from one goroutine at a time.
func Locate(ctx context.Context, root string, from, to time.Time, progress func(done, total int)) ([]Photo, int, error) {
	found, err := scrapbook.Scan(ctx, root, from, to)
	if err != nil {
		return nil, 0, err
	}
	located := make([]*Photo, len(found))
	var next, done int
	var lock sync.Mutex
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), max(1, len(found))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				lock.Lock()
				i := next
				next++
				lock.Unlock()
				if i >= len(found) || ctx.Err() != nil {
					return
				}
				if metadata, err := sorter.ReadMetadata(ctx, found[i].Path); err == nil && metadata.HasGPS {
					located[i] = &Photo{Path: found[i].Path, Day: found[i].Day, At: metadata.TakenAt, Pos: mapbase.LatLon{Lat: metadata.Lat, Lon: metadata.Lon}}
				}
				lock.Lock()
				done++
				if progress != nil {
					progress(done, len(found))
				}
				lock.Unlock()
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	var photos []Photo
	for _, photo := range located {
		if photo != nil {
			photos = append(photos, *photo)
		}
	}
	return photos, len(found), nil
}

type group struct {
	name   string
	first  mapbase.LatLon
	photos []Photo
}

// Group gathers photos by place: photos named alike by resolver and within
// 100 km of each other form a place; photos with no name are gathered in cells
// of 0.2°. Places come most photographed first.
func Group(photos []Photo, resolver Resolver) []Place {
	var groups []*group
	byName := map[string][]*group{}
	for _, photo := range photos {
		name, named := resolver.Nearest(photo.Pos.Lat, photo.Pos.Lon)
		if !named {
			name = fmt.Sprintf("\x00%d,%d", int(math.Floor(photo.Pos.Lat/anonymousCell)), int(math.Floor(photo.Pos.Lon/anonymousCell)))
		}
		var target *group
		for _, candidate := range byName[name] {
			if !named || DistanceKm(candidate.first, photo.Pos) <= sameNameKm {
				target = candidate
				break
			}
		}
		if target == nil {
			target = &group{name: name, first: photo.Pos}
			groups = append(groups, target)
			byName[name] = append(byName[name], target)
		}
		target.photos = append(target.photos, photo)
	}
	places := make([]Place, len(groups))
	for i, g := range groups {
		sort.SliceStable(g.photos, func(a, b int) bool {
			if !g.photos[a].Day.Equal(g.photos[b].Day) {
				return g.photos[a].Day.Before(g.photos[b].Day)
			}
			return g.photos[a].Path < g.photos[b].Path
		})
		var lat, lon float64
		for _, photo := range g.photos {
			lat += photo.Pos.Lat
			lon += photo.Pos.Lon
		}
		n := float64(len(g.photos))
		pos := mapbase.LatLon{Lat: lat / n, Lon: lon / n}
		name := g.name
		if name[0] == 0 {
			name = Coordinates(pos)
		}
		places[i] = Place{Name: name, Pos: pos, Count: len(g.photos), Shown: pick(g.photos)}
	}
	sort.SliceStable(places, func(i, j int) bool {
		if places[i].Count != places[j].Count {
			return places[i].Count > places[j].Count
		}
		return places[i].Name < places[j].Name
	})
	return places
}

// pick chooses the photos to picture: the first and one from the middle of the
// stay, so two shots show different moments.
func pick(photos []Photo) []string {
	if len(photos) == 1 {
		return []string{photos[0].Path}
	}
	return []string{photos[0].Path, photos[len(photos)/2].Path}
}

// Coordinates writes a position as 48.85°N 2.35°E.
func Coordinates(p mapbase.LatLon) string {
	ns, ew := "N", "E"
	if p.Lat < 0 {
		ns = "S"
	}
	if p.Lon < 0 {
		ew = "W"
	}
	return fmt.Sprintf("%.2f°%s %.2f°%s", math.Abs(p.Lat), ns, math.Abs(p.Lon), ew)
}

// DistanceKm is the great-circle distance between two positions.
func DistanceKm(a, b mapbase.LatLon) float64 {
	const radius = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (b.Lat-a.Lat)*rad, (b.Lon-a.Lon)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * radius * math.Asin(math.Min(1, math.Sqrt(h)))
}
