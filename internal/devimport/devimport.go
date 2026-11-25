// Package devimport imports photos from a mounted DCIM volume (iPhone, SD
// card, camera) in batches: detect the volume, list what is new, copy a batch
// into a private transit folder, and remember what was delivered in a journal.
// Nothing on the device is ever modified.
package devimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Error codes reported to the UI.
const (
	CodeNoDevice = "noDevice"
	CodeLowDisk  = "lowDisk"
	CodeCopy     = "copy"
)

// Error is an import failure with a code the UI turns into a message.
type Error struct {
	Code   string
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Detail
}

func (e *Error) Unwrap() error { return e.Err }

// BatchBytes is the target size of a batch; a larger file is its own batch.
const BatchBytes = 200 << 20

// Device is a mounted volume holding a DCIM folder.
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Path is the DCIM folder.
	Path string `json:"path"`
}

// File is a photo on the device. Rel is relative to the DCIM folder.
type File struct {
	Rel     string
	Size    int64
	ModTime time.Time
}

// Scan is what a DCIM folder holds.
type Scan struct {
	Files []File
	// Videos counts videos left out (they are never imported).
	Videos int
}

var photoExt = map[string]bool{".heic": true, ".jpg": true, ".jpeg": true, ".png": true, ".dng": true}
var videoExt = map[string]bool{".mov": true, ".mp4": true, ".m4v": true, ".avi": true, ".3gp": true, ".mts": true}

// IDForPath names a manually chosen folder so it keeps its own journal.
func IDForPath(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return "folder-" + hex.EncodeToString(sum[:6])
}

// ScanDCIM lists the importable photos below dcim, oldest first, skipping
// symlinks and hidden files. It reads directory entries only, never contents.
func ScanDCIM(ctx context.Context, dcim string) (Scan, error) {
	var scan Scan
	err := filepath.WalkDir(dcim, func(path string, entry fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if path == dcim {
				return err
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !entry.Type().IsRegular() || strings.HasPrefix(name, ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case videoExt[ext]:
			scan.Videos++
		case photoExt[ext]:
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(dcim, path)
			if err != nil {
				return nil
			}
			scan.Files = append(scan.Files, File{Rel: rel, Size: info.Size(), ModTime: info.ModTime()})
		}
		return nil
	})
	sort.Slice(scan.Files, func(i, j int) bool {
		a, b := scan.Files[i], scan.Files[j]
		if !a.ModTime.Equal(b.ModTime) {
			return a.ModTime.Before(b.ModTime)
		}
		return a.Rel < b.Rel
	})
	return scan, err
}

// Filter keeps what the mode asks for: a non-nil journal removes files already
// delivered, and a non-zero from/to keeps files whose modification time may
// match a capture date in [from, to] (a day of slack either side; the sorter
// applies the exact capture-date filter).
func (s Scan) Filter(journal *Journal, from, to time.Time) []File {
	var out []File
	for _, f := range s.Files {
		if journal != nil && journal.Has(f) {
			continue
		}
		if !from.IsZero() && f.ModTime.Before(from.AddDate(0, 0, -1)) {
			continue
		}
		if !to.IsZero() && f.ModTime.After(to.AddDate(0, 0, 1)) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// Batches groups files (already oldest first) into batches of about limit bytes.
func Batches(files []File, limit int64) [][]File {
	var batches [][]File
	var current []File
	var size int64
	for _, f := range files {
		if len(current) > 0 && size+f.Size > limit {
			batches = append(batches, current)
			current, size = nil, 0
		}
		current = append(current, f)
		size += f.Size
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// CopyFile copies src to dst through dst+".part", checks the size, keeps the
// device modification time (the sorter's date fallback relies on it) and
// removes the partial file on any failure.
func CopyFile(ctx context.Context, src, dst string, f File) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	part := dst + ".part"
	out, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.Close()
			os.Remove(part)
		}
	}()
	n, err := io.Copy(out, &ctxReader{ctx, in})
	if err != nil {
		return err
	}
	if n != f.Size {
		return fmt.Errorf("%s: copied %d bytes, expected %d", f.Rel, n, f.Size)
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = os.Rename(part, dst); err != nil {
		return err
	}
	return os.Chtimes(dst, f.ModTime, f.ModTime)
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// Gone reports whether err means the device went away rather than a local failure.
func Gone(dcim string, err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	_, statErr := os.Stat(dcim)
	return statErr != nil
}
