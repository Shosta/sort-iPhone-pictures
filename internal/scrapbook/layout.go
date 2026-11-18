package scrapbook

import (
	"math"
	"strings"
)

// Sizes are in millimetres.
const (
	// MinGutter is the smallest space left between two photos, and between two
	// rows, so that they can be cut apart.
	MinGutter = 5.0
	// MaxGutter is the largest space a caller can ask for.
	MaxGutter = 30.0
	// Margin is the safety zone kept free inside the trimmed page: printers
	// cannot reach the edges and cutting is never exact.
	Margin = 10.0
	// DefaultLongSide is the usual size of a photo in the planner (6.5 cm).
	DefaultLongSide = 65.0

	epsilon = 1e-6
)

// MaxBleed is the largest bleed a caller can ask for.
const MaxBleed = 10.0

// DefaultBleed is the usual print bleed.
const DefaultBleed = 3.0

// Paper is a page size. Width and Height are the trimmed page; Bleed is the
// extra band printed beyond each edge, to be cut off.
type Paper struct {
	Name          string
	Width, Height float64
	Bleed         float64
}

// WithBleed returns the paper with a bleed of mm (clamped to 0..MaxBleed).
func (p Paper) WithBleed(mm float64) Paper {
	p.Bleed = clamp(mm, 0, MaxBleed)
	return p
}

// SheetWidth is the width of the printed sheet, bleed included.
func (p Paper) SheetWidth() float64 { return p.Width + 2*p.Bleed }

// SheetHeight is the height of the printed sheet, bleed included.
func (p Paper) SheetHeight() float64 { return p.Height + 2*p.Bleed }

var papers = map[string]Paper{
	"a4-landscape":     {Name: "A4", Width: 297, Height: 210},
	"a4-portrait":      {Name: "A4", Width: 210, Height: 297},
	"letter-landscape": {Name: "Letter", Width: 279.4, Height: 215.9},
	"letter-portrait":  {Name: "Letter", Width: 215.9, Height: 279.4},
	"a5-landscape":     {Name: "A5", Width: 210, Height: 148},
	"a5-portrait":      {Name: "A5", Width: 148, Height: 210},
	"10x15-landscape":  {Name: "10x15", Width: 150, Height: 100},
	"10x15-portrait":   {Name: "10x15", Width: 100, Height: 150},
	"13x18-landscape":  {Name: "13x18", Width: 180, Height: 130},
	"13x18-portrait":   {Name: "13x18", Width: 130, Height: 180},
	"square-20":        {Name: "20x20", Width: 200, Height: 200},
	"square-21":        {Name: "21x21", Width: 210, Height: 210},
}

// DefaultPaper is the page used when none is given.
const DefaultPaper = "a4-landscape"

// ParsePaper resolves a page name such as "a4-landscape"; an empty name
// selects DefaultPaper.
func ParsePaper(name string) (Paper, bool) {
	if name == "" {
		name = DefaultPaper
	}
	paper, ok := papers[strings.ToLower(name)]
	return paper, ok
}

// Landscape reports whether the sheet is wider than tall.
func (p Paper) Landscape() bool { return p.Width > p.Height }

// Placement is a photo on a sheet. Index is the position of the photo in the
// list given to Layout.
type Placement struct {
	Index      int
	X, Y, W, H float64
}

// Sheet is one printed page.
type Sheet struct {
	Placements []Placement
}

type item struct {
	index int
	w, h  float64
}

type row struct {
	items  []item
	width  float64
	height float64
}

func (r *row) add(it item, gutter float64) {
	if len(r.items) > 0 {
		r.width += gutter
	}
	r.items = append(r.items, it)
	r.width += it.w
	r.height = math.Max(r.height, it.h)
}

// Layout places photos on sheets, each scaled so that its longest side is
// longSide, without cropping. ratios holds the width/height ratio of every
// photo as it is shown (upright). At least gutter (and never less than
// MinGutter) is left between neighbours.
//
// Photos are laid out in rows, so that a sheet is cut in strips and then the
// strips in photos. Portraits and landscapes are kept in separate rows, whose
// height is that of their photos, and the two partial rows left over are
// merged into one when they fit side by side. Rows are then put on the first
// sheet with room for them.
func Layout(paper Paper, longSide, gutter float64, ratios []float64) []Sheet {
	gutter = clamp(gutter, MinGutter, MaxGutter)
	usableW, usableH := paper.Width-2*Margin, paper.Height-2*Margin
	longSide = clamp(longSide, 10, math.Min(usableW, usableH))

	var portraits, landscapes []item
	for index, ratio := range ratios {
		if !(ratio > 0) || math.IsInf(ratio, 0) {
			ratio = 1
		}
		it := item{index: index, w: longSide, h: longSide}
		if ratio > 1 {
			it.h = longSide / ratio
			landscapes = append(landscapes, it)
			continue
		}
		it.w = longSide * ratio
		portraits = append(portraits, it)
	}
	portraitRows, landscapeRows := fill(portraits, usableW, gutter), fill(landscapes, usableW, gutter)

	var mixed []row
	if p, l := len(portraitRows), len(landscapeRows); p > 0 && l > 0 && portraitRows[p-1].width+gutter+landscapeRows[l-1].width <= usableW+epsilon {
		merged := portraitRows[p-1]
		for _, it := range landscapeRows[l-1].items {
			merged.add(it, gutter)
		}
		mixed = []row{merged}
		portraitRows, landscapeRows = portraitRows[:p-1], landscapeRows[:l-1]
	}

	type sheetRows struct {
		rows []row
		used float64
	}
	var pages []*sheetRows
	for _, rows := range [][]row{portraitRows, mixed, landscapeRows} {
		for _, r := range rows {
			var page *sheetRows
			for _, candidate := range pages {
				if candidate.used+gutter+r.height <= usableH+epsilon {
					page = candidate
					break
				}
			}
			switch {
			case page == nil:
				pages = append(pages, &sheetRows{rows: []row{r}, used: r.height})
			default:
				page.rows = append(page.rows, r)
				page.used += gutter + r.height
			}
		}
	}

	sheets := make([]Sheet, len(pages))
	for i, page := range pages {
		y := Margin + paper.Bleed
		for _, r := range page.rows {
			x := Margin + paper.Bleed
			for _, it := range r.items {
				sheets[i].Placements = append(sheets[i].Placements, Placement{Index: it.index, X: round(x), Y: round(y), W: round(it.w), H: round(it.h)})
				x += it.w + gutter
			}
			y += r.height + gutter
		}
	}
	return sheets
}

// fill cuts items, in order, into rows no wider than width.
func fill(items []item, width, gutter float64) []row {
	var rows []row
	var current row
	for _, it := range items {
		if len(current.items) > 0 && current.width+gutter+it.w > width+epsilon {
			rows = append(rows, current)
			current = row{}
		}
		current.add(it, gutter)
	}
	if len(current.items) > 0 {
		rows = append(rows, current)
	}
	return rows
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}

func round(value float64) float64 {
	return math.Round(value*1000) / 1000
}
