package main

import (
	"context"
	"errors"
	"fmt"

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
func (s *server) yearMapLayout(req request) error {
	layout, err := yearMapPage(req)
	if err != nil {
		return err
	}
	base := mapbase.Default()
	pins := make([]yearPin, len(layout.Pins))
	for i, pin := range layout.Pins {
		pins[i] = yearPin{X: pin.Pos.X, Y: pin.Pos.Y, Name: pin.Place.Name, Count: pin.Place.Count, Shown: nonNil(pin.Place.Shown), Card: pin.Card}
	}
	s.send(event{
		"id": req.ID, "type": "layout",
		"pageWidth": layout.Width, "pageHeight": layout.Height, "scale": layout.Scale,
		"land":  mapbase.SVGPath(base.Land(layout.View)),
		"lakes": mapbase.SVGPath(base.Lakes(layout.View)),
		"pins":  pins,
	})
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
	var throttle statusThrottle
	result, err := yearmap.Render(ctx, yearmap.Job{
		Layout:      layout,
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
	s.send(event{"id": req.ID, "type": "done", "path": result.Path, "places": len(layout.Pins), "failed": failed, "fallback": result.Fallback})
	return nil
}
