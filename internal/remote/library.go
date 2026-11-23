package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Fingerprint identifies a photo already sent, wherever it is stored remotely.
type Fingerprint struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	// Taken is the modification time (RFC 3339) of the photo when it was sent.
	Taken  string `json:"taken"`
	Target string `json:"target"`
}

// Index is the list of fingerprints of the photos sent to a library. It is
// kept locally and copied into the remote library, so that a photo moved or
// renamed there is still recognised.
type Index struct {
	Version int           `json:"version"`
	Photos  []Fingerprint `json:"photos"`
}

// Merge adds the fingerprints of other that are not known yet.
func (x *Index) Merge(other Index) {
	known := make(map[string]bool, len(x.Photos))
	for _, p := range x.Photos {
		known[p.SHA256] = true
	}
	for _, p := range other.Photos {
		if !known[p.SHA256] {
			known[p.SHA256] = true
			x.Photos = append(x.Photos, p)
		}
	}
	x.Version = 1
}

func parseIndex(data []byte) Index {
	var index Index
	if json.Unmarshal(data, &index) != nil {
		return Index{Version: 1}
	}
	return index
}

// HashFile returns the SHA-256 of a file.
func HashFile(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Library is what is known about the remote library before sending.
type Library struct {
	Files     []RemoteFile `json:"files"`
	Index     Index        `json:"index"`
	FetchedAt time.Time    `json:"fetchedAt"`
	// Cached is set when the listing comes from the local cache.
	Cached bool `json:"-"`
}

// LibraryOptions tunes LoadLibrary.
type LibraryOptions struct {
	// CacheDir holds the local copy; empty disables caching.
	CacheDir string
	// MaxAge is how long a cached listing stays valid.
	MaxAge time.Duration
	// Refresh ignores the cache.
	Refresh bool
}

func cacheFile(dir string, cfg Config) string {
	sum := sha256.Sum256([]byte(cfg.URL()))
	return filepath.Join(dir, "library-"+hex.EncodeToString(sum[:8])+".json")
}

// LoadLibrary lists the remote library (cached, since listing a large library
// is slow) and merges the remote fingerprint index with the local one.
func LoadLibrary(ctx context.Context, t Transport, cfg Config, opts LibraryOptions) (Library, error) {
	file := ""
	var local Library
	if opts.CacheDir != "" {
		file = cacheFile(opts.CacheDir, cfg)
		if data, err := os.ReadFile(file); err == nil && json.Unmarshal(data, &local) == nil {
			if !opts.Refresh && time.Since(local.FetchedAt) < opts.MaxAge {
				local.Cached = true
				return local, nil
			}
		}
	}
	files, err := t.ListTree(ctx, cfg)
	if err != nil {
		return Library{}, err
	}
	lib := Library{Files: files, FetchedAt: time.Now(), Index: local.Index}
	data, err := t.FetchIndex(ctx, cfg)
	if err != nil {
		return Library{}, err
	}
	if data != nil {
		lib.Index.Merge(parseIndex(data))
	}
	lib.save(file)
	return lib, nil
}

func (l Library) save(file string) {
	if file == "" {
		return
	}
	if data, err := json.Marshal(l); err == nil && os.MkdirAll(filepath.Dir(file), 0o700) == nil {
		os.WriteFile(file, data, 0o600)
	}
}

// Reason says why a photo is not sent.
type Reason string

const (
	// ReasonPath: the library already has a file at the destination path.
	ReasonPath Reason = "path"
	// ReasonSame: same name, size and date somewhere in the library.
	ReasonSame Reason = "same"
	// ReasonIndex: the fingerprint index knows this exact photo (renamed or moved).
	ReasonIndex Reason = "index"
)

// Skipped is a photo the library already has.
type Skipped struct {
	Item   Item
	Reason Reason
	// Existing is the path of the file already in the library, when known.
	Existing string
}

// Report is the dedup outcome, also shown for a dry run.
type Report struct {
	ToSend  []Item
	Skipped []Skipped
}

func (r Report) Count(reason Reason) int {
	n := 0
	for _, s := range r.Skipped {
		if s.Reason == reason {
			n++
		}
	}
	return n
}

// closeTimes tolerates the 2 s granularity of some file systems.
func closeTimes(a, b time.Time) bool {
	d := a.Sub(b)
	return d > -2*time.Second && d < 2*time.Second
}

// Dedupe splits items into what must be sent and what the library already has.
// A photo is already there if the library holds a file at the same path, or
// one with the same name, size and date, or if the fingerprint index knows its
// SHA-256 (only computed for photos whose size matches an indexed one).
// Dry runs and real runs use the same function so the report is identical.
func Dedupe(items []Item, lib Library) Report {
	byPath := make(map[string]RemoteFile, len(lib.Files))
	byName := map[string][]RemoteFile{}
	for _, f := range lib.Files {
		byPath[strings.ToLower(f.Path)] = f
		name := strings.ToLower(path.Base(f.Path))
		byName[name] = append(byName[name], f)
	}
	bySize := map[int64][]Fingerprint{}
	for _, p := range lib.Index.Photos {
		bySize[p.Size] = append(bySize[p.Size], p)
	}
	var report Report
	for _, item := range items {
		info, err := os.Stat(item.Source)
		if err != nil {
			report.ToSend = append(report.ToSend, item)
			continue
		}
		item.Size, item.ModTime = info.Size(), info.ModTime()
		target := strings.ToLower(normalize(item.Target))
		if f, ok := byPath[target]; ok {
			report.Skipped = append(report.Skipped, Skipped{item, ReasonPath, f.Path})
			continue
		}
		found := false
		for _, f := range byName[strings.ToLower(path.Base(target))] {
			if f.Size == item.Size && closeTimes(f.ModTime, item.ModTime) {
				report.Skipped = append(report.Skipped, Skipped{item, ReasonSame, f.Path})
				found = true
				break
			}
		}
		if found {
			continue
		}
		if known := bySize[item.Size]; len(known) > 0 {
			if sum, err := HashFile(item.Source); err == nil {
				for _, p := range known {
					if p.SHA256 == sum {
						report.Skipped = append(report.Skipped, Skipped{item, ReasonIndex, p.Target})
						found = true
						break
					}
				}
			}
		}
		if !found {
			report.ToSend = append(report.ToSend, item)
		}
	}
	return report
}

// Remember adds the sent photos to the fingerprint index, saves it locally and
// copies it into the remote library.
func Remember(ctx context.Context, t Transport, cfg Config, lib Library, cacheDir string, sent []Item) error {
	index := lib.Index
	var fresh Index
	for _, item := range sent {
		sum, err := HashFile(item.Source)
		if err != nil {
			continue
		}
		fresh.Photos = append(fresh.Photos, Fingerprint{SHA256: sum, Size: item.Size, Taken: item.ModTime.UTC().Format(time.RFC3339), Target: normalize(item.Target)})
	}
	index.Merge(fresh)
	lib.Index = index
	for _, item := range sent {
		lib.Files = append(lib.Files, RemoteFile{Path: normalize(item.Target), Size: item.Size, ModTime: item.ModTime})
	}
	if cacheDir != "" {
		lib.save(cacheFile(cacheDir, cfg))
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return t.StoreIndex(ctx, cfg, data)
}
