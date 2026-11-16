package thumbs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
)

// headBytes is how much of a file Embedded reads: the Exif segment is at
// most 64 KiB and comes first in camera JPEGs.
const headBytes = 160 << 10

var exifHeader = []byte("Exif\x00\x00")

// ScanJPEG walks the JPEG stream in data and calls visit for each segment
// (marker bytes included) and, with marker 0, for the entropy-coded data
// that follows each start-of-scan segment. Bytes after the end-of-image
// marker are ignored. It reports false when data is not a complete JPEG.
func ScanJPEG(data []byte, visit func(marker byte, segment []byte)) bool {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return false
	}
	for i := 2; i+1 < len(data); {
		if data[i] != 0xFF {
			return false
		}
		marker := data[i+1]
		switch {
		case marker == 0xFF:
			i++
			continue
		case marker == 0xD9:
			return true
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7):
			visit(marker, data[i:i+2])
			i += 2
			continue
		}
		if i+4 > len(data) {
			return false
		}
		end := i + 2 + int(binary.BigEndian.Uint16(data[i+2:]))
		if end < i+4 || end > len(data) {
			return false
		}
		visit(marker, data[i:end])
		i = end
		if marker != 0xDA {
			continue
		}
		// Entropy-coded data runs to the next marker other than a stuffed
		// 0xFF00, a restart marker or fill bytes.
		j := i
		for {
			k := bytes.IndexByte(data[j:], 0xFF)
			if k < 0 || j+k+1 >= len(data) {
				return false
			}
			j += k
			if next := data[j+1]; next != 0x00 && next != 0xFF && (next < 0xD0 || next > 0xD7) {
				break
			}
			j++
		}
		visit(0, data[i:j])
		i = j
	}
	return false
}

// ExifSegment returns the Exif payload (TIFF header onwards) of the JPEG
// whose start is in head, or nil.
func ExifSegment(head []byte) []byte {
	if len(head) < 4 || head[0] != 0xFF || head[1] != 0xD8 {
		return nil
	}
	for i := 2; i+4 <= len(head); {
		if head[i] != 0xFF {
			return nil
		}
		marker := head[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return nil
		}
		end := i + 2 + int(binary.BigEndian.Uint16(head[i+2:]))
		if end < i+4 || end > len(head) {
			return nil
		}
		if segment := head[i+4 : end]; marker == 0xE1 && bytes.HasPrefix(segment, exifHeader) {
			return segment[len(exifHeader):]
		}
		i = end
	}
	return nil
}

// ExifThumbnail returns the JPEG thumbnail stored in an Exif payload and
// the photo's orientation (1 when unknown).
func ExifThumbnail(exif []byte) (thumbnail []byte, orientation int, ok bool) {
	if len(exif) < 8 {
		return nil, 1, false
	}
	var order binary.ByteOrder
	switch string(exif[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, 1, false
	}
	if order.Uint16(exif[2:]) != 42 {
		return nil, 1, false
	}
	// ifd calls visit for each entry and returns the offset of the next IFD.
	ifd := func(offset uint32, visit func(tag uint16, value []byte)) uint32 {
		if offset < 8 || uint64(offset)+2 > uint64(len(exif)) {
			return 0
		}
		count := int(order.Uint16(exif[offset:]))
		start := int(offset) + 2
		if start+count*12+4 > len(exif) {
			return 0
		}
		for n := 0; n < count; n++ {
			entry := exif[start+n*12 : start+n*12+12]
			visit(order.Uint16(entry), entry[8:12])
		}
		return order.Uint32(exif[start+count*12:])
	}
	orientation = 1
	next := ifd(order.Uint32(exif[4:]), func(tag uint16, value []byte) {
		if tag == 0x0112 {
			orientation = int(order.Uint16(value))
		}
	})
	var offset, length uint32
	ifd(next, func(tag uint16, value []byte) {
		switch tag {
		case 0x0201:
			offset = order.Uint32(value)
		case 0x0202:
			length = order.Uint32(value)
		}
	})
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	end := uint64(offset) + uint64(length)
	if offset == 0 || length < 4 || end > uint64(len(exif)) {
		return nil, orientation, false
	}
	thumbnail = exif[offset:end]
	if thumbnail[0] != 0xFF || thumbnail[1] != 0xD8 {
		return nil, orientation, false
	}
	return thumbnail, orientation, true
}

// FromExif decodes the thumbnail embedded in an Exif payload, upright.
func FromExif(exif []byte) (*image.NRGBA, bool) {
	thumbnail, orientation, ok := ExifThumbnail(exif)
	if !ok {
		return nil, false
	}
	img, err := jpeg.Decode(bytes.NewReader(thumbnail))
	if err != nil || img.Bounds().Dx() < 8 || img.Bounds().Dy() < 8 {
		return nil, false
	}
	bounds := img.Bounds()
	upright := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(upright, upright.Bounds(), img, bounds.Min, draw.Src)
	return Orient(upright, orientation), true
}

// Embedded returns the upright thumbnail embedded in the Exif data of the
// JPEG at path, reading only the start of the file. Much faster than
// decoding the photo, but small (usually 160×120).
func Embedded(path string) (*image.NRGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	head := make([]byte, headBytes)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if img, ok := FromExif(ExifSegment(head[:n])); ok {
		return img, nil
	}
	return nil, errors.New("no embedded thumbnail")
}
