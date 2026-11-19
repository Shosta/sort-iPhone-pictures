package mapbase

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type flatProvider struct{ calls int }

func (*flatProvider) ID() string          { return "flat" }
func (*flatProvider) Name() string        { return "Flat" }
func (*flatProvider) Attribution() string { return "test" }
func (*flatProvider) MaxZoom() int        { return 19 }
func (p *flatProvider) Tile(_ context.Context, z, x, y int) (image.Image, error) {
	p.calls++
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{10, 200, 30, 255}), image.Point{}, draw.Src)
	return img, nil
}

func TestBackdropFillsThePageWithFewTiles(t *testing.T) {
	view := Fit([]LatLon{{Lat: 48.8, Lon: 2.3}, {Lat: 55.6, Lon: 12.5}}, 297, 210, FitOptions{Padding: 30})
	p := &flatProvider{}
	img, err := Backdrop(context.Background(), p, view, 3)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 891 || img.Bounds().Dy() != 630 {
		t.Errorf("size = %v", img.Bounds())
	}
	if p.calls == 0 || p.calls > maxTiles {
		t.Errorf("tiles fetched = %d", p.calls)
	}
	if c := img.RGBAAt(400, 300); c.G != 200 {
		t.Errorf("centre pixel = %v", c)
	}
}

func TestTileServerCachesOnDisk(t *testing.T) {
	var hits int
	var agent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		agent = r.Header.Get("User-Agent")
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 256, 256)))
	}))
	defer server.Close()
	s := &TileServer{Key: "t", URL: server.URL + "/{z}/{x}/{y}.png", Zoom: 5, CacheDir: t.TempDir()}
	for range 2 {
		if _, err := s.Tile(context.Background(), 3, 4, 5); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 || agent != UserAgent {
		t.Errorf("hits = %d, agent = %q", hits, agent)
	}
}

func inLand(b *Base, p LatLon) bool {
	inside := false
	for _, polygon := range b.land {
		for _, ring := range polygon.Rings {
			for i, a := range ring {
				c := ring[(i+1)%len(ring)]
				if (a.Lat > p.Lat) != (c.Lat > p.Lat) && p.Lon < (c.Lon-a.Lon)*(p.Lat-a.Lat)/(c.Lat-a.Lat)+a.Lon {
					inside = !inside
				}
			}
		}
	}
	return inside
}

func TestDefaultData(t *testing.T) {
	base := Default()
	if len(base.land) < 150 || len(base.lakes) == 0 {
		t.Fatalf("land %d lakes %d", len(base.land), len(base.lakes))
	}
	for name, want := range map[string]struct {
		p    LatLon
		land bool
	}{
		"Paris":         {LatLon{48.8566, 2.3522}, true},
		"Roskilde":      {LatLon{55.64, 12.08}, true},
		"Sydney":        {LatLon{-33.87, 151.2}, true},
		"mid-Atlantic":  {LatLon{30, -40}, false},
		"Bay of Biscay": {LatLon{45.5, -5}, false},
		"Tokyo":         {LatLon{35.68, 139.69}, true},
		"South Pacific": {LatLon{-30, -120}, false},
	} {
		if got := inLand(base, want.p); got != want.land {
			t.Errorf("%s: land = %v, want %v", name, got, want.land)
		}
	}
}

func TestFitKeepsPointsInsidePadding(t *testing.T) {
	points := []LatLon{{43.3, -1.5}, {48.9, 2.4}, {55.7, 12.6}, {38.7, -9.1}}
	for _, size := range [][2]float64{{297, 210}, {210, 297}} {
		view := Fit(points, size[0], size[1], FitOptions{Padding: 20})
		for _, p := range points {
			xy := view.XY(p)
			if xy.X < 20-1e-6 || xy.X > size[0]-20+1e-6 || xy.Y < 20-1e-6 || xy.Y > size[1]-20+1e-6 {
				t.Errorf("%v on %v lands at %v", p, size, xy)
			}
		}
	}
}

func TestFitNorthIsUpAndCentred(t *testing.T) {
	view := Fit([]LatLon{{40, 0}, {50, 10}}, 200, 200, FitOptions{})
	north, south := view.XY(LatLon{50, 5}), view.XY(LatLon{40, 5})
	if north.Y >= south.Y {
		t.Errorf("north %v should be above south %v", north, south)
	}
	if c := view.XY(LatLon{Lat: 45.4, Lon: 5}); math.Abs(c.X-100) > 1e-6 || math.Abs(c.Y-100) > 6 {
		t.Errorf("centre of the points at %v", c)
	}
}

func TestFitSinglePointShowsSurroundings(t *testing.T) {
	view := Fit([]LatLon{{48.85, 2.35}}, 200, 200, FitOptions{Padding: 10})
	if got := view.MMPerDegree(); got > 100 {
		t.Errorf("scale %v mm/° zooms in too far", got)
	}
	if len(Default().Land(view)) == 0 {
		t.Error("no land around Paris")
	}
}

func TestSVGPath(t *testing.T) {
	view := Fit([]LatLon{{43, -2}, {51, 8}}, 297, 210, FitOptions{})
	d := SVGPath(Default().Land(view))
	if !strings.HasPrefix(d, "M") || !strings.HasSuffix(d, "Z") {
		t.Errorf("unexpected path %.40q", d)
	}
	if SVGPath(nil) != "" {
		t.Error("empty input should give an empty path")
	}
}
