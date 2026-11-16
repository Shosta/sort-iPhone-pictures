// Package jpeg8 decodes JPEG images at 1/8 scale, from the DC coefficient
// of each 8×8 block only. It skips the inverse DCT (and, for progressive
// images, the AC scans), which makes it several times faster than
// image/jpeg for previews and perceptual hashes.
//
// It is derived from the Go standard library's image/jpeg package; see
// LICENSE (BSD-style) in this directory.
package jpeg8

// blockSize is the number of coefficients in an 8×8 block.
const blockSize = 8 * 8

type block [blockSize]int32
