package scrapbook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"sort-iphone-pictures/internal/thumbs"

	"github.com/go-pdf/fpdf"
)

const (
	// printDPI is the resolution photos are reduced to: sharp on paper without
	// producing huge files.
	printDPI    = 300
	jpegQuality = 90
	mmPerInch   = 25.4
)

// Job describes a PDF to write.
type Job struct {
	Paper Paper
	Frame Frame
	// Background is the colour of the page; the zero value leaves it white.
	Background Background
	// Photos are the file paths, indexed by Placement.Index.
	Photos []string
	Sheets []Sheet
	// Dir and Name give the output file; an existing file is never replaced,
	// a numbered name is used instead.
	Dir  string
	Name string
	// FallbackDir is used when Dir cannot be written to.
	FallbackDir string
	Title       string
	// Progress (optional) is called after each photo, from one goroutine.
	Progress func(done, total int)
}

// Background is a page colour.
type Background string

var backgrounds = map[Background][3]int{
	"cream":    {250, 241, 220},
	"rose":     {248, 224, 228},
	"mint":     {222, 242, 230},
	"sky":      {220, 236, 248},
	"lavender": {233, 226, 247},
	"kraft":    {222, 200, 166},
}

// ParseBackground resolves a background name; an empty name or "none" is a
// plain white page.
func ParseBackground(name string) (Background, bool) {
	name = strings.ToLower(name)
	if name == "" || name == "none" {
		return "", true
	}
	_, ok := backgrounds[Background(name)]
	return Background(name), ok
}

// Failure is a photo that could not be printed.
type Failure struct {
	Path  string
	Error string
}

// Result describes the written PDF.
type Result struct {
	Path     string
	Pages    int
	Failed   []Failure
	Fallback bool
}

type rendered struct {
	data          []byte
	width, height int
	err           error
}

var unsafeName = regexp.MustCompile(`[\x00-\x1f\\/:*?"<>|]+`)

// SafeName turns text into a file name ending with .pdf.
func SafeName(text string) string {
	name := unsafeName.ReplaceAllString(text, "-")
	name = strings.Trim(strings.TrimSpace(name), ". ")
	if strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name = strings.Trim(name[:len(name)-4], ". ")
	}
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	if strings.TrimSpace(name) == "" {
		name = "Photosort"
	}
	return name + ".pdf"
}

// Render writes the PDF of job. Photos are decoded and reduced in parallel;
// pages are assembled in order. Nothing is left behind if ctx is cancelled.
func Render(ctx context.Context, job Job) (Result, error) {
	dir, fallback, err := outputDir(job.Dir, job.FallbackDir)
	if err != nil {
		return Result{}, err
	}
	temp, err := os.CreateTemp(dir, ".photosort-*.pdf.tmp")
	if err != nil {
		return Result{}, fmt.Errorf("cannot write in %s: %w", dir, err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()

	pdf, failed, err := draw(ctx, job)
	if err != nil {
		return Result{}, err
	}
	if err := pdf.Output(temp); err != nil {
		return Result{}, err
	}
	if err := temp.Close(); err != nil {
		return Result{}, err
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return Result{}, err
	}
	path, err := publish(temp.Name(), dir, SafeName(job.Name))
	if err != nil {
		return Result{}, err
	}
	return Result{Path: path, Pages: len(job.Sheets), Failed: failed, Fallback: fallback}, nil
}

// draw builds the document of job.
func draw(ctx context.Context, job Job) (*fpdf.Fpdf, []Failure, error) {
	// The size is given as is: fpdf would swap it again for "L".
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: fpdf.SizeType{Wd: job.Paper.SheetWidth(), Ht: job.Paper.SheetHeight()}})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCreator("Photosort", true)
	if job.Title != "" {
		pdf.SetTitle(job.Title, true)
	}
	pdf.SetDrawColor(170, 170, 170)
	pdf.SetLineWidth(0.15)

	type slot struct {
		sheet     int
		placement Placement
	}
	var slots []slot
	for sheet, s := range job.Sheets {
		for _, placement := range s.Placements {
			slots = append(slots, slot{sheet, placement})
		}
	}
	results := make([]chan rendered, len(slots))
	for i := range results {
		results[i] = make(chan rendered, 1)
	}
	workCtx, stop := context.WithCancel(ctx)
	defer stop()
	var next atomic.Int64
	var wg sync.WaitGroup
	defer wg.Wait()
	for range min(runtime.NumCPU(), max(1, len(slots))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(slots) || workCtx.Err() != nil {
					return
				}
				placement := slots[i].placement
				results[i] <- reduce(job.Photos[placement.Index], math.Max(placement.PW, placement.PH), job.Frame == FramePolaroid)
			}
		}()
	}

	var failed []Failure
	for sheet := range job.Sheets {
		pdf.AddPage()
		if colour, ok := backgrounds[job.Background]; ok {
			pdf.SetFillColor(colour[0], colour[1], colour[2])
			pdf.Rect(0, 0, job.Paper.SheetWidth(), job.Paper.SheetHeight(), "F")
		}
		trimMarks(pdf, job.Paper)
		for i, current := range slots {
			if current.sheet != sheet {
				continue
			}
			var result rendered
			select {
			case result = <-results[i]:
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
			placement := current.placement
			if result.err != nil {
				failed = append(failed, Failure{Path: job.Photos[placement.Index], Error: result.err.Error()})
			} else {
				place(pdf, job.Frame, i, result, placement)
			}
			if job.Progress != nil {
				job.Progress(i+1, len(slots))
			}
		}
	}
	if err := pdf.Error(); err != nil {
		return nil, nil, err
	}
	return pdf, failed, nil
}

