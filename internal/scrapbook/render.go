package scrapbook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"math"
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
	path, err := publish(temp.Name(), dir, SafeName(job.Name))
	if err != nil {
		return Result{}, err
	}
	return Result{Path: path, Pages: len(job.Sheets), Failed: failed, Fallback: fallback}, nil
}

// draw builds the document of job.
func draw(ctx context.Context, job Job) (*fpdf.Fpdf, []Failure, error) {
	orientation := "P"
	if job.Paper.Landscape() {
		orientation = "L"
	}
	pdf := fpdf.New(orientation, "mm", job.Paper.Name, "")
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
				results[i] <- reduce(job.Photos[placement.Index], math.Max(placement.W, placement.H))
			}
		}()
	}

	var failed []Failure
	for sheet := range job.Sheets {
		pdf.AddPage()
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
				place(pdf, fmt.Sprintf("photo%d", i), result, placement)
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

// place draws a reduced photo, as large as its box allows (the layout gives
// the box the photo's own proportions), with a thin cutting guide around it.
func place(pdf *fpdf.Fpdf, name string, photo rendered, box Placement) {
	scale := math.Min(box.W/float64(photo.width), box.H/float64(photo.height))
	w, h := float64(photo.width)*scale, float64(photo.height)*scale
	x, y := box.X+(box.W-w)/2, box.Y+(box.H-h)/2
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(photo.data))
	pdf.ImageOptions(name, x, y, w, h, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	const guide = 0.3
	pdf.Rect(x-guide, y-guide, w+2*guide, h+2*guide, "D")
}

// reduce decodes a photo upright and reduces it to print at sizeMM on its
// longest side, as a JPEG.
func reduce(path string, sizeMM float64) rendered {
	pixels := int(math.Ceil(sizeMM / mmPerInch * printDPI))
	img, err := thumbs.Analysis(path, pixels, pixels)
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
