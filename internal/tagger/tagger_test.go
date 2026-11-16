package tagger

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMomentTag(t *testing.T) {
	paris := func(h, m int) time.Time { return time.Date(2025, 6, 21, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"sunrise", paris(5, 55), TagSunrise},
		{"noon", paris(13, 0), ""},
		{"sunset", paris(21, 50), TagSunset},
		{"night", paris(23, 59), TagNight},
		{"small hours", paris(2, 0), TagNight},
	}
	for _, c := range cases {
		if got := momentTag(c.at, 48.85, 2.35, 7200, true); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := momentTag(time.Time{}, 48.85, 2.35, 0, false); got != "" {
		t.Errorf("zero time: %q", got)
	}
}

func TestSceneTags(t *testing.T) {
	labels := []string{"beach", "coast", "street", "sky", "castle"}
	tags, outdoor := sceneTags(labels, []float64{0.2, 0.2, 0.05, 0.4, 0.15})
	if len(tags) != 1 || tags[0] != TagBeach || !outdoor {
		t.Fatalf("got %v outdoor=%v", tags, outdoor)
	}
	if tags, _ := sceneTags(labels, []float64{0.1, 0.1, 0.1, 0.1, 0.1}); len(tags) != 0 {
		t.Fatalf("weak scores must not tag: %v", tags)
	}
}

func TestSceneTagsIndoorAndSpecificPlaces(t *testing.T) {
	labels := []string{"kitchen", "restaurant", "pizzeria", "bridge", "river", "street"}
	tags, outdoor := sceneTags(labels, []float64{0.45, 0.2, 0.15, 0.05, 0.05, 0.1})
	if outdoor || !hasTag(tags, TagKitchen) || !hasTag(tags, TagRestaurant) || !hasTag(tags, TagIndoor) {
		t.Fatalf("got %v outdoor=%v", tags, outdoor)
	}
	if hasTag(tags, TagBridge) || hasTag(tags, TagRiver) {
		t.Fatalf("weak groups must not tag: %v", tags)
	}
	if tags, _ := sceneTags(labels, []float64{0.05, 0.05, 0.05, 0.35, 0.35, 0.1}); hasTag(tags, TagIndoor) || !hasTag(tags, TagBridge) || !hasTag(tags, TagRiver) {
		t.Fatalf("outdoor scene: %v", tags)
	}
}

func TestSceneGroupLabelsExist(t *testing.T) {
	labels, err := loadCategories(filepath.Join("..", "..", "photosort-electron", "ai-assets", "categories.txt"))
	if err != nil {
		t.Skip("categories.txt not downloaded:", err)
	}
	known := setOf(strings.Join(labels, " "))
	for tag, members := range sceneGroups {
		for _, member := range members {
			if !known[member] {
				t.Errorf("%s: unknown Places365 category %q", tag, member)
			}
		}
	}
	for label := range indoorLabels {
		if !known[label] {
			t.Errorf("indoor: unknown Places365 category %q", label)
		}
	}
}

func TestMetaTags(t *testing.T) {
	front := "iPhone 12 Pro front camera 2.71mm f/2.2"
	cases := []struct {
		name  string
		photo Photo
		want  []string
		not   []string
	}{
		{"saturday new year", Photo{Path: "a.jpg", TakenAt: time.Date(2022, 1, 1, 12, 0, 0, 0, time.UTC)},
			[]string{TagWinter, "2022", TagWeekend, TagNewYear}, nil},
		{"weekday summer", Photo{Path: "a.jpg", TakenAt: time.Date(2025, 7, 16, 12, 0, 0, 0, time.UTC)},
			[]string{TagSummer, "2025"}, []string{TagWeekend, TagNewYear}},
		{"southern winter", Photo{Path: "a.jpg", TakenAt: time.Date(2025, 7, 16, 12, 0, 0, 0, time.UTC), Lat: -33.9, HasGPS: true},
			[]string{TagWinter}, []string{TagSummer}},
		{"no date", Photo{Path: "a.jpg"}, nil, []string{"0001", TagWinter}},
		{"selfie", Photo{Path: "a.jpg", Shot: Shot{Lens: front}}, []string{TagSelfie}, nil},
		{"screenshot comment", Photo{Path: "IMG_1.PNG", Shot: Shot{Comment: "Screenshot", Lens: front}}, []string{TagScreenshot}, []string{TagSelfie}},
		{"screenshot name", Photo{Path: "/x/Capture d’écran 2026-01-01.png"}, []string{TagScreenshot}, nil},
		{"panorama", Photo{Path: "a.jpg", Shot: Shot{Width: 12000, Height: 3000}}, []string{TagPanorama}, nil},
		{"wide not panorama", Photo{Path: "a.jpg", Shot: Shot{Width: 4032, Height: 3024}}, nil, []string{TagPanorama}},
		{"zoom", Photo{Path: "a.jpg", Shot: Shot{Focal35: 120}}, []string{TagZoom}, nil},
		{"wide angle", Photo{Path: "a.jpg", Shot: Shot{Focal35: 26}}, nil, []string{TagZoom}},
		{"macro distance", Photo{Path: "a.jpg", Shot: Shot{SubjectDistance: 0.08}}, []string{TagMacro}, nil},
		{"low light brightness", Photo{Path: "a.jpg", Shot: Shot{Brightness: -2.5, HasBrightness: true, ISO: 100}}, []string{TagLowLight}, nil},
		{"daylight", Photo{Path: "a.jpg", Shot: Shot{Brightness: 7, HasBrightness: true, ISO: 25}}, nil, []string{TagLowLight}},
		{"low light iso", Photo{Path: "a.jpg", Shot: Shot{ISO: 3200}}, []string{TagLowLight}, nil},
	}
	for _, c := range cases {
		got := metaTags(c.photo)
		for _, tag := range c.want {
			if !hasTag(got, tag) {
				t.Errorf("%s: missing %q in %v", c.name, tag, got)
			}
		}
		for _, tag := range c.not {
			if hasTag(got, tag) {
				t.Errorf("%s: unexpected %q in %v", c.name, tag, got)
			}
		}
	}
}

func TestMetaTagsReadImageHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewGray(image.Rect(0, 0, 300, 100))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if got := metaTags(Photo{Path: path}); !hasTag(got, TagPanorama) {
		t.Fatalf("header size not used: %v", got)
	}
}

func TestClassifyFaces(t *testing.T) {
	big := faceBox{100, 100, 400, 500, 0.9}
	if people, portrait, n := classifyFaces([]faceBox{big}, 1000); !people || !portrait || n != 1 {
		t.Error("single large face should be a portrait")
	}
	if people, portrait, n := classifyFaces([]faceBox{big, {450, 100, 700, 450, 0.9}}, 1000); !people || portrait || n != 2 {
		t.Error("two similar faces: people, not portrait")
	}
	if people, _, n := classifyFaces([]faceBox{{0, 0, 10, 10, 0.9}}, 1000); people || n != 0 {
		t.Error("tiny face ignored")
	}
	var crowd []faceBox
	for i := 0; i < 5; i++ {
		x := float64(i * 150)
		crowd = append(crowd, faceBox{x, 100, x + 100, 200, 0.9})
	}
	if _, _, n := classifyFaces(crowd, 1000); n < groupMinFaces {
		t.Errorf("crowd counted %d faces", n)
	}
}

func TestXMPEscapesTags(t *testing.T) {
	out := string(XMP([]string{"église", "a&b"}))
	if !strings.Contains(out, "<rdf:li>église</rdf:li>") || !strings.Contains(out, "a&amp;b") {
		t.Fatalf("unexpected xmp:\n%s", out)
	}
}
