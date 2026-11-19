package travelmap

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"

	"sort-iphone-pictures/internal/mapbase"
	"sort-iphone-pictures/internal/scrapbook"
	"sort-iphone-pictures/internal/yearmap"

	"github.com/go-pdf/fpdf"
)

type colour struct{ r, g, b int }

// The notebook palette: paper, sand-coloured land, ink and a red pencil.
var (
	paperColour  = colour{250, 245, 232}
	landColour   = colour{236, 223, 194}
	borderColour = colour{160, 138, 104}
	routeColour  = colour{176, 64, 52}
	inkColour    = colour{66, 52, 40}
	mutedColour  = colour{128, 112, 96}
)

// Job describes a PDF to write.
type Job struct {
	Layout Layout
	// Backdrop (optional) replaces the vector map; Credit is then written on
	// the page.
	Backdrop image.Image
	Credit   string
	// Title and Subtitle are written in the corner of the page.
	Title, Subtitle string
	// Dir and Name give the output file; an existing file is never replaced.
	Dir, Name string
	// FallbackDir is used when Dir cannot be written to.
	FallbackDir string
	// Progress (optional) is called after each photo, from one goroutine.
	Progress func(done, total int)
}

// Render writes the one-page PDF of job.
func Render(ctx context.Context, job Job) (yearmap.Result, error) {
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
		return yearmap.Result{}, err
	}
	return yearmap.Result{Path: path, Failed: failed, Fallback: fallback}, nil
}

func fill(pdf *fpdf.Fpdf, c colour) { pdf.SetFillColor(c.r, c.g, c.b) }
func ink(pdf *fpdf.Fpdf, c colour)  { pdf.SetTextColor(c.r, c.g, c.b) }

func draw(ctx context.Context, job Job) (*fpdf.Fpdf, []scrapbook.Failure, error) {
	layout := job.Layout
	page := layout.Page
	thumbs, failed, err := yearmap.LoadThumbs(ctx, page, job.Progress)
	if err != nil {
		return nil, nil, err
	}
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: fpdf.SizeType{Wd: page.Width, Ht: page.Height}})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCreator("Photosort", true)
	if job.Title != "" {
		pdf.SetTitle(job.Title, true)
	}
	text := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.AddPage()

	area := yearmap.MapArea(page.Width, page.Height)
	pdf.ClipRect(area.X, area.Y, area.W, area.H, false)
	fill(pdf, paperColour)
	pdf.Rect(0, 0, page.Width, page.Height, "F")
	if job.Backdrop != nil {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, job.Backdrop, &jpeg.Options{Quality: 85}); err != nil {
			return nil, nil, err
		}
		options := fpdf.ImageOptions{ImageType: "JPG"}
		pdf.RegisterImageOptionsReader("backdrop", options, &buf)
		pdf.ImageOptions("backdrop", 0, 0, page.Width, page.Height, false, options, 0, "")
	}
	base := mapbase.Default()
	if job.Backdrop != nil {
		base = &mapbase.Base{}
	}
	fill(pdf, landColour)
	pdf.SetDrawColor(borderColour.r, borderColour.g, borderColour.b)
	pdf.SetLineWidth(0.15)
	pdf.SetLineJoinStyle("round")
	for _, polygon := range base.Land(page.View) {
		yearmap.Trace(pdf, polygon)
		pdf.DrawPath("FD*")
	}
	fill(pdf, paperColour)
	for _, polygon := range base.Lakes(page.View) {
		yearmap.Trace(pdf, polygon)
		pdf.DrawPath("FD*")
	}
	pdf.ClipEnd()

	frame(pdf, page)
	route(pdf, layout.Stops)
	banner(pdf, text, job.Title, job.Subtitle)
	if job.Credit != "" {
		yearmap.Credit(pdf, text, page, job.Credit)
	}

	pdf.SetDrawColor(routeColour.r, routeColour.g, routeColour.b)
	pdf.SetLineWidth(0.25)
	for _, stop := range layout.Stops {
		if stop.Card != nil {
			to := yearmap.Nearest(stop.Pos, *stop.Card)
			pdf.Line(stop.Pos.X, stop.Pos.Y, to.X, to.Y)
		}
	}
	for _, stop := range layout.Stops {
		if stop.Card != nil {
			card(pdf, text, page.Scale, stop, thumbs)
		}
	}
	for _, stop := range layout.Stops {
		marker(pdf, stop.Pos, stop.N)
	}
	if err := pdf.Error(); err != nil {
		return nil, nil, err
	}
	return pdf, failed, nil
}

