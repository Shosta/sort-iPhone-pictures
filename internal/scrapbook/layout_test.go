package scrapbook

import (
	"math"
	"testing"
)

func repeat(ratio float64, n int) []float64 {
	ratios := make([]float64, n)
	for i := range ratios {
		ratios[i] = ratio
	}
	return ratios
}

func a4Landscape(t *testing.T) Paper {
	t.Helper()
	paper, ok := ParsePaper("")
	if !ok || paper.Width != 297 || paper.Height != 210 {
		t.Fatalf("default paper = %+v", paper)
	}
	return paper
}

// checkSheets verifies what matters on paper: the photos have the requested
// size and proportions, stay inside the margins and keep their distance.
func checkSheets(t *testing.T, paper Paper, longSide float64, ratios []float64, sheets []Sheet) {
	t.Helper()
	seen := map[int]bool{}
	for s, sheet := range sheets {
		for i, a := range sheet.Placements {
			if seen[a.Index] {
				t.Errorf("photo %d placed twice", a.Index)
			}
			seen[a.Index] = true
			if math.Abs(math.Max(a.W, a.H)-longSide) > 0.1 {
				t.Errorf("sheet %d photo %d: long side %.3f, want %.1f", s, a.Index, math.Max(a.W, a.H), longSide)
			}
			if math.Abs(a.W/a.H-ratios[a.Index]) > 0.01 {
				t.Errorf("photo %d: ratio %.3f, want %.3f", a.Index, a.W/a.H, ratios[a.Index])
			}
			if a.X < Margin-epsilon || a.Y < Margin-epsilon || a.X+a.W > paper.Width-Margin+0.001 || a.Y+a.H > paper.Height-Margin+0.001 {
				t.Errorf("sheet %d photo %d outside the margins: %+v", s, a.Index, a)
			}
			for _, b := range sheet.Placements[i+1:] {
				gapX := math.Max(b.X-(a.X+a.W), a.X-(b.X+b.W))
				gapY := math.Max(b.Y-(a.Y+a.H), a.Y-(b.Y+b.H))
				if math.Max(gapX, gapY) < MinGutter-0.001 {
					t.Errorf("sheet %d: photos %d and %d are %.3f mm apart", s, a.Index, b.Index, math.Max(gapX, gapY))
				}
			}
		}
	}
	if len(seen) != len(ratios) {
		t.Errorf("%d photos placed, want %d", len(seen), len(ratios))
	}
}

func TestLayoutPortraitsTenPerA4LandscapePage(t *testing.T) {
	ratios := repeat(3.0/4.0, 10)
	sheets := Layout(a4Landscape(t), DefaultLongSide, MinGutter, ratios)
	if len(sheets) != 1 {
		t.Fatalf("%d sheets, want 1", len(sheets))
	}
	checkSheets(t, a4Landscape(t), DefaultLongSide, ratios, sheets)
	if more := Layout(a4Landscape(t), DefaultLongSide, MinGutter, repeat(3.0/4.0, 11)); len(more) != 2 {
		t.Errorf("11 portraits give %d sheets, want 2", len(more))
	}
}

func TestLayoutLandscapesTwelvePerA4LandscapePage(t *testing.T) {
	ratios := repeat(4.0/3.0, 12)
	sheets := Layout(a4Landscape(t), DefaultLongSide, MinGutter, ratios)
	if len(sheets) != 1 {
		t.Fatalf("%d sheets, want 1", len(sheets))
	}
	checkSheets(t, a4Landscape(t), DefaultLongSide, ratios, sheets)
	if more := Layout(a4Landscape(t), DefaultLongSide, MinGutter, repeat(4.0/3.0, 13)); len(more) != 2 {
		t.Errorf("13 landscapes give %d sheets, want 2", len(more))
	}
}

func TestLayoutMixedPhotosKeepTheirSize(t *testing.T) {
	var ratios []float64
	for i := 0; i < 37; i++ {
		ratios = append(ratios, []float64{3.0 / 4.0, 4.0 / 3.0, 1, 16.0 / 9.0, 9.0 / 16.0, 2.0 / 3.0}[i%6])
	}
	for _, name := range []string{"a4-landscape", "a4-portrait", "letter-landscape", "letter-portrait"} {
		paper, _ := ParsePaper(name)
		for _, gutter := range []float64{0, 5, 8, 10} {
			checkSheets(t, paper, DefaultLongSide, ratios, Layout(paper, DefaultLongSide, gutter, ratios))
		}
	}
}

func TestLayoutNeverGoesBelowTheMinimumSpace(t *testing.T) {
	ratios := repeat(3.0/4.0, 25)
	sheets := Layout(a4Landscape(t), DefaultLongSide, 1, ratios)
	checkSheets(t, a4Landscape(t), DefaultLongSide, ratios, sheets)
}

func TestLayoutBiggerSpaceMeansFewerPhotosPerPage(t *testing.T) {
	ratios := repeat(3.0/4.0, 10)
	if sheets := Layout(a4Landscape(t), DefaultLongSide, 10, ratios); len(sheets) < 2 {
		t.Errorf("a 10 mm space should not fit 10 portraits on one page, got %d sheets", len(sheets))
	}
}

func TestLayoutPartialRowsShareARow(t *testing.T) {
	// 3 portraits and 1 landscape fit in one row of the sheet.
	ratios := []float64{3.0 / 4.0, 3.0 / 4.0, 3.0 / 4.0, 4.0 / 3.0}
	sheets := Layout(a4Landscape(t), DefaultLongSide, MinGutter, ratios)
	if len(sheets) != 1 {
		t.Fatalf("%d sheets, want 1", len(sheets))
	}
	rows := map[float64]bool{}
	for _, placement := range sheets[0].Placements {
		rows[placement.Y] = true
	}
	if len(rows) != 1 {
		t.Errorf("photos are on %d rows, want 1", len(rows))
	}
	checkSheets(t, a4Landscape(t), DefaultLongSide, ratios, sheets)
}

