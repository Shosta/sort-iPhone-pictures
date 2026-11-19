package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sort-iphone-pictures/internal/geo"
	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/scrapbook"
	"sort-iphone-pictures/internal/travelmap"
	"sort-iphone-pictures/internal/yearmap"
)

const dayFormat = "2006-01-02"

// travelStep is a step of a trip as exchanged with the renderer, which keeps the
// steps between requests.
type travelStep struct {
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Count int     `json:"count"`
	First string  `json:"first"`
	Last  string  `json:"last"`
	Photo string  `json:"photo"`
	Key   bool    `json:"key"`
}

type travelTrip struct {
	From       string       `json:"from"`
	To         string       `json:"to"`
	Photos     int          `json:"photos"`
	DistanceKm float64      `json:"distanceKm"`
	Steps      []travelStep `json:"steps"`
}

type travelStop struct {
	X     float64       `json:"x"`
	Y     float64       `json:"y"`
	N     int           `json:"n"`
	Name  string        `json:"name"`
	Dates string        `json:"dates"`
	Photo string        `json:"photo"`
	Card  *yearmap.Rect `json:"card"`
}

func toTravelSteps(steps []travelmap.Step) []travelStep {
	out := make([]travelStep, len(steps))
	for i, step := range steps {
		out[i] = travelStep{Name: step.Name, Lat: step.Pos.Lat, Lon: step.Pos.Lon, Count: step.Count, First: step.First.Format(dayFormat), Last: step.Last.Format(dayFormat), Photo: step.Photo, Key: step.Key}
	}
	return out
}

func fromTravelSteps(steps []travelStep) ([]travelmap.Step, error) {
	out := make([]travelmap.Step, len(steps))
	for i, step := range steps {
		first, err1 := time.Parse(dayFormat, step.First)
		last, err2 := time.Parse(dayFormat, step.Last)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid dates of step %d", i+1)
		}
		out[i] = travelmap.Step{Name: step.Name, Pos: mapbase.LatLon{Lat: step.Lat, Lon: step.Lon}, Count: step.Count, First: first, Last: last, Photo: step.Photo, Key: step.Key}
	}
	return out, nil
}

func toTravelTrip(trip travelmap.Trip) travelTrip {
	return travelTrip{From: trip.From.Format(dayFormat), To: trip.To.Format(dayFormat), Photos: trip.Photos, DistanceKm: trip.DistanceKm, Steps: toTravelSteps(trip.Steps)}
}

// travelPage composes the page of a request. Maps have no bleed.
func travelPage(req request) (travelmap.Layout, []travelmap.Step, error) {
	paper, ok := scrapbook.ParsePaper(req.Page)
	if !ok {
		return travelmap.Layout{}, nil, fmt.Errorf("unknown page format %q", req.Page)
	}
	if len(req.Steps) == 0 {
		return travelmap.Layout{}, nil, errors.New("no steps")
	}
	steps, err := fromTravelSteps(req.Steps)
	if err != nil {
		return travelmap.Layout{}, nil, err
	}
	travelmap.MarkKey(steps, travelmap.MaxKeySteps)
	return travelmap.Compose(steps, paper.Width, paper.Height, req.Lang), steps, nil
}

func (s *server) locate(ctx context.Context, req request, from, to time.Time) ([]yearmap.Photo, int, error) {
	var throttle statusThrottle
	return yearmap.Locate(ctx, req.Root, from, to, func(done, total int) {
		if throttle.allow("travel", done, total) {
			s.send(event{"id": req.ID, "type": "status", "stage": "travel", "done": done, "total": total})
		}
	})
}

