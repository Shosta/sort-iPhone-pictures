// Package travelmap builds the "travel map": the geotagged photos of a trip are
// ordered in time, gathered in steps (one per stay), and drawn as a route on the
// offline base map (package mapbase) in the style of a travel notebook, with
// numbered steps, a photo and the place name for the key ones, the dates and
// the approximate distance. The page is written as a PDF.
//
// Photos are read and named with package yearmap, which this package reuses.
package travelmap

import (
	"math"
	"sort"
	"time"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/yearmap"
)

const (
	// sameStayKm: consecutive photos named alike and within this distance of the
	// first one of the stay belong to the same step; unnamed photos must be
	// within unnamedStayKm.
	sameStayKm    = 100.0
	unnamedStayKm = 20.0
	// AwayKm is how far from home a day must be to belong to a trip.
	AwayKm = 100.0
	// maxGapDays is the longest run of days without away photos inside a trip.
	maxGapDays = 2
	// MaxKeySteps is the number of steps pictured with a photo and a name.
	MaxKeySteps = 12
)

// Step is a stay of a trip: consecutive photos taken at the same place.
type Step struct {
	Name  string
	Pos   mapbase.LatLon
	Count int
	// First and Last are the days of the first and last photo.
	First, Last time.Time
	// Photo is the one picturing the step.
	Photo string
	// Key steps are pictured with a photo and their name.
	Key bool
}

// Trip is a run of days spent away from home.
type Trip struct {
	From, To   time.Time
	Photos     int
	Steps      []Step
	DistanceKm float64
}

// chronological orders photos by the time they were taken.
func chronological(photos []yearmap.Photo) []yearmap.Photo {
	sorted := append([]yearmap.Photo(nil), photos...)
	moment := func(p yearmap.Photo) time.Time {
		if p.At.IsZero() {
			return p.Day
		}
		return p.At
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := moment(sorted[i]), moment(sorted[j])
		if !a.Equal(b) {
			return a.Before(b)
		}
		return sorted[i].Path < sorted[j].Path
	})
	return sorted
}

// Steps gathers photos into the steps of a route, in chronological order. A
// place visited twice gives two steps.
func Steps(photos []yearmap.Photo, resolver yearmap.Resolver) []Step {
	type stay struct {
		name   string
		first  mapbase.LatLon
		photos []yearmap.Photo
	}
	var stays []*stay
	for _, photo := range chronological(photos) {
		name, _ := resolver.Nearest(photo.Pos.Lat, photo.Pos.Lon)
		if last := len(stays) - 1; last >= 0 && stays[last].name == name {
			limit := sameStayKm
			if name == "" {
				limit = unnamedStayKm
			}
			if yearmap.DistanceKm(stays[last].first, photo.Pos) <= limit {
				stays[last].photos = append(stays[last].photos, photo)
				continue
			}
		}
		stays = append(stays, &stay{name: name, first: photo.Pos, photos: []yearmap.Photo{photo}})
	}
	steps := make([]Step, len(stays))
	for i, s := range stays {
		var lat, lon float64
		for _, photo := range s.photos {
			lat += photo.Pos.Lat
			lon += photo.Pos.Lon
		}
		n := float64(len(s.photos))
		pos := mapbase.LatLon{Lat: lat / n, Lon: lon / n}
		name := s.name
		if name == "" {
			name = yearmap.Coordinates(pos)
		}
		steps[i] = Step{
			Name: name, Pos: pos, Count: len(s.photos),
			First: s.photos[0].Day, Last: s.photos[len(s.photos)-1].Day,
			Photo: s.photos[len(s.photos)/2].Path,
		}
	}
	MarkKey(steps, MaxKeySteps)
	return steps
}

