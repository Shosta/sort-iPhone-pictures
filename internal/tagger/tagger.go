// Package tagger derives photo tags (places, moment of the day, people) with
// local ONNX models; nothing leaves the machine.
package tagger

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"

	"sort-iphone-pictures/internal/thumbs"
)

// Tags that need no model.
const (
	TagSun      = "Soleil"
	TagCloud    = "Nuage"
	TagPeople   = "People"
	TagPortrait = "Portrait"
)

// Options selects which tag families are produced.
type Options struct {
	Scenes bool // plage, montagne, ville, campagne, château, église
	Moment bool // lever/coucher de soleil, nuit, soleil, nuage
	People bool // People, Portrait
}

// Photo is the information about a photo the tagger needs.
type Photo struct {
	Path      string
	TakenAt   time.Time // local wall-clock time
	Lat, Lon  float64
	HasGPS    bool
	UTCOffset int
	HasOffset bool
}

// Tagger holds the loaded models. It is safe for concurrent use.
type Tagger struct {
	opts     Options
	lock     sync.Mutex
	scene    *ort.AdvancedSession
	sceneIn  *ort.Tensor[float32]
	sceneOut *ort.Tensor[float32]
	labels   []string
	face     *faceModel
}

var (
	envLock sync.Mutex
	envUp   bool
)

// LibraryName is the ONNX Runtime shared library file name on this OS.
func LibraryName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	}
	return "libonnxruntime.so"
}

// FindAssets returns the directory holding the models, or "" if none found.
// It checks dir, $PHOTOSORT_AI_DIR, then "ai" next to the executable.
func FindAssets(dir string) string {
	candidates := []string{dir, os.Getenv("PHOTOSORT_AI_DIR")}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(base, "ai"), filepath.Join(base, "..", "ai"))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(candidate, LibraryName())); err == nil {
			return candidate
		}
	}
	return ""
}

// New loads the models needed by opts from dir. If no model is needed (only
// the solar moment tags) no assets are required.
func New(dir string, opts Options) (*Tagger, error) {
	t := &Tagger{opts: opts}
	if !opts.Scenes && !opts.People {
		return t, nil
	}
	if dir == "" {
		return nil, errors.New("AI models not found (run the fetch-ai-assets script)")
	}
	envLock.Lock()
	if !envUp {
		ort.SetSharedLibraryPath(filepath.Join(dir, LibraryName()))
		if err := ort.InitializeEnvironment(); err != nil {
			envLock.Unlock()
			return nil, fmt.Errorf("init ONNX Runtime: %w", err)
		}
		envUp = true
	}
	envLock.Unlock()
	if opts.Scenes {
		if err := t.loadScene(dir); err != nil {
			t.Close()
			return nil, err
		}
	}
	if opts.People {
		face, err := newFaceModel(filepath.Join(dir, "yunet.onnx"))
		if err != nil {
			t.Close()
			return nil, err
		}
		t.face = face
	}
	return t, nil
}

func (t *Tagger) loadScene(dir string) error {
	labels, err := loadCategories(filepath.Join(dir, "categories.txt"))
	if err != nil || len(labels) != 365 {
		return fmt.Errorf("load scene categories: %v", err)
	}
	t.labels = labels
	if t.sceneIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 3, sceneSize, sceneSize)); err != nil {
		return err
	}
	if t.sceneOut, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 365)); err != nil {
		return err
	}
	t.scene, err = ort.NewAdvancedSession(filepath.Join(dir, "places.onnx"),
		[]string{"serving_default_args_0"}, []string{"serving_default_output_0_output"},
		[]ort.Value{t.sceneIn}, []ort.Value{t.sceneOut}, nil)
	return err
}

// Close releases the models.
func (t *Tagger) Close() {
	if t.scene != nil {
		t.scene.Destroy()
	}
	if t.sceneIn != nil {
		t.sceneIn.Destroy()
	}
	if t.sceneOut != nil {
		t.sceneOut.Destroy()
	}
	if t.face != nil {
		t.face.close()
	}
}