// trimMarks draws short corner marks in the bleed band showing where to cut.
func trimMarks(pdf *fpdf.Fpdf, paper Paper) {
	if paper.Bleed <= 0 {
		return
	}
	b, length := paper.Bleed, math.Min(paper.Bleed, 3)
	for _, x := range []float64{b, b + paper.Width} {
		for _, y := range []float64{b, b + paper.Height} {
			dx, dy := -length, -length
			if x > b {
				dx = length
			}
			if y > b {
				dy = length
			}
			pdf.Line(x, y, x+dx, y)
			pdf.Line(x, y, x, y+dy)
		}
	}
}

// place draws a reduced photo in the photo area of its box (the layout gives
// that area the photo's own proportions), inside its frame, with a thin
// cutting guide around the whole piece.
func place(pdf *fpdf.Fpdf, frame Frame, seed int, photo rendered, placed Placement) {
	// A turned piece is drawn upright around its centre, then turned.
	top, side, bottom := frame.insets()
	box := Placement{X: placed.PX - side, Y: placed.PY - top, W: placed.PW + 2*side, H: placed.PH + top + bottom, PX: placed.PX, PY: placed.PY, PW: placed.PW, PH: placed.PH}
	if placed.Angle != 0 {
		pdf.TransformBegin()
		defer pdf.TransformEnd()
		pdf.TransformRotate(placed.Angle, placed.X+placed.W/2, placed.Y+placed.H/2)
	}
	scale := math.Min(box.PW/float64(photo.width), box.PH/float64(photo.height))
	w, h := float64(photo.width)*scale, float64(photo.height)*scale
	x, y := box.PX+(box.PW-w)/2, box.PY+(box.PH-h)/2
	switch frame {
	case FrameStamp:
		stampOutline(pdf, box)
	case FramePolaroid:
		polaroidCard(pdf, box)
	default:
		pdf.Rect(box.X, box.Y, box.W, box.H, "D")
	}
	name := fmt.Sprintf("photo%d", seed)
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(photo.data))
	pdf.ImageOptions(name, x, y, w, h, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	switch frame {
	case FrameNone:
	case FrameTape:
		tapes(pdf, seed, x, y, w, h)
	case FramePolaroid:
		pdf.SetDrawColor(205, 200, 190)
		pdf.Rect(x, y, w, h, "D")
		pdf.SetDrawColor(170, 170, 170)
	default:
		pdf.Rect(x, y, w, h, "D")
	}
}

