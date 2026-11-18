package travelmap

import (
	"fmt"
	"time"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/yearmap"
)

// Stop is a step on the page.
type Stop struct {
	Step Step
	// N is the number of the step, from 1.
	N   int
	Pos mapbase.XY
	// Card is the box of the photo and name; nil for steps without one.
	Card *yearmap.Rect
	// Dates is the label of the days of the step.
	Dates string
}

// Layout is the arranged page.
type Layout struct {
	// Page holds the map view and the cards of the key steps, in order.
	Page  yearmap.Layout
	Stops []Stop
}

var months = map[string][12]string{
	"fr": {"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"da": {"jan.", "feb.", "mar.", "apr.", "maj", "jun.", "jul.", "aug.", "sep.", "okt.", "nov.", "dec."},
}

func formatDay(lang string, d time.Time) string {
	m := months[lang][d.Month()-1]
	switch lang {
	case "en":
		return fmt.Sprintf("%s %d", m, d.Day())
	case "da":
		return fmt.Sprintf("%d. %s", d.Day(), m)
	}
	return fmt.Sprintf("%d %s", d.Day(), m)
}

// Dates writes a day or a range of days in the language lang (fr, en or da;
// French by default), with the year when the range does not stay in one.
func Dates(lang string, from, to time.Time) string {
	if _, ok := months[lang]; !ok {
		lang = "fr"
	}
	if from.Equal(to) {
		return formatDay(lang, from)
	}
	if from.Year() != to.Year() {
		return fmt.Sprintf("%s %d – %s %d", formatDay(lang, from), from.Year(), formatDay(lang, to), to.Year())
	}
	if from.Month() == to.Month() && lang != "en" {
		return fmt.Sprintf("%d–%s", from.Day(), formatDay(lang, to))
	}
	return fmt.Sprintf("%s – %s", formatDay(lang, from), formatDay(lang, to))
}

// Compose frames the whole route on a page of width x height millimetres and
// puts the photo and name of each key step next to its marker.
func Compose(steps []Step, width, height float64, lang string) Layout {
	var places []yearmap.Place
	var keys []int
	var extra []mapbase.LatLon
	for i, step := range steps {
		if step.Key {
			places = append(places, yearmap.Place{Name: step.Name, Pos: step.Pos, Count: step.Count, Shown: []string{step.Photo}})
			keys = append(keys, i)
		} else {
			extra = append(extra, step.Pos)
		}
	}
	page := yearmap.ComposeWith(places, extra, width, height)
	layout := Layout{Page: page, Stops: make([]Stop, len(steps))}
	for i, step := range steps {
		layout.Stops[i] = Stop{Step: step, N: i + 1, Pos: page.View.XY(step.Pos), Dates: Dates(lang, step.First, step.Last)}
	}
	for k, i := range keys {
		layout.Stops[i].Card = page.Pins[k].Card
	}
	return layout
}
