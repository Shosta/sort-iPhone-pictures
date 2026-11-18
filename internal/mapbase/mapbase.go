// Package mapbase draws an offline, network-free base map: land and lakes from
// an embedded Natural Earth dataset (public domain, see NOTICE.md), a Mercator
// projection and automatic framing of a set of GPS points.
//
// It does not depend on any drawing library: Fit gives a View (a conversion
// from GPS to millimetres on a page), and Base.Land / Base.Lakes give polygons
// already converted, to be drawn by the caller (PDF, SVG, bitmap...). The
// photo map of a year and a future trip map share it.
package mapbase

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
)

//go:embed world.bin.gz
var embedded []byte

// maxLat is the latitude where Mercator is cut, as for web maps.
const maxLat = 85.0

// LatLon is a GPS position in degrees.
type LatLon struct{ Lat, Lon float64 }

// XY is a position on the page, in millimetres, y growing downwards.
type XY struct{ X, Y float64 }

// Polygon is an outline with its holes, in degrees. It is filled with the
// even-odd rule.
type Polygon struct {
	Rings [][]LatLon
	// minLat..maxLon is the box around the polygon.
	minLat, minLon, maxLat, maxLon float64
}

// Base is the world map.
type Base struct {
	land  []Polygon
	lakes []Polygon
}

var (
	defaultOnce sync.Once
	defaultBase *Base
)

// Default returns the map built from the embedded data.
func Default() *Base {
	defaultOnce.Do(func() {
		base, err := Load(embedded)
		if err != nil {
			base = &Base{}
		}
		defaultBase = base
	})
	return defaultBase
}

// Load reads the gzip data produced by internal/mapbase/gen: "MB1", then the
// land layer and the lakes layer, each a count of polygons, each polygon a
// count of rings, each ring a count of points stored as zigzag varint deltas
// of latitude and longitude in hundredths of a degree.
func Load(data []byte) (*Base, error) {
	zip, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zip.Close()
	in := bufio.NewReader(zip)
	magic := make([]byte, 3)
	if _, err := io.ReadFull(in, magic); err != nil || string(magic) != "MB1" {
		return nil, errors.New("mapbase: unknown data format")
	}
	base := &Base{}
	for _, layer := range []*[]Polygon{&base.land, &base.lakes} {
		polygons, err := readLayer(in)
		if err != nil {
			return nil, fmt.Errorf("mapbase: %w", err)
		}
		*layer = polygons
	}
	return base, nil
}

func readLayer(in *bufio.Reader) ([]Polygon, error) {
	count, err := binary.ReadUvarint(in)
	if err != nil {
		return nil, err
	}
	polygons := make([]Polygon, 0, min(count, 1<<16))
	for range count {
		rings, err := binary.ReadUvarint(in)
		if err != nil {
			return nil, err
		}
		polygon := Polygon{minLat: 90, minLon: 180, maxLat: -90, maxLon: -180}
		for range rings {
			size, err := binary.ReadUvarint(in)
			if err != nil {
				return nil, err
			}
			ring := make([]LatLon, 0, min(size, 1<<16))
			var lat, lon int64
			for range size {
				dLat, err := binary.ReadVarint(in)
				if err != nil {
					return nil, err
				}
				dLon, err := binary.ReadVarint(in)
				if err != nil {
					return nil, err
				}
				lat, lon = lat+dLat, lon+dLon
				p := LatLon{float64(lat) / 100, float64(lon) / 100}
				polygon.minLat, polygon.maxLat = math.Min(polygon.minLat, p.Lat), math.Max(polygon.maxLat, p.Lat)
				polygon.minLon, polygon.maxLon = math.Min(polygon.minLon, p.Lon), math.Max(polygon.maxLon, p.Lon)
				ring = append(ring, p)
			}
			polygon.Rings = append(polygon.Rings, ring)
		}
		polygons = append(polygons, polygon)
	}
	return polygons, nil
}

// mercator projects a position to degrees on both axes, north up.
func mercator(p LatLon) (x, y float64) {
	lat := math.Max(-maxLat, math.Min(maxLat, p.Lat))
	return p.Lon, math.Log(math.Tan(math.Pi/4+lat*math.Pi/360)) * 180 / math.Pi
}

