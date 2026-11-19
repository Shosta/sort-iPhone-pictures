package yearmap

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/scrapbook"

	"github.com/go-pdf/fpdf"
)

type colour struct{ r, g, b int }

var (
	waterColour  = colour{206, 225, 236}
	landColour   = colour{246, 240, 226}
	borderColour = colour{186, 176, 152}
	pinColour    = colour{214, 69, 65}
	inkColour    = colour{58, 50, 42}
	mutedColour  = colour{120, 112, 102}
)

// Job describes a PDF to write.
type Job struct {
	Layout Layout
	// Title and Subtitle are written in the corner of the page.
	Title, Subtitle string
	// Dir and Name give the output file; an existing file is never replaced.
	Dir, Name string
	// FallbackDir is used when Dir cannot be written to.
	FallbackDir string
	// Progress (optional) is called after each photo, from one goroutine.
	Progress func(done, total int)
	// Backdrop (optional) replaces the vector map; Credit is then written on
	// the page.
	Backdrop image.Image
	Credit   string
}

// Result describes the written PDF.
type Result struct {
	Path     string
	Failed   []scrapbook.Failure
	Fallback bool
}

type thumb struct {
	data []byte
	err  error
}

// Render writes the one-page PDF of job.
func Render(ctx context.Context, job Job) (Result, error) {
	var failed []scrapbook.Failure
	path, fallback, err := scrapbook.SavePDF(job.Dir, job.FallbackDir, job.Name, func(w io.Writer) error {
		pdf, failures, err := draw(ctx, job)
		if err != nil {
			return err
		}
		failed = failures
		return pdf.Output(w)
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Path: path, Failed: failed, Fallback: fallback}, nil
}

func draw(ctx context.Context, job Job) (*fpdf.Fpdf, []scrapbook.Failure, error) {
	layout := job.Layout
	thumbs, failed, err := loadThumbs(ctx, layout, job.Progress)
	if err != nil {
		return nil, nil, err
	}
	// The size is given as is: fpdf would swap it again for "L".
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: fpdf.SizeType{Wd: layout.Width, Ht: layout.Height}})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCreator("Photosort", true)
	if job.Title != "" {
		pdf.SetTitle(job.Title, true)
	}
	text := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.AddPage()

	fill(pdf, waterColour)
	pdf.Rect(0, 0, layout.Width, layout.Height, "F")
	if job.Backdrop != nil {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, job.Backdrop, &jpeg.Options{Quality: 85}); err != nil {
			return nil, nil, err
		}
		pdf.RegisterImageOptionsReader("backdrop", fpdf.ImageOptions{ImageType: "JPG"}, &buf)
		pdf.ImageOptions("backdrop", 0, 0, layout.Width, layout.Height, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	}
	pdf.ClipRect(0, 0, layout.Width, layout.Height, false)
	base := mapbase.Default()
	if job.Backdrop != nil {
		base = &mapbase.Base{}
	}
	fill(pdf, landColour)
	pdf.SetDrawColor(borderColour.r, borderColour.g, borderColour.b)
	pdf.SetLineWidth(0.15)
	pdf.SetLineJoinStyle("round")
	for _, polygon := range base.Land(layout.View) {
		trace(pdf, polygon)
		pdf.DrawPath("FD*")
	}
	fill(pdf, waterColour)
	for _, polygon := range base.Lakes(layout.View) {
		trace(pdf, polygon)
		pdf.DrawPath("FD*")
	}
	pdf.ClipEnd()

	banner(pdf, text, job.Title, job.Subtitle)

	if job.Credit != "" {
		credit(pdf, text, layout, job.Credit)
	}

	pdf.SetDrawColor(pinColour.r, pinColour.g, pinColour.b)
	pdf.SetLineWidth(0.3)
	for _, pin := range layout.Pins {
		if pin.Card != nil {
			to := nearest(pin.Pos, *pin.Card)
			pdf.Line(pin.Pos.X, pin.Pos.Y, to.X, to.Y)
		}
	}
	for i, pin := range layout.Pins {
		if pin.Card != nil {
			card(pdf, text, layout.Scale, pin, thumbs, i)
		}
	}
	for _, pin := range layout.Pins {
		marker(pdf, pin.Pos)
	}
	if err := pdf.Error(); err != nil {
		return nil, nil, err
	}
	return pdf, failed, nil
}

