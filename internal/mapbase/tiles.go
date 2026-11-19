package mapbase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // tile formats
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Provider supplies raster map tiles (Web Mercator, 256 px, XYZ scheme). It is
// the seam between the page layout and where the picture of the map comes
// from: the vector base of this package needs no provider, any other source
// plugs in by implementing it.
type Provider interface {
	ID() string
	Name() string
	// Attribution must be shown wherever the map is displayed.
	Attribution() string
	MaxZoom() int
	Tile(ctx context.Context, z, x, y int) (image.Image, error)
}

// UserAgent identifies the application to tile servers, as their usage
// policies require.
const UserAgent = "Photosort/1.0 (desktop photo sorter; +https://github.com/remilavedrine/sort-iPhone-pictures)"

// Referer is sent with tile requests; some servers refuse anonymous ones.
const Referer = "https://github.com/remilavedrine/sort-iPhone-pictures"

// cacheVersion names the cache layout; bumping it drops tiles stored by older
// versions, which could include error pages.
const cacheVersion = "v2"

// fetchSlots caps concurrent tile requests (OSM policy: at most 2).
var fetchSlots = make(chan struct{}, 2)

// maxTiles bounds what one map downloads; tiles are also cached on disk.
const maxTiles = 36

// TileServer is a Provider fetching tiles over HTTP with a disk cache.
type TileServer struct {
	Key, Label, Credit string
	// URL has the {z}, {x} and {y} placeholders.
	URL      string
	Zoom     int
	CacheDir string
	Client   *http.Client
}

func (s *TileServer) ID() string          { return s.Key }
func (s *TileServer) Name() string        { return s.Label }
func (s *TileServer) Attribution() string { return s.Credit }
func (s *TileServer) MaxZoom() int        { return s.Zoom }

func (s *TileServer) Tile(ctx context.Context, z, x, y int) (image.Image, error) {
	file := ""
	if s.CacheDir != "" {
		file = filepath.Join(s.CacheDir, cacheVersion, s.Key, fmt.Sprint(z), fmt.Sprint(x), fmt.Sprint(y)+".tile")
		if data, err := os.ReadFile(file); err == nil {
			if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
				return img, nil
			}
		}
	}
	url := strings.NewReplacer("{z}", fmt.Sprint(z), "{x}", fmt.Sprint(x), "{y}", fmt.Sprint(y)).Replace(s.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", Referer)
	req.Header.Set("Accept", "image/png,image/jpeg,image/*;q=0.8")
	select {
	case fetchSlots <- struct{}{}:
		defer func() { <-fetchSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile server answered %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/png") && !strings.HasPrefix(ct, "image/jpeg") {
		return nil, fmt.Errorf("tile server sent %q instead of an image", ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if file != "" && os.MkdirAll(filepath.Dir(file), 0o755) == nil {
		tmp := file + ".part"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, file)
		}
	}
	return img, nil
}

// DefaultProviderID is the background whose policy allows desktop apps
// without a key; the others are options or fallbacks.
const DefaultProviderID = "osm"

// Providers lists the online backgrounds, the default first. None needs an account or a key.
// cacheDir keeps downloaded tiles; empty disables the cache.
func Providers(cacheDir string) []Provider {
	osm := "© OpenStreetMap contributors"
	return []Provider{
		&TileServer{Key: "osm", Label: "OpenStreetMap", Credit: osm, URL: "https://tile.openstreetmap.org/{z}/{x}/{y}.png", Zoom: 19, CacheDir: cacheDir},
		&TileServer{Key: "esritopo", Label: "Esri World Topo", Credit: "Esri, HERE, Garmin, FAO, NOAA, USGS, " + osm, URL: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Topo_Map/MapServer/tile/{z}/{y}/{x}", Zoom: 18, CacheDir: cacheDir},
	}
}

// ProviderByID finds a provider; ok is false for "offline" and unknown ids.
func ProviderByID(providers []Provider, id string) (Provider, bool) {
	for _, p := range providers {
		if p.ID() == id {
			return p, true
		}
	}
	return nil, false
}

// TileCacheDir is the default place for downloaded tiles.
func TileCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Photosort", "tiles")
}

// Backdrop paints the tiles covering the view into an image of
// Width*ppmm x Height*ppmm pixels. The zoom is the finest that keeps the
// download small and the picture sharp enough for ppmm pixels per millimetre.
func Backdrop(ctx context.Context, p Provider, v View, ppmm float64) (*image.RGBA, error) {
	if v.scale <= 0 || ppmm <= 0 {
		return nil, errors.New("empty view")
	}
	pxW, pxH := int(math.Ceil(v.Width*ppmm)), int(math.Ceil(v.Height*ppmm))
	// World size in pixels at zoom z is 256*2^z for 360 Mercator degrees.
	z := int(math.Ceil(math.Log2(v.scale * ppmm * 360 / 256)))
	z = max(0, min(z, p.MaxZoom()))
	var x0, x1, y0, y1 int
	for ; ; z-- {
		n := 256 * math.Pow(2, float64(z))
		left, top := v.worldPixel(0, 0, n)
		right, bottom := v.worldPixel(v.Width, v.Height, n)
		x0, x1 = int(math.Floor(left/256)), int(math.Floor(right/256))
		y0, y1 = int(math.Floor(top/256)), int(math.Floor(bottom/256))
		if (x1-x0+1)*(y1-y0+1) <= maxTiles || z == 0 {
			break
		}
	}
	tiles := 1 << z
	type key struct{ x, y int }
	loaded := map[key]image.Image{}
	var lock sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	slots := make(chan struct{}, 2)
	for ty := max(y0, 0); ty <= min(y1, tiles-1); ty++ {
		for tx := x0; tx <= x1; tx++ {
			wx := ((tx % tiles) + tiles) % tiles
			wg.Add(1)
			go func() {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				img, err := p.Tile(ctx, z, wx, ty)
				lock.Lock()
				defer lock.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					return
				}
				loaded[key{tx, ty}] = img
			}()
		}
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, pxW, pxH))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.RGBA{221, 221, 221, 255}), image.Point{}, draw.Src)
	n := 256 * math.Pow(2, float64(z))
	for py := 0; py < pxH; py++ {
		for px := 0; px < pxW; px++ {
			wx, wy := v.worldPixel(float64(px)/ppmm, float64(py)/ppmm, n)
			img, ok := loaded[key{int(math.Floor(wx / 256)), int(math.Floor(wy / 256))}]
			if !ok {
				continue
			}
			b := img.Bounds()
			sx := b.Min.X + int(math.Mod(wx, 256))*b.Dx()/256
			sy := b.Min.Y + int(math.Mod(wy, 256))*b.Dy()/256
			out.Set(px, py, img.At(sx, sy))
		}
	}
	return out, nil
}

// worldPixel is the position, in a world of n pixels, of the page point
// (xmm, ymm).
func (v View) worldPixel(xmm, ymm, n float64) (float64, float64) {
	lon := v.cx + (xmm-v.Width/2)/v.scale
	merc := v.cy - (ymm-v.Height/2)/v.scale
	return (lon + 180) / 360 * n, (0.5 - merc/360) * n
}
