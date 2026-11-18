package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sort-iphone-pictures/internal/scrapbook"
)

// maxWeekRange bounds the days a request may cover: a calendar grid is six
// weeks, so anything beyond a year is a mistake.
const maxWeekRange = 366

type scrapPhoto struct {
	Path string `json:"path"`
	Date string `json:"date"`
}

type scrapPlacement struct {
	Path string  `json:"path"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

type scrapSheet struct {
	Placements []scrapPlacement `json:"placements"`
}

type scrapFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// weekRange validates the days of a request.
func weekRange(req request) (from, to time.Time, err error) {
	if req.Root == "" {
		return from, to, errors.New("no library folder")
	}
	if from, err = scrapbook.ParseDay(req.From); err != nil {
		return from, to, err
	}
	if to, err = scrapbook.ParseDay(req.To); err != nil {
		return from, to, err
	}
	if to.Before(from) || to.Sub(from) > maxWeekRange*24*time.Hour {
		return from, to, errors.New("invalid date range")
	}
	return from, to, nil
}

// weekDays tells how many photos each day of a range holds, for the calendar.
func (s *server) weekDays(ctx context.Context, req request) error {
	from, to, err := weekRange(req)
	if err != nil {
		return err
	}
	photos, err := scrapbook.Scan(ctx, req.Root, from, to)
	if err != nil {
		return err
	}
	s.send(event{"id": req.ID, "type": "weekDays", "days": scrapbook.CountByDay(photos), "rootOk": scrapbook.LooksLikeRoot(req.Root)})
	return nil
}

// weekPhotos lists the photos of a range, oldest first.
func (s *server) weekPhotos(ctx context.Context, req request) error {
	from, to, err := weekRange(req)
	if err != nil {
		return err
	}
	found, err := scrapbook.Scan(ctx, req.Root, from, to)
	if err != nil {
		return err
	}
	photos := make([]scrapPhoto, len(found))
	for i, photo := range found {
		photos[i] = scrapPhoto{Path: photo.Path, Date: scrapbook.DayKey(photo.Day)}
	}
	s.send(event{"id": req.ID, "type": "photos", "photos": photos})
	return nil
}

// arrangement is the placement of the chosen photos.
type arrangement struct {
	paper  scrapbook.Paper
	photos []string
	sheets []scrapbook.Sheet
	failed []scrapFailure
}

// arrange measures the chosen photos and places them. Photos that cannot be
// read are left out and reported. The preview and the PDF both use it, so
// they always agree.
func (s *server) arrange(ctx context.Context, req request) (arrangement, error) {
	paper, ok := scrapbook.ParsePaper(req.Page)
	if !ok {
		return arrangement{}, fmt.Errorf("unknown page format %q", req.Page)
	}
	paper = paper.WithBleed(req.Bleed)
	longSide := req.LongSide
	if longSide <= 0 {
		longSide = scrapbook.DefaultLongSide
	}
	ratios, errs := s.measurer.Measure(ctx, req.Files)
	if err := ctx.Err(); err != nil {
		return arrangement{}, err
	}
	result := arrangement{paper: paper}
	var kept []float64
	for i, path := range req.Files {
		if errs[i] != nil {
			result.failed = append(result.failed, scrapFailure{Path: path, Error: errs[i].Error()})
			continue
		}
		result.photos = append(result.photos, path)
		kept = append(kept, ratios[i])
	}
	result.sheets = scrapbook.Layout(paper, longSide, req.Gutter, kept)
	return result, nil
}

// scrapbookLayout answers with the pages the chosen photos would fill.
func (s *server) scrapbookLayout(ctx context.Context, req request) error {
	arranged, err := s.arrange(ctx, req)
	if err != nil {
		return err
	}
	sheets := make([]scrapSheet, len(arranged.sheets))
	for i, sheet := range arranged.sheets {
		sheets[i].Placements = make([]scrapPlacement, len(sheet.Placements))
		for j, placement := range sheet.Placements {
			sheets[i].Placements[j] = scrapPlacement{Path: arranged.photos[placement.Index], X: placement.X, Y: placement.Y, W: placement.W, H: placement.H}
		}
	}
	s.send(event{"id": req.ID, "type": "layout", "pageWidth": arranged.paper.SheetWidth(), "pageHeight": arranged.paper.SheetHeight(), "bleed": arranged.paper.Bleed, "sheets": sheets, "failed": nonNil(arranged.failed)})
	return nil
}

// scrapbook writes the PDF of the chosen photos.
func (s *server) scrapbook(ctx context.Context, req request) error {
	if len(req.Files) == 0 {
		return errors.New("no photos selected")
	}
	s.busy.Add(1)
	defer s.busy.Add(-1)
	arranged, err := s.arrange(ctx, req)
	if err != nil {
		return err
	}
	if len(arranged.photos) == 0 {
		return errors.New("none of the selected photos can be read")
	}
	var throttle statusThrottle
	result, err := scrapbook.Render(ctx, scrapbook.Job{
		Paper:       arranged.paper,
		Photos:      arranged.photos,
		Sheets:      arranged.sheets,
		Dir:         req.OutDir,
		FallbackDir: req.FallbackDir,
		Name:        req.Name,
		Title:       req.Name,
		Progress: func(done, total int) {
			if throttle.allow("scrapbook", done, total) {
				s.send(event{"id": req.ID, "type": "progress", "done": done, "total": total})
			}
		},
	})
	if err != nil {
		return err
	}
	failed := arranged.failed
	for _, failure := range result.Failed {
		failed = append(failed, scrapFailure{Path: failure.Path, Error: failure.Error})
	}
	s.send(event{"id": req.ID, "type": "done", "path": result.Path, "pages": result.Pages, "photos": len(arranged.photos) - len(result.Failed), "failed": nonNil(failed), "fallback": result.Fallback})
	return nil
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
