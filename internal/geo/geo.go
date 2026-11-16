// Package geo resolves GPS coordinates to the nearest city name, offline.
//
// The embedded data comes from GeoNames (cities1000, CC BY 4.0,
// https://www.geonames.org). City names are the local names given by GeoNames.
package geo

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"math"
	"strconv"
	"strings"
	"sync"
)

//go:embed cities.tsv.gz
var embedded []byte

//go:embed places.tsv.gz
var embeddedPlaces []byte

const (
	// MaxDistanceKm is the farthest a city may be from the point to be returned.
	MaxDistanceKm = 50.0
	// groupingKm lets a larger city within this extra distance of the nearest
	// one win, so districts and suburbs are filed under the main city.
	groupingKm = 6.0
)

type city struct {
	name       string
	lat, lon   float64
	population int
}

// Index finds the nearest city using a one-degree grid.
type Index struct {
	cities []city
	grid   map[[2]int][]int32
}

var (
	defaultOnce  sync.Once
	defaultIndex *Index
)

// Default returns the index built from the embedded data.
func Default() *Index {
	defaultOnce.Do(func() {
		index, err := Load(embedded)
		if err != nil {
			index = &Index{grid: map[[2]int][]int32{}}
		}
		defaultIndex = index
	})
	return defaultIndex
}

// Load builds an index from gzip-compressed "name<TAB>lat<TAB>lon<TAB>population" lines.
func Load(data []byte) (*Index, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	index := &Index{grid: make(map[[2]int][]int32)}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 4 {
			continue
		}
		lat, errLat := strconv.ParseFloat(fields[1], 64)
		lon, errLon := strconv.ParseFloat(fields[2], 64)
		population, errPop := strconv.Atoi(fields[3])
		if errLat != nil || errLon != nil || errPop != nil {
			continue
		}
		key := cell(lat, lon)
		index.grid[key] = append(index.grid[key], int32(len(index.cities)))
		index.cities = append(index.cities, city{fields[0], lat, lon, population})
	}
	return index, scanner.Err()
}

// Len returns the number of cities in the index.
func (index *Index) Len() int { return len(index.cities) }

// Nearest returns the closest city within MaxDistanceKm.
func (index *Index) Nearest(lat, lon float64) (string, bool) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return "", false
	}
	center := cell(lat, lon)
	spanLat := int(math.Ceil(MaxDistanceKm / 111.0))
	spanLon := 180
	if cosine := math.Cos(lat * math.Pi / 180); cosine > 0.01 {
		spanLon = min(180, int(math.Ceil(MaxDistanceKm/(111.0*cosine))))
	}
	type match struct {
		city     city
		distance float64
	}
	var matches []match
	nearest := MaxDistanceKm
	for dLat := -spanLat; dLat <= spanLat; dLat++ {
		row := center[0] + dLat
		if row < -90 || row > 89 {
			continue
		}
		for dLon := -spanLon; dLon <= spanLon; dLon++ {
			column := (center[1]+dLon+180+360)%360 - 180
			for _, id := range index.grid[[2]int{row, column}] {
				candidate := index.cities[id]
				if distance := haversineKm(lat, lon, candidate.lat, candidate.lon); distance < MaxDistanceKm {
					matches = append(matches, match{candidate, distance})
					nearest = min(nearest, distance)
				}
			}
		}
	}
	var best *city
	for index := range matches {
		candidate := &matches[index]
		if candidate.distance > nearest+groupingKm {
			continue
		}
		if best == nil || candidate.city.population > best.population {
			best = &candidate.city
		}
	}
	if best == nil {
		return "", false
	}
	return best.name, true
}

func cell(lat, lon float64) [2]int {
	column := int(math.Floor(lon))
	if column >= 180 {
		column = 179
	}
	return [2]int{int(math.Floor(lat)), column}
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const radius = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * radius * math.Asin(math.Sqrt(a))
}

// Landmark radii: large venues get more slack than small monuments because
// GPS fixes drift and the reference point is the middle of the site.
const (
	venueRadiusKm    = 0.25
	monumentRadiusKm = 0.12
)

type place struct {
	name     string
	lat, lon float64
	rank     int
}

// Places finds landmarks (museums, castles, stadiums...) near a point.
type Places struct {
	places []place
	grid   map[[2]int][]int32
}

var (
	placesOnce    sync.Once
	defaultPlaces *Places
)

// DefaultPlaces returns the landmark index built from the embedded data.
func DefaultPlaces() *Places {
	placesOnce.Do(func() {
		places, err := LoadPlaces(embeddedPlaces)
		if err != nil {
			places = &Places{grid: map[[2]int][]int32{}}
		}
		defaultPlaces = places
	})
	return defaultPlaces
}

// LoadPlaces builds an index from gzip "name<TAB>lat<TAB>lon<TAB>rank" lines.
func LoadPlaces(data []byte) (*Places, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	index := &Places{grid: make(map[[2]int][]int32)}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 4 {
			continue
		}
		lat, errLat := strconv.ParseFloat(fields[1], 64)
		lon, errLon := strconv.ParseFloat(fields[2], 64)
		rank, errRank := strconv.Atoi(fields[3])
		if errLat != nil || errLon != nil || errRank != nil {
			continue
		}
		key := cell(lat, lon)
		index.grid[key] = append(index.grid[key], int32(len(index.places)))
		index.places = append(index.places, place{fields[0], lat, lon, rank})
	}
	return index, scanner.Err()
}

// Len returns the number of landmarks in the index.
func (index *Places) Len() int { return len(index.places) }

// Near returns the most significant landmark around the point; among equals
// the closest one wins.
func (index *Places) Near(lat, lon float64) (string, bool) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return "", false
	}
	center := cell(lat, lon)
	var best *place
	bestDistance := 0.0
	for dLat := -1; dLat <= 1; dLat++ {
		for dLon := -1; dLon <= 1; dLon++ {
			row := center[0] + dLat
			if row < -90 || row > 89 {
				continue
			}
			column := (center[1]+dLon+180+360)%360 - 180
			for _, id := range index.grid[[2]int{row, column}] {
				candidate := &index.places[id]
				limit := monumentRadiusKm
				if candidate.rank >= 2 {
					limit = venueRadiusKm
				}
				distance := haversineKm(lat, lon, candidate.lat, candidate.lon)
				if distance > limit {
					continue
				}
				if best == nil || candidate.rank > best.rank || (candidate.rank == best.rank && distance < bestDistance) {
					best, bestDistance = candidate, distance
				}
			}
		}
	}
	if best == nil {
		return "", false
	}
	return best.name, true
}
