package travelmap

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/yearmap"
)

type fakeResolver map[[2]int]string

func (f fakeResolver) Nearest(lat, lon float64) (string, bool) {
	name, ok := f[[2]int{int(math.Round(lat)), int(math.Round(lon))}]
	return name, ok
}

var resolver = fakeResolver{{49, 2}: "Paris", {45, 5}: "Lyon", {43, 5}: "Marseille", {44, 4}: "Avignon", {42, 12}: "Rome"}

func at(month time.Month, d, hour int) time.Time {
	return time.Date(2026, month, d, hour, 0, 0, 0, time.UTC)
}

func photo(path string, month time.Month, d, hour int, lat, lon float64) yearmap.Photo {
	return yearmap.Photo{Path: path, Day: at(month, d, 0), At: at(month, d, hour), Pos: mapbase.LatLon{Lat: lat, Lon: lon}}
}

func TestStepsFollowTimeAndKeepRevisits(t *testing.T) {
	photos := []yearmap.Photo{
		photo("c.jpg", time.July, 2, 9, 45.1, 5.1), // Lyon, taken first out of order below
		photo("a.jpg", time.July, 1, 9, 48.9, 2.3),
		photo("b.jpg", time.July, 1, 18, 48.8, 2.4),
		photo("d.jpg", time.July, 3, 9, 43.2, 5.2),
		photo("e.jpg", time.July, 4, 9, 48.9, 2.3), // back in Paris: a new step
	}
	steps := Steps(photos, resolver)
	var names []string
	for _, step := range steps {
		names = append(names, step.Name)
	}
	if want := []string{"Paris", "Lyon", "Marseille", "Paris"}; len(names) != 4 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] || names[3] != want[3] {
		t.Fatalf("steps = %v, want %v", names, want)
	}
	if steps[0].Count != 2 || steps[0].Photo != "b.jpg" {
		t.Errorf("first step = %+v", steps[0])
	}
	if d := Distance(steps); d < 1000 || d > 1600 {
		t.Errorf("distance = %v km", d)
	}
}

func TestMarkKeyKeepsEndsAndBusiestSteps(t *testing.T) {
	steps := make([]Step, 8)
	for i := range steps {
		steps[i].Count = 1
	}
	steps[5].Count = 9
	steps[3].Count = 5
	MarkKey(steps, 4)
	var keys []int
	for i, step := range steps {
		if step.Key {
			keys = append(keys, i)
		}
	}
	if len(keys) != 4 || keys[0] != 0 || keys[1] != 3 || keys[2] != 5 || keys[3] != 7 {
		t.Errorf("key steps = %v", keys)
	}
}

func TestDetectFindsTripsAwayFromHome(t *testing.T) {
	var photos []yearmap.Photo
	for d := 1; d <= 10; d++ { // a month of life in Paris, with one day trip
		photos = append(photos, photo("home.jpg", time.June, d, 10, 48.9, 2.3))
	}
	photos = append(photos,
		photo("day.jpg", time.June, 12, 10, 45.1, 5.1), // one day away: no trip
		photo("h2.jpg", time.June, 13, 10, 48.9, 2.3),
		photo("t1.jpg", time.July, 1, 10, 43.2, 5.3),
		photo("t2.jpg", time.July, 3, 10, 44.0, 4.4), // a day without photos in between
		photo("t3.jpg", time.July, 4, 10, 41.9, 12.2),
		photo("h3.jpg", time.July, 8, 10, 48.9, 2.3),
		photo("t4.jpg", time.July, 20, 10, 43.2, 5.3), // a second trip
		photo("t5.jpg", time.July, 21, 10, 45.1, 5.1),
	)
	trips := Detect(photos, resolver)
	if len(trips) != 2 {
		t.Fatalf("trips = %+v", trips)
	}
	if !trips[0].From.Equal(at(time.July, 20, 0)) || len(trips[0].Steps) != 2 {
		t.Errorf("most recent trip = %+v", trips[0])
	}
	first := trips[1]
	if !first.From.Equal(at(time.July, 1, 0)) || !first.To.Equal(at(time.July, 4, 0)) || first.Photos != 3 || len(first.Steps) != 3 || first.Steps[2].Name != "Rome" || first.DistanceKm < 700 {
		t.Errorf("first trip = %+v", first)
	}
}

func TestDatesPerLanguage(t *testing.T) {
	from, to := at(time.May, 12, 0), at(time.May, 14, 0)
	for lang, want := range map[string]string{"fr": "12–14 mai", "en": "May 12 – May 14", "da": "12–14. maj", "": "12–14 mai"} {
		if got := Dates(lang, from, to); got != want {
			t.Errorf("Dates(%q) = %q, want %q", lang, got, want)
		}
	}
	if got := Dates("fr", at(time.December, 30, 0), time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)); got != "30 déc. 2026 – 2 janv. 2027" {
		t.Errorf("across years = %q", got)
	}
}

func writePhoto(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 40)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComposeAndRender(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.jpg")
	writePhoto(t, good)
	steps := []Step{
		{Name: "Paris", Pos: mapbase.LatLon{Lat: 48.85, Lon: 2.35}, Count: 3, First: at(time.July, 1, 0), Last: at(time.July, 2, 0), Photo: good, Key: true},
		{Name: "Dijon", Pos: mapbase.LatLon{Lat: 47.3, Lon: 5.0}, Count: 1, First: at(time.July, 3, 0), Last: at(time.July, 3, 0), Photo: good},
		{Name: "Rome", Pos: mapbase.LatLon{Lat: 41.9, Lon: 12.5}, Count: 2, First: at(time.July, 5, 0), Last: at(time.July, 5, 0), Photo: filepath.Join(dir, "missing.jpg"), Key: true},
	}
	layout := Compose(steps, 297, 210, "fr")
	if len(layout.Stops) != 3 || layout.Stops[0].Card == nil || layout.Stops[1].Card != nil || layout.Stops[2].Card == nil {
		t.Fatalf("stops = %+v", layout.Stops)
	}
	if layout.Stops[2].N != 3 || layout.Stops[0].Dates != "1–2 juil." {
		t.Errorf("stops = %+v", layout.Stops)
	}
	result, err := Render(context.Background(), Job{Layout: layout, Title: "Été", Subtitle: "3 étapes", Dir: filepath.Join(dir, "out"), Name: "Voyage.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(result.Path); err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("not a PDF: %v", err)
	}
	if len(result.Failed) != 1 {
		t.Errorf("the missing photo should be reported, got %v", result.Failed)
	}
}
