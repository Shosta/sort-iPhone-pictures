package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/scrapbook"
	"sort-iphone-pictures/internal/yearmap"
)

// yearPlace is a place of the map as exchanged with the renderer, which keeps
// the places between requests.
type yearPlace struct {
	Name  string   `json:"name"`
	Lat   float64  `json:"lat"`
	Lon   float64  `json:"lon"`
	Count int      `json:"count"`
	Shown []string `json:"shown"`
}

type yearPin struct {
	X     float64       `json:"x"`
	Y     float64       `json:"y"`
	Name  string        `json:"name"`
	Count int           `json:"count"`
	Shown []string      `json:"shown"`
	Card  *yearmap.Rect `json:"card"`
}

func toYearPlaces(places []yearmap.Place) []yearPlace {
	out := make([]yearPlace, len(places))
	for i, place := range places {
		out[i] = yearPlace{Name: place.Name, Lat: place.Pos.Lat, Lon: place.Pos.Lon, Count: place.Count, Shown: nonNil(place.Shown)}
	}
	return out
}

func fromYearPlaces(places []yearPlace) []yearmap.Place {
	out := make([]yearmap.Place, len(places))
	for i, place := range places {
		out[i] = yearmap.Place{Name: place.Name, Pos: mapbase.LatLon{Lat: place.Lat, Lon: place.Lon}, Count: place.Count, Shown: place.Shown}
	}
	return out
}

// yearMapPage composes the page of a request. Maps have no bleed.
func yearMapPage(req request) (yearmap.Layout, error) {
	paper, ok := scrapbook.ParsePaper(req.Page)
	if !ok {
		return yearmap.Layout{}, fmt.Errorf("unknown page format %q", req.Page)
	}
	if len(req.Places) == 0 {
		return yearmap.Layout{}, errors.New("no places")
	}
	return yearmap.Compose(fromYearPlaces(req.Places), paper.Width, paper.Height), nil
}

var tileProviders = mapbase.Providers(mapbase.TileCacheDir())

// backdrop downloads the online map behind a layout. It returns a nil image
// when the request wants the offline map. When the requested source fails it
// tries the default provider, then gives up with an error so that the caller
// draws the offline map; warn tells which source was refused.
func backdrop(ctx context.Context, req request, layout yearmap.Layout, ppmm float64) (img image.Image, credit, warn string, err error) {
	provider, ok := mapbase.ProviderByID(tileProviders, req.Provider)
	if !ok {
		return nil, "", "", nil
	}
	candidates := []mapbase.Provider{provider}
	if fallback, ok := mapbase.ProviderByID(tileProviders, mapbase.DefaultProviderID); ok && fallback.ID() != provider.ID() {
		candidates = append(candidates, fallback)
	}
	for i, p := range candidates {
		m, e := mapbase.Backdrop(ctx, p, layout.View, ppmm)
		if e == nil {
			if i > 0 {
				warn = fmt.Sprintf("%s: %v; %s used", provider.Name(), err, p.Name())
			}
			return m, p.Attribution(), warn, nil
		}
		if ctx.Err() != nil {
			return nil, "", "", ctx.Err()
		}
		if i == 0 {
			err = fmt.Errorf("%s: %w", p.Name(), e)
		} else {
			err = fmt.Errorf("%v; %s: %w", err, p.Name(), e)
		}
	}
	return nil, "", "", err
}

// yearMapYears lists the years of the library.
func (s *server) yearMapYears(req request) error {
	if req.Root == "" {
		return errors.New("no library folder")
	}
	s.send(event{"id": req.ID, "type": "yearMapYears", "years": nonNil(yearmap.Years(req.Root)), "rootOk": scrapbook.LooksLikeRoot(req.Root)})
	return nil
}

// yearMapScan reads the GPS positions of a year and groups the photos by place.
func (s *server) yearMapScan(ctx context.Context, req request) error {
	if req.Root == "" {
		return errors.New("no library folder")
	}
	if req.Year < 1 || req.Year > 9999 {
		return errors.New("invalid year")
	}
	var throttle statusThrottle
	summary, err := yearmap.Collect(ctx, req.Root, req.Year, func(done, total int) {
		if throttle.allow("yearmap", done, total) {
			s.send(event{"id": req.ID, "type": "status", "stage": "yearmap", "done": done, "total": total})
		}
	})
	if err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "places", "places": toYearPlaces(summary.Places), "total": summary.Total, "located": summary.Located})
	return nil
}

// yearMapLayout answers with the map and the pins of the places for a page.
func (s *server) yearMapLayout(ctx context.Context, req request) error {
	layout, err := yearMapPage(req)
	if err != nil {
		return err
	}
	pins := make([]yearPin, len(layout.Pins))
	for i, pin := range layout.Pins {
		pins[i] = yearPin{X: pin.Pos.X, Y: pin.Pos.Y, Name: pin.Place.Name, Count: pin.Place.Count, Shown: nonNil(pin.Place.Shown), Card: pin.Card}
	}
	reply := event{"id": req.ID, "type": "layout", "pageWidth": layout.Width, "pageHeight": layout.Height, "scale": layout.Scale, "pins": pins}
	img, credit, warn, err := backdrop(ctx, req, layout, 3)
	if warn != "" {
		reply["backdropError"] = warn
	}
	if err == nil && img != nil {
		var buf bytes.Buffer
		if err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err == nil {
			reply["backdrop"] = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
			reply["credit"] = credit
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reply["backdropError"] = err.Error()
	}
	if reply["backdrop"] == nil {
		base := mapbase.Default()
		reply["land"] = mapbase.SVGPath(base.Land(layout.View))
		reply["lakes"] = mapbase.SVGPath(base.Lakes(layout.View))
	}
	s.send(reply)
	return nil
}

// yearMapPdf writes the map as a one-page PDF.
func (s *server) yearMapPdf(ctx context.Context, req request) error {
	layout, err := yearMapPage(req)
	if err != nil {
		return err
	}
	s.busy.Add(1)
	defer s.busy.Add(-1)
	// A failed download only costs the online background.
	img, credit, warn, backdropErr := backdrop(ctx, req, layout, 6)
	if backdropErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		img, credit = nil, ""
	}
	var throttle statusThrottle
	result, err := yearmap.Render(ctx, yearmap.Job{
		Layout:      layout,
		Backdrop:    img,
		Credit:      credit,
		Title:       req.Title,
		Subtitle:    req.Subtitle,
		Dir:         req.OutDir,
		FallbackDir: req.FallbackDir,
		Name:        req.Name,
		Progress: func(done, total int) {
			if throttle.allow("yearmap-pdf", done, total) {
				s.send(event{"id": req.ID, "type": "progress", "done": done, "total": total})
			}
		},
	})
	if err != nil {
		return err
	}
	failed := make([]scrapFailure, len(result.Failed))
	for i, failure := range result.Failed {
		failed[i] = scrapFailure{Path: failure.Path, Error: failure.Error}
	}
	s.send(event{"id": req.ID, "type": "done", "path": result.Path, "places": len(layout.Pins), "failed": failed, "fallback": result.Fallback, "backdropFailed": backdropErr != nil, "backdropWarning": warn})
	return nil
}