// MarkKey flags the steps to picture: the first, the last and the most
// photographed ones, at most limit in all.
func MarkKey(steps []Step, limit int) {
	for i := range steps {
		steps[i].Key = false
	}
	if len(steps) == 0 {
		return
	}
	order := make([]int, 0, len(steps))
	for i := 1; i < len(steps)-1; i++ {
		order = append(order, i)
	}
	sort.SliceStable(order, func(a, b int) bool { return steps[order[a]].Count > steps[order[b]].Count })
	steps[0].Key = true
	steps[len(steps)-1].Key = true
	room := limit - 2
	for _, i := range order {
		if room <= 0 {
			break
		}
		steps[i].Key = true
		room--
	}
}

// Distance is the length in kilometres of the route through the steps, as the
// crow flies, rounded to 10 km above 100 km.
func Distance(steps []Step) float64 {
	var total float64
	for i := 1; i < len(steps); i++ {
		total += yearmap.DistanceKm(steps[i-1].Pos, steps[i].Pos)
	}
	if total >= 100 {
		return math.Round(total/10) * 10
	}
	return math.Round(total)
}

// Build makes a trip of the photos taken between two days (inclusive).
func Build(photos []yearmap.Photo, from, to time.Time, resolver yearmap.Resolver) Trip {
	var inside []yearmap.Photo
	for _, photo := range photos {
		if !photo.Day.Before(from) && !photo.Day.After(to) {
			inside = append(inside, photo)
		}
	}
	steps := Steps(inside, resolver)
	return Trip{From: from, To: to, Photos: len(inside), Steps: steps, DistanceKm: Distance(steps)}
}

type day struct {
	date   time.Time
	centre mapbase.LatLon
}

// Detect finds the trips of a library: runs of days whose photos were taken
// farther than AwayKm from home, with at most one day without photos between
// two of them and no day at home in between. Home is the busiest area of the
// library. Trips of at least two days come most recent first.
func Detect(photos []yearmap.Photo, resolver yearmap.Resolver) []Trip {
	byDay := map[time.Time][]yearmap.Photo{}
	for _, photo := range photos {
		byDay[photo.Day] = append(byDay[photo.Day], photo)
	}
	days := make([]day, 0, len(byDay))
	for date, list := range byDay {
		var lat, lon float64
		for _, photo := range list {
			lat += photo.Pos.Lat
			lon += photo.Pos.Lon
		}
		n := float64(len(list))
		days = append(days, day{date: date, centre: mapbase.LatLon{Lat: lat / n, Lon: lon / n}})
	}
	if len(days) == 0 {
		return nil
	}
	sort.Slice(days, func(i, j int) bool { return days[i].date.Before(days[j].date) })
	home := homeOf(days)

	var trips []Trip
	var open []day
	sawHome := false
	flush := func() {
		if len(open) > 0 && open[len(open)-1].date.Sub(open[0].date) >= 24*time.Hour {
			trips = append(trips, Build(photos, open[0].date, open[len(open)-1].date, resolver))
		}
		open = nil
	}
	for _, d := range days {
		if yearmap.DistanceKm(home, d.centre) <= AwayKm {
			sawHome = true
			continue
		}
		if len(open) > 0 {
			gap := int(d.date.Sub(open[len(open)-1].date).Hours() / 24)
			if sawHome || gap > maxGapDays {
				flush()
			}
		}
		open = append(open, d)
		sawHome = false
	}
	flush()
	sort.Slice(trips, func(i, j int) bool { return trips[i].From.After(trips[j].From) })
	return trips
}

// homeOf is the centre of the area where most days were spent.
func homeOf(days []day) mapbase.LatLon {
	best, bestCount := 0, -1
	for i, a := range days {
		count := 0
		for _, b := range days {
			if yearmap.DistanceKm(a.centre, b.centre) <= AwayKm {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = i, count
		}
	}
	var lat, lon, n float64
	for _, d := range days {
		if yearmap.DistanceKm(days[best].centre, d.centre) <= AwayKm {
			lat += d.centre.Lat
			lon += d.centre.Lon
			n++
		}
	}
	return mapbase.LatLon{Lat: lat / n, Lon: lon / n}
}
