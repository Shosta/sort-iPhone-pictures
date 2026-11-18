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

// Frame is the look of a printed photo.
type Frame string

const (
	FrameNone     Frame = "none"
	FramePolaroid Frame = "polaroid"
	FrameStamp    Frame = "stamp"
	FrameTape     Frame = "tape"
)

// ParseFrame resolves a frame name; an empty name selects FrameNone.
func ParseFrame(name string) (Frame, bool) {
	switch f := Frame(strings.ToLower(name)); f {
	case "":
		return FrameNone, true
	case FrameNone, FramePolaroid, FrameStamp, FrameTape:
		return f, true
	}
	return "", false
}

// Insets are the widths, in millimetres, of the border around the photo.
func (f Frame) Insets() (top, side, bottom float64) { return f.insets() }

func (f Frame) insets() (top, side, bottom float64) {
	switch f {
	case FramePolaroid:
		return 4, 4, 14
	case FrameStamp:
		return 3.5, 3.5, 3.5
	case FrameTape:
		return tapeInset, tapeInset, tapeInset
	}
	return 0, 0, 0
}

// Style is the arrangement of photos on a sheet.
type Style string

const (
	// StyleGrid puts photos of one size in tidy rows.
	StyleGrid Style = "grid"
	// StyleCollage varies photo sizes (every fifth photo is a larger hero) and
	// turns each one slightly.
	StyleCollage Style = "collage"
)

// ParseStyle resolves a layout style name; an empty name selects StyleGrid.
func ParseStyle(name string) (Style, bool) {
	switch s := Style(strings.ToLower(name)); s {
	case "":
		return StyleGrid, true
	case StyleGrid, StyleCollage:
		return s, true
	}
	return "", false
}

const heroScale = 1.4

var (
	collageScales = []float64{1, 0.85, 1, 0.9}
	collageAngles = []float64{-3, 2, -1.5, 3, 1, -2}
)

// Placement is a photo on a sheet. Index is the position of the photo in the
// list given to Layout. X, Y, W, H is the whole printed piece, frame
// included, which is what gets cut out (for a turned piece, the box around
// it); PX, PY, PW, PH is the photo inside it, before turning, with the same
// centre as the piece. Angle is the turn in degrees, counter-clockwise.
type Placement struct {
	Index          int
	X, Y, W, H     float64
	PX, PY, PW, PH float64
	Angle          float64
}

// Sheet is one printed page.
type Sheet struct {
	Placements []Placement
}

type item struct {
	index int
	w, h  float64
	// photo size, inside the frame
	pw, ph float64
	angle  float64
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
	return LayoutFramed(paper, FrameNone, longSide, gutter, ratios)
}

// LayoutFramed is Layout with a frame around every photo: longSide is the
// size of the photo itself, the frame comes on top, and gutter is kept
// between frames.
func LayoutFramed(paper Paper, frame Frame, longSide, gutter float64, ratios []float64) []Sheet {
	return LayoutStyled(paper, StyleGrid, frame, longSide, gutter, ratios)
}

// LayoutStyled is LayoutFramed with a choice of arrangement. In a collage the
// size of a photo varies around longSide and the pieces are turned a few
// degrees; the space between the boxes around the pieces is still gutter.
func LayoutStyled(paper Paper, style Style, frame Frame, longSide, gutter float64, ratios []float64) []Sheet {
	top, side, bottom := frame.insets()
	gutter = clamp(gutter, MinGutter, MaxGutter)
	usableW, usableH := paper.Width-2*Margin, paper.Height-2*Margin
	longSide = clamp(longSide, 10, math.Min(usableW-2*side, usableH-top-bottom))

	var portraits, landscapes []item
	for index, ratio := range ratios {
		if frame == FramePolaroid {
			ratio = 1 // photos are cropped square
		}
		if !(ratio > 0) || math.IsInf(ratio, 0) {
			ratio = 1
		}
		size, angle := longSide, 0.0
		if style == StyleCollage {
			scale := collageScales[index%len(collageScales)]
			if index%5 == 0 {
				scale = heroScale
			}
			size, angle = longSide*scale, collageAngles[(index*5+index/6)%len(collageAngles)]
		}
		var it item
		for range 40 {
			it = item{index: index, pw: size, ph: size, angle: angle}
			if ratio > 1 {
				it.ph = size / ratio
			} else {
				it.pw = size * ratio
			}
			it.w, it.h = turnedBox(it.pw+2*side, it.ph+top+bottom, angle)
			if (it.w <= usableW+epsilon && it.h <= usableH+epsilon) || size <= 10 {
				break
			}
			size *= 0.95
		}
		if ratio > 1 {
			landscapes = append(landscapes, it)
		} else {
			portraits = append(portraits, it)
		}
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
				// The unturned piece shares its centre with the box around it.
				pieceW, pieceH := it.pw+2*side, it.ph+top+bottom
				px, py := x+(it.w-pieceW)/2, y+(it.h-pieceH)/2
				sheets[i].Placements = append(sheets[i].Placements, Placement{
					Index: it.index, X: round(x), Y: round(y), W: round(it.w), H: round(it.h),
					PX: round(px + side), PY: round(py + top), PW: round(it.pw), PH: round(it.ph),
					Angle: it.angle,
				})
				x += it.w + gutter
			}
			y += r.height + gutter
		}
	}
	return sheets
}

// turnedBox is the size of the box around a w x h piece turned by angle degrees.
func turnedBox(w, h, angle float64) (float64, float64) {
	sin, cos := math.Abs(math.Sin(angle*math.Pi/180)), math.Abs(math.Cos(angle*math.Pi/180))
	return w*cos + h*sin, w*sin + h*cos
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
