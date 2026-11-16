package tagger

import (
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

func TestClassifyFaces(t *testing.T) {
	big := faceBox{100, 100, 400, 500, 0.9}
	small := faceBox{600, 300, 640, 350, 0.9}
	if people, portrait := classifyFaces([]faceBox{big}, 1000); !people || !portrait {
		t.Error("single large face should be a portrait")
	}
	if people, portrait := classifyFaces([]faceBox{big, {450, 100, 700, 450, 0.9}}, 1000); !people || portrait {
		t.Error("two similar faces: people, not portrait")
	}
	if people, _ := classifyFaces([]faceBox{{0, 0, 10, 10, 0.9}}, 1000); people {
		t.Error("tiny face ignored")
	}
	_ = small
}

func TestXMPEscapesTags(t *testing.T) {
	out := string(XMP([]string{"église", "a&b"}))
	if !strings.Contains(out, "<rdf:li>église</rdf:li>") || !strings.Contains(out, "a&amp;b") {
		t.Fatalf("unexpected xmp:\n%s", out)
	}
}
