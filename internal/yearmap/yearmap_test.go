package yearmap

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sort-iphone-pictures/internal/mapbase"
)

type fakeResolver map[[2]int]string

func (f fakeResolver) Nearest(lat, lon float64) (string, bool) {
	name, ok := f[[2]int{int(math.Round(lat)), int(math.Round(lon))}]
	return name, ok
}

func day(d int) time.Time { return time.Date(2026, time.May, d, 0, 0, 0, 0, time.UTC) }

func TestYears(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"2024", "2026", "2025", "misc", "20255"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := Years(root)
	if len(got) != 3 || got[0] != 2026 || got[1] != 2025 || got[2] != 2024 {
		t.Errorf("Years = %v", got)
	}
}

func TestGroupByPlace(t *testing.T) {
	// Paris, Texas is named "Paris" too: the distance keeps it apart.
	resolver := fakeResolver{{49, 2}: "Paris", {56, 12}: "Roskilde", {34, -96}: "Paris"}
	photos := []Photo{
		{Path: "p3.jpg", Day: day(3), Pos: mapbase.LatLon{Lat: 48.9, Lon: 2.3}},
		{Path: "p1.jpg", Day: day(1), Pos: mapbase.LatLon{Lat: 48.8, Lon: 2.4}},
		{Path: "p2.jpg", Day: day(2), Pos: mapbase.LatLon{Lat: 48.85, Lon: 2.35}},
		{Path: "r1.jpg", Day: day(5), Pos: mapbase.LatLon{Lat: 55.64, Lon: 12.08}},
		{Path: "tx.jpg", Day: day(6), Pos: mapbase.LatLon{Lat: 33.66, Lon: -95.55}},
		{Path: "sea1.jpg", Day: day(7), Pos: mapbase.LatLon{Lat: 30.51, Lon: -40.52}},
		{Path: "sea2.jpg", Day: day(8), Pos: mapbase.LatLon{Lat: 30.55, Lon: -40.55}},
	}
	places := Group(photos, resolver)
	if len(places) != 4 {
		t.Fatalf("places = %+v", places)
	}
	paris := places[0]
	if paris.Name != "Paris" || paris.Count != 3 {
		t.Fatalf("most photographed = %+v", paris)
	}
	if len(paris.Shown) != 2 || paris.Shown[0] != "p1.jpg" || paris.Shown[1] == paris.Shown[0] {
		t.Errorf("shown = %v, want two photos starting with the oldest", paris.Shown)
	}
	if math.Abs(paris.Pos.Lat-48.85) > 1e-9 {
		t.Errorf("position = %+v", paris.Pos)
	}
	var sea, single *Place
	for i := range places {
		switch places[i].Count {
		case 2:
			if places[i].Name != "Paris" {
				sea = &places[i]
			}
		case 1:
			if places[i].Name == "Roskilde" {
				single = &places[i]
			}
		}
	}
	if sea == nil || sea.Name != "30.53°N 40.53°W" {
		t.Errorf("unnamed place = %+v", sea)
	}
	if single == nil || len(single.Shown) != 1 {
		t.Errorf("single = %+v", single)
	}
}

func TestComposeKeepsCardsApartAndOnThePage(t *testing.T) {
	var places []Place
	for i, p := range []mapbase.LatLon{{Lat: 48.85, Lon: 2.35}, {Lat: 48.9, Lon: 2.5}, {Lat: 45.76, Lon: 4.83}, {Lat: 43.3, Lon: 5.37}, {Lat: 43.6, Lon: 1.44}, {Lat: 44.84, Lon: -0.58}, {Lat: 47.2, Lon: -1.55}, {Lat: 50.63, Lon: 3.06}, {Lat: 48.58, Lon: 7.75}, {Lat: 45.18, Lon: 5.72}, {Lat: 43.7, Lon: 7.26}, {Lat: 47.32, Lon: 5.04}} {
		places = append(places, Place{Name: "Lieu", Pos: p, Count: 12 - i, Shown: []string{"a", "b"}})
	}
	for _, size := range [][2]float64{{297, 210}, {210, 297}} {
		layout := Compose(places, size[0], size[1])
		var boxes []Rect
		for _, pin := range layout.Pins {
			if pin.Card == nil {
				t.Errorf("%v: %v has no card at scale %v", size, pin.Place.Pos, layout.Scale)
				continue
			}
			c := *pin.Card
			if c.X < 0 || c.Y < 0 || c.X+c.W > size[0] || c.Y+c.H > size[1] {
				t.Errorf("card %+v leaves the page %v", c, size)
			}
			if c.overlaps(TitleBox) {
				t.Errorf("card %+v covers the title", c)
			}
			for _, other := range boxes {
				if c.overlaps(other) {
					t.Errorf("cards %+v and %+v overlap", c, other)
				}
			}
			boxes = append(boxes, c)
		}
	}
}

func TestComposeSinglePlace(t *testing.T) {
	layout := Compose([]Place{{Name: "Paris", Pos: mapbase.LatLon{Lat: 48.85, Lon: 2.35}, Count: 1, Shown: []string{"a"}}}, 297, 210)
	if len(layout.Pins) != 1 || layout.Pins[0].Card == nil || layout.Scale != 1 {
		t.Fatalf("%+v", layout)
	}
	if got := Compose(nil, 297, 210); len(got.Pins) != 0 {
		t.Errorf("no places should give no pins, got %d", len(got.Pins))
	}
}

func writePhoto(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 120, 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRenderWritesAPDF(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.jpg")
	writePhoto(t, good, 400, 300)
	places := []Place{
		{Name: "Łódź — très long nom de lieu à tronquer", Pos: mapbase.LatLon{Lat: 51.76, Lon: 19.46}, Count: 9, Shown: []string{good, good}},
		{Name: "Paris", Pos: mapbase.LatLon{Lat: 48.85, Lon: 2.35}, Count: 1, Shown: []string{filepath.Join(dir, "missing.jpg")}},
	}
	job := Job{Layout: Compose(places, 210, 297), Title: "2026", Subtitle: "10 photos · 2 lieux", Dir: filepath.Join(dir, "out"), Name: "Carte 2026.pdf"}
	var last int
	job.Progress = func(done, total int) { last = done }
	result, err := Render(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil || len(data) < 1000 || string(data[:4]) != "%PDF" {
		t.Fatalf("not a PDF: %v (%d bytes)", err, len(data))
	}
	if len(result.Failed) != 1 || last != 2 {
		t.Errorf("failed = %+v, progress = %d", result.Failed, last)
	}
	second, err := Render(context.Background(), job)
	if err != nil || second.Path == result.Path {
		t.Errorf("an existing PDF must not be replaced: %v %q", err, second.Path)
	}
}

func TestRenderCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	_, err := Render(ctx, Job{Layout: Compose(nil, 297, 210), Dir: dir, Name: "x.pdf"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}
