// Command gen builds internal/mapbase/world.bin.gz from Natural Earth 1:50m
// GeoJSON files (public domain, https://www.naturalearthdata.com).
//
//	go run ./internal/mapbase/gen countries.geojson lakes.geojson internal/mapbase/world.bin.gz
//
// Rings are simplified (Douglas-Peucker), small ones dropped, and stored in
// units of 0.01° as zigzag varint deltas; see mapbase.Load for the format.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

const (
	unit       = 100.0 // stored units per degree
	tolerance  = 0.012 // simplification tolerance, degrees
	minAreaDeg = 0.004 // rings smaller than this (square degrees) are dropped
)

type pt struct{ lat, lon float64 }

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: gen <countries.geojson> <lakes.geojson> <world.bin.gz>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(countriesPath, lakesPath, outPath string) error {
	var out bytes.Buffer
	out.WriteString("MB1")
	for _, path := range []string{countriesPath, lakesPath} {
		polygons, err := read(path)
		if err != nil {
			return err
		}
		writeUvarint(&out, uint64(len(polygons)))
		for _, polygon := range polygons {
			writeUvarint(&out, uint64(len(polygon)))
			for _, ring := range polygon {
				writeUvarint(&out, uint64(len(ring)))
				prevLat, prevLon := int64(0), int64(0)
				for _, p := range ring {
					lat, lon := int64(math.Round(p.lat*unit)), int64(math.Round(p.lon*unit))
					writeVarint(&out, lat-prevLat)
					writeVarint(&out, lon-prevLon)
					prevLat, prevLon = lat, lon
				}
			}
		}
	}
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()
	zip, _ := gzip.NewWriterLevel(file, gzip.BestCompression)
	if _, err := zip.Write(out.Bytes()); err != nil {
		return err
	}
	if err := zip.Close(); err != nil {
		return err
	}
	return file.Close()
}

func writeUvarint(out *bytes.Buffer, v uint64) {
	var buf [binary.MaxVarintLen64]byte
	out.Write(buf[:binary.PutUvarint(buf[:], v)])
}

func writeVarint(out *bytes.Buffer, v int64) {
	var buf [binary.MaxVarintLen64]byte
	out.Write(buf[:binary.PutVarint(buf[:], v)])
}

type collection struct {
	Features []struct {
		Geometry struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

// read returns the polygons of a GeoJSON file, each a list of simplified rings.
func read(path string) ([][][]pt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c collection
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var polygons [][][]pt
	for _, feature := range c.Features {
		var raw [][][][2]float64
		switch feature.Geometry.Type {
		case "Polygon":
			var one [][][2]float64
			if err := json.Unmarshal(feature.Geometry.Coordinates, &one); err != nil {
				return nil, err
			}
			raw = [][][][2]float64{one}
		case "MultiPolygon":
			if err := json.Unmarshal(feature.Geometry.Coordinates, &raw); err != nil {
				return nil, err
			}
		default:
			continue
		}
		for _, polygon := range raw {
			var rings [][]pt
			for i, ring := range polygon {
				points := make([]pt, len(ring))
				for j, p := range ring {
					points[j] = pt{lat: p[1], lon: p[0]}
				}
				if len(points) > 1 && points[0] == points[len(points)-1] {
					points = points[:len(points)-1]
				}
				points = simplify(points)
				if len(points) < 3 || math.Abs(area(points)) < minAreaDeg {
					if i == 0 {
						break // no outer ring: drop the polygon
					}
					continue
				}
				rings = append(rings, points)
			}
			if len(rings) > 0 {
				polygons = append(polygons, rings)
			}
		}
	}
	return polygons, nil
}

func area(ring []pt) float64 {
	sum := 0.0
	for i, p := range ring {
		q := ring[(i+1)%len(ring)]
		sum += p.lon*q.lat - q.lon*p.lat
	}
	return sum / 2
}

// simplify is Douglas-Peucker on a closed ring, anchored on the point
// farthest from the first one.
func simplify(ring []pt) []pt {
	if len(ring) < 4 {
		return ring
	}
	far, best := 0, -1.0
	for i, p := range ring {
		if d := dist2(ring[0], p); d > best {
			far, best = i, d
		}
	}
	first := dp(ring[:far+1])
	second := dp(append(append([]pt{}, ring[far:]...), ring[0]))
	return append(first[:len(first)-1], second[:len(second)-1]...)
}

func dp(points []pt) []pt {
	if len(points) < 3 {
		return points
	}
	a, b := points[0], points[len(points)-1]
	index, worst := 0, 0.0
	for i := 1; i < len(points)-1; i++ {
		if d := segDist(points[i], a, b); d > worst {
			index, worst = i, d
		}
	}
	if worst <= tolerance {
		return []pt{a, b}
	}
	left := dp(points[:index+1])
	right := dp(points[index:])
	return append(left[:len(left)-1], right...)
}

func dist2(a, b pt) float64 { return (a.lat-b.lat)*(a.lat-b.lat) + (a.lon-b.lon)*(a.lon-b.lon) }

func segDist(p, a, b pt) float64 {
	dx, dy := b.lon-a.lon, b.lat-a.lat
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Sqrt(dist2(p, a))
	}
	t := math.Max(0, math.Min(1, ((p.lon-a.lon)*dx+(p.lat-a.lat)*dy)/l2))
	return math.Sqrt(dist2(p, pt{a.lat + t*dy, a.lon + t*dx}))
}