func TestLayoutKeepsOrderWithinOrientation(t *testing.T) {
	ratios := repeat(3.0/4.0, 6)
	var order []int
	for _, sheet := range Layout(a4Landscape(t), DefaultLongSide, MinGutter, ratios) {
		for _, placement := range sheet.Placements {
			order = append(order, placement.Index)
		}
	}
	for i, index := range order {
		if index != i {
			t.Fatalf("order = %v", order)
		}
	}
}

func TestLayoutHandlesEmptyAndOddInput(t *testing.T) {
	if sheets := Layout(a4Landscape(t), DefaultLongSide, MinGutter, nil); len(sheets) != 0 {
		t.Errorf("no photos should give no sheets, got %d", len(sheets))
	}
	ratios := []float64{0, math.NaN(), math.Inf(1), -2, 1}
	checkSheets(t, a4Landscape(t), DefaultLongSide, []float64{1, 1, 1, 1, 1}, Layout(a4Landscape(t), DefaultLongSide, MinGutter, ratios))
	// A photo larger than the page is brought back to what fits.
	huge := Layout(a4Landscape(t), 500, MinGutter, []float64{1})
	if len(huge) != 1 || huge[0].Placements[0].H > 190+epsilon {
		t.Errorf("oversized photo = %+v", huge)
	}
}

func TestParsePaper(t *testing.T) {
	if _, ok := ParsePaper("tabloid"); ok {
		t.Error("unknown paper accepted")
	}
	if paper, ok := ParsePaper("A4-Portrait"); !ok || paper.Landscape() {
		t.Errorf("A4-Portrait = %+v", paper)
	}
}

func TestAlbumFormats(t *testing.T) {
	sizes := map[string][2]float64{
		"10x15-landscape": {150, 100}, "10x15-portrait": {100, 150}, "square-20": {200, 200},
	}
	if _, ok := ParsePaper("a5-portrait"); ok {
		t.Error("a5 should not be offered")
	}
	for name, size := range sizes {
		paper, ok := ParsePaper(name)
		if !ok || paper.Width != size[0] || paper.Height != size[1] {
			t.Errorf("%s = %+v, %v", name, paper, ok)
			continue
		}
		ratios := repeat(3.0/4.0, 6)
		checkSheets(t, paper, 40, ratios, Layout(paper, 40, MinGutter, ratios))
	}
}

func TestBleedKeepsSafetyZone(t *testing.T) {
	paper, _ := ParsePaper("square-20")
	paper = paper.WithBleed(DefaultBleed)
	if paper.SheetWidth() != 206 || paper.SheetHeight() != 206 {
		t.Fatalf("sheet = %v x %v", paper.SheetWidth(), paper.SheetHeight())
	}
	if clamped := paper.WithBleed(99); clamped.Bleed != MaxBleed {
		t.Errorf("bleed not clamped: %v", clamped.Bleed)
	}
	for _, sheet := range Layout(paper, 80, MinGutter, repeat(1, 8)) {
		for _, a := range sheet.Placements {
			low := paper.Bleed + Margin
			if a.X < low-epsilon || a.Y < low-epsilon || a.X+a.W > paper.Bleed+paper.Width-Margin+0.001 || a.Y+a.H > paper.Bleed+paper.Height-Margin+0.001 {
				t.Errorf("photo %d leaves the safety zone: %+v", a.Index, a)
			}
		}
	}
}

func TestFramedLayoutKeepsCuttingSpace(t *testing.T) {
	paper := a4Landscape(t)
	ratios := append(repeat(3.0/4.0, 7), repeat(4.0/3.0, 6)...)
	for _, frame := range []Frame{FramePolaroid, FrameStamp} {
		top, side, bottom := frame.insets()
		sheets := LayoutFramed(paper, frame, DefaultLongSide, MinGutter, ratios)
		placed := 0
		for _, sheet := range sheets {
			for i, a := range sheet.Placements {
				placed++
				if math.Abs(math.Max(a.PW, a.PH)-DefaultLongSide) > 0.1 || math.Abs(a.PW/a.PH-ratios[a.Index]) > 0.01 {
					t.Errorf("%s: photo %d resized: %+v", frame, a.Index, a)
				}
				if math.Abs(a.PX-a.X-side) > 0.01 || math.Abs(a.PY-a.Y-top) > 0.01 || math.Abs(a.H-a.PH-top-bottom) > 0.01 {
					t.Errorf("%s: wrong frame around photo %d: %+v", frame, a.Index, a)
				}
				if a.X < Margin-epsilon || a.Y < Margin-epsilon || a.X+a.W > paper.Width-Margin+0.001 || a.Y+a.H > paper.Height-Margin+0.001 {
					t.Errorf("%s: piece %d outside the margins: %+v", frame, a.Index, a)
				}
				for _, b := range sheet.Placements[i+1:] {
					gapX := math.Max(b.X-(a.X+a.W), a.X-(b.X+b.W))
					gapY := math.Max(b.Y-(a.Y+a.H), a.Y-(b.Y+b.H))
					if math.Max(gapX, gapY) < MinGutter-0.001 {
						t.Errorf("%s: pieces %d and %d too close", frame, a.Index, b.Index)
					}
				}
			}
		}
		if placed != len(ratios) {
			t.Errorf("%s: %d placed, want %d", frame, placed, len(ratios))
		}
	}
	if _, ok := ParseFrame("wooden"); ok {
		t.Error("unknown frame accepted")
	}
}