// polaroidCard draws the instant-film card, in vector: a soft shadow, a
// cream card with a hairline edge and slightly rounded corners.
func polaroidCard(pdf *fpdf.Fpdf, box Placement) {
	pdf.SetFillColor(0, 0, 0)
	pdf.SetAlpha(0.12, "Normal")
	pdf.RoundedRect(box.X+0.5, box.Y+0.7, box.W, box.H, 0.8, "1234", "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetFillColor(252, 250, 244)
	pdf.SetDrawColor(215, 210, 200)
	pdf.RoundedRect(box.X, box.Y, box.W, box.H, 0.8, "1234", "DF")
	pdf.SetDrawColor(170, 170, 170)
}

// tapeInset is the room left around a photo for its tape: each strip is
// centred a little inside a corner and turned 45°, which sticks out of the
// photo by about 4.8 mm; the rest of the inset absorbs the slight random turn.
const (
	tapeInset  = 6.0
	tapeLength = 12.0
	tapeWidth  = 5.0
	tapeShift  = 1.2
)

var tapeColours = [][3]int{{236, 213, 170}, {244, 190, 200}, {180, 224, 200}, {178, 208, 236}}

// tapes sticks a translucent washi strip with notched ends across each corner
// of the photo. Colours are drawn at random from a generator seeded by the
// photo, so a page looks the same each time it is printed.
func tapes(pdf *fpdf.Fpdf, seed int, x, y, w, h float64) {
	random := rand.New(rand.NewPCG(uint64(seed), 0x7a5b1))
	pdf.SetAlpha(0.75, "Normal")
	defer pdf.SetAlpha(1, "Normal")
	jitter := [4]float64{1.5, -1.0, 1.0, -1.5}
	corners := []struct{ cx, cy, angle float64 }{
		{x + tapeShift, y + tapeShift, 45},
		{x + w - tapeShift, y + tapeShift, -45},
		{x + w - tapeShift, y + h - tapeShift, 45},
		{x + tapeShift, y + h - tapeShift, -45},
	}
	for i, c := range corners {
		colour := tapeColours[random.IntN(len(tapeColours))]
		pdf.SetFillColor(colour[0], colour[1], colour[2])
		pdf.TransformBegin()
		pdf.TransformRotate(c.angle+jitter[(i+seed)%4], c.cx, c.cy)
		pdf.Polygon(tapeShape(tapeLength, tapeWidth, c.cx, c.cy), "F")
		pdf.TransformEnd()
	}
}

// tapeShape is a strip centred on (cx, cy), lying along x, with zigzag ends.
func tapeShape(length, width, cx, cy float64) []fpdf.PointType {
	const teeth, depth = 4, 0.7
	var points []fpdf.PointType
	for k := 0; k <= teeth; k++ {
		inset := depth * float64(k%2)
		points = append(points, fpdf.PointType{X: cx - length/2 + inset, Y: cy - width/2 + width*float64(k)/teeth})
	}
	for k := teeth; k >= 0; k-- {
		inset := depth * float64(k%2)
		points = append(points, fpdf.PointType{X: cx + length/2 - inset, Y: cy - width/2 + width*float64(k)/teeth})
	}
	return points
}

// stampOutline draws the box with perforated edges, like a postage stamp.
func stampOutline(pdf *fpdf.Fpdf, box Placement) {
	corners := [][2]float64{{box.X, box.Y}, {box.X + box.W, box.Y}, {box.X + box.W, box.Y + box.H}, {box.X, box.Y + box.H}}
	var points []fpdf.PointType
	for i, from := range corners {
		to := corners[(i+1)%4]
		dx, dy := to[0]-from[0], to[1]-from[1]
		length := math.Hypot(dx, dy)
		ux, uy := dx/length, dy/length
		// The inward normal: the corners run clockwise on the page.
		nx, ny := -uy, ux
		holes := max(1, int(math.Round(length/2.5)))
		pitch := length / float64(holes)
		radius := pitch * 0.3
		for hole := range holes {
			centre := (float64(hole) + 0.5) * pitch
			for step := 0; step <= 8; step++ {
				angle := math.Pi * float64(step) / 8
				along := centre - radius*math.Cos(angle)
				depth := radius * math.Sin(angle)
				points = append(points, fpdf.PointType{X: from[0] + ux*along + nx*depth, Y: from[1] + uy*along + ny*depth})
			}
		}
	}
	pdf.Polygon(points, "D")
}

// reduce decodes a photo upright and reduces it to print at sizeMM on its
// longest side, as a JPEG.
// With square set, the photo is cropped to a square around its subject.
func reduce(path string, sizeMM float64, square bool) rendered {
	pixels := int(math.Ceil(sizeMM / mmPerInch * printDPI))
	var img image.Image
	var err error
	if square {
		img, err = thumbs.Analysis(path, pixels, 2*pixels)
		if err == nil {
			img = SubjectSquare(img)
		}
	} else {
		img, err = thumbs.Analysis(path, pixels, pixels)
	}
	if err != nil {
		return rendered{err: err}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return rendered{err: err}
	}
	bounds := img.Bounds()
	return rendered{data: buffer.Bytes(), width: bounds.Dx(), height: bounds.Dy()}
}

// outputDir creates and checks the folder to write to, falling back when it
// is not usable.
func outputDir(dir, fallback string) (string, bool, error) {
	err := errors.New("no output folder")
	if dir != "" {
		if err = usable(dir); err == nil {
			return dir, false, nil
		}
	}
	if fallback != "" && fallback != dir {
		if fallbackErr := usable(fallback); fallbackErr == nil {
			return fallback, true, nil
		}
	}
	return "", false, err
}

func usable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".photosort-*.probe")
	if err != nil {
		return err
	}
	probe.Close()
	return os.Remove(probe.Name())
}

// publish moves the finished temporary file to dir/name, or to the first free
// "name (2).pdf", "name (3).pdf"...
func publish(temp, dir, name string) (string, error) {
	base := strings.TrimSuffix(name, ".pdf")
	for n := 1; n < 10_000; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d).pdf", base, n)
		}
		path := filepath.Join(dir, candidate)
		// Link fails if the target exists, so a file is never overwritten.
		if err := os.Link(temp, path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrExist) {
			if renameErr := renameFree(temp, path); renameErr == nil {
				return path, nil
			} else if !errors.Is(renameErr, os.ErrExist) {
				return "", renameErr
			}
		}
	}
	return "", errors.New("no free file name")
}

// renameFree renames when hard links are unavailable (some file systems),
// refusing to replace an existing file.
func renameFree(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return os.ErrExist
	}
	return os.Rename(from, to)
}