// View converts GPS positions to a page of Width x Height millimetres.
type View struct {
	Width, Height float64
	// cx, cy is the Mercator position at the centre of the page, scale the
	// millimetres per Mercator degree.
	cx, cy, scale float64
}

// FitOptions tune Fit.
type FitOptions struct {
	// Padding is the empty band kept around the points, in millimetres.
	Padding float64
	// MinSpan is the smallest extent shown, in degrees, so a single place
	// still shows its surroundings (default 2).
	MinSpan float64
	// MaxScale caps millimetres per degree; zero means no cap.
	MaxScale float64
}

// Fit frames the points on a page of the given size: the points fill the page
// minus Padding, centred, with the aspect ratio of the page kept. Without
// points the whole world is shown. Points spanning the antimeridian are not
// handled.
func Fit(points []LatLon, width, height float64, opts FitOptions) View {
	if opts.MinSpan <= 0 {
		opts.MinSpan = 2
	}
	minX, minY, maxX, maxY := -180.0, -math.MaxFloat64, 180.0, math.MaxFloat64
	if len(points) > 0 {
		minX, minY, maxX, maxY = math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64
		for _, p := range points {
			x, y := mercator(p)
			minX, maxX = math.Min(minX, x), math.Max(maxX, x)
			minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		}
	} else {
		_, minY = mercator(LatLon{Lat: -60})
		_, maxY = mercator(LatLon{Lat: 80})
	}
	spanX, spanY := math.Max(maxX-minX, opts.MinSpan), math.Max(maxY-minY, opts.MinSpan)
	pad := math.Min(opts.Padding, math.Min(width, height)/3)
	scale := math.Min((width-2*pad)/spanX, (height-2*pad)/spanY)
	if opts.MaxScale > 0 {
		scale = math.Min(scale, opts.MaxScale)
	}
	return View{Width: width, Height: height, cx: (minX + maxX) / 2, cy: (minY + maxY) / 2, scale: scale}
}

// XY is the position of p on the page.
func (v View) XY(p LatLon) XY {
	x, y := mercator(p)
	return XY{v.Width/2 + (x-v.cx)*v.scale, v.Height/2 - (y-v.cy)*v.scale}
}

// MMPerDegree is the scale at the equator.
func (v View) MMPerDegree() float64 { return v.scale }

// Land returns the land polygons that touch the page, converted for drawing.
func (b *Base) Land(v View) [][][]XY { return project(b.land, v) }

// Lakes returns the lake polygons that touch the page.
func (b *Base) Lakes(v View) [][][]XY { return project(b.lakes, v) }

// project converts the polygons whose box touches the page (with a margin).
func project(polygons []Polygon, v View) [][][]XY {
	var out [][][]XY
	for _, polygon := range polygons {
		if !v.touches(polygon) {
			continue
		}
		rings := make([][]XY, len(polygon.Rings))
		for i, ring := range polygon.Rings {
			points := make([]XY, len(ring))
			for j, p := range ring {
				points[j] = v.XY(p)
			}
			rings[i] = points
		}
		out = append(out, rings)
	}
	return out
}

func (v View) touches(p Polygon) bool {
	a, b := v.XY(LatLon{p.minLat, p.minLon}), v.XY(LatLon{p.maxLat, p.maxLon})
	return math.Max(a.X, b.X) >= 0 && math.Min(a.X, b.X) <= v.Width && math.Max(a.Y, b.Y) >= 0 && math.Min(a.Y, b.Y) <= v.Height
}

// SVGPath turns projected polygons into the "d" attribute of an SVG path to
// be filled with the even-odd rule.
func SVGPath(polygons [][][]XY) string {
	var out strings.Builder
	num := func(f float64) string { return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64) }
	for _, polygon := range polygons {
		for _, ring := range polygon {
			for i, p := range ring {
				if i == 0 {
					out.WriteByte('M')
				} else {
					out.WriteByte('L')
				}
				out.WriteString(num(p.X) + " " + num(p.Y))
			}
			out.WriteByte('Z')
		}
	}
	return out.String()
}
