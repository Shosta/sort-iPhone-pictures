package tagger

import (
	"image"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

const faceSize = 640

var faceStrides = []int{8, 16, 32}

type faceModel struct {
	lock    sync.Mutex
	session *ort.AdvancedSession
	input   *ort.Tensor[float32]
	outputs map[string]*ort.Tensor[float32]
	all     []ort.Value
}

func newFaceModel(path string, threads int) (*faceModel, error) {
	m := &faceModel{outputs: map[string]*ort.Tensor[float32]{}}
	var err error
	if m.input, err = ort.NewEmptyTensor[float32](ort.NewShape(1, 3, faceSize, faceSize)); err != nil {
		return nil, err
	}
	var names []string
	for _, kind := range []struct {
		name string
		cols int
	}{{"cls", 1}, {"obj", 1}, {"bbox", 4}, {"kps", 10}} {
		for _, stride := range faceStrides {
			n := (faceSize / stride) * (faceSize / stride)
			name := kind.name + "_" + itoa(stride)
			tensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(n), int64(kind.cols)))
			if err != nil {
				m.close()
				return nil, err
			}
			m.outputs[name] = tensor
			m.all = append(m.all, tensor)
			names = append(names, name)
		}
	}
	options, err := sessionOptions(threads)
	if err != nil {
		m.close()
		return nil, err
	}
	defer options.Destroy()
	m.session, err = ort.NewAdvancedSession(path, []string{"input"}, names, []ort.Value{m.input}, m.all, options)
	if err != nil {
		m.close()
		return nil, err
	}
	return m, nil
}

func itoa(n int) string {
	if n == 8 {
		return "8"
	}
	if n == 16 {
		return "16"
	}
	return "32"
}

func (m *faceModel) close() {
	if m.session != nil {
		m.session.Destroy()
	}
	if m.input != nil {
		m.input.Destroy()
	}
	for _, tensor := range m.outputs {
		tensor.Destroy()
	}
}

// detect returns the people/portrait verdict and how many faces are visible.
func (m *faceModel) detect(img image.Image) (people, portrait bool, faces int, err error) {
	bounds := img.Bounds()
	scale := math.Min(float64(faceSize)/float64(bounds.Dx()), float64(faceSize)/float64(bounds.Dy()))
	w, h := int(math.Round(float64(bounds.Dx())*scale)), int(math.Round(float64(bounds.Dy())*scale))
	canvas := image.NewRGBA(image.Rect(0, 0, faceSize, faceSize))
	draw.BiLinear.Scale(canvas, image.Rect(0, 0, w, h), img, bounds, draw.Src, nil)
	m.lock.Lock()
	defer m.lock.Unlock()
	data := m.input.GetData()
	plane := faceSize * faceSize
	for y := 0; y < faceSize; y++ {
		for x := 0; x < faceSize; x++ {
			c := canvas.RGBAAt(x, y)
			i := y*faceSize + x
			data[i], data[plane+i], data[2*plane+i] = float32(c.B), float32(c.G), float32(c.R)
		}
	}
	if err := m.session.Run(); err != nil {
		return false, false, 0, err
	}
	var boxes []faceBox
	for _, stride := range faceStrides {
		cols := faceSize / stride
		cls := m.outputs["cls_"+itoa(stride)].GetData()
		obj := m.outputs["obj_"+itoa(stride)].GetData()
		bbox := m.outputs["bbox_"+itoa(stride)].GetData()
		for i := range cls {
			score := math.Sqrt(clamp01(float64(cls[i])) * clamp01(float64(obj[i])))
			if score < 0.7 {
				continue
			}
			row, col := i/cols, i%cols
			cx := (float64(col) + float64(bbox[i*4])) * float64(stride)
			cy := (float64(row) + float64(bbox[i*4+1])) * float64(stride)
			bw := math.Exp(float64(bbox[i*4+2])) * float64(stride)
			bh := math.Exp(float64(bbox[i*4+3])) * float64(stride)
			boxes = append(boxes, faceBox{cx - bw/2, cy - bh/2, cx + bw/2, cy + bh/2, score})
		}
	}
	people, portrait, faces = classifyFaces(nms(boxes, 0.3), float64(h))
	return people, portrait, faces, nil
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
