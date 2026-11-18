package yearmap

import (
	"math"

	"sort-iphone-pictures/internal/mapbase"
)

// Sizes at card scale 1, in millimetres.
const (
	thumbSize = 24.0
	cardPad   = 1.8
	labelH    = 6.0
	minCardW  = 32.0
	pinRadius = 2.4
	// pagePad keeps pins away from the edges so their cards have room.
	pagePad    = 30.0
	pageMargin = 5.0
	cardGap    = 1.5
)

// scales are the card sizes tried, from the largest.
var scales = []float64{1, 0.8, 0.65, 0.5, 0.4}

// TitleBox is the corner of the page reserved for the title.
var TitleBox = Rect{X: 6, Y: 6, W: 90, H: 18}

// Rect is a box on the page, in millimetres.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (r Rect) grow(by float64) Rect {
	return Rect{r.X - by, r.Y - by, r.W + 2*by, r.H + 2*by}
}

func (r Rect) overlaps(o Rect) bool {
	return r.X < o.X+o.W && o.X < r.X+r.W && r.Y < o.Y+o.H && o.Y < r.Y+r.H
}

func (r Rect) contains(p mapbase.XY) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}

// Pin is a place on the page.
type Pin struct {
	Place Place
	// Pos is the tip of the pin.
	Pos mapbase.XY
	// Card is the box of the photos and name; nil when the place did not fit.
	Card *Rect
}

// Layout is the arranged page.
type Layout struct {
	Width, Height float64
	View          mapbase.View
	// Scale is the size of the cards relative to their nominal size.
	Scale float64
	Pins  []Pin
}

// CardSize is the box of a place's card at the given scale.
func CardSize(place Place, scale float64) (w, h float64) {
	n := float64(max(1, len(place.Shown)))
	w = math.Max(minCardW, cardPad+n*(thumbSize+cardPad)) * scale
	h = (cardPad + thumbSize + labelH + cardPad/2) * scale
	return w, h
}

// Compose frames the places on a page of width x height millimetres and puts a
// card next to each pin, without overlaps. When they do not all fit, the cards
// shrink; the places that still do not fit stay bare pins.
func Compose(places []Place, width, height float64) Layout {
	points := make([]mapbase.LatLon, len(places))
	for i, place := range places {
		points[i] = place.Pos
	}
	view := mapbase.Fit(points, width, height, mapbase.FitOptions{Padding: pagePad})
	positions := make([]mapbase.XY, len(places))
	for i, point := range points {
		positions[i] = view.XY(point)
	}
	best, bestPlaced, bestScale := []*Rect(nil), -1, 1.0
	for _, scale := range scales {
		cards, placed := arrange(places, positions, width, height, scale)
		if placed > bestPlaced {
			best, bestPlaced, bestScale = cards, placed, scale
		}
		if placed == len(places) {
			break
		}
	}
	layout := Layout{Width: width, Height: height, View: view, Scale: bestScale, Pins: make([]Pin, len(places))}
	for i, place := range places {
		layout.Pins[i] = Pin{Place: place, Pos: positions[i], Card: best[i]}
	}
	return layout
}

// arrange places the cards one by one, the most photographed place first (the
// order of places), at the first free spot around its pin.
func arrange(places []Place, positions []mapbase.XY, width, height, scale float64) ([]*Rect, int) {
	cards := make([]*Rect, len(places))
	obstacles := []Rect{TitleBox}
	placed := 0
	for i, place := range places {
		w, h := CardSize(place, scale)
		if card, ok := freeSpot(positions, i, w, h, width, height, obstacles); ok {
			cards[i] = &card
			obstacles = append(obstacles, card)
			placed++
		}
	}
	return cards, placed
}

// directions are tried in order: above the pin first, then the sides, below last.
var directions = [][2]float64{{0, -1}, {1, -1}, {-1, -1}, {1, 0}, {-1, 0}, {1, 1}, {-1, 1}, {0, 1}}

var distances = []float64{3, 7, 12, 18, 26, 36}

func freeSpot(positions []mapbase.XY, self int, w, h, width, height float64, obstacles []Rect) (Rect, bool) {
	pin := positions[self]
	page := Rect{pageMargin, pageMargin, width - 2*pageMargin, height - 2*pageMargin}
	for _, gap := range distances {
		for _, d := range directions {
			card := Rect{X: pin.X + d[0]*(w/2+gap) - w/2, Y: pin.Y + d[1]*(h/2+gap) - h/2, W: w, H: h}
			if card.X < page.X || card.Y < page.Y || card.X+card.W > page.X+page.W || card.Y+card.H > page.Y+page.H {
				continue
			}
			if free(card, positions, obstacles) {
				return card, true
			}
		}
	}
	return Rect{}, false
}

func free(card Rect, positions []mapbase.XY, obstacles []Rect) bool {
	padded := card.grow(cardGap)
	for _, other := range obstacles {
		if padded.overlaps(other) {
			return false
		}
	}
	around := card.grow(pinRadius)
	for _, p := range positions {
		if around.contains(p) {
			return false
		}
	}
	return true
}