// frame draws the double border of the notebook page around the map.
func frame(pdf *fpdf.Fpdf, page yearmap.Layout) {
	area := yearmap.MapArea(page.Width, page.Height)
	pdf.SetDrawColor(inkColour.r, inkColour.g, inkColour.b)
	pdf.SetLineWidth(0.5)
	pdf.Rect(area.X, area.Y, area.W, area.H, "D")
	pdf.SetLineWidth(0.15)
	pdf.Rect(area.X+1.2, area.Y+1.2, area.W-2.4, area.H-2.4, "D")
}

// route draws the dotted line through the steps, in order.
func route(pdf *fpdf.Fpdf, stops []Stop) {
	if len(stops) < 2 {
		return
	}
	pdf.SetDrawColor(routeColour.r, routeColour.g, routeColour.b)
	pdf.SetLineWidth(0.6)
	pdf.SetLineCapStyle("round")
	pdf.SetDashPattern([]float64{0.01, 1.7}, 0)
	for i, stop := range stops {
		if i == 0 {
			pdf.MoveTo(stop.Pos.X, stop.Pos.Y)
		} else {
			pdf.LineTo(stop.Pos.X, stop.Pos.Y)
		}
	}
	pdf.DrawPath("D")
	pdf.SetDashPattern([]float64{}, 0)
	pdf.SetLineCapStyle("butt")
}