// Tags analyses a photo and returns its tags, sorted by relevance.
func (t *Tagger) Tags(photo Photo) ([]string, error) {
	var tags []string
	if t.opts.Moment && photo.HasGPS {
		if tag := momentTag(photo.TakenAt, photo.Lat, photo.Lon, photo.UTCOffset, photo.HasOffset); tag != "" {
			tags = append(tags, tag)
		}
	}
	if t.scene == nil && t.face == nil {
		return tags, nil
	}
	img, err := thumbs.Image(photo.Path, 640)
	if err != nil {
		return tags, err
	}
	outdoor := false
	if t.scene != nil {
		sceneTagList, isOutdoor, err := t.classify(img)
		if err != nil {
			return tags, err
		}
		tags = append(tags, sceneTagList...)
		outdoor = isOutdoor
	}
	if t.opts.Moment && t.scene != nil && outdoor && !hasTag(tags, TagNight) &&
		isDaylight(photo.TakenAt, photo.Lat, photo.Lon, photo.UTCOffset, photo.HasOffset, photo.HasGPS) {
		sun, cloud := skyTags(img)
		if sun {
			tags = append(tags, TagSun)
		}
		if cloud {
			tags = append(tags, TagCloud)
		}
	}
	if t.face != nil {
		people, portrait, err := t.face.detect(img)
		if err != nil {
			return tags, err
		}
		if people {
			tags = append(tags, TagPeople)
		}
		if portrait {
			tags = append(tags, TagPortrait)
		}
	}
	return tags, nil
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

const sceneSize = 224

func (t *Tagger) classify(img image.Image) ([]string, bool, error) {
	resized := image.NewRGBA(image.Rect(0, 0, sceneSize, sceneSize))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Src, nil)
	mean := [3]float32{0.485, 0.456, 0.406}
	std := [3]float32{0.229, 0.224, 0.225}
	t.lock.Lock()
	defer t.lock.Unlock()
	data := t.sceneIn.GetData()
	plane := sceneSize * sceneSize
	for y := 0; y < sceneSize; y++ {
		for x := 0; x < sceneSize; x++ {
			c := resized.RGBAAt(x, y)
			i := y*sceneSize + x
			data[i] = (float32(c.R)/255 - mean[0]) / std[0]
			data[plane+i] = (float32(c.G)/255 - mean[1]) / std[1]
			data[2*plane+i] = (float32(c.B)/255 - mean[2]) / std[2]
		}
	}
	if err := t.scene.Run(); err != nil {
		return nil, false, err
	}
	tags, outdoor := sceneTags(t.labels, softmax(t.sceneOut.GetData()))
	return tags, outdoor, nil
}

// skyTags inspects the top third of the picture: lots of saturated blue means
// sun, lots of bright grey/white means clouds.
func skyTags(img image.Image) (sun, cloud bool) {
	bounds := img.Bounds()
	top := bounds.Min.Y + bounds.Dy()/3
	blue, grey, total := 0, 0, 0
	for y := bounds.Min.Y; y < top; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
			maxC, minC := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
			saturation := 0.0
			if maxC > 0 {
				saturation = (maxC - minC) / maxC
			}
			total++
			switch {
			case b == maxC && b-r > 0.12 && saturation > 0.25 && maxC > 0.45:
				blue++
			case saturation < 0.15 && maxC > 0.6:
				grey++
			}
		}
	}
	if total == 0 {
		return false, false
	}
	blueShare, greyShare := float64(blue)/float64(total), float64(grey)/float64(total)
	return blueShare >= 0.35, greyShare >= 0.35
}

type faceBox struct {
	x1, y1, x2, y2, score float64
}

func (b faceBox) area() float64 { return math.Max(0, b.x2-b.x1) * math.Max(0, b.y2-b.y1) }

func iou(a, b faceBox) float64 {
	w := math.Min(a.x2, b.x2) - math.Max(a.x1, b.x1)
	h := math.Min(a.y2, b.y2) - math.Max(a.y1, b.y1)
	if w <= 0 || h <= 0 {
		return 0
	}
	inter := w * h
	return inter / (a.area() + b.area() - inter)
}

func nms(boxes []faceBox, limit float64) []faceBox {
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].score > boxes[j].score })
	var kept []faceBox
	for _, box := range boxes {
		ok := true
		for _, k := range kept {
			if iou(box, k) > limit {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, box)
		}
	}
	return kept
}

// classifyFaces decides People/Portrait from boxes in fractions of the image
// height: People when any face is visible, Portrait when one face clearly
// dominates the frame.
func classifyFaces(boxes []faceBox, imageHeight float64) (people, portrait bool) {
	var visible []faceBox
	for _, box := range boxes {
		if (box.y2-box.y1)/imageHeight >= 0.04 {
			visible = append(visible, box)
		}
	}
	if len(visible) == 0 {
		return false, false
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].area() > visible[j].area() })
	main := visible[0]
	dominant := (main.y2-main.y1)/imageHeight >= 0.18
	if dominant && (len(visible) == 1 || visible[1].area() < main.area()*0.4) {
		portrait = true
	}
	return true, portrait
}