// credit writes the attribution of the map in the bottom right corner.
func credit(pdf *fpdf.Fpdf, text func(string) string, layout Layout, credit string) {
	pdf.SetFont("Helvetica", "", 5)
	label := text(latin(credit))
	w := pdf.GetStringWidth(label) + 3
	pdf.SetAlpha(0.8, "Normal")
	pdf.SetFillColor(255, 255, 255)
	pdf.Rect(layout.Width-w, layout.Height-4.2, w, 4.2, "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetTextColor(inkColour.r, inkColour.g, inkColour.b)
	pdf.Text(layout.Width-w+1.5, layout.Height-1.4, label)
}

func fill(pdf *fpdf.Fpdf, c colour) { pdf.SetFillColor(c.r, c.g, c.b) }

// trace builds the path of the rings of a polygon; the caller paints it.
func trace(pdf *fpdf.Fpdf, rings [][]mapbase.XY) {
	for _, ring := range rings {
		for i, p := range ring {
			if i == 0 {
				pdf.MoveTo(p.X, p.Y)
			} else {
				pdf.LineTo(p.X, p.Y)
			}
		}
		pdf.ClosePath()
	}
}

func banner(pdf *fpdf.Fpdf, text func(string) string, title, subtitle string) {
	box := TitleBox
	pdf.SetAlpha(0.72, "Normal")
	pdf.SetFillColor(255, 255, 255)
	pdf.RoundedRect(box.X, box.Y, box.W, box.H, 3, "1234", "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetTextColor(inkColour.r, inkColour.g, inkColour.b)
	pdf.SetFont("Helvetica", "B", 20)
	pdf.Text(box.X+4, box.Y+9.5, fitText(pdf, text(latin(title)), box.W-8))
	pdf.SetTextColor(mutedColour.r, mutedColour.g, mutedColour.b)
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.Text(box.X+4, box.Y+15, fitText(pdf, text(latin(subtitle)), box.W-8))
}

// marker draws a pin: a red disc with a white ring and centre.
func marker(pdf *fpdf.Fpdf, p mapbase.XY) {
	pdf.SetFillColor(255, 255, 255)
	pdf.Circle(p.X, p.Y, pinRadius+0.7, "F")
	fill(pdf, pinColour)
	pdf.Circle(p.X, p.Y, pinRadius, "F")
	pdf.SetFillColor(255, 255, 255)
	pdf.Circle(p.X, p.Y, pinRadius*0.38, "F")
}

// card draws the photos and the name of a place.
func card(pdf *fpdf.Fpdf, text func(string) string, scale float64, pin Pin, thumbs map[string]thumb, index int) {
	box := *pin.Card
	pdf.SetAlpha(0.2, "Normal")
	pdf.SetFillColor(0, 0, 0)
	pdf.RoundedRect(box.X+0.5, box.Y+0.7, box.W, box.H, 1.2*scale, "1234", "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(205, 200, 190)
	pdf.SetLineWidth(0.15)
	pdf.RoundedRect(box.X, box.Y, box.W, box.H, 1.2*scale, "1234", "FD")

	side := thumbSize * scale
	gap := cardPad * scale
	shown := pin.Place.Shown
	x := box.X + (box.W-(float64(len(shown))*side+float64(len(shown)-1)*gap))/2
	for i, path := range shown {
		if t, ok := thumbs[path]; ok && t.err == nil {
			name := fmt.Sprintf("p%d-%d", index, i)
			pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(t.data))
			pdf.ImageOptions(name, x, box.Y+gap, side, side, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
		} else {
			pdf.SetFillColor(228, 223, 210)
			pdf.Rect(x, box.Y+gap, side, side, "F")
		}
		x += side + gap
	}
	pdf.SetFont("Helvetica", "B", math.Max(4, 6.5*scale))
	pdf.SetTextColor(inkColour.r, inkColour.g, inkColour.b)
	name := fitText(pdf, text(latin(pin.Place.Name)), box.W-2*gap)
	pdf.Text(box.X+(box.W-pdf.GetStringWidth(name))/2, box.Y+gap+side+labelH*scale*0.72, name)

	if more := pin.Place.Count - len(shown); more > 0 {
		r := 3.4 * scale
		fill(pdf, pinColour)
		pdf.Circle(box.X+box.W, box.Y, r, "F")
		pdf.SetFont("Helvetica", "B", math.Max(3.5, 5*scale))
		pdf.SetTextColor(255, 255, 255)
		label := fmt.Sprintf("+%d", more)
		if more > 99 {
			label = "99+"
		}
		pdf.Text(box.X+box.W-pdf.GetStringWidth(label)/2, box.Y+r*0.3, label)
	}
}

// nearest is the point of box closest to p.
func nearest(p mapbase.XY, box Rect) mapbase.XY {
	return mapbase.XY{X: math.Max(box.X, math.Min(p.X, box.X+box.W)), Y: math.Max(box.Y, math.Min(p.Y, box.Y+box.H))}
}

// fitText shortens s with an ellipsis until it is at most width mm wide in the
// current font.
func fitText(pdf *fpdf.Fpdf, s string, width float64) string {
	if pdf.GetStringWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if candidate := strings.TrimSpace(string(runes)) + "…"; pdf.GetStringWidth(candidate) <= width {
			return candidate
		}
	}
	return string(runes)
}

// folded maps letters missing from the PDF core fonts (Windows-1252) to their
// plain form; other scripts cannot be written with them.
var folded = strings.NewReplacer(
	"ł", "l", "Ł", "L", "ő", "o", "Ő", "O", "ű", "u", "Ű", "U", "ă", "a", "Ă", "A",
	"ș", "s", "Ș", "S", "ț", "t", "Ț", "T", "ě", "e", "Ě", "E", "č", "c", "Č", "C",
	"ř", "r", "Ř", "R", "ć", "c", "Ć", "C", "ń", "n", "Ń", "N", "ś", "s", "Ś", "S",
	"ź", "z", "Ź", "Z", "ż", "z", "Ż", "Z", "ą", "a", "Ą", "A", "ę", "e", "Ę", "E",
	"ı", "i", "İ", "I", "ğ", "g", "Ğ", "G", "ş", "s", "Ş", "S", "ď", "d", "ť", "t",
	"ň", "n", "ů", "u", "ā", "a", "ē", "e", "ī", "i", "ō", "o", "ū", "u",
)

func latin(s string) string { return folded.Replace(s) }

// loadThumbs reduces the photos of the placed cards, in parallel.
func loadThumbs(ctx context.Context, layout Layout, progress func(done, total int)) (map[string]thumb, []scrapbook.Failure, error) {
	var paths []string
	seen := map[string]bool{}
	for _, pin := range layout.Pins {
		if pin.Card == nil {
			continue
		}
		for _, path := range pin.Place.Shown {
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	results := make([]thumb, len(paths))
	size := thumbSize * layout.Scale
	var wg sync.WaitGroup
	var lock sync.Mutex
	next, done := 0, 0
	for range min(runtime.NumCPU(), max(1, len(paths))) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				lock.Lock()
				i := next
				next++
				lock.Unlock()
				if i >= len(paths) || ctx.Err() != nil {
					return
				}
				data, _, _, err := scrapbook.JPEG(paths[i], size, true)
				results[i] = thumb{data: data, err: err}
				lock.Lock()
				done++
				if progress != nil {
					progress(done, len(paths))
				}
				lock.Unlock()
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	thumbs := make(map[string]thumb, len(paths))
	var failed []scrapbook.Failure
	for i, path := range paths {
		thumbs[path] = results[i]
		if results[i].err != nil {
			failed = append(failed, scrapbook.Failure{Path: path, Error: results[i].err.Error()})
		}
	}
	return thumbs, failed, nil
}