// banner writes the title in a notebook label.
func banner(pdf *fpdf.Fpdf, text func(string) string, title, subtitle string) {
	box := yearmap.TitleBox
	pdf.SetAlpha(0.85, "Normal")
	pdf.SetFillColor(255, 252, 244)
	pdf.Rect(box.X, box.Y, box.W, box.H, "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetDrawColor(inkColour.r, inkColour.g, inkColour.b)
	pdf.SetLineWidth(0.4)
	pdf.Rect(box.X, box.Y, box.W, box.H, "D")
	pdf.SetLineWidth(0.12)
	pdf.Rect(box.X+1, box.Y+1, box.W-2, box.H-2, "D")
	ink(pdf, inkColour)
	const vanW = 13.0
	van(pdf, box.X+3.5, box.Y+(box.H-vanW*0.62)/2, vanW)
	left := box.X + 3.5 + vanW + 3
	pdf.SetFont("Times", "B", 19)
	pdf.Text(left, box.Y+10, yearmap.FitText(pdf, text(yearmap.Latin(title)), box.W-(left-box.X)-4))
	ink(pdf, mutedColour)
	pdf.SetFont("Times", "I", 8.5)
	pdf.Text(left, box.Y+16.6, yearmap.FitText(pdf, text(yearmap.Latin(subtitle)), box.W-(left-box.X)-4))
}

// van draws a camper van facing right in a box of width w and height 0.62*w
// whose top left corner is (x, y).
func van(pdf *fpdf.Fpdf, x, y, w float64) {
	u := w / 100
	pdf.SetLineWidth(0.3)
	pdf.SetDrawColor(inkColour.r, inkColour.g, inkColour.b)
	fill(pdf, routeColour)
	pdf.RoundedRect(x, y+4*u, 96*u, 44*u, 9*u, "1234", "FD")
	// A cream roof stripe, the windows and the headlight.
	pdf.SetFillColor(255, 252, 244)
	pdf.Rect(x+2*u, y+30*u, 92*u, 6*u, "F")
	pdf.RoundedRect(x+8*u, y+11*u, 22*u, 16*u, 2*u, "1234", "FD")
	pdf.RoundedRect(x+34*u, y+11*u, 22*u, 16*u, 2*u, "1234", "FD")
	pdf.RoundedRect(x+62*u, y+11*u, 26*u, 16*u, 2*u, "1234", "FD")
	pdf.SetFillColor(240, 190, 70)
	pdf.Circle(x+92*u, y+36*u, 2.6*u, "F")
	// A roof rack and the wheels.
	pdf.Line(x+20*u, y+4*u, x+20*u, y)
	pdf.Line(x+60*u, y+4*u, x+60*u, y)
	pdf.Line(x+14*u, y, x+66*u, y)
	for _, cx := range []float64{24, 74} {
		fill(pdf, inkColour)
		pdf.Circle(x+cx*u, y+50*u, 9*u, "F")
		pdf.SetFillColor(255, 252, 244)
		pdf.Circle(x+cx*u, y+50*u, 3.5*u, "F")
	}
}

// badge is a red disc holding the number of a step.
func badge(pdf *fpdf.Fpdf, x, y, radius float64, n int) {
	pdf.SetFillColor(255, 255, 255)
	pdf.Circle(x, y, radius+0.5, "F")
	fill(pdf, routeColour)
	pdf.Circle(x, y, radius, "F")
	size := math.Max(4, radius*2.7)
	if n >= 10 {
		size *= 0.8
	}
	pdf.SetFont("Helvetica", "B", size)
	pdf.SetTextColor(255, 255, 255)
	label := fmt.Sprint(n)
	pdf.Text(x-pdf.GetStringWidth(label)/2, y+size*0.36*0.3528, label)
}

func marker(pdf *fpdf.Fpdf, p mapbase.XY, n int) { badge(pdf, p.X, p.Y, yearmap.PinRadius, n) }

// card draws the photo, name and dates of a step, like a taped-on snapshot.
func card(pdf *fpdf.Fpdf, text func(string) string, scale float64, stop Stop, thumbs map[string]yearmap.Thumb) {
	box := *stop.Card
	pdf.SetAlpha(0.2, "Normal")
	pdf.SetFillColor(0, 0, 0)
	pdf.Rect(box.X+0.6, box.Y+0.8, box.W, box.H, "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(205, 196, 180)
	pdf.SetLineWidth(0.15)
	pdf.Rect(box.X, box.Y, box.W, box.H, "FD")

	side := yearmap.ThumbSize * scale
	gap := yearmap.CardPad * scale
	x := box.X + (box.W-side)/2
	if data, ok := thumbs[stop.Step.Photo].JPEG(); ok {
		name := fmt.Sprintf("s%d", stop.N)
		options := fpdf.ImageOptions{ImageType: "JPG"}
		pdf.RegisterImageOptionsReader(name, options, bytes.NewReader(data))
		pdf.ImageOptions(name, x, box.Y+gap, side, side, false, options, 0, "")
	} else {
		pdf.SetFillColor(228, 223, 210)
		pdf.Rect(x, box.Y+gap, side, side, "F")
	}
	labelTop := box.Y + gap + side
	pdf.SetFont("Times", "B", math.Max(4, 6.8*scale))
	ink(pdf, inkColour)
	name := yearmap.FitText(pdf, text(yearmap.Latin(stop.Step.Name)), box.W-2*gap)
	pdf.Text(box.X+(box.W-pdf.GetStringWidth(name))/2, labelTop+2.9*scale, name)
	pdf.SetFont("Times", "I", math.Max(3.5, 5*scale))
	ink(pdf, mutedColour)
	dates := yearmap.FitText(pdf, text(yearmap.Latin(stop.Dates)), box.W-2*gap)
	pdf.Text(box.X+(box.W-pdf.GetStringWidth(dates))/2, labelTop+5.6*scale, dates)
	badge(pdf, box.X, box.Y, 2.6*scale, stop.N)
}
