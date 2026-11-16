package thumbs

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

// shrink averages factor×factor blocks of pixels. Common decoder outputs are
// read directly from their pixel buffers; other types use a cheap
// approximation since a proper filter runs on the result anyway.
func shrink(img image.Image, factor int) image.Image {
	bounds := img.Bounds()
	width, height := bounds.Dx()/factor, bounds.Dy()/factor
	switch source := img.(type) {
	case *image.YCbCr:
		return shrinkYCbCr(source, factor, width, height)
	case *image.Gray:
		target := image.NewGray(image.Rect(0, 0, width, height))
		sums := make([]uint32, width)
		for y := 0; y < height; y++ {
			clear(sums)
			for dy := 0; dy < factor; dy++ {
				row := source.Pix[(y*factor+dy)*source.Stride:]
				for x := range sums {
					for _, value := range row[x*factor : x*factor+factor] {
						sums[x] += uint32(value)
					}
				}
			}
			area := uint32(factor * factor)
			for x, sum := range sums {
				target.Pix[y*target.Stride+x] = uint8(sum / area)
			}
		}
		return target
	case *image.RGBA:
		target := image.NewRGBA(image.Rect(0, 0, width, height))
		shrink4(source.Pix, source.Stride, target.Pix, target.Stride, factor, width, height)
		return target
	case *image.NRGBA:
		target := image.NewNRGBA(image.Rect(0, 0, width, height))
		shrink4(source.Pix, source.Stride, target.Pix, target.Stride, factor, width, height)
		return target
	}
	target := image.NewRGBA(image.Rect(0, 0, width*2, height*2))
	draw.ApproxBiLinear.Scale(target, target.Bounds(), img, bounds, draw.Src, nil)
	return target
}

func shrink4(source []uint8, sourceStride int, target []uint8, targetStride, factor, width, height int) {
	sums := make([]uint32, width*4)
	area := uint32(factor * factor)
	for y := 0; y < height; y++ {
		clear(sums)
		for dy := 0; dy < factor; dy++ {
			row := source[(y*factor+dy)*sourceStride:]
			for x := 0; x < width; x++ {
				block := row[x*factor*4 : (x+1)*factor*4]
				sum := sums[x*4 : x*4+4]
				for i := 0; i < len(block); i += 4 {
					sum[0] += uint32(block[i])
					sum[1] += uint32(block[i+1])
					sum[2] += uint32(block[i+2])
					sum[3] += uint32(block[i+3])
				}
			}
		}
		out := target[y*targetStride:]
		for i, sum := range sums {
			out[i] = uint8(sum / area)
		}
	}
}

func shrinkYCbCr(source *image.YCbCr, factor, width, height int) *image.RGBA {
	// Chroma planes are subsampled by (hx, hy) relative to luma.
	hx, hy := 1, 1
	switch source.SubsampleRatio {
	case image.YCbCrSubsampleRatio422:
		hx = 2
	case image.YCbCrSubsampleRatio420:
		hx, hy = 2, 2
	case image.YCbCrSubsampleRatio440:
		hy = 2
	case image.YCbCrSubsampleRatio411:
		hx = 4
	case image.YCbCrSubsampleRatio410:
		hx, hy = 4, 2
	}
	min := source.Rect.Min
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	luma := make([]uint32, width)
	blue := make([]uint32, width)
	red := make([]uint32, width)
	count := make([]uint32, width)
	for y := 0; y < height; y++ {
		clear(luma)
		clear(blue)
		clear(red)
		clear(count)
		for dy := 0; dy < factor; dy++ {
			row := source.Y[(y*factor+dy)*source.YStride:]
			for x := range luma {
				for _, value := range row[x*factor : x*factor+factor] {
					luma[x] += uint32(value)
				}
			}
		}
		// Chroma rows covering this block of luma rows.
		top, bottom := (min.Y+y*factor)/hy-min.Y/hy, (min.Y+(y+1)*factor-1)/hy-min.Y/hy
		for cy := top; cy <= bottom; cy++ {
			cb, cr := source.Cb[cy*source.CStride:], source.Cr[cy*source.CStride:]
			for x := range blue {
				left, right := (min.X+x*factor)/hx-min.X/hx, (min.X+(x+1)*factor-1)/hx-min.X/hx
				for cx := left; cx <= right; cx++ {
					blue[x] += uint32(cb[cx])
					red[x] += uint32(cr[cx])
					count[x]++
				}
			}
		}
		area := uint32(factor * factor)
		out := target.Pix[y*target.Stride:]
		for x := range luma {
			r, g, b := color.YCbCrToRGB(uint8(luma[x]/area), uint8(blue[x]/count[x]), uint8(red[x]/count[x]))
			out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = r, g, b, 255
		}
	}
	return target
}