// travelTrips detects the trips of the library, in one year when given.
func (s *server) travelTrips(ctx context.Context, req request) error {
	if req.Root == "" {
		return errors.New("no library folder")
	}
	var from, to time.Time
	if req.Year > 0 {
		from, to = time.Date(req.Year, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(req.Year, time.December, 31, 0, 0, 0, 0, time.UTC)
	} else {
		years := yearmap.Years(req.Root)
		if len(years) == 0 {
			s.send(event{"id": req.ID, "type": "trips", "trips": []travelTrip{}, "total": 0, "located": 0})
			return nil
		}
		from, to = time.Date(years[len(years)-1], time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(years[0], time.December, 31, 0, 0, 0, 0, time.UTC)
	}
	photos, total, err := s.locate(ctx, req, from, to)
	if err != nil {
		return err
	}
	trips := []travelTrip{}
	for _, trip := range travelmap.Detect(photos, geo.Default()) {
		trips = append(trips, toTravelTrip(trip))
	}
	s.send(event{"id": req.ID, "type": "trips", "trips": trips, "total": total, "located": len(photos)})
	return nil
}

// travelScan builds the route of the photos taken between two days.
func (s *server) travelScan(ctx context.Context, req request) error {
	if req.Root == "" {
		return errors.New("no library folder")
	}
	from, err1 := time.Parse(dayFormat, req.From)
	to, err2 := time.Parse(dayFormat, req.To)
	if err1 != nil || err2 != nil || to.Before(from) {
		return errors.New("invalid period")
	}
	photos, total, err := s.locate(ctx, req, from, to)
	if err != nil {
		return err
	}
	trip := travelmap.Build(photos, from, to, geo.Default())
	s.send(event{"id": req.ID, "type": "trip", "trip": toTravelTrip(trip), "total": total, "located": len(photos)})
	return nil
}

// travelLayout answers with the map and the stops of a route for a page.
func (s *server) travelLayout(ctx context.Context, req request) error {
	layout, steps, err := travelPage(req)
	if err != nil {
		return err
	}
	stops := make([]travelStop, len(layout.Stops))
	var route strings.Builder
	for i, stop := range layout.Stops {
		stops[i] = travelStop{X: stop.Pos.X, Y: stop.Pos.Y, N: stop.N, Name: stop.Step.Name, Dates: stop.Dates, Photo: stop.Step.Photo, Card: stop.Card}
		command := "L"
		if i == 0 {
			command = "M"
		}
		route.WriteString(command + strconv.FormatFloat(stop.Pos.X, 'f', 2, 64) + " " + strconv.FormatFloat(stop.Pos.Y, 'f', 2, 64))
	}
	reply := event{
		"id": req.ID, "type": "layout",
		"pageWidth": layout.Page.Width, "pageHeight": layout.Page.Height, "scale": layout.Page.Scale,
		"route": route.String(),
		"stops": stops, "distanceKm": travelmap.Distance(steps),
	}
	if err := attachMap(ctx, reply, req, layout.Page); err != nil {
		return err
	}
	s.send(reply)
	return nil
}

// travelPdf writes the travel map as a one-page PDF.
func (s *server) travelPdf(ctx context.Context, req request) error {
	layout, _, err := travelPage(req)
	if err != nil {
		return err
	}
	s.busy.Add(1)
	defer s.busy.Add(-1)
	// A failed download only costs the online background.
	img, credit, backdropErr := backdrop(ctx, req, layout.Page, 6)
	if backdropErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		img, credit = nil, ""
	}
	var throttle statusThrottle
	result, err := travelmap.Render(ctx, travelmap.Job{
		Layout:      layout,
		Backdrop:    img,
		Credit:      credit,
		Title:       req.Title,
		Subtitle:    req.Subtitle,
		Dir:         req.OutDir,
		FallbackDir: req.FallbackDir,
		Name:        req.Name,
		Progress: func(done, total int) {
			if throttle.allow("travel-pdf", done, total) {
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
	s.send(event{"id": req.ID, "type": "done", "path": result.Path, "steps": len(layout.Stops), "failed": failed, "fallback": result.Fallback, "backdropFailed": backdropErr != nil})
	return nil
}
